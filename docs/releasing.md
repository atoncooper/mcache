# 发布指南

mcache 项目包含三类可独立发布的产物，本文档统一约定各自的版本规划、tag 命名、发布流程与检查清单。

| 产物 | 类型 | 分发渠道 | tag 规则 |
|------|------|---------|---------|
| 服务端二进制 / 根 module | Go module + 可执行文件 | GitHub Release + `pkg.go.dev` | `vMAJOR.MINOR.PATCH` |
| Go SDK | Go module（子目录） | `pkg.go.dev` + `proxy.golang.org` | `sdk/go/vMAJOR.MINOR.PATCH` |
| Python SDK | Wheel + sdist | PyPI | `py-vMAJOR.MINOR.PATCH` |

> 详细的 SDK 特定流程见各 SDK 目录下的文档：
> - Go：[`sdk/go/RELEASING.md`](../sdk/go/RELEASING.md)
> - Python：[`sdk/python/PUBLISHING.md`](../sdk/python/PUBLISHING.md)

---

## 1. 版本规划

### 1.1 语义化版本

所有产物遵循 [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html)：

| 段 | 含义 | 示例 |
|----|------|------|
| MAJOR | 不兼容的 API 变更 | `2.0.0` |
| MINOR | 向后兼容的新功能 | `1.1.0` |
| PATCH | 向后兼容的修复 | `1.0.1` |

预发布版本使用 `-` 后缀：`v1.0.0-rc.1`、`v1.0.0-beta.2`。

### 1.2 三类产物的版本关系

三个产物**独立编号**，但 SDK 的兼容性依赖于服务端协议版本：

```
服务端  v0.1.x  ─┐
                 ├─→  Go SDK     v1.0.x  /  v1.1.x
                 └─→  Python SDK v1.0.x  /  v1.1.x

服务端  v0.2.x  ─┐  (协议变更)
                 ├─→  Go SDK     v1.2.0+
                 └─→  Python SDK v1.2.0+
```

CHANGELOG 中必须显式标注最低服务端版本要求：

```markdown
### Requirements
- mcache server v0.1.x+
```

### 1.3 v2+ 重大版本

对于 Go SDK，发 `v2.0.0` 必须改 module path：

```diff
- module github.com/atoncooper/mcache/sdk/go
+ module github.com/atoncooper/mcache/sdk/go/v2
```

Python SDK 不受此约束，但建议同步说明 breaking 变更。

---

## 2. Tag 命名约定

不同 module 的 tag 必须用前缀区分，否则 Go module 系统会无法正确解析子目录 module。

| 产物 | tag 格式 | 例子 | 推送命令 |
|------|---------|------|---------|
| 根 module（服务端） | `v{X}.{Y}.{Z}` | `v0.1.0` | `git tag v0.1.0 && git push origin v0.1.0` |
| Go SDK | `sdk/go/v{X}.{Y}.{Z}` | `sdk/go/v1.0.0` | `git tag sdk/go/v1.0.0 && git push origin sdk/go/v1.0.0` |
| Python SDK | `py-v{X}.{Y}.{Z}` | `py-v1.0.0` | `git tag py-v1.0.0 && git push origin py-v1.0.0` |

> Go 子目录 module 的前缀规则由 [Go 官方规范](https://go.dev/ref/mod#vcs-version) 强制要求，不可变更。

---

## 3. 服务端 / 根 module 发布

### 3.1 准备工作

```bash
# 1. 拉取最新代码并切到 master
git checkout master && git pull origin master

# 2. 全平台编译验证
make build-all

# 3. 全量测试
make test
go vet ./...
```

### 3.2 更新版本

```bash
# CHANGELOG.md 添加版本条目（如有）
# README.md 中的版本号引用（如有）
```

### 3.3 打 tag 并推送

```bash
git tag -a v0.1.0 -m "release: mcache server v0.1.0"
git push origin v0.1.0
```

### 3.4 触发 GitHub Release（可选）

若仓库配置了 `release.yml` workflow，tag push 后会自动：
- 编译全平台二进制（linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64）
- 上传到 GitHub Release 页面

否则手动上传 `make build-all` 产物到 https://github.com/atoncooper/mcache/releases/new。

### 3.5 强制 Go proxy 索引（可选）

```bash
GOPROXY=https://proxy.golang.org \
  go list -m github.com/atoncooper/mcache@v0.1.0
```

---

## 4. Go SDK 发布

### 4.1 关键约束

Go SDK 是 monorepo 中的**独立 module**（`sdk/go/go.mod`），它依赖根 module。本地开发时通过 `replace` 指令指向上级目录：

```go
replace github.com/atoncooper/mcache => ../../
```

**发布前必须移除 `replace`**，且 `require` 必须指向一个已发布的根版本。

### 4.2 流程概览

```
[1] 发布根 module        →   git tag v0.1.0
[2] 改 sdk/go/go.mod     →   去 replace + 升 require 版本
[3] go mod tidy + 测试   →   go build && go test ./...
[4] commit 改动
[5] 打 SDK tag           →   git tag sdk/go/v1.0.0
[6] 推送                  →   git push origin sdk/go/v1.0.0
[7] 索引 + 验证 pkg.go.dev
```

### 4.3 具体步骤

完整步骤见 [`sdk/go/RELEASING.md`](../sdk/go/RELEASING.md)。

简化命令序列：

```bash
# Step 1: 发根 module（若已发，跳过）
git tag v0.1.0
git push origin v0.1.0

# Step 2: 编辑 sdk/go/go.mod
#   - require github.com/atoncooper/mcache v0.0.0   → v0.1.0
#   - 删除 replace 行
cd sdk/go
go mod tidy
go build ./... && go test ./...

# Step 3: commit
git add sdk/go/go.mod sdk/go/go.sum
git commit -m "release: sdk/go v1.0.0 — pin root to v0.1.0"
git push origin master

# Step 4: 打 SDK tag
git tag sdk/go/v1.0.0
git push origin sdk/go/v1.0.0

# Step 5: 触发 proxy 索引（可选）
GOPROXY=https://proxy.golang.org \
  go list -m github.com/atoncooper/mcache/sdk/go@v1.0.0
```

### 4.4 用户安装

```bash
go get github.com/atoncooper/mcache/sdk/go@v1.0.0
# 或最新
go get github.com/atoncooper/mcache/sdk/go@latest
```

### 4.5 发布后恢复本地开发

把 `replace` 加回去（**不要 commit**）：

```diff
 require github.com/atoncooper/mcache v0.1.0
+
+replace github.com/atoncooper/mcache => ../../
```

---

## 5. Python SDK 发布

### 5.1 分发名

PyPI 上的名字是 `mcache-py`（`mcache` 已被占用），但导入仍是 `import mcache`。

### 5.2 发布渠道

| 渠道 | 用途 | 触发方式 |
|------|------|---------|
| TestPyPI | 演练 / 灰度 | GitHub Actions 手动 dispatch → `testpypi` |
| PyPI | 正式发布 | tag push `py-v*` 自动触发 |

CI workflow 文件：`.github/workflows/python-publish.yml`。使用 OIDC Trusted Publishing，**无需 token**。

### 5.3 流程

完整步骤见 [`sdk/python/PUBLISHING.md`](../sdk/python/PUBLISHING.md)。

简化版：

```bash
# 1. 改三处版本号
#    sdk/python/pyproject.toml         version = "1.1.0"
#    sdk/python/mcache/__init__.py     __version__ = "1.1.0"
#    sdk/python/CHANGELOG.md           ## [1.1.0] — 2026-MM-DD

# 2. commit
git add sdk/python/
git commit -m "release: mcache-py v1.1.0"
git push origin master

# 3. （可选）TestPyPI 演练
#    GitHub → Actions → "Publish Python SDK to PyPI" → Run workflow → testpypi

# 4. 打 tag 触发 PyPI 自动发布
git tag py-v1.1.0
git push origin py-v1.1.0
```

### 5.4 本地手工发布（备用）

CI 不可用时：

```bash
cd sdk/python
./scripts/publish.sh test    # 演练
./scripts/publish.sh         # 正式（需输入 yes 二次确认）
```

需要本地配置 `~/.pypirc` 或 `PYPI_API_TOKEN` 环境变量。

### 5.5 用户安装

```bash
pip install mcache-py
```

---

## 6. 联动发布场景

### 6.1 三个产物同时发布初版

```bash
# 1. 根 module
git tag v0.1.0 && git push origin v0.1.0

# 2. Go SDK（改 go.mod 去 replace、改 require）
# ...编辑 sdk/go/go.mod...
git add sdk/go/go.mod sdk/go/go.sum
git commit -m "release: sdk/go v1.0.0 — pin root to v0.1.0"
git push origin master
git tag sdk/go/v1.0.0 && git push origin sdk/go/v1.0.0

# 3. Python SDK（已确认版本号）
git tag py-v1.0.0 && git push origin py-v1.0.0
```

### 6.2 仅 SDK 修 bug

服务端不变：

```bash
# Go SDK
git tag sdk/go/v1.0.1 && git push origin sdk/go/v1.0.1

# Python SDK
git tag py-v1.0.1 && git push origin py-v1.0.1
```

### 6.3 服务端协议变更

需要 SDK 同步跟进：

```bash
# 1. 发服务端
git tag v0.2.0 && git push origin v0.2.0

# 2. Go SDK 跟进（更新 require 到 v0.2.0）
# ...编辑 + commit...
git tag sdk/go/v1.2.0 && git push origin sdk/go/v1.2.0

# 3. Python SDK 跟进
git tag py-v1.2.0 && git push origin py-v1.2.0
```

CHANGELOG 中必须更新 `Requirements` 段，标注最低服务端版本。

---

## 7. 检查清单

### 7.1 通用

- [ ] `master` 已同步远程
- [ ] 全量测试通过（`make test`、`go vet`、`go test -race ./...`）
- [ ] CHANGELOG 已添加新版本条目（含日期）
- [ ] 版本号在所有相关位置一致

### 7.2 服务端

- [ ] `make build-all` 成功
- [ ] `bin/` 下二进制无依赖警告
- [ ] CLI `mcache --version` 输出新版本

### 7.3 Go SDK

- [ ] `sdk/go/go.mod` **已移除** `replace` 指令
- [ ] `sdk/go/go.mod` 的 `require` 指向**已发布**的根版本
- [ ] `cd sdk/go && go mod tidy` 无变更
- [ ] `cd sdk/go && go build ./...` 通过
- [ ] `cd sdk/go && go test ./...` 通过
- [ ] `cd sdk/go && go vet ./...` 通过

### 7.4 Python SDK

- [ ] `sdk/python/pyproject.toml` 的 `version` 已更新
- [ ] `sdk/python/mcache/__init__.py` 的 `__version__` 一致
- [ ] `cd sdk/python && python -m build` 成功生成 sdist + wheel
- [ ] `cd sdk/python && python -m twine check dist/*` 通过
- [ ] `mcache/py.typed` 文件存在（PEP 561 marker）
- [ ] PyPI Trusted Publisher 已配置（或 token 可用）

---

## 8. 回滚与撤销

### 8.1 Go module

**Tag 不可删除**（`proxy.golang.org` 永久缓存）。错版本只能通过 `retract` 标记：

在 `go.mod` 中：

```go
retract v1.0.0 // 内存泄漏，使用 v1.0.1
```

或针对范围：

```go
retract [v1.0.0, v1.0.2] // 包含 v1.0.0 ~ v1.0.2
```

然后发新版本（如 `sdk/go/v1.0.3`）。用户 `go get` 不会再升级到被 retract 的版本。

### 8.2 PyPI

**已上传的文件不能修改、不能重传同版本号**。错版本处理：

1. 在 https://pypi.org/manage/project/mcache-py/ 删除该版本（限发布后 30 天内）或标为 "yanked"
2. 修复后发新版本（如 `1.0.1`）

### 8.3 GitHub Release

可直接删除/编辑，但若 tag 已被 Go proxy 索引，需配合 8.1 的 retract。

---

## 9. 常见问题

### Q1：Go SDK 用户报 `module declares its path as...`
A：你忘了去 `replace` 或 `require` 还指向 `v0.0.0`。检查 `sdk/go/go.mod`。

### Q2：`pkg.go.dev` 没显示新版本
A：等 5~10 分钟，或手动触发：`GOPROXY=https://proxy.golang.org go list -m <module>@<ver>`。

### Q3：PyPI 报 `400 File already exists`
A：同版本号不可重传。bump version 后重新发布。

### Q4：CI Trusted Publishing 失败
A：检查 PyPI 后台 https://pypi.org/manage/project/mcache-py/settings/publishing/ 的 Owner/Repo/Workflow/Environment 是否与实际匹配。

### Q5：发了不该发的版本怎么办
A：Go 用 `retract`（见 8.1），PyPI 用 yank（见 8.2）。**绝不要尝试改 tag 或删 tag 后重发** —— 这会让所有已升级的用户拉到不同内容的同一版本。

---

## 10. 参考

- [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html)
- [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/)
- [Go Modules Reference](https://go.dev/ref/mod)
- [PEP 440 — Version Identification](https://peps.python.org/pep-0440/)
- [PyPI Trusted Publishing](https://docs.pypi.org/trusted-publishers/)
