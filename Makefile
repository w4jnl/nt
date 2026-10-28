BIN := nt
PREFIX ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/w4jnl/nt/internal/cli.Version=$(VERSION)

.PHONY: build install test vet fmt brand e2e nt-universal mac-editor mac-project mac-build mac-test mac-run mac-release

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

# nt for Mac (app/macos, docs/design.md phase 6). The Xcode project is generated from
# app/macos/project.yml (brew install xcodegen). The app bundles a universal nt built from this
# tree, and the editor page in app/macos/Nt/Resources/editor, which mac-editor rebuilds (Node)
# after a change under app/macos/editor/src; the built page is committed.
MAC := app/macos
MAC_XCODEBUILD = cd $(MAC) && xcodebuild -project Nt.xcodeproj -scheme Nt -configuration Debug -destination 'platform=macOS,arch=arm64' -derivedDataPath build/DerivedData

nt-universal:
	mkdir -p $(MAC)/build
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o $(MAC)/build/nt-arm64 ./cmd/$(BIN)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o $(MAC)/build/nt-amd64 ./cmd/$(BIN)
	lipo -create -output $(MAC)/build/nt $(MAC)/build/nt-arm64 $(MAC)/build/nt-amd64

mac-editor:
	cd $(MAC)/editor && npm ci --no-fund --no-audit && npm test && npm run build

mac-project: nt-universal
	@[ -f $(MAC)/Local.xcconfig ] || printf 'NT_TEAM_ID =\n' > $(MAC)/Local.xcconfig
	cd $(MAC) && xcodegen generate --quiet

mac-build: mac-project
	$(MAC_XCODEBUILD) build -quiet

mac-test: mac-project
	$(MAC_XCODEBUILD) test -quiet

mac-run: mac-build
	open $(MAC)/build/DerivedData/Build/Products/Debug/nt.app

# nt for Mac for a release, V=0.5.1: signed with the Developer ID, notarized, stapled, and the
# DMG that carries it, in app/macos/build/release (scripts/mac-release.sh; release.sh runs it).
mac-release:
	@[ -n "$(V)" ] || { echo "make mac-release V=<version>, e.g. V=0.5.1" >&2; exit 2; }
	scripts/mac-release.sh $(V)
