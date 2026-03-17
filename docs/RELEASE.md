# 自动发布指南

本文档介绍如何使用 GitHub Actions 自动构建和发布多平台二进制文件。

## 方案对比

| 方案 | 优点 | 缺点 | 推荐度 |
|------|------|------|--------|
| **GitHub Actions (手动配置)** | 灵活可控 | 配置较复杂 | ⭐⭐⭐ |
| **GoReleaser** | 简单强大、生态完善 | 需要学习成本 | ⭐⭐⭐⭐⭐ |

## 推荐方案：GoReleaser

### 1. 准备工作

确保仓库中已存在以下文件：
- `.goreleaser.yml` - GoReleaser 配置文件
- `.github/workflows/release-goreleaser.yml` - GitHub Actions 工作流

### 2. 发布步骤

#### 方法一：通过 Git 标签发布（推荐）

```bash
# 1. 确保代码已提交
git add .
git commit -m "Release v0.2.0"

# 2. 创建标签
git tag -a v0.2.0 -m "Release version 0.2.0"

# 3. 推送标签到 GitHub
git push origin v0.2.0
```

推送标签后，GitHub Actions 会自动触发构建并创建 Release。

#### 方法二：通过 GitHub Web 界面发布

1. 进入仓库的 **Actions** 标签页
2. 选择 **Release with GoReleaser** 工作流
3. 点击 **Run workflow**
4. 输入版本号（如 `v0.2.0`）
5. 点击运行

### 3. 支持的架构

GoReleaser 会自动构建以下平台：

| OS | amd64 | arm64 | 386 |
|----|-------|-------|-----|
| Linux | ✅ | ✅ | ✅ |
| macOS | ✅ | ✅ (M1/M2) | ❌ |
| Windows | ✅ | ❌ | ✅ |
| FreeBSD | ✅ | ❌ | ❌ |
| OpenBSD | ✅ | ❌ | ❌ |

### 4. 发布内容

每个 Release 包含：
- 各平台的二进制文件（.tar.gz 或 .zip）
- checksums.txt（校验和）
- 自动生成的 CHANGELOG
- 安装说明

## 备用方案：手动 GitHub Actions

如果你不想使用 GoReleaser，可以使用手动配置的 GitHub Actions：

```bash
# 使用 .github/workflows/release.yml 而不是 release-goreleaser.yml
git tag v0.2.0
git push origin v0.2.0
```

## 安装脚本

用户可以通过以下方式一键安装：

```bash
# 自动检测平台并安装最新版
curl -fsSL https://raw.githubusercontent.com/ejfkdev/apix/main/install.sh | bash

# 安装指定版本
VERSION=v0.2.0 curl -fsSL https://raw.githubusercontent.com/ejfkdev/apix/main/install.sh | bash
```

## 常见问题

### Q: 如何更新版本号？
A: 在 `main.go` 中修改 `AppVersion` 变量，或者通过 `-ldflags` 注入：
```bash
go build -ldflags="-X main.AppVersion=v0.2.0" .
```

### Q: 如何发布预发布版本？
A: 使用带后缀的标签：
```bash
git tag -a v0.2.0-beta.1 -m "Beta release"
git push origin v0.2.0-beta.1
```

### Q: 如何调试构建？
A: 本地安装 GoReleaser 进行测试：
```bash
# 安装 GoReleaser
go install github.com/goreleaser/goreleaser/v2@latest

# 本地测试构建（不发布）
goreleaser release --snapshot --clean
```

### Q: 发布失败了怎么办？
A: 
1. 检查 GitHub Actions 日志
2. 确保 `GITHUB_TOKEN` 有 `contents: write` 权限
3. 检查 `.goreleaser.yml` 配置是否正确
4. 删除失败的 tag 重新发布：
```bash
git tag -d v0.2.0
git push origin :refs/tags/v0.2.0
# 修复问题后重新创建 tag
```

## 进阶配置

### 添加到 Homebrew

在 `.goreleaser.yml` 中取消注释 `brews` 部分，并创建 `homebrew-tap` 仓库。

### Docker 镜像

在 `.goreleaser.yml` 中取消注释 `dockers` 部分，配置 Docker 镜像构建。

### 签名

可以为二进制文件添加 GPG 签名：
```yaml
# .goreleaser.yml
signs:
  - artifacts: checksum
    args: ["--batch", "-u", "<key id>", "--output", "${signature}", "--detach-sign", "${artifact}"]
```

## 参考链接

- [GoReleaser 文档](https://goreleaser.com/)
- [GitHub Actions 文档](https://docs.github.com/cn/actions)
- [GitHub Releases 文档](https://docs.github.com/cn/repositories/releasing-projects-on-github)
