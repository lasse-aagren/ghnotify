MODULE  := github.com/boyvinall/ghnotify
BINARY  := ghnotify
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
APP_DIR := dist/ghnotify.app

# Current-platform build (CGO required for systray on macOS)
.PHONY: build
build:
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

# Assemble a macOS .app bundle from the local build
.PHONY: app
app: build
	bash scripts/make-app.sh bin/$(BINARY) $(APP_DIR)

# Install the .app bundle into /Applications
.PHONY: install
install: app
	rm -rf /Applications/ghnotify.app
	cp -r $(APP_DIR) /Applications/

# Cross-compile all supported platforms via goreleaser (snapshot, no publish)
.PHONY: build-all
build-all:
	goreleaser build --snapshot --clean

# Tag a new version and push, which triggers the release workflow
.PHONY: release
release:
	@if [ -z "$(VERSION_TAG)" ]; then echo "Usage: make release VERSION_TAG=v1.2.3"; exit 1; fi
	git tag $(VERSION_TAG)
	git push origin $(VERSION_TAG)

# Publish a release locally via goreleaser (requires GITHUB_TOKEN)
.PHONY: release-local
release-local:
	goreleaser release --clean

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test
test:
	go test ./...

.PHONY: clean
clean:
	rm -rf bin/ dist/
