# Make 构建指南

`Makefile` 提供编译、测试、交叉编译、打包、校验的完整工作流。全部使用 make 内置函数实现，跨平台兼容 Linux / macOS / Windows。

> **平台差异**：Linux / macOS 上使用 `make`；Windows 上使用 `mingw32-make`（MinGW）或 Git Bash 自带的 `make`。
> 建议在 PowerShell 中设置别名：`Set-Alias make mingw32-make`。

## 速查表

### 编译

| 命令 | 产物 | 说明 |
|------|------|------|
| `make build` | `bin/mcache` | 编译当前平台 |
| `make build-all` | `bin/mcache-*` | 全平台交叉编译 (5 个目标) |
| `make build-linux-amd64` | `bin/mcache-linux-amd64` | 单平台交叉编译 |

### 测试

| 命令 | 说明 |
|------|------|
| `make test` | 运行全部测试 |
| `make test-race` | 竞态检测（Linux/macOS） |
| `make test-cover` | 覆盖率报告 |
| `make lint` | `go vet ./...` |

### 打包

| 命令 | 说明 |
|------|------|
| `make dist` | **编译 + 打包** 当前平台 |
| `make dist-all` | **编译 + 打包** 全平台 + SHA256SUMS |
| `make dist-linux-amd64` | **编译 + 打包** linux/amd64 |
| `make package` | **仅打包** 当前平台（不编译） |
| `make package-all` | **仅打包** 全平台 + SHA256SUMS |
| `make package-linux-amd64` | **仅打包** linux/amd64 |
| `make src-dist` | **源码打包** → `dist/mcache-<ver>.tar.gz` |

### 其他

| 命令 | 说明 |
|------|------|
| `make install` | 安装到 `$GOPATH/bin` |
| `make clean` | 清理 `bin/` `dist/` |
| `make help` | 打印帮助 |
| `make help` | 打印帮助 |
| `make dist-checksums` | 单独生成 SHA256SUMS |

## 编译

### 基本编译

```bash
make build
# → bin/mcache  (linux → mcache, windows → mcache.exe)
```

产物是单一静态链接二进制，零运行时依赖。

### 交叉编译

```bash
make build-linux-amd64     # → bin/mcache-linux-amd64
make build-linux-arm64     # → bin/mcache-linux-arm64
make build-darwin-amd64    # → bin/mcache-darwin-amd64
make build-darwin-arm64    # → bin/mcache-darwin-arm64
make build-windows-amd64   # → bin/mcache-windows-amd64.exe

make build-all             # 一次性编译全部 5 个平台
```

### 手动编译（绕过 Make）

```bash
# 简单编译
go build -o bin/mcache ./cmd/mcache

# 带版本注入
go build -ldflags "\
  -X github.com/atoncooper/mcache/cli.Version=2.0.0 \
  -X github.com/atoncooper/mcache/cli.GitCommit=$(git rev-parse --short HEAD) \
  -X github.com/atoncooper/mcache/cli.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/mcache ./cmd/mcache
```

## 版本注入

Makefile 通过 `-ldflags` 将版本信息注入到 `cli` 包的全局变量：

```
cli/version.go:
  var Version   = "dev"       ← -X cli.Version=$(VERSION)
  var GitCommit = "unknown"   ← -X cli.GitCommit=$(COMMIT)
  var BuildTime = "unknown"   ← -X cli.BuildTime=$(DATE)
```

**取值优先级：**

| 变量 | 来源 | 默认值 |
|------|------|--------|
| `VERSION` | `git describe --tags --always --dirty` | `1.0.0` |
| `COMMIT` | `git rev-parse --short HEAD` | `unknown` |
| `DATE` | `date -u +%Y-%m-%dT%H:%M:%SZ` | — |

**手动指定版本：**

```bash
VERSION=2.0.0 make build
VERSION=2.0.0-rc1 make build-all
VERSION=2.0.0 make dist-all
```

版本号不要带 `v` 前缀（GitHub release 约定除外）。

### 验证版本

```bash
./bin/mcache version
# mcache 1.0.0 (commit: a1b2c3d, built: 2026-05-10T04:00:00Z)
```

### LDFLAGS 说明

| 标志 | 作用 |
|------|------|
| `-s` | 去除符号表（减小 ~25% 体积） |
| `-w` | 去除 DWARF 调试信息（减小 ~40% 体积） |
| `-X package.Var=value` | 编译期注入字符串值 |

> `-s -w` 不影响性能和功能，仅影响 `dlv` / `gdb` 调试体验。
> 开发调试时可去掉 `-s -w`。

## 测试

```bash
make test          # go test ./...
make test-race     # go test -race ./... （Linux/macOS 可用）
make test-cover    # go test -cover ./...
make lint          # go vet ./...
```

## 发布打包

两种工作流：

| 方式 | 命令 | 行为 |
|------|------|------|
| 一站式 | `make dist` / `make dist-all` | 编译 + 打包（一步到位） |
| 分阶段 | `make build-*` → `make package` | 编译和打包分离 |

### 一站式：`make dist`

```bash
# 当前平台
make dist

# 指定版本 + 全平台
VERSION=2.0.0 make dist-all
```

`dist` 自动触发对应平台的 `build-*`，然后打包。适合本地开发和单次发布。

### 分阶段：`make package`

```bash
# 阶段 1：并行编译全部平台（可复用、可缓存）
make -j5 build-all

# 阶段 2：并行打包（纯文件操作，秒级完成）
make -j5 package-all
```

`package` **不编译**，仅将 `bin/` 中已有的二进制打包为 `.tar.gz`。如果二进制缺失，直接报错退出：

```
ERROR: binary not found — run 'make build-windows-amd64' first
```

分阶段工作流适合 CI/CD：

- **编译阶段**可复用缓存（Go build cache），失败时只重跑编译
- **打包阶段**纯文件操作，秒级完成，不会因编译失败而需要重打包
- 两个阶段可独立调试

### 产物结构

```
dist/
├── mcache-2.0.0-linux-amd64.tar.gz
├── mcache-2.0.0-linux-arm64.tar.gz
├── mcache-2.0.0-darwin-amd64.tar.gz
├── mcache-2.0.0-darwin-arm64.tar.gz
├── mcache-2.0.0-windows-amd64.tar.gz
└── SHA256SUMS
```

### Tarball 内部结构

```
mcache-2.0.0-linux-amd64/
├── mcache              # 可执行文件
├── config.yaml         # 参考配置文件
├── LICENSE             # MIT 协议
└── README.md           # 项目说明
```

### 源码打包：`make src-dist`

创建不含平台后缀的源码 tarball，适合上传到 GitHub Releases 的 "Source code" 附件，或分发给从源码编译的用户。

```bash
make src-dist
# → dist/mcache-2.0.0.tar.gz  (~176 files)
```

**包含的文件：**

| 类别 | 内容 |
|------|------|
| Go 源码 | `*.go`, `go.mod`, `go.sum` |
| SDK | `sdk/go/`, `sdk/python/`, `sdk/c/` |
| 文档 | `docs/`（排除 plans/） |
| 工具 | `Makefile`, `scripts/pack.py`, `Dockerfile` |
| 配置 | `config.yaml` |
| 测试 | `tests/`, `*_test.go` |
| 项目 | `README.md`, `LICENSE`, `.gitignore` |

**排除的类别：**

| 类别 | 排除项 |
|------|--------|
| 构建产物 | `bin/`, `dist/`, `*.exe`, `*.test` |
| IDE | `.idea/`, `.vscode/`, `*.swp` |
| Python | `__pycache__/`, `.pytest_cache/` |
| 本地配置 | `config.local.yaml` |
| AI 辅助 | `.claude/`, `docs/plans/` |
| 日志 | `logs/`, `*.log`, `*.prof` |

### SHA256 校验

```bash
# 生成（dist-all / package-all 自动调用）
make dist-checksums

# 验证
cd dist && sha256sum -c SHA256SUMS
```

## CI/CD 集成

### 一站式（小型项目）

```yaml
- name: Build & Package
  run: VERSION=${{ github.ref_name }} make dist-all
```

### 分阶段（推荐，可缓存）

```yaml
# GitHub Actions
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24' }

      - name: Cross-compile all platforms
        run: VERSION=${{ github.ref_name }} make -j5 build-all

      - name: Package all platforms
        run: VERSION=${{ github.ref_name }} make -j5 package-all

      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: release
          path: dist/

  release:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with: { name: release, path: dist }
      - run: |
          gh release create ${{ github.ref_name }} \
            --title "mcache ${{ github.ref_name }}" \
            dist/*.tar.gz dist/SHA256SUMS
```

### GitLab CI（分阶段）

```yaml
stages:
  - build
  - package

build:
  stage: build
  script:
    - VERSION=$CI_COMMIT_TAG make -j5 build-all
  artifacts:
    paths: [bin/]
    expire_in: 1h

package:
  stage: package
  script:
    - VERSION=$CI_COMMIT_TAG make -j5 package-all
  artifacts:
    paths: [dist/]
```

## 平台矩阵

| 目标 | GOOS | GOARCH | 二进制名 |
|------|------|--------|---------|
| `linux-amd64` | linux | amd64 | `mcache` |
| `linux-arm64` | linux | arm64 | `mcache` |
| `darwin-amd64` | darwin | amd64 | `mcache` |
| `darwin-arm64` | darwin | arm64 | `mcache` |
| `windows-amd64` | windows | amd64 | `mcache.exe` |

> macOS 上 `darwin-amd64` 的产物需通过 `xcode-select` 确认 CGO 状态。
> 所有产物均使用 `CGO_ENABLED=0`（Go 默认静态链接，无 C 依赖）。

## 常见问题

### Windows：`make: command not found` / `系统找不到指定的路径`

Windows 自带终端（CMD / PowerShell）没有 `make`。两种解决方案：

**方案一：MinGW（推荐，无需安装 Git Bash）**

MinGW 提供原生 `mingw32-make.exe`，编译产物直接兼容 Windows CMD/PowerShell。

```powershell
# 安装：https://winlibs.com/ 或 chocolatey
choco install mingw

# 使用
mingw32-make build
mingw32-make package-windows-amd64

# 永久别名（添加到 $PROFILE）
Set-Alias make mingw32-make
```

**方案二：Git Bash**

Git for Windows 自带 Unix 版 `make`。打开 Git Bash 窗口：

```bash
# 在 Git Bash 中直接使用
make build
make package-linux-amd64
```

> Git Bash 的 `make` 会启动一个 Unix shell，`test -f` 等命令在 Git Bash 中可用，在 CMD/PowerShell 中不可用。
> 本 Makefile 已全部改用 make 内置 `$(wildcard)` 做文件检查，两种方案均兼容。

### macOS / Linux：`make: command not found`

```bash
# macOS
xcode-select --install        # 或 Homebrew
brew install make

# Ubuntu / Debian
sudo apt install build-essential

# CentOS / RHEL
sudo yum install make
```

### Windows：`tar: command not found`

Windows 10 1803+ 自带 `tar`（CMD/PowerShell 直接可用）。如缺失：

```powershell
# 验证
tar --version

# 或安装 Git Bash（自带 tar）
```

### `go: command not found`

安装 Go 1.24.3+：https://go.dev/dl/

### 交叉编译失败

```bash
# 检查是否支持目标平台
go tool dist list | grep -E "linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64"

# 确保 CGO_ENABLED=0（Makefile 默认行为）
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/mcache
```

### SHA256SUMS 为空或缺失

Makefile 自带四级回退机制：`sha256sum` → `shasum -a 256` → `certutil` → `python hashlib`。

手动生成：
```bash
make dist-checksums

# 或直接用 Python（全部平台通用）
python -c "
import hashlib, glob
for f in sorted(glob.glob('dist/*.tar.gz')):
    h = hashlib.sha256(open(f,'rb').read()).hexdigest()
    print(h, f.rsplit('/',1)[-1])
" > dist/SHA256SUMS
```
