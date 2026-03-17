# 构建优化指南

本文档介绍 apix 的构建优化配置，包括全平台支持和二进制体积优化。

## 🎯 优化目标

- **最小化二进制体积** - 使用 UPX 压缩后约 2-5MB
- **去除调试信息** - 移除符号表、DWARF 调试信息
- **去除路径信息** - 使用 `-trimpath` 去除本地构建路径
- **最大平台支持** - 支持 30+ 种平台架构

## 📦 支持的平台

### Linux (16 种)
- amd64 (v1, v2, v3, v4 - 针对不同 CPU 指令集优化)
- 386
- arm (v5, v6, v7)
- arm64
- mips, mips64, mips64le, mipsle
- ppc64, ppc64le
- riscv64
- s390x

### macOS (5 种)
- amd64 (v2, v3, v4 - 针对 Intel CPU 优化)
- arm64 (Apple Silicon M1/M2/M3)

### Windows (6 种)
- amd64 (v1, v2, v3, v4)
- 386
- arm64

### BSD 系列 (9 种)
- FreeBSD: amd64, 386, arm64, armv7
- OpenBSD: amd64, 386, arm64
- NetBSD: amd64, 386, arm64
- DragonFly BSD: amd64

### 其他 (2 种)
- Solaris/Illumos: amd64
- Plan 9: amd64, 386, arm

**总计: 38+ 种平台架构**

## 🔧 优化参数说明

### 编译器标志

```yaml
ldflags:
  - -s                    # 去除符号表
  - -w                    # 去除 DWARF 调试信息
  - -trimpath             # 去除本地文件系统路径
  - -X main.AppVersion=... # 注入版本信息

flags:
  - -trimpath             # 去除构建路径
  - -tags=osusergo,netgo  # 使用纯 Go 实现，避免 CGO
```

### UPX 压缩

```yaml
upx:
  enabled: true
  compress: best         # 最佳压缩比
  lzma: true            # 使用 LZMA 算法
```

## 📊 体积对比

| 优化级别 | 大小 | 说明 |
|----------|------|------|
| 无优化 | ~25 MB | 默认编译，包含调试信息 |
| -s -w | ~18 MB | 去除符号表和调试信息 |
| +trimpath | ~18 MB | 去除路径信息 |
| +UPX | ~3-5 MB | 最终压缩大小 |

## 🚀 本地构建

### 标准构建

```bash
# 开发构建（包含调试信息）
go build -o apix .

# 发布构建（已优化）
go build -trimpath -ldflags="-s -w" -o apix .

# 压缩（需要安装 UPX）
upx --best --lzma apix
```

### 交叉编译

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o apix-linux-amd64 .

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o apix-darwin-arm64 .

# Windows amd64
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o apix-windows-amd64.exe .
```

## 🐳 Docker 构建

```bash
# 构建多平台镜像
docker buildx build --platform linux/amd64,linux/arm64 -t apix:latest .

# 构建并加载到本地
docker buildx build --load -t apix:latest .

# 运行
docker run --rm apix:latest version
```

## 📋 发布流程

### 1. 本地测试构建

```bash
# 安装 GoReleaser
go install github.com/goreleaser/goreleaser/v2@latest

# 测试构建（不发布）
goreleaser release --snapshot --clean

# 查看构建结果
ls -la dist/
```

### 2. 验证压缩效果

```bash
# 检查文件大小
ls -lh dist/apix_*/apix

# 验证 UPX 压缩
file dist/apix_linux_amd64/apix
```

### 3. 正式发布

```bash
# 提交代码
git add .
git commit -m "Prepare for v0.2.0"

# 创建标签（触发 GitHub Actions）
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```

## 🔍 验证构建

### 检查二进制信息

```bash
# 查看文件大小
ls -lh apix

# 检查是否去除符号表
nm apix 2>&1 | head -5
# 应该显示: no symbols

# 检查是否去除 DWARF
readelf --debug-dump=info apix 2>&1 | head -5
# 应该显示为空或错误

# 检查是否去除路径
strings apix | grep "/home/" || echo "Paths stripped: OK"
```

### 测试运行

```bash
# 验证功能正常
./apix version
./apix --help

# 测试核心功能
./apix pure --no-cache curl https://httpbin.org/get
```

## ⚠️ 注意事项

### UPX 压缩限制

某些平台不支持 UPX 压缩：
- macOS (被 Apple 签名机制限制)
- 某些 BSD 系统
- 非常见架构

GoReleaser 配置中已自动排除这些平台。

### 调试问题

如果压缩后的二进制出现问题：

```bash
# 禁用 UPX 测试
goreleaser build --snapshot --clean --skip=upx

# 检查原始二进制
./dist/apix_linux_amd64/apix version
```

### 安全扫描

某些杀毒软件可能误报 UPX 压缩的二进制：
- 这是 UPX 的已知问题
- 可以向杀毒软件厂商提交误报
- 或提供未压缩版本供下载

## 📚 参考链接

- [Go 链接器标志](https://pkg.go.dev/cmd/link)
- [UPX 文档](https://upx.github.io/)
- [GoReleaser 构建](https://goreleaser.com/customization/build/)
- [Go 交叉编译](https://go.dev/doc/install/source#environment)
