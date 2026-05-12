# 发布指南 — mcache Go SDK

> Go module 的发布**不需要中央仓库**(无 PyPI/npm 对等物),也**不需要 token**。
> 全过程只有 `git tag` + `git push`,`proxy.golang.org` 会自动索引。

## 前置:理解项目布局

```
mcache/                          ← module: github.com/atoncooper/mcache
├── go.mod                       (服务端 + 公共 errors)
├── net/, errors.go, ...
└── sdk/
    └── go/                      ← module: github.com/atoncooper/mcache/sdk/go
        ├── go.mod               (SDK 独立 module)
        ├── client.go
        └── ...
```

这是 **monorepo 双 module** 布局,SDK 依赖根 module 的部分类型(如 `mcache.ErrKeyNotFound`)。

## 重要约束:不能带 `replace` 指令发布

当前 `sdk/go/go.mod`:

```go
replace github.com/atoncooper/mcache => ../../
```

这条 `replace` 指令**仅供本地开发**,因为外部用户没有 `../../` 路径。

**发布前必须移除 `replace`**,并把 `require` 指向一个已发布的根版本。

## Tag 命名规则(必须遵守)

| Module | Tag 格式 | 例子 |
|--------|---------|------|
| 根模块 `github.com/atoncooper/mcache` | `vMAJOR.MINOR.PATCH` | `v0.1.0` |
| 子模块 `.../sdk/go` | `sdk/go/vMAJOR.MINOR.PATCH` | `sdk/go/v1.0.0` |

[Go 官方文档:Modules in subdirectories](https://go.dev/ref/mod#vcs-version) 强制要求子目录 module 使用 `subdir/vX.Y.Z` 形式的 tag。

## 完整发布流程

### Step 1 — 发布根 module(首次必须)

```bash
cd D:/paper/cache/mcache

# 1. 确认 root go.mod 干净
git status

# 2. 打 root tag
git tag v0.1.0
git push origin v0.1.0

# 3. 验证(可选 — 让 proxy.golang.org 立刻索引)
GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod \
  go list -m github.com/atoncooper/mcache@v0.1.0
```

### Step 2 — 改 SDK 的 go.mod 指向已发布的根版本

编辑 `sdk/go/go.mod`:

```diff
 module github.com/atoncooper/mcache/sdk/go

 go 1.24.3

-require github.com/atoncooper/mcache v0.0.0
+require github.com/atoncooper/mcache v0.1.0

 require gopkg.in/yaml.v3 v3.0.1 // indirect

-replace github.com/atoncooper/mcache => ../../
```

跑一遍 `go mod tidy`:

```bash
cd sdk/go
go mod tidy
go build ./...
go test ./...
```

提交:

```bash
git add sdk/go/go.mod sdk/go/go.sum
git commit -m "release: sdk/go v1.0.0 — pin root to v0.1.0"
```

### Step 3 — 打 SDK tag 并推送

```bash
git tag sdk/go/v1.0.0
git push origin sdk/go/v1.0.0
```

### Step 4 — 强制 proxy 索引(可选,加速)

```bash
GOPROXY=https://proxy.golang.org \
  go list -m github.com/atoncooper/mcache/sdk/go@v1.0.0
```

`proxy.golang.org` 会在第一次有人 `go get` 时自动抓取,但你也可以主动触发。

### Step 5 — 验证 pkg.go.dev

访问 https://pkg.go.dev/github.com/atoncooper/mcache/sdk/go@v1.0.0,
首次访问会触发 pkg.go.dev 抓取并渲染 `doc.go` + `README.md`。约 5-10 分钟生效。

## 用户安装

```bash
go get github.com/atoncooper/mcache/sdk/go@v1.0.0
# 或拉最新
go get github.com/atoncooper/mcache/sdk/go@latest
```

## 发布后恢复本地开发模式

把 `replace` 加回去,这样本地修改根 module 时 SDK 立刻能看到:

```diff
 require github.com/atoncooper/mcache v0.1.0
+
+replace github.com/atoncooper/mcache => ../../
```

**注意**:本地 `replace` 不能 commit 进 release 分支。可以:
- 用 git stash 暂存 / 还原
- 或开发分支放 replace,release 分支不放

## 版本规划建议

| 场景 | 根 module 版本 | SDK 版本 |
|------|--------------|---------|
| 服务端 + SDK 同时发布初版 | `v0.1.0` | `sdk/go/v1.0.0` |
| 仅 SDK 修 bug | 不变 | `sdk/go/v1.0.1` |
| SDK 加新 API(向后兼容) | 不变 | `sdk/go/v1.1.0` |
| 服务端协议变更,SDK 跟进 | `v0.2.0` | `sdk/go/v1.2.0`(更新 require) |
| SDK Breaking 变更 | 不变 | `sdk/go/v2.0.0` + 改 module path 加 `/v2` |

## 检查清单

发布前确认:

- [ ] `sdk/go/CHANGELOG.md` 已添加新版本条目
- [ ] `sdk/go/go.mod` 已**移除** `replace` 指令
- [ ] `sdk/go/go.mod` 的 `require` 已指向**已发布**的根版本
- [ ] `cd sdk/go && go mod tidy` 通过
- [ ] `cd sdk/go && go build ./...` 通过
- [ ] `cd sdk/go && go test ./...` 通过
- [ ] `cd sdk/go && go vet ./...` 通过
- [ ] 根 module 的对应 tag(如 `v0.1.0`)已 push

## v2+ 重大变更

若发 `sdk/go/v2.0.0`,**必须**:

1. 修改 `sdk/go/go.mod`:
   ```
   module github.com/atoncooper/mcache/sdk/go/v2
   ```
2. 所有导入路径加 `/v2`:
   ```go
   import mcache "github.com/atoncooper/mcache/sdk/go/v2"
   ```
3. tag 仍是 `sdk/go/v2.0.0`(不变)

这是 Go [Major Version Suffix](https://go.dev/ref/mod#major-version-suffixes) 强制要求。

## 常见问题

### `go list -m ... no matching versions`
proxy.golang.org 还没索引。等几分钟,或者手动触发:
```bash
GOPROXY=https://proxy.golang.org go list -m github.com/atoncooper/mcache/sdk/go@v1.0.0
```

### `unknown revision sdk/go/v1.0.0`
tag 没 push 到远程。`git push origin sdk/go/v1.0.0`。

### `module declares its path as: ... but was required as ...`
你忘了去掉 `replace`,或者 `require` 的版本号还是 `v0.0.0`。检查 `sdk/go/go.mod`。

### 不小心发布了错版本
Go module 的 tag **不可撤回**(`proxy.golang.org` 永久缓存)。补救:
1. 在 https://pkg.go.dev/github.com/atoncooper/mcache/sdk/go 申请 retract,或
2. 在 `sdk/go/go.mod` 中声明 retract:
   ```go
   retract v1.0.0 // 错误版本
   ```
   然后发 `sdk/go/v1.0.1`。

### pkg.go.dev 没渲染 README
确认:
- tag 已 push
- README.md 文件存在于 `sdk/go/` 目录
- 首次访问 https://pkg.go.dev/github.com/atoncooper/mcache/sdk/go@v1.0.0 触发抓取
- 等待 5-10 分钟
