#!/bin/bash

set -e

# apix 安装脚本
# 支持全平台自动检测和安装
# 使用方法: curl -fsSL https://raw.githubusercontent.com/ejfkdev/apix/main/install.sh | bash

REPO="ejfkdev/apix"
BINARY_NAME="apix"
INSTALL_DIR="/usr/local/bin"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 检测操作系统
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux";;
        Darwin*)    echo "darwin";;
        CYGWIN*|MINGW*|MSYS*|Windows_NT*) echo "windows";;
        FreeBSD*)   echo "freebsd";;
        OpenBSD*)   echo "openbsd";;
        NetBSD*)    echo "netbsd";;
        DragonFly*) echo "dragonfly";;
        SunOS*)     echo "solaris";;
        *)          echo "unknown";;
    esac
}

# 检测架构
detect_arch() {
    local arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)   echo "amd64";;
        arm64|aarch64)  echo "arm64";;
        i386|i686)      echo "386";;
        armv7l|armv7)   echo "arm_7";;
        armv6l)         echo "arm_6";;
        armv5l)         echo "arm_5";;
        mips)           echo "mips";;
        mips64)         echo "mips64";;
        mips64el)       echo "mips64le";;
        mipsel)         echo "mipsle";;
        ppc64)          echo "ppc64";;
        ppc64le)        echo "ppc64le";;
        riscv64)        echo "riscv64";;
        s390x)          echo "s390x";;
        *)              echo "$arch";;
    esac
}

# 打印信息
info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

OS=$(detect_os)
ARCH=$(detect_arch)

info "Detected platform: $OS $ARCH"

if [ "$OS" = "unknown" ]; then
    error "Unsupported platform: $(uname -s)"
    error "Please download manually from https://github.com/$REPO/releases"
    exit 1
fi

# 检查是否需要 sudo
if [ ! -w "$INSTALL_DIR" ]; then
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
    warn "Installing to $INSTALL_DIR (requires adding to PATH)"
fi

# 获取最新版本
if [ -z "$VERSION" ]; then
    info "Fetching latest version..."
    VERSION=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$VERSION" ]; then
        error "Failed to get latest version"
        exit 1
    fi
fi

success "Installing $BINARY_NAME $VERSION"

# 构建下载 URL
FILENAME="${BINARY_NAME}_${VERSION}_${OS}_${ARCH}"
if [ "$OS" = "windows" ]; then
    FILENAME="${FILENAME}.zip"
else
    FILENAME="${FILENAME}.tar.gz"
fi

URL="https://github.com/$REPO/releases/download/$VERSION/$FILENAME"

info "Downloading from $URL..."

# 下载
curl -fsSL "$URL" -o "/tmp/$FILENAME" || {
    error "Download failed!"
    error "URL: $URL"
    error "Please check if the platform is supported."
    exit 1
}

success "Downloaded successfully"

# 解压
info "Extracting..."
cd /tmp
if [ "$OS" = "windows" ]; then
    unzip -o "$FILENAME" "$BINARY_NAME.exe" || {
        error "Extraction failed!"
        exit 1
    }
else
    tar -xzf "$FILENAME" "$BINARY_NAME" || {
        error "Extraction failed!"
        exit 1
    }
fi

# 安装
echo "Installing to $INSTALL_DIR..."

if [ "$OS" = "windows" ]; then
    mv "$BINARY_NAME.exe" "$INSTALL_DIR/"
    success "Installed to $INSTALL_DIR/$BINARY_NAME.exe"
else
    mv "$BINARY_NAME" "$INSTALL_DIR/"
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    success "Installed to $INSTALL_DIR/$BINARY_NAME"
fi

# 清理
rm -f "/tmp/$FILENAME"

# 检查 PATH
echo ""
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    warn "$INSTALL_DIR is not in your PATH"
    echo ""
    echo "Please add the following line to your shell profile:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    echo ""
    echo "For bash: echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.bashrc"
    echo "For zsh:  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.zshrc"
fi

echo ""
success "$BINARY_NAME $VERSION installed successfully!"
echo ""
"$INSTALL_DIR/$BINARY_NAME" version 2>/dev/null || true
