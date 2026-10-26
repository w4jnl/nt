BIN := nt
PREFIX ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/w4jnl/nt/internal/cli.Version=$(VERSION)

.PHONY: build install test vet fmt brand e2e

build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o bin/$(BIN) ./cmd/$(BIN)

# A new file renamed over the old one, never a copy into it: macOS kills a binary whose pages
# change under a cached code signature (exit 137), and the sync job runs this one.
install: build
	mkdir -p $(PREFIX)/bin
	cp bin/$(BIN) $(PREFIX)/bin/.$(BIN).new
	mv -f $(PREFIX)/bin/.$(BIN).new $(PREFIX)/bin/$(BIN)

test:
	go test -timeout 120s ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal ./skills

# The brand SVGs and render pages come from one geometry (assets/brand/gen.py, needs fontTools
# and brotli); render-hero.sh turns them into the PNGs (headless Chrome, rsvg-convert).
brand:
	python3 assets/brand/gen.py
	assets/brand/render-hero.sh

# End-to-end suites against a bare repository and isolated tmux servers (phase 1 onwards).
e2e: build
	@if [ -x scripts/e2e/run-all.sh ]; then scripts/e2e/run-all.sh; else echo "no e2e suites yet"; fi
