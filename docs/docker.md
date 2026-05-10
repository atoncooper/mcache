# Docker 部署与发布指南

## 目录

- [快速运行](#快速运行)
- [构建镜像](#构建镜像)
- [运行容器](#运行容器)
- [Docker Compose](#docker-compose)
- [自定义配置](#自定义配置)
- [发布到 Docker Hub](#发布到-docker-hub)
- [多平台构建](#多平台构建)
- [常用命令](#常用命令)

---

## 快速运行

已发布到 Docker Hub，一行命令启动：

```bash
docker run -d --name mcache -p 11211:11211 atoncooper/mcache:latest
```

验证：

```bash
docker exec mcache mcache ping
# PONG (0.12ms)

docker exec mcache mcache set hello world
docker exec mcache mcache get hello
# world
```

---

## 构建镜像

### 本地构建

```bash
# 默认版本
docker build -t mcache:latest .

# 指定版本
docker build \
    --build-arg VERSION=2.0.0 \
    --build-arg COMMIT=$(git rev-parse --short HEAD) \
    --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
    -t mcache:2.0.0 \
    -t mcache:latest .
```

### 镜像分层说明

```dockerfile
# Stage 1: 编译（~800 MB，构建后丢弃）
FROM golang:1.24-alpine AS builder
COPY go.mod go.sum ./    # ← 先复制依赖（利用 Docker 缓存层）
RUN go mod download      # ← 依赖下载层（仅在 go.mod 变化时重建）
COPY . .                 # ← 源码层
RUN go build ...         # ← 编译 → 静态二进制 ~8 MB

# Stage 2: 运行（~13 MB）
FROM alpine:3.21
COPY --from=builder ...  # ← 仅复制编译产物
HEALTHCHECK ...          # ← 健康检查
```

**最终镜像体积：~13 MB**（alpine + 静态二进制）。

---

## 运行容器

### 基本运行

```bash
docker run -d \
    --name mcache \
    -p 11211:11211 \
    atoncooper/mcache:latest
```

### 带 Prometheus 指标

```bash
docker run -d \
    --name mcache \
    -p 11211:11211 \
    -p 9090:9090 \
    -e MCACHE_METRICS_ENABLED=true \
    atoncooper/mcache:latest
```

### 挂载自定义配置

```bash
docker run -d \
    --name mcache \
    -p 11211:11211 \
    -v $(pwd)/config.yaml:/etc/mcache/config.yaml:ro \
    -v mcache_logs:/var/log/mcache \
    atoncooper/mcache:latest
```

### 限制资源

```bash
docker run -d \
    --name mcache \
    -p 11211:11211 \
    --memory=512m \
    --cpus=2 \
    atoncooper/mcache:latest
```

### 查看日志

```bash
docker logs -f mcache
# {"level":"info","ts":"2026-05-10T10:00:00Z","msg":"server listening","address":":11211"}
```

---

## Docker Compose

### 启动

```bash
docker compose up -d
```

### 查看状态

```bash
docker compose ps
docker compose logs -f mcache
```

### 停止

```bash
docker compose down          # 保留数据卷
docker compose down -v       # 删除数据卷
```

### 带 Prometheus 监控

```bash
docker compose --profile monitoring up -d
```

---

## 自定义配置

三种方式，优先级从高到低：

### 1. 挂载配置文件（推荐生产环境）

```bash
# 准备自定义配置
cp config.yaml my-mcache.yaml
vim my-mcache.yaml

# 挂载启动
docker run -d -p 11211:11211 \
    -v $(pwd)/my-mcache.yaml:/etc/mcache/config.yaml:ro \
    atoncooper/mcache:latest
```

### 2. 环境变量覆盖（推荐 Docker Compose）

Dockerfile 默认配置是一个合理的起点。调整关键参数用环境变量不方便时，用配置挂载。

### 3. 构建自定义镜像

```dockerfile
FROM atoncooper/mcache:latest
COPY my-config.yaml /etc/mcache/config.yaml
```

---

## 发布到 Docker Hub

### 前置准备

1. 注册 [Docker Hub](https://hub.docker.com/) 账号
2. 创建仓库：https://hub.docker.com/repository/create （名称 `mcache`）

### 登录

```bash
docker login
# Username: your-username
# Password: your-token   （Docker Hub → Account Settings → Personal Access Token）
```

### 构建并标记

```bash
VERSION=2.0.0
docker build \
    --build-arg VERSION=${VERSION} \
    --build-arg COMMIT=$(git rev-parse --short HEAD) \
    --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
    -t atoncooper/mcache:${VERSION} \
    -t atoncooper/mcache:latest .
```

### 推送

```bash
docker push atoncooper/mcache:2.0.0
docker push atoncooper/mcache:latest
```

### 验证

```bash
# 本地删除再拉取验证
docker rmi atoncooper/mcache:latest
docker run --rm atoncooper/mcache:latest version
# mcache 2.0.0 (commit: a1b2c3d, built: 2026-05-10T10:00:00Z)
```

### 完整发布脚本

```bash
#!/bin/bash
set -euo pipefail

VERSION="${1:?Usage: $0 <version>}"
IMAGE="atoncooper/mcache"

echo "=== Building mcache ${VERSION} ==="
docker build \
    --build-arg VERSION="${VERSION}" \
    --build-arg COMMIT="$(git rev-parse --short HEAD)" \
    --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -t "${IMAGE}:${VERSION}" \
    -t "${IMAGE}:latest" .

echo "=== Testing ==="
docker run --rm "${IMAGE}:${VERSION}" version

echo "=== Pushing ==="
docker push "${IMAGE}:${VERSION}"
docker push "${IMAGE}:latest"

echo "=== Done ==="
echo "docker run --rm atoncooper/mcache:${VERSION} version"
```

保存为 `scripts/docker-release.sh`，执行：

```bash
chmod +x scripts/docker-release.sh
./scripts/docker-release.sh 2.0.0
```

---

## 多平台构建

支持 `linux/amd64` 和 `linux/arm64`（Apple Silicon / AWS Graviton 原生运行）：

### 设置 buildx

```bash
docker buildx create --name multiarch --use
docker buildx inspect --bootstrap
```

### 构建并推送多平台镜像

```bash
VERSION=2.0.0

docker buildx build \
    --platform linux/amd64,linux/arm64 \
    --build-arg VERSION=${VERSION} \
    --build-arg COMMIT=$(git rev-parse --short HEAD) \
    --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
    -t atoncooper/mcache:${VERSION} \
    -t atoncooper/mcache:latest \
    --push .
```

一条命令完成：编译 → 打标签 → 推送两个架构的镜像。用户 `docker pull` 时自动匹配平台。

---

## 常用命令

| 场景 | 命令 |
|------|------|
| 启动服务 | `docker run -d --name mcache -p 11211:11211 atoncooper/mcache` |
| 查看版本 | `docker run --rm atoncooper/mcache version` |
| 交互式 CLI | `docker exec -it mcache mcache repl` |
| 设置键值 | `docker exec mcache mcache set k v` |
| 获取键值 | `docker exec mcache mcache get k` |
| 查看条目数 | `docker exec mcache mcache len` |
| 健康检查 | `docker exec mcache mcache ping` |
| 查看日志 | `docker logs -f mcache` |
| 查看资源 | `docker stats mcache` |
| 重启服务 | `docker restart mcache` |
| 停止删除 | `docker rm -f mcache` |
| 进入 shell | `docker exec -it mcache sh` |
| 拉取更新 | `docker pull atoncooper/mcache:latest` |
