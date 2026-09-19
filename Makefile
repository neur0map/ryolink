.PHONY: ryolink run test check clean

# Build the single static binary (assets embedded; runs anywhere)
ryolink:
	CGO_ENABLED=0 go build -trimpath -o ryolink ./cmd/ryolink

# Dev: build, start on :2222, connect. Needs ./ryolink.yaml (or `./ryolink init`)
run: ryolink
	@./ryolink up & sleep 1 && ssh localhost -p 2222; kill %1 2>/dev/null

# Run all tests with race detector
test:
	go test -race ./internal/... ./ui/...

# Run before push — lint + build + test (mirrors CI)
check:
	gofmt -w .
	go vet ./...
	CGO_ENABLED=0 go build -trimpath -o ryolink ./cmd/ryolink
	go test -race ./internal/... ./ui/...
	@echo "All good."

# Remove binaries and db
clean:
	rm -f ryolink
	rm -rf bin/
