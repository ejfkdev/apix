# apix Makefile
# 简化构建和版本管理

BINARY_NAME := apix
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILDTIME := $(shell date -u +%Y%m%d_%H%M%S)

# 构建标志
LDFLAGS := -s -w \
	-X main.AppVersion=$(VERSION) \
	-X main.GitCommit=$(COMMIT) \
	-X main.BuildTime=$(BUILDTIME)

BUILD_FLAGS := -trimpath -ldflags "$(LDFLAGS)"

# 默认目标
.PHONY: all build clean test release-snapshot help version

all: build

## 构建当前平台
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	go build $(BUILD_FLAGS) -o $(BINARY_NAME) .
	@echo "Done: ./$(BINARY_NAME)"

## 开发构建（快速，无优化）
dev:
	go build -o $(BINARY_NAME) .

## 运行测试
test:
	go test -v ./...

## 清理构建产物
clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-*
	rm -rf dist/

## 生成本地快照（不发布）
release-snapshot:
	@if ! command -v goreleaser &> /dev/null; then \
		echo "Installing GoReleaser..."; \
		go install github.com/goreleaser/goreleaser/v2@latest; \
	fi
	goreleaser release --snapshot --clean

## 检查版本一致性
check-version:
	@echo "Current git tag: $(VERSION)"
	@echo "Checking if tag matches expected format..."
	@if echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+'; then \
		echo "✅ Version format is valid"; \
	else \
		echo "⚠️  Version $(VERSION) doesn't match semver format vX.Y.Z"; \
		echo "   Examples: v0.2.0, v1.0.0, v2.1.3"; \
	fi

## 创建新标签（交互式）
bump-version:
	@read -p "Enter new version (e.g., 0.2.0): " new_version; \
	if echo "v$$new_version" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "Creating tag v$$new_version..."; \
		git tag -a "v$$new_version" -m "Release v$$new_version"; \
		echo "✅ Tag created: v$$new_version"; \
		echo "Run 'git push origin v$$new_version' to trigger release"; \
	else \
		echo "❌ Invalid version format. Use: X.Y.Z (e.g., 0.2.0)"; \
		exit 1; \
	fi

## 构建所有平台（本地测试）
build-all:
	@echo "Building for all platforms..."
	@mkdir -p dist
	# Linux
	GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-linux-arm64 .
	# macOS
	GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 .
	# Windows
	GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe .
	@echo "Done. Binaries in dist/"
	@ls -lh dist/

## 显示当前版本
version:
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"
	@echo "Time:    $(BUILDTIME)"

## 显示帮助
help:
	@echo "Available targets:"
	@echo "  make build          - Build for current platform"
	@echo "  make dev            - Quick dev build (no optimization)"
	@echo "  make test           - Run tests"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make version        - Show current version info"
	@echo "  make check-version  - Validate version format"
	@echo "  make bump-version   - Create new version tag (interactive)"
	@echo "  make release-snapshot - Build snapshot with GoReleaser"
	@echo "  make build-all      - Build for common platforms locally"
