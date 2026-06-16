export CGO_ENABLED=1

.PHONY: build vet unit integration
build:
	go build ./...
vet:
	go vet ./...
unit:
	go test ./internal/... -short -v
integration:
	go test ./... -v -timeout 20m -p 1
