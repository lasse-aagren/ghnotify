MODULE  := github.com/boyvinall/ghnotify
BINARY  := ghnotify
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
APP_DIR := dist/ghnotify.app

.PHONY: help all build app install build-all release release-local lint test clean

define PROMPT
	@echo
	@echo "**********************************************************"
	@echo "*"
	@echo "*   $(1)"
	@echo "*"
	@echo "**********************************************************"
	@echo
endef

#: build app, lint, and test (default)
all: app lint test

#: compile for the current platform (CGO required for systray on macOS)
build:
	$(call PROMPT, $@)
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

#: assemble a macOS .app bundle from the local build
app: build
	$(call PROMPT, $@)
	bash scripts/make-app.sh bin/$(BINARY) $(APP_DIR)

#: install the .app bundle into /Applications
install: app
	$(call PROMPT, $@)
	rm -rf /Applications/ghnotify.app
	cp -r $(APP_DIR) /Applications/

#: cross-compile all supported platforms via goreleaser (snapshot, no publish)
build-all:
	$(call PROMPT, $@)
	goreleaser build --snapshot --clean

#: tag a new version and push, triggering the release workflow (VERSION_TAG=v1.2.3)
release:
	$(call PROMPT, $@)
	@if [ -z "$(VERSION_TAG)" ]; then echo "Usage: make release VERSION_TAG=v1.2.3"; exit 1; fi
	git tag $(VERSION_TAG)
	git push origin $(VERSION_TAG)

#: publish a release locally via goreleaser (requires GITHUB_TOKEN)
release-local:
	$(call PROMPT, $@)
	goreleaser release --clean

#: run all linters
lint:
	$(call PROMPT, $@)
	golangci-lint run ./...

#: run all tests
test:
	$(call PROMPT, $@)
	go test ./...

#: remove build artifacts
clean:
	$(call PROMPT, $@)
	rm -rf bin/ dist/

#: print Makefile targets and short descriptions
help:
	@echo "make targets:\n"
	@awk '/^#:[[:space:]]/ { sub(/^#:[[:space:]]*/, ""); desc=$$0; next } \
		/^[[:space:]]*$$/ { next } \
		/^#/ { next } \
		/^[a-zA-Z][a-zA-Z0-9_.-]*:/ { \
			if (desc != "") { \
				split($$0, a, ":"); \
				tgt=a[1]; \
				gsub(/^[[:space:]]+|[[:space:]]+$$/, "", tgt); \
				printf "  %-18s %s\n", tgt, desc; \
				desc="" \
			} \
		}' $(firstword $(MAKEFILE_LIST))
