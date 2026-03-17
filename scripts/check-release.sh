#!/bin/bash
# 发布前版本检查脚本
# 确保 git tag 与构建版本一致

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "🔍 Pre-release version check..."
echo ""

# 获取当前 git tag
CURRENT_TAG=$(git describe --tags --exact-match 2>/dev/null || echo "")

if [ -z "$CURRENT_TAG" ]; then
    echo -e "${RED}✗ Not on a git tag${NC}"
    echo "   Please checkout a tag or create one:"
    echo "   git tag -a v0.2.0 -m 'Release v0.2.0'"
    exit 1
fi

echo "📌 Current git tag: $CURRENT_TAG"

# 验证 tag 格式 (语义化版本)
if ! echo "$CURRENT_TAG" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo -e "${RED}✗ Invalid tag format: $CURRENT_TAG${NC}"
    echo "   Expected format: vX.Y.Z (e.g., v0.2.0, v1.0.0)"
    exit 1
fi

echo -e "${GREEN}✓ Tag format is valid${NC}"

# 检查是否有未提交的更改
if ! git diff-index --quiet HEAD --; then
    echo -e "${YELLOW}⚠ Warning: You have uncommitted changes${NC}"
    echo "   These changes will NOT be included in the release."
    read -p "   Continue anyway? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
else
    echo -e "${GREEN}✓ Working tree is clean${NC}"
fi

# 检查 tag 是否在 origin 存在
if git ls-remote --tags origin | grep -q "refs/tags/$CURRENT_TAG"; then
    echo -e "${GREEN}✓ Tag exists on remote${NC}"
else
    echo -e "${YELLOW}⚠ Tag not pushed to remote${NC}"
    echo "   Run: git push origin $CURRENT_TAG"
fi

# 计算预期构建版本
VERSION=${CURRENT_TAG#v}  # Remove 'v' prefix
echo ""
echo "📦 Build will inject:"
echo "   AppVersion: $CURRENT_TAG"
echo "   GitCommit:  $(git rev-parse --short HEAD)"
echo "   BuildTime:  $(date -u +%Y%m%d_%H%M%S)"
echo ""

# 验证 GoReleaser 配置
echo "🔧 Checking GoReleaser config..."
if ! command -v goreleaser &> /dev/null; then
    echo -e "${YELLOW}⚠ GoReleaser not installed${NC}"
    echo "   Install: go install github.com/goreleaser/goreleaser/v2@latest"
else
    if goreleaser check &> /dev/null; then
        echo -e "${GREEN}✓ GoReleaser config is valid${NC}"
    else
        echo -e "${RED}✗ GoReleaser config has errors${NC}"
        goreleaser check
        exit 1
    fi
fi

echo ""
echo -e "${GREEN}✅ All checks passed!${NC}"
echo ""
echo "🚀 Ready to release: $CURRENT_TAG"
echo ""
echo "Next steps:"
echo "   1. Ensure tag is pushed: git push origin $CURRENT_TAG"
echo "   2. GitHub Actions will auto-trigger the release"
echo "   3. Or run locally: make release-snapshot"
