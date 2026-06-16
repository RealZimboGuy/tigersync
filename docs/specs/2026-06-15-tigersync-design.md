# tigersync — Design

**Date:** 2026-06-15
**Status:** Implemented

## Goal

A Go service that continuously mirrors a **source** TigerBeetle cluster to a fresh
**destination** cluster, **byte-identical including timestamps**, purely via the
client query API. It starts from zero, catches up to current, and tails forever.

"Perfection, nothing less": every replicated account and transfer must be
identical in every field — id, timestamp, ledger, code, flags, amounts, user
data — and any divergence is a fatal, loudly-reported error, never silently
skipped.

## Context

- TigerBeetle 0.17.6 is the reference version (`imported` flag and
  `query_accounts` / `query_transfers` are available).
- TigerBeetle has only two record types: **Accounts** and **Transfers**. Both
  are immutable/append-only; account balances are derived from transfers.
- Every object has a cluster-assigned 128-bit `id` and a unique, monotonic
  `timestamp` (ns) drawn from a **single global counter shared across accounts
  and transfers**.
- A native `amqp` CDC connector exists but is explicitly **out of scope** — this
  is a query-only design by requirement.

## Decisions (locked during brainstorming)

| Topic | Decision |
|---|---|
| Sync model | Continuous tailing. Start from 0, catch up, keep running. Initial full copy is just "catch-up from empty". |
| Fidelity | Byte-identical **including original timestamps** → `flags.imported = true` on every create. |
| Resume cursor | Derived from the destination (`max` existing timestamp). No external state. |
| Destination | Fresh, exclusively-imported cluster (only ever receives tigersync data). |
| Enumeration | Global timestamp scan via `query_accounts` / `query_transfers`, ascending, paginated. |
| Logging target | Structured logs to stdout, errors to stderr. |
| Language | Go, using `tigerbeetle-go`. |

## Architecture

A single-threaded **poll → merge → apply** loop, plus a verifier.

```
              ┌──────────── tigersync ────────────┐
 SOURCE  ───► │ Reader → MergeEngine → ApplyEngine │ ───► DEST
 cluster      │            ▲              │         │      cluster
 (read-only)  │         LagGuard      Cursor◄───────┼──── (max ts)
              └────────────────────────────────────┘
                  Observability (logs + counters)
              Verifier (reads both, asserts equality)
```

### Components

1. **tbclient** — thin wrappers over `tigerbeetle-go`: a read-only **source**
   client and a write **destination** client. One place to centralise client
   lifecycle, retries, and batch-size limits.

2. **Reader** — paginated enumeration of the source. For each type, issue a
   query with an **all-match filter** (zeroed selective fields), ascending by
   `timestamp_min`, fixed `limit` per page. Returns one page of accounts and one
   page of transfers per request.

3. **MergeEngine** — a 2-way **safe-watermark** merge of the account and
   transfer pages into a single global-timestamp-ordered apply stream
   (correctness detail below).

4. **ApplyEngine** — writes records to the destination with
   `flags.imported = true`, batched (≤ 8189 events/batch), in strictly
   increasing timestamp order; classifies every result code.

5. **Cursor** — on startup, `T = max(dest_max_account_ts, dest_max_transfer_ts)`.
   The destination is its own progress ledger; there is no separate state file.

6. **LagGuard** — withholds any record with `ts > now − margin` (destination
   wall clock) so imports never violate TigerBeetle's "imported timestamp must
   be in the past" rule.

7. **Observability** — structured leveled logging + running counters (below).

8. **Verifier** — reads both clusters and asserts every field (incl. timestamp)
   is identical. Runs as a test harness and as an optional live audit.

## Data flow (one iteration)

1. Fetch `query_accounts(ts > T, asc, limit)` and
   `query_transfers(ts > T, asc, limit)`.
2. Compute a **frontier** per stream:
   - page came back **full** → frontier = its last `ts` (we only know the stream
     is complete through there);
   - page came back **short** → stream is drained → frontier = `now − margin`.
3. **Watermark** `W = min(accountFrontier, transferFrontier, now − margin)`.
4. Merge-sort both lists; emit every record with `ts ≤ W`.
5. Apply emitted records to the destination as imported, in ts order, in
   batches.
6. Set `T = W`. If nothing was emitted, sleep `poll_interval`; otherwise loop
   immediately.

**Why the watermark is the correctness crux:** with two independently-paginated,
limited streams we can only safely emit up to the lowest point through which
*both* streams are known-complete. This guarantees we never apply a record from
one stream and *later* discover an earlier-timestamped record we had paged past
in the other. Records above `W` are simply re-fetched next iteration.

**Linked-chain atomicity:** a chain of events sharing `flags.linked` must be
submitted in one create request or TigerBeetle rejects it
(`LinkedEventChainOpen`). A chain commits atomically on the source and gets
consecutive timestamps, so by global order it is a contiguous, same-kind run.
tigersync never emits an incomplete trailing chain — it trims any open chain at
the watermark/page boundary and resumes before it, and the apply engine packs
whole chains into one batch (never splitting at the batch limit). A chain larger
than `batch_limit` is genuinely unrepresentable and is a defined fatal error
rather than an infinite retry.

## Crash safety & idempotency

- The cursor derives from the destination, so a restart resumes exactly where it
  left off.
- Re-importing an already-present, identical record returns an `exists` result,
  which is treated as **success** (makes re-applying a partially-done batch
  safe).
- `exists_with_different_*` (same id, different content) is a **fatal
  divergence** → halt + report. Any unexpected result code halts the loop loudly.
  Perfection means never silently skipping or accepting a mismatch.

## FUNDAMENTAL LIMITATION — pending-transfer timeouts (confirmed)

This boundary is imposed by TigerBeetle itself and is **not solvable** by any
query-based replicator. It is stated up front because it bounds the meaning of
"perfection."

Verified from the official docs (TigerBeetle 0.17.x):

- *"Imported transfers cannot have a timeout."* — when `flags.imported` is set,
  `timeout` **must be 0**.
- `timeout` is a relative interval managed by the primary; on expiry the full
  amount is returned to the origin account, applied best-effort in expiry order.
- Pending **expiry produces no queryable transfer record**. CDC even defines a
  `two_phase_expired` event type, but it is **not supported**. Expiry only
  mutates account `*_pending` balances — and accounts are not re-emitted on
  balance change, so a timestamp scan never observes it.

**Consequence:** a pending transfer that was created **with a non-zero
`timeout`** cannot be mirrored byte-identically:
- import with its real `timeout` → rejected by TigerBeetle;
- import with `timeout = 0` → the destination never expires it, so when the
  source expires it the balances diverge, with **no queryable event** to detect
  or reproduce the expiry.

**Policy:** tigersync treats any source transfer with `timeout != 0` as an
**unsupported record**. Default behaviour is to **halt with a fatal, fully
logged error** rather than produce a silently-divergent mirror (perfection or
nothing). This is configurable to a "warn-and-skip-timeout" best-effort mode,
but that mode explicitly forfeits byte-identity and is off by default.
**(Confirmed: fatal halt is the default policy.)**

Everything else is fully replicable: single-phase transfers, pending transfers
**without** a timeout resolved by `post_pending` / `void_pending`, balancing,
closing, linked chains, and all account flags.

## INHERENT DEVIATION — the `imported` flag bit is persisted (confirmed by Phase 0 spike)

TigerBeetle **stores the `imported` flag permanently** in each imported record.
A source row created normally has its `imported` bit clear; the destination row —
which *must* be created with `imported` set to preserve the timestamp — reads
back with that bit set (account bit `0x10`, transfer bit `0x100`). The Phase 0
spike confirmed this empirically: every other field including `timestamp`
matched exactly; only `Flags` differed by the `imported` bit.

**Therefore "byte-identical" is defined precisely as:** every field of every
destination record equals the source, **with the single exception that the
destination's `imported` bit is set** (it truthfully records that the row was
imported). This is unavoidable — it is a property of the only timestamp-
preserving mechanism TigerBeetle offers — and it is the same outcome TigerBeetle's
own migration tooling produces.

The verifier enforces this exactly by comparing each destination record against
the **expected imported form** of the source record (source record with its
`imported` bit set) using `==`. It does **not** loosely mask flags: any other
difference, in any field, is a divergence.

Edge case (rare, unsupported): an account created with `closed` set **at
creation** and never touched by a closing transfer cannot be reproduced — the
import transform must clear `closed` so transfers can apply, and nothing re-sets
it. The verifier surfaces this as a divergence (fatal). Accounts closed via a
`closing_*` transfer are fully reproduced.

## Edge cases to prove empirically (Phase 0 spike)

Replicable, but where imported-replay fidelity must be *verified*, not assumed:

- **Balancing transfers** (`flags.balancing_debit` / `balancing_credit`) and
  **closing transfers** (`flags.closing_debit` / `closing_credit`) — amounts /
  effects are computed at execution time on the source; confirm imported replay
  stores the *recorded* amount/effect exactly rather than recomputing.
- **`post_pending` partial posts** — confirm the posted amount and the residual
  released to the origin reproduce exactly.
- **Linked chains** — atomic success and atomic-rollback chains.
- **`closed` accounts** — reproduced via `closing_*` transfers or the account
  `closed` flag at creation; confirm state matches.

The Phase 0 spike writes fixtures exercising each of these to the source, runs
the sync, and asserts byte-identity — turning unknowns into verified facts
before the production loop is built.

## Transaction scenario matrix (the test scenarios)

tigersync must pass **every** scenario below. Each is a fixture written to the
source; the verifier then asserts byte-identity on the destination.

**Account creation scenarios**
1. Plain account (no flags).
2. `debits_must_not_exceed_credits`.
3. `credits_must_not_exceed_debits`.
4. `history` enabled.
5. `closed` set at creation.
6. Linked account-creation chain (all-succeed) and (atomic-rollback).
7. Populated `user_data_128/64/32`, distinct `ledger` / `code`.
8. (All accounts created with `imported` + original timestamp.)

**Transfer scenarios**
1. Single-phase immediate transfer.
2. Pending (timeout = 0) → `post_pending` **full** amount.
3. Pending (timeout = 0) → `post_pending` **partial** amount.
4. Pending (timeout = 0) → `void_pending`.
5. Pending **with** `timeout != 0` → **expiry** — *unsupported boundary*; test
   asserts tigersync halts (or warns) per policy, **not** byte-identity.
6. `balancing_debit` — fully filled and partially filled (amount capped).
7. `balancing_credit` — fully filled and partially filled.
8. `closing_debit` (closes the debit account) and `closing_credit`.
9. Linked transfer chain — atomic success and atomic-rollback.
10. `amount = 0`.
11. `amount = AMOUNT_MAX` (post-full sentinel, balancing cap).
12. Multiple ledgers interleaved.
13. Two-phase against a `closed` account (void of a still-pending transfer is
    the only permitted operation).
14. Large run crossing the 8189-events/batch boundary, with accounts and
    transfers interleaved in global timestamp order.

**Cross-cutting invariants asserted by the verifier for every scenario:** equal
account/transfer counts; every field byte-identical including `timestamp`,
`flags`, `amount`, `pending_id`, `user_data_*`, `ledger`, `code`; and identical
derived balances (`debits_pending/posted`, `credits_pending/posted`).

**`flags.history` accounts:** the `history` flag is replicated like any other
flag, but it also makes TigerBeetle retain a per-transfer balance snapshot
(`get_account_balances`). These snapshots are *not* separate objects and are
never copied directly — they are reproduced automatically because the
destination account also has `history` set and the identical transfers are
replayed in the identical order, yielding identical snapshots with identical
timestamps. The verifier asserts this explicitly: for every history-flagged
account it compares the full `get_account_balances` history (paged) on both
clusters. Covered by the scenario matrix (account 4 receives transfers) and a
dedicated `TestHistoryBalancesReplicate` test.

## Observability

- **Structured leveled logging** (INFO / WARN / ERROR) to stdout (errors also to
  stderr):
  - **Startup:** resolved cursor `T`, source/dest clusters, full config.
  - **Per iteration:** accounts/transfers fetched, watermark `W`, records
    applied, batch count, current lag (`now − W`).
  - **Per batch:** applied-OK count, `exists` count, applied timestamp range.
  - **State transitions:** "caught up — idle, tailing at lag Xs".
- **Running counters**, emitted periodically and on shutdown: total accounts
  synced, total transfers synced, total batches, total `exists` skips, total
  errors, uptime, current lag, throughput (records/s).
- **Error tiers:**
  - *Transient* (network blip, client timeout) → WARN, retry with backoff,
    counted.
  - *Fatal* (`exists_with_different_*`, unexpected result code, malformed data)
    → ERROR with full record context, **halt**, non-zero exit.
- **Verifier** emits a pass/fail summary and the first N mismatches on divergence.

## Configuration

CLI flags / env vars:

- `source.addresses`, `source.cluster`
- `dest.addresses`, `dest.cluster`
- `poll_interval` (idle poll cadence)
- `batch_limit` (page size / apply batch size, ≤ 8189)
- `safety_margin` (LagGuard hold-back window)
- `log_level`

## Testing

All clusters in tests run as **Docker** TigerBeetle containers, spun up and torn
down by the harness.

- **Phase 0 spike:** two single-node containers; write fixtures exercising the
  edge-case scenarios; sync; assert byte-identical. Resolves the
  imported-replay unknowns before the production loop is built.
- **Scenario suite:** the full transaction scenario matrix above, each fixture
  synced and verified byte-identical (or, for the timeout boundary, asserting
  the configured halt/warn behaviour).
- **Integration:** docker-compose (source + dest) with a load generator writing
  to the source while tigersync runs; the verifier asserts equality continuously.
- **Property/fuzz:** random valid transfer streams → sync → assert equality.

### Failure scenarios (must be covered)

**Runtime / operational failures** — tigersync must survive or fail loudly,
never silently diverge:
- Source unreachable at startup and mid-run → retry with backoff, WARN, recover.
- Destination unreachable mid-batch → retry; on recovery resume from the
  destination-derived cursor with no duplication or gaps.
- tigersync crash/kill mid-batch → restart resumes exactly (idempotent
  re-apply; `exists` treated as success).
- Destination cluster restart → reconnect and continue.
- Clock skew: destination wall clock **behind** source timestamps → LagGuard
  holds records back; assert no `imported_event_timestamp_out_of_range`.
- Source written faster than sync drains → lag grows but stays bounded and
  recovers when writes slow (measured in load tests).

**Divergence / data failures** — must be fatal, never accepted:
- `exists_with_different_flags` / `_amount` / `_*` → fatal halt + full record
  context (a real mirror can never reach this; if it does, it's a bug to catch).
- Unexpected / unhandled create result code → fatal halt.
- Source transfer with `timeout != 0` → fatal halt (default policy) or counted
  WARN-skip (opt-in mode).

**Rejected-source-transfer handling** — `create_transfers` failures on the
*source* (e.g. `exceeds_credits`, `debit_account_not_found`) are **not
persisted** by TigerBeetle, so they never appear in the source enumeration. A
fixture deliberately attempts rejected transfers, then asserts they are absent
from both source reads and the destination — proving tigersync only ever
replicates committed records.

### Load testing (basic)

Goal: prove tigersync **keeps up** with a realistically-loaded source and that
lag is bounded.

- **Generator:** a Go load generator (or the official `tigerbeetle benchmark`)
  writes a high volume across many accounts, ledgers, and a representative mix of
  transfer types (single-phase, two-phase post/void, balancing, linked).
- **Scale targets (basic):** ~1M+ transfers across ~10k accounts and several
  ledgers, plus a sustained write-rate run.
- **Metrics captured:** cold-start catch-up time from empty; steady-state lag
  (`now − W`) under sustained writes; throughput (records/s); batch fill
  efficiency; memory footprint.
- **Pass criteria:** destination reaches byte-identity with the source after
  writes stop; steady-state lag stabilises (sync rate ≥ source write rate) and
  does not grow unbounded; zero divergence.

## Deliverables

- The `tigersync` Go service (components above).
- The verifier (test harness + optional live audit mode).
- The Docker-based test harness, scenario fixtures, and load generator.
- **`README.md`** covering, end to end:
  - what tigersync does and the byte-identity guarantee (plus the
    pending-timeout limitation, stated plainly);
  - quick start: standing up source + destination clusters via Docker, building
    and running tigersync, every config flag/env var with defaults;
  - how to read the logs and running counters (what each field means, how to
    spot lag and divergence);
  - how to run the full test stack: Phase 0 spike, scenario suite, integration,
    property/fuzz, and the load test, with expected output;
  - troubleshooting the fatal failure modes (divergence, timeout records,
    clock skew) and what each means.

## Resolved decisions

- **Pending-timeout policy (confirmed):** **fatal halt** on any `timeout != 0`
  source transfer (perfection or nothing). The warn-and-skip best-effort mode
  remains available as an explicit opt-in but is off by default.

## Explicit non-goals

- No CDC / AMQP — query-only by requirement.
- No destination with pre-existing data — fresh, exclusively-imported.
- Byte-identity holds up to `now − margin`, never to the literal instant — this
  is inherent to the `imported` flag, not a design limitation.
- Byte-identity is "identical modulo the persisted `imported` bit" — the
  destination's records necessarily carry that marker (see the inherent-deviation
  section). The verifier accounts for it precisely.
