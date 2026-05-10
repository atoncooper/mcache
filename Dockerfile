# ---- Build stage ----
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=1.0.0
ARG COMMIT=unknown
ARG BUILD_TIME

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w \
    -X github.com/atoncooper/mcache/cli.Version=${VERSION} \
    -X github.com/atoncooper/mcache/cli.GitCommit=${COMMIT} \
    -X github.com/atoncooper/mcache/cli.BuildTime=${BUILD_TIME}" \
    -o mcache ./cmd/mcache

# ---- Runtime stage ----
FROM alpine:3.21

RUN adduser -D -H mcache && \
    mkdir -p /etc/mcache /var/log/mcache && \
    chown mcache:mcache /var/log/mcache

COPY --from=builder /build/mcache /usr/local/bin/mcache

# Default config (override via volume mount or env)
COPY config.yaml /etc/mcache/config.yaml

USER mcache
EXPOSE 11211

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD mcache ping 2>/dev/null || exit 1

ENTRYPOINT ["mcache"]
CMD ["server", "--config", "/etc/mcache/config.yaml"]
