BUILD_DIR := build
GO_TAGS := -tags fts5
LDFLAGS := -s -w

PLATFORMS := darwin-arm64 linux-arm64 linux-amd64

# Platform → GOOS/GOARCH/CC mapping.
# CGO is required for go-sqlite3. Cross-compiling Linux targets from macOS
# requires a C cross-compiler (e.g. zig cc, musl-cross, or GNU toolchain).
# Set CC_LINUX_ARM64 / CC_LINUX_AMD64 to override.
darwin-arm64_GOOS   := darwin
darwin-arm64_GOARCH := arm64
darwin-arm64_CC     := $(CC)

linux-arm64_GOOS   := linux
linux-arm64_GOARCH := arm64
linux-arm64_CC     := $(or $(CC_LINUX_ARM64),aarch64-linux-gnu-gcc)

linux-amd64_GOOS   := linux
linux-amd64_GOARCH := amd64
linux-amd64_CC     := $(or $(CC_LINUX_AMD64),x86_64-linux-gnu-gcc)

.PHONY: all clean server migrate \
	$(PLATFORMS:%=server-%) $(PLATFORMS:%=migrate-%)

all: server migrate

server: $(PLATFORMS:%=server-%)
migrate: $(PLATFORMS:%=migrate-%)

$(PLATFORMS:%=server-%):
	$(eval P := $(@:server-%=%))
	CGO_ENABLED=1 GOOS=$($(P)_GOOS) GOARCH=$($(P)_GOARCH) CC="$($(P)_CC)" \
		go build $(GO_TAGS) -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/server-$(P) ./cmd/

$(PLATFORMS:%=migrate-%):
	$(eval P := $(@:migrate-%=%))
	CGO_ENABLED=1 GOOS=$($(P)_GOOS) GOARCH=$($(P)_GOARCH) CC="$($(P)_CC)" \
		go build $(GO_TAGS) -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/migrate-$(P) ./cmd/migrate/

clean:
	rm -rf $(BUILD_DIR)
