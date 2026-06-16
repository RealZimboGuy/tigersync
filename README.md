# tigersync

Continuously mirror one [TigerBeetle](https://tigerbeetle.com) cluster to another,
**byte-identical including timestamps**, purely through the client query API — no
CDC, no access to data files. tigersync starts from zero, catches up to the
source, and tails it forever.

## WARNING

* Pending transfers are NOT SUPPORTED with a non zero timeout, this will ERROR and the SYNC will stop

## What "byte-identical" means here

Every destination account and transfer is identical to the source in **every
field** — `id`, `timestamp`, `ledger`, `code`, `amount`, `pending_id`,
`user_data_*`, and recomputed balances — with **two unavoidable, TigerBeetle-imposed
exceptions**:

1. **The `imported` bit is set on the destination.** Preserving original
   timestamps requires TigerBeetle's `imported` flag, and that flag is *persisted*
   in the stored record. The destination truthfully records that its rows were
   imported. The verifier compares each destination row against the source row
   *with its imported bit set*, so this is checked exactly — any other difference
   is a divergence.
2. **A small, bounded lag at the live edge.** Imported events must have a
   timestamp in the past relative to the destination's clock, so records within a
   `safety-margin` of "now" are held back until safe. tigersync is byte-identical
   up to `now − safety-margin`, never to the literal instant.

One transaction type **cannot** be mirrored and is treated as a fatal error:
**pending transfers created with a non-zero `timeout`**. Imported transfers may
not carry a timeout, and pending *expiry* emits no queryable event, so such a
transfer is unreplicable. tigersync halts loudly rather than produce a silently
divergent mirror. See `docs/specs/2026-06-15-tigersync-design.md` for the full
analysis.

## How it works

A single poll → merge → apply loop:

1. **Read** the source with `query_accounts` / `query_transfers`, ascending by
   timestamp, paginated.
2. **Merge** the two streams into one global-timestamp order using a *safe
   watermark*, so a later-fetched, earlier-timestamped record can never be skipped.
3. **Apply** to the destination with the `imported` flag, in strictly increasing
   timestamp order, in batches of ≤ 8189.

The resume cursor is derived from the destination's own max timestamp — no
external state, crash-safe and idempotent (`exists` results are accepted; any
`exists_with_different_*` is a fatal divergence).

## What gets synced, and how

TigerBeetle has only two record types — **accounts** and **transfers** — and
everything else (ledgers, balances, two-phase state, closures) is expressed
through them. tigersync reproduces each by replaying the source's own records, in
the source's own global timestamp order, with the original timestamps preserved.

- **Ledgers and codes** are not separate objects — they are fields (`ledger`,
  `code`) on every account and transfer. They are copied verbatim, so accounts
  and transfers land on exactly the same ledgers and codes on the destination.
  Cross-ledger rules are re-enforced naturally because the transfers carry the
  same ledgers as their accounts.
- **Account creation** is mirrored by re-creating each source account with its
  `id`, `ledger`, `code`, `user_data_*`, and flags
  (`debits_must_not_exceed_credits`, `credits_must_not_exceed_debits`,
  `history`, …) — at its original creation timestamp via the `imported` flag.
  Balance fields are **not** copied at creation (TigerBeetle requires them to be
  zero); they are rebuilt from the replayed transfers (see below). The `closed`
  flag is cleared at creation and re-established by replaying the closing
  transfer, so closure happens at the same point in history as on the source.
- **Transfers** — single-phase and two-phase — are re-created with every field
  (`debit_account_id`, `credit_account_id`, `amount`, `pending_id`,
  `user_data_*`, `ledger`, `code`, flags) and the original timestamp. Because the
  pending transfer, its post/void, balancing transfers, and closing transfers are
  all just transfer records, replaying them in order reproduces the exact same
  effects.
- **Balances** (`debits_pending`, `debits_posted`, `credits_pending`,
  `credits_posted`) are never copied directly — TigerBeetle **recomputes** them
  as the transfers are applied. Replaying the identical transfer history in the
  identical order yields identical balances. The verifier checks these recomputed
  balance fields too.
- **Ordering** matters: accounts and transfers share one global, monotonic
  timestamp counter on the source. tigersync merges both streams into that single
  order so every account exists before any transfer that references it, and so
  imported timestamps strictly increase on the destination.
- **Linked chains** (`flags.linked`) are applied atomically — a chain is always
  submitted in one create request and is never split across a batch or page
  boundary, so an all-or-nothing chain stays all-or-nothing on the destination.

## How new data is handled while running

tigersync does not stop after the initial copy — it **tails** the source forever:

1. Each iteration asks the source for any accounts/transfers with a timestamp
   greater than the cursor (newest data the destination hasn't seen). New
   accounts and new transfers created on the source since the last poll are
   picked up here, in timestamp order.
2. New records are merged, then applied to the destination with their original
   timestamps. The cursor advances to the highest safely-applied timestamp.
3. When the source is **caught up**, the loop finds nothing new and sleeps for
   `--poll-interval` before checking again; as soon as new writes appear they are
   pulled on the next poll. There is no push/subscription — it is a tight poll.
4. **The live edge is held back by `--safety-margin`.** Records whose timestamp is
   within the margin of "now" are not applied yet, because imported timestamps
   must be safely in the past relative to the destination's clock. This means the
   destination trails the source by a small, bounded amount (the margin plus poll
   interval) — it converges continuously but is never byte-identical to the
   *literal* latest instant. The `stats` log line shows how far behind it is.
5. Because the cursor lives on the destination, a restart resumes exactly where it
   left off, and re-applying already-synced records is a harmless no-op
   (`exists`). New data is never skipped and never double-counted.

Note: source records are immutable — TigerBeetle never updates or deletes an
account or transfer — so "new data while running" only ever means *additional*
records appended after the cursor, never changes to records tigersync has already
mirrored.

## Requirements

- Go 1.22+
- Docker (for the test suite — it spins up real TigerBeetle clusters)
- `CGO_ENABLED=1` (the `tigerbeetle-go` client links a native library)

### macOS (Apple Silicon) note

With the clang-21 linker, `tigerbeetle-go` v0.17.6's bundled static library is not
8-byte aligned and fails to link. This repo uses a local re-archived copy via a
`replace` directive in `go.mod` pointing at `../tigerbeetle-go-local`. If you
clone elsewhere, re-create it:

```bash
# extract the bundled lib and re-archive it 8-byte aligned
cd "$(go env GOMODCACHE)/github.com/tigerbeetle/tigerbeetle-go@v0.17.6"
# copy the module to a writable local dir, then inside its native lib dir:
libtool -static -o libtb_client.a libtb_client_aarch64-macos.a
```
Then point the `replace` directive at that copy. On linux/amd64 this is
unnecessary — remove the `replace` directive there. Avoid `go mod tidy` clobbering
the directive.

## Build

```bash
make build         # CGO_ENABLED=1 go build ./...
```

## Quick start: create data and watch it sync

This is a complete, tested walkthrough using the `tigerbeetle` binary directly
(get it from <https://tigerbeetle.com> or extract it from the Docker image:
`docker run --rm --entrypoint cat ghcr.io/tigerbeetle/tigerbeetle:0.17.6 /tigerbeetle > tigerbeetle && chmod +x tigerbeetle`).
Set `TB` to wherever your binary is:

```bash
export TB=./tigerbeetle      # path to the tigerbeetle binary
mkdir -p /tmp/tb-demo
```

**1. Format two fresh data files** — a source and a (fresh, exclusively-imported) destination:

```bash
$TB format --cluster=0 --replica=0 --replica-count=1 /tmp/tb-demo/source.tigerbeetle
$TB format --cluster=0 --replica=0 --replica-count=1 /tmp/tb-demo/dest.tigerbeetle
```

**2. Start each cluster — one per terminal.** Source on 3000, destination on 3001:

```bash
# terminal 1 — source
$TB start --addresses=3000 /tmp/tb-demo/source.tigerbeetle
```
```bash
# terminal 2 — destination
$TB start --addresses=3001 /tmp/tb-demo/dest.tigerbeetle
```

**3. Run tigersync — terminal 3** (watch this log; it prints `applied count=N` as it mirrors):

```bash
# from the tigersync/ directory
CGO_ENABLED=1 go run ./cmd/tigersync \
  --source-addresses=127.0.0.1:3000 --source-cluster=0 \
  --dest-addresses=127.0.0.1:3001   --dest-cluster=0 \
  --safety-margin=1s --poll-interval=500ms
```

| Flag | Default | Meaning |
|---|---|---|
| `--source-addresses` | *(required)* | Comma-separated source replica addresses |
| `--source-cluster` | `0` | Source cluster id |
| `--dest-addresses` | *(required)* | Comma-separated destination replica addresses |
| `--dest-cluster` | `0` | Destination cluster id |
| `--poll-interval` | `250ms` | How often to poll when idle (caught up) |
| `--batch-limit` | `8189` | Page size / apply batch size (max 8189) |
| `--safety-margin` | `2s` | LagGuard hold-back window at the live edge |
| `--log-level` | `info` | `debug` \| `info` \| `warn` \| `error` (also `LOG_LEVEL` env) |

**4. Create data on the SOURCE and look it up on the DESTINATION — terminal 4.**
Each line is a one-shot REPL call (`repl … --command="…"`):

```bash
# create two accounts on the SOURCE (3000)
$TB repl --cluster=0 --addresses=3000 --command="create_accounts id=1 ledger=1 code=1, id=2 ledger=1 code=1;"

# a transfer on the SOURCE
$TB repl --cluster=0 --addresses=3000 --command="create_transfers id=1 debit_account_id=1 credit_account_id=2 amount=100 ledger=1 code=1;"

# ~1.5s later (poll + margin), look it up on the DESTINATION (3001):
# same ids, same timestamps, balances rebuilt, flags include "imported"
$TB repl --cluster=0 --addresses=3001 --command="lookup_accounts id=1, id=2;"
$TB repl --cluster=0 --addresses=3001 --command="lookup_transfers id=1;"
```

**5. Do it a few more times** — add to the source and watch terminal 3 apply each batch live:

```bash
$TB repl --cluster=0 --addresses=3000 --command="create_accounts id=3 ledger=1 code=1;"
$TB repl --cluster=0 --addresses=3000 --command="create_transfers id=2 debit_account_id=1 credit_account_id=3 amount=40 ledger=1 code=1, id=3 debit_account_id=2 credit_account_id=3 amount=25 ledger=1 code=1;"

# verify on the DESTINATION
$TB repl --cluster=0 --addresses=3001 --command="lookup_accounts id=1, id=2, id=3;"
$TB repl --cluster=0 --addresses=3001 --command="lookup_transfers id=1, id=2, id=3;"
```

You'll see each destination record carry the **same `id` and `timestamp`** as the
source, balances **recomputed** from the replayed transfers
(`debits_posted` / `credits_posted`), and `flags` showing `imported` (plus any
original flags such as `linked`).

**6. Tear down:** `Ctrl-C` the three terminals, then `rm -rf /tmp/tb-demo`.

### Run the clusters with Docker instead (optional)

If you'd rather not install the binary, run each cluster in Docker (then use the
same `tigerbeetle repl` commands above, or `docker exec` into a container):

```bash
IMG=ghcr.io/tigerbeetle/tigerbeetle:0.17.6
docker run --rm -v tb-src:/data $IMG format --cluster=0 --replica=0 --replica-count=1 /data/0.tigerbeetle
docker run -d --name tb-src -v tb-src:/data -p 3000:3000 $IMG start --addresses=0.0.0.0:3000 /data/0.tigerbeetle
docker run --rm -v tb-dst:/data $IMG format --cluster=0 --replica=0 --replica-count=1 /data/0.tigerbeetle
docker run -d --name tb-dst -v tb-dst:/data -p 3001:3000 $IMG start --addresses=0.0.0.0:3000 /data/0.tigerbeetle
```

### Generate load (optional)

Drive the source while tigersync mirrors it:

```bash
go run ./cmd/loadgen --addresses=127.0.0.1:3000 --accounts=10000 --transfers=1000000
```

## Reading the logs

Structured JSON on stdout. Key events:

- `starting` — resolved resume `cursor`, source/dest, and config.
- `applied` — `count` of records applied this iteration and the new `cursor`.
- `stats` (every 10s) — running totals: `accounts`, `transfers`, `exists`
  (idempotent re-applies), `batches`, `errors`.
- `stopping` — final totals and the stop `reason`.

**Divergence / fatal errors** cause the process to log the offending record and
exit non-zero. The message names the result status, e.g.
`fatal transfer result TransferExistsWithDifferentAmount at ts ...`.

## Testing

```bash
make unit          # fast, no Docker (-short)
make integration   # spins up real TigerBeetle clusters in Docker (-p 1, serialized)
```

`make integration` runs (serialized to bound Docker disk/CPU; each cluster
pre-allocates ~1 GiB):

- **Scenario matrix** (`test/scenario_test.go`) — seeds one of every supported
  transaction type (single-phase; pending + full/partial post; pending + void;
  balancing; linked chain; closing; zero-amount; multiple ledgers; account flags)
  and asserts byte-identity after sync.
- **Failure scenarios** (`test/failure_test.go`) — timeout transfer is fatal;
  re-applying the same records is idempotent and stays equal.
- **Load catch-up** (`test/integration_test.go`) — seeds 1,000 accounts + 50,000
  transfers and asserts the engine converges to byte-identity (typically < 2s of
  catch-up).

The decisive `imported` round-trip experiment lives in
`internal/testcluster/spike_test.go`.

### Docker cleanup

Tests remove their own containers/volumes via `defer`. If a run is interrupted,
reclaim space with:

```bash
docker ps -aq --filter name=tigersync-test | xargs -r docker rm -f
docker volume ls -q | grep tigersync-test | xargs -r docker volume rm
```

## Load testing

The basic load test (`TestLoadCatchUpByteIdentical`) measures cold-start catch-up
and asserts byte-identity at scale. To push harder, raise the `accounts` /
`transfers` constants in that test, or run `cmd/loadgen` against a live source
while tigersync runs and watch the `stats` lag. Pass criteria: the destination
converges to byte-identity and steady-state lag stays bounded (sync rate ≥ source
write rate).

## Troubleshooting

| Symptom | Meaning |
|---|---|
| exits with `fatal ... ExistsWithDifferent*` | A destination row diverges from the source — should never happen on a fresh, exclusively-imported destination; indicates a bug or a non-fresh destination. |
| exits with `non-zero timeout ... not importable` | The source has a pending transfer with a `timeout`; this type is unreplicable (see limitations). |
| `ImportedEventTimestampOutOfRange` (or no progress at the live edge) | Destination clock is behind the source's newest timestamps; increase `--safety-margin`. |
| repeated `on_connect: error ... ConnectionRefused` warnings | A cluster is unreachable. The TigerBeetle client blocks and retries internally, so tigersync **pauses** and resumes automatically when the cluster returns — this is expected resilience, not a crash. Ctrl-C still stops it within ~2s (it force-exits if a blocked client call can't unwind). |
| `NoSpaceLeft` in tests | Docker VM disk is full of leftover ~1 GiB cluster volumes — run the cleanup above. |

## Layout

```
cmd/tigersync     CLI entry point
cmd/loadgen       load generator
internal/record   unified record + imported/expected transforms
internal/merge    safe-watermark merge (pure)
internal/reader   ascending paginated source reader
internal/cursor   resume cursor from destination max timestamp
internal/apply    imported apply engine + result classification
internal/syncengine  poll -> merge -> apply loop + LagGuard
internal/verify   byte-identity verifier
internal/observ   structured logging + counters
internal/config   CLI/env config
internal/testcluster  Docker test clusters, fixtures, scenario seeder, spike
docs/specs        design spec
```
