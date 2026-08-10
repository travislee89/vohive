BINARY_NAME ?= vohive
GO_TAGS ?= with_utls nomsgpack
GOOS ?= linux
CGO_ENABLED ?= 0
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "unknown")
VERSION_TAG = $(if $(filter v%,$(VERSION)),$(VERSION),v$(VERSION))
BUILD_TIME ?= $(shell date "+%Y-%m-%d %H:%M:%S")
DIST_DIR ?= dist
MAIN_PACKAGE ?= ./cmd/vohive
CI ?= ./scripts/ci.sh

LDFLAGS = -s -w -X 'github.com/boa-z/vohive/internal/global.Version=$(VERSION)' -X 'github.com/boa-z/vohive/internal/global.BuildTime=$(BUILD_TIME)'
GO_BUILD = go build -trimpath -buildvcs=false -tags "$(GO_TAGS)" -ldflags "$(LDFLAGS)"

AMD64_OUT = $(DIST_DIR)/$(BINARY_NAME)_$(VERSION_TAG)_linux_amd64
ARM64_OUT = $(DIST_DIR)/$(BINARY_NAME)_$(VERSION_TAG)_linux_arm64
ARMV7_OUT = $(DIST_DIR)/$(BINARY_NAME)_$(VERSION_TAG)_linux_armv7
# 压缩二进制：UPX 存在时启用，不存在时静默跳过（不影响编译）。
# 由调用方显式禁用（如 CI 的 disable-upx-by-default）或设置 UPX= 可跳过。
UPX ?= $(shell command -v upx || command -v upx-ucl)
UPX_FLAGS ?= --best --lzma

# compress-if-upx 当 UPX 可用时压缩目标二进制，否则静默跳过。
define compress-if-upx
	@if [ -n "$(UPX)" ]; then \
		echo "→ 压缩 $1 ($(UPX) $(UPX_FLAGS))"; \
		$(UPX) $(UPX_FLAGS) "$1" || { echo "警告: UPX 压缩失败，保留未压缩二进制"; }; \
	fi
endef

.PHONY: all ci build build-amd64 build-arm64 build-armv7 build-all frontend-dist clean

all: build-all

ci:
	GO_BIN="$${GO_BIN:-$$(command -v go 2>/dev/null || printf /usr/local/go/bin/go)}" $(CI)

build: build-amd64

build-all: build-amd64 build-arm64 build-armv7

frontend-dist:
	npm ci --prefix web
	npm run build --prefix web
	rm -rf internal/web/dist
	mkdir -p internal/web
	cp -R web/dist internal/web/dist

build-amd64: frontend-dist
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=amd64 $(GO_BUILD) -o $(AMD64_OUT) $(MAIN_PACKAGE)
	$(call compress-if-upx,$(AMD64_OUT))

build-arm64: frontend-dist
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=arm64 $(GO_BUILD) -o $(ARM64_OUT) $(MAIN_PACKAGE)
	$(call compress-if-upx,$(ARM64_OUT))

build-armv7: frontend-dist
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=arm GOARM=7 $(GO_BUILD) -o $(ARMV7_OUT) $(MAIN_PACKAGE)
	$(call compress-if-upx,$(ARMV7_OUT))

clean:
	go clean
	rm -rf $(DIST_DIR)
