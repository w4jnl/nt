BIN := nt
PREFIX ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/w4jnl/nt/internal/cli.Version=$(VERSION)

.PHONY: build install test vet fmt brand e2e nt-universal mac-editor mac-project mac-build mac-test mac-run mac-release ios-core ios-libgit2 ios-project ios-build ios-test ios-ssh-test ios-device

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
	gofmt -w ./cmd ./internal ./skills ./mobile

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

# nt for iPhone and iPad (app/ios, docs/design.md §9). ios-core builds the phone's nt, the
# mobile package, into NtCore.xcframework with gomobile (go install
# golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest); ios-libgit2
# builds its git, libgit2 with ssh, into Git2.xcframework (scripts/ios-libgit2.sh, cmake).
# ios-project builds either when it is missing. The simulator build and tests are signed ad hoc,
# so the Keychain, which holds the phone's key, has the entitlement it asks for.
IOS := app/ios
GOMOBILE ?= $(shell command -v gomobile || echo $(HOME)/go/bin/gomobile)
IOS_SIM ?= platform=iOS Simulator,name=iPhone 17
IOS_XCODEBUILD = cd $(IOS) && xcodebuild -project Nt.xcodeproj -scheme Nt -configuration Debug -derivedDataPath build/DerivedData

ios-core:
	@[ -x "$(GOMOBILE)" ] || { echo "gomobile: go install golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest" >&2; exit 1; }
	mkdir -p $(IOS)/build
	PATH="$(dir $(GOMOBILE)):$$PATH" $(GOMOBILE) bind -target=ios,iossimulator -iosversion=26.0 -ldflags '$(LDFLAGS) -X github.com/w4jnl/nt/mobile.version=$(VERSION)' -o $(IOS)/build/NtCore.xcframework ./mobile

ios-libgit2:
	scripts/ios-libgit2.sh

ios-project:
	@[ -d $(IOS)/build/NtCore.xcframework ] || $(MAKE) ios-core
	@[ -d $(IOS)/build/Git2.xcframework ] || $(MAKE) ios-libgit2
	@[ -f $(IOS)/Local.xcconfig ] || printf 'NT_TEAM_ID =\n' > $(IOS)/Local.xcconfig
	cd $(IOS) && xcodegen generate --quiet

ios-build: ios-project
	$(IOS_XCODEBUILD) -destination '$(IOS_SIM)' build CODE_SIGN_IDENTITY=- -quiet

ios-test: ios-project
	$(IOS_XCODEBUILD) -destination '$(IOS_SIM)' test CODE_SIGN_IDENTITY=- -collect-test-diagnostics never -quiet

# The phone's ssh in the simulator, against a throwaway sshd on 127.0.0.1 (scripts/ios-ssh-test.sh).
ios-ssh-test: ios-project
	IOS_SIM='$(IOS_SIM)' scripts/ios-ssh-test.sh

# Build, install and launch on the iPhone or iPad (unlocked; on a cable, or on Wi-Fi once
# paired), signed with Local.xcconfig's team. The first device devicectl lists as reachable is
# taken; IOS_DEVICE=<udid> picks one.
ios-device: ios-project
	@udid=$(IOS_DEVICE); [ -n "$$udid" ] || udid=$$(xcrun devicectl list devices 2>/dev/null | grep -v simulated | grep -E ' connected | available \(paired\) ' | grep -oE '[0-9A-F]{8}-[0-9A-F]{16}' | head -1); \
	[ -n "$$udid" ] || { echo "ios-device: no iPhone or iPad reachable: unlock it, on a cable or on the same Wi-Fi" >&2; exit 1; }; \
	xcrun devicectl list devices 2>/dev/null | grep "$$udid" | grep -qE ' connected | available \(paired\) ' || { \
	  echo "ios-device: $$udid is not reachable: wake and unlock it and connect it with a cable (Trust this computer); xcrun devicectl list devices shows it as connected once it is" >&2; exit 1; }; \
	$(IOS_XCODEBUILD) -destination "id=$$udid" -allowProvisioningUpdates -allowProvisioningDeviceRegistration build -quiet && \
	xcrun devicectl device install app --device "$$udid" build/DerivedData/Build/Products/Debug-iphoneos/nt.app >/dev/null && \
	xcrun devicectl device process launch --terminate-existing --device "$$udid" nl.w4j.nt.ios >/dev/null && echo "ios-device: installed and launched on $$udid"

# nt for Mac for a release, V=0.5.1: signed with the Developer ID, notarized, stapled, and the
# DMG that carries it, in app/macos/build/release (scripts/mac-release.sh; release.sh runs it).
mac-release:
	@[ -n "$(V)" ] || { echo "make mac-release V=<version>, e.g. V=0.5.1" >&2; exit 2; }
	scripts/mac-release.sh $(V)
