# 多阶段构建，生成最小化镜像
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG CGO_ENABLED=0

RUN apk add --no-cache git upx

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 构建并压缩
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=${CGO_ENABLED} \
    go build -trimpath -ldflags="-s -w -X main.AppVersion=${VERSION} -X main.BuildTime=$(date -u +%Y%m%d)" \
    -o apix . && \
    upx --best --lzma apix || true

# 最小化运行时镜像
FROM scratch

LABEL org.opencontainers.image.title="apix"
LABEL org.opencontainers.image.description="AI-friendly HTTP request minimization tool"
LABEL org.opencontainers.image.source="https://github.com/ejfkdev/apix"
LABEL org.opencontainers.image.licenses="MIT"

COPY --from=builder /build/apix /apix
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/apix"]
