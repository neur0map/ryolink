.PHONY: ryolink run test check changelog changelog-check clean

# Build the single static binary (assets embedded; runs anywhere)
ryolink:
	CGO_ENABLED=0 go build -trimpath -o ryolink ./cmd/ryolink

# Dev: build, start on :2222, connect. Needs ./ryolink.yaml (or `./ryolink init`)
run: ryolink
	@./ryolink up & sleep 1 && ssh localhost -p 2222; kill %1 2>/dev/null

# Run all tests with race detector
test:
	go test -race ./internal/... ./ui/...

# Keep the embedded changelog copy in sync with the root file
changelog:
	cp CHANGELOG.md internal/version/CHANGELOG.md

changelog-check:
	@cmp -s CHANGELOG.md internal/version/CHANGELOG.md || \
		(echo "CHANGELOG.md drifted from the embedded copy — run: make changelog" && exit 1)

# Run before push — lint + build + test (mirrors CI)
check: changelog-check
	gofmt -w .
	go vet ./...
	CGO_ENABLED=0 go build -trimpath -o ryolink ./cmd/ryolink
	go test -race ./internal/... ./ui/...
	@echo "All good."

# Remove binaries and db
clean:
	rm -f ryolink
	rm -rf bin/
