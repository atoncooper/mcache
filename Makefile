.PHONY: all build build-all test test-race test-cover lint install clean \
        dist dist-all dist-linux-amd64 dist-linux-arm64 dist-darwin-amd64 dist-darwin-arm64 dist-windows-amd64 dist-checksums \
        src-dist \
        package package-all package-linux-amd64 package-linux-arm64 package-darwin-amd64 package-darwin-arm64 package-windows-amd64 \
        help

# ── version ────────────────────────────────────────────────────────────────
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/atoncooper/mcache/cli.Version=$(VERSION) \
	-X github.com/atoncooper/mcache/cli.GitCommit=$(COMMIT) \
	-X github.com/atoncooper/mcache/cli.BuildTime=$(DATE)

# ── output layout ──────────────────────────────────────────────────────────
OUTDIR   := bin
BINARY   := $(OUTDIR)/mcache
DISTDIR  := dist
PKG_NAME := mcache-$(VERSION)

# platform matrix
PLATFORMS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64

# ── default ────────────────────────────────────────────────────────────────
all: build

# ── compile ────────────────────────────────────────────────────────────────
build:
	@mkdir -p $(OUTDIR)
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/mcache

build-linux-amd64:
	@mkdir -p $(OUTDIR)
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/mcache-linux-amd64   ./cmd/mcache

build-linux-arm64:
	@mkdir -p $(OUTDIR)
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/mcache-linux-arm64   ./cmd/mcache

build-darwin-amd64:
	@mkdir -p $(OUTDIR)
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/mcache-darwin-amd64  ./cmd/mcache

build-darwin-arm64:
	@mkdir -p $(OUTDIR)
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/mcache-darwin-arm64  ./cmd/mcache

build-windows-amd64:
	@mkdir -p $(OUTDIR)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/mcache-windows-amd64.exe ./cmd/mcache

build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64

# ── release packaging ──────────────────────────────────────────────────────
#
#  Packaging delegates to scripts/pack.py (cross-platform, Python stdlib only).
#  Layout inside each tarball:
#
#    mcache-<version>-<platform>/
#    ├── mcache              (binary)
#    ├── config.yaml         (reference config)
#    ├── LICENSE
#    └── README.md

define package
	@echo "  → packaging $(1) …"
	@python scripts/pack.py "$(1)" --output-dir "$(DISTDIR)" --pkg-name "$(PKG_NAME)" --bin-dir "$(OUTDIR)"
endef

dist-linux-amd64:   build-linux-amd64   ; $(call package,linux-amd64)
dist-linux-arm64:   build-linux-arm64   ; $(call package,linux-arm64)
dist-darwin-amd64:  build-darwin-amd64  ; $(call package,darwin-amd64)
dist-darwin-arm64:  build-darwin-arm64  ; $(call package,darwin-arm64)
dist-windows-amd64: build-windows-amd64 ; $(call package,windows-amd64)

# 单平台打包 — picks the right build target automatically
dist:
	@$(MAKE) build-$(shell go env GOOS)-$(shell go env GOARCH)
	$(call package,$(shell go env GOOS)-$(shell go env GOARCH))

# 全平台打包
dist-all: build-all dist-linux-amd64 dist-linux-arm64 dist-darwin-amd64 dist-darwin-arm64 dist-windows-amd64 dist-checksums

# 生成 SHA256 校验文件
dist-checksums:
	@echo "  → generating SHA256 checksums …"
	@python scripts/pack.py --checksums --output-dir "$(DISTDIR)"

# 源码打包
src-dist:
	@echo "  → packaging source …"
	@python scripts/pack.py --source --output-dir "$(DISTDIR)" --pkg-name "$(PKG_NAME)"

# ── package-only (no build — expects pre-compiled binaries in bin/) ─────────
# File existence is checked with make's $(wildcard), which works on all platforms.

_BIN_linux_amd64   := $(OUTDIR)/mcache-linux-amd64
_BIN_linux_arm64   := $(OUTDIR)/mcache-linux-arm64
_BIN_darwin_amd64  := $(OUTDIR)/mcache-darwin-amd64
_BIN_darwin_arm64  := $(OUTDIR)/mcache-darwin-arm64
_BIN_windows_amd64 := $(OUTDIR)/mcache-windows-amd64.exe

package-linux-amd64:
	@$(if $(wildcard $(_BIN_linux_amd64)),,\
		$(error ERROR: binary not found ($(_BIN_linux_amd64)) — run 'make build-linux-amd64' first))
	$(call package,linux-amd64)

package-linux-arm64:
	@$(if $(wildcard $(_BIN_linux_arm64)),,\
		$(error ERROR: binary not found ($(_BIN_linux_arm64)) — run 'make build-linux-arm64' first))
	$(call package,linux-arm64)

package-darwin-amd64:
	@$(if $(wildcard $(_BIN_darwin_amd64)),,\
		$(error ERROR: binary not found ($(_BIN_darwin_amd64)) — run 'make build-darwin-amd64' first))
	$(call package,darwin-amd64)

package-darwin-arm64:
	@$(if $(wildcard $(_BIN_darwin_arm64)),,\
		$(error ERROR: binary not found ($(_BIN_darwin_arm64)) — run 'make build-darwin-arm64' first))
	$(call package,darwin-arm64)

package-windows-amd64:
	@$(if $(wildcard $(_BIN_windows_amd64)),,\
		$(error ERROR: binary not found ($(_BIN_windows_amd64)) — run 'make build-windows-amd64' first))
	$(call package,windows-amd64)

# 当前平台打包（不编译）
package:
	$(call package,$(shell go env GOOS)-$(shell go env GOARCH))

# 全平台打包（不编译）
package-all: package-linux-amd64 package-linux-arm64 package-darwin-amd64 package-darwin-arm64 package-windows-amd64 dist-checksums

# ── test ────────────────────────────────────────────────────────────────────
test:
	go test ./...

test-race:
	go test -race ./...

test-cover:
	go test -cover ./...

# ── lint ────────────────────────────────────────────────────────────────────
lint:
	go vet ./...

# ── install ─────────────────────────────────────────────────────────────────
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/mcache

# ── clean ───────────────────────────────────────────────────────────────────
clean:
	rm -rf $(OUTDIR)
	rm -rf $(DISTDIR)

# ── help ────────────────────────────────────────────────────────────────────
help:
	@echo "make build               编译当前平台"
	@echo "make build-all           全平台交叉编译"
	@echo "make build-linux-amd64   交叉编译 linux/amd64"
	@echo "make build-linux-arm64   交叉编译 linux/arm64"
	@echo "make build-darwin-amd64  交叉编译 darwin/amd64"
	@echo "make build-darwin-arm64  交叉编译 darwin/arm64"
	@echo "make build-windows-amd64 交叉编译 windows/amd64"
	@echo ""
	@echo "make dist                编译 + 打包当前平台 → dist/*.tar.gz"
	@echo "make dist-all            编译 + 全平台打包 + SHA256SUMS"
	@echo "make dist-linux-amd64    编译 + 打包 linux/amd64"
	@echo ""
	@echo "make package             打包当前平台 (不编译，需预编译)"
	@echo "make package-all         全平台打包 (不编译)"
	@echo "make package-linux-amd64 打包 linux/amd64 (不编译)"
	@echo "make src-dist            源码打包 → dist/mcache-<ver>.tar.gz"
	@echo "make dist-checksums      生成 SHA256SUMS"
	@echo ""
	@echo "make test                运行测试"
	@echo "make test-race           竞态检测"
	@echo "make test-cover          覆盖率"
	@echo "make lint                go vet"
	@echo "make install             安装到 GOPATH/bin"
	@echo "make clean               清理 bin/ dist/"
	@echo ""
	@echo "VERSION  = $(VERSION)"
	@echo "COMMIT   = $(COMMIT)"
