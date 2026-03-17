# apix - AI 友好的 HTTP 请求最小化工具

[![Release](https://img.shields.io/github/v/release/ejfkdev/apix?include_prereleases&style=flat-square)](https://github.com/ejfkdev/apix/releases)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue?style=flat-square)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)
[![Languages](https://img.shields.io/badge/languages-39-blueviolet?style=flat-square)](#多语言支持)
[![Go Report Card](https://goreportcard.com/badge/github.com/ejfkdev/apix?style=flat-square)](https://goreportcard.com/report/github.com/ejfkdev/apix)
[![Code Size](https://img.shields.io/github/languages/code-size/ejfkdev/apix?style=flat-square)](https://github.com/ejfkdev/apix)
[![Downloads](https://img.shields.io/github/downloads/ejfkdev/apix/total?style=flat-square)](https://github.com/ejfkdev/apix/releases)

自动精简 HTTP 请求，移除冗余参数，生成最小可复现的 curl 命令。有效减少 LLM 上下文中的 token 消耗，让 AI 更专注于核心逻辑而非冗长请求。支持响应结构分析、JSON Schema 提取、表达式匹配验证。适用于 AI 辅助调试、API 文档生成、自动化测试、问题复现等场景。

[English](docs/i18n/README.en.md) | 简体中文

---

## 🚀 功能特性

- **🧹 请求精简** - 自动识别并移除不影响响应的 headers、query、body、cookie 参数
- **📊 响应分析** - 生成 JSON Schema 结构，获取精简响应数据
- **🌍 多语言支持** - 支持 39 种语言，自动检测系统语言
- **⚡ 高性能** - 支持并行处理，内置缓存加速
- **🎯 表达式匹配** - 使用 expr 表达式自定义匹配条件
- **🔄 多客户端** - 支持 Go HTTP 客户端或系统 curl

---

## 📦 安装

### 从源码安装

```bash
go install github.com/ejfkdev/apix@latest
```

### 二进制下载

从 [Releases](https://github.com/ejfkdev/apix/releases) 页面下载对应平台的二进制文件。

### 验证安装

```bash
apix version
```

---

## 🚀 快速开始

### 1. 精简 HTTP 请求

```bash
# 基础精简 - 自动移除冗余参数
apix pure curl https://api.example.com -H "Authorization: Bearer token" -H "X-Custom: value"

# 串行模式 - 逐个测试参数（适合有依赖关系的参数）
apix pure -s curl https://api.example.com -H "X-A: 1" -H "X-B: 2"

# 输出 HTTP 协议格式
apix pure -t curl https://api.example.com

# 输出 JSON 格式
apix pure -j curl https://api.example.com
```

### 2. 获取响应 Schema

```bash
# 从 HTTP 获取 Schema
apix schema curl https://api.example.com

# 输出 YAML 格式
apix schema -o yaml curl https://api.example.com

# 从 stdin 读取 JSON
echo '{"id":1,"name":"test"}' | apix schema -p
```

### 3. 获取精简响应

```bash
# 获取精简后的 JSON 响应（长字符串截断，数组只保留首尾）
apix mini curl https://api.example.com

# 从 stdin 处理
cat data.json | apix mini
```

---

## 🌍 多语言支持

apix 支持 **39 种语言**，自动检测系统语言：

```bash
# 自动检测系统语言
apix --help

# 强制指定语言
LANG=en apix --help      # 英语
```

---

## 📖 命令详解

### `apix pure` - 精简 HTTP 请求

执行 curl 请求并净化，输出最小化后的请求。

```bash
apix pure [选项] curl [curl参数...]
```

**选项：**
- `-j, --json` - 输出 JSON 格式
- `-t, --text` - 输出 HTTP 协议文本格式
- `-s, --serial` - 使用串行模式（逐个精简）
- `--parallel N` - 设置并行度（默认 8）
- `--no-cache` - 禁用缓存
- `-c, --use-curl` - 使用系统 curl 命令
- `-m, --match-mode` - 匹配模式 (structure/status/expr/...)
- `--match-expr` - 表达式匹配条件

**示例：**

```bash
# 默认精简
apix pure curl https://httpbin.org/get

# 串行模式（适合参数有依赖的情况）
apix pure -s curl https://httpbin.org/get -H "X-A: 1" -H "X-B: 2"

# 表达式匹配：状态码 200 且 JSON 中 code 为 0
apix pure -m expr --match-expr "status==200 && $.code==0" curl https://api.example.com

# 匹配 Content-Type 包含 json
apix pure -m expr --match-expr "headers['content-type'].contains('json')" curl https://httpbin.org/get

# POST 表单数据精简
apix pure curl -X POST -d "a=1&b=2&c=3" https://httpbin.org/post

# Cookie 精简（单独测试每个 cookie）
apix pure curl https://httpbin.org/cookies -b "session=abc;tracking=xyz"
```

### `apix mini` - 获取精简 JSON 数据

获取精简后的 JSON 响应数据，长字符串自动截断（>30 字符），数组只保留首尾元素。

```bash
# 从 HTTP 获取
apix mini curl https://api.example.com

# 输出 YAML 格式
apix mini -o yaml curl https://api.example.com

# 从 stdin 读取
echo '{"key":"value"}' | apix mini
cat data.json | apix mini -p
```

### `apix schema` - 获取响应 Schema

获取响应的 JSON Schema 结构。

```bash
# 从 HTTP 获取 Schema
apix schema curl https://api.example.com

# 输出 YAML 格式
apix schema -o yaml curl https://api.example.com

# 从 stdin 生成 Schema
echo '{"id":1,"name":"test"}' | apix schema -p
```

### `apix clean` - 清理缓存

清理存储在临时目录中的所有 HTTP 响应缓存文件。

```bash
apix clean
```

---

## 🎯 表达式匹配语法

使用 `-m expr --match-expr "表达式"` 可以自定义匹配条件。

### 可用变量

| 变量 | 类型 | 说明 |
|------|------|------|
| `status` | int | HTTP 状态码 |
| `content_length` | int | 响应体长度 |
| `line_count` | int | 响应体行数 |
| `headers` | map | 响应头（访问时不区分大小写）|
| `body` | any | 响应体 JSON 数据 |
| `$.field` | any | body.field 的简写 |

### 运算符

- 比较：`==` `!=` `<` `>` `<=` `>=`
- 逻辑：`&&` `||` `!`
- 方法：`contains()`

### 示例

```bash
# 状态码为 200
status == 200

# 状态码 200 且 JSON 中 code 为 0
status == 200 && $.code == 0

# Content-Type 包含 json（headers 访问不区分大小写）
headers['content-type'].contains('json')

# 状态码 200 且行数小于 50
status == 200 && line_count < 50

# 检查字段是否存在
$.data != nil
```

---

## ⚙️ 匹配模式详解

| 模式 | 判断逻辑 | 使用场景 |
|------|----------|----------|
| `structure` (默认) | 比较 HTTP 状态码是否相同，且响应体 JSON 结构是否相同（忽略具体值） | 确保响应数据结构一致，不关心具体数值 |
| `status` | 仅比较 HTTP 状态码（如 200, 404） | 只关心请求是否成功，不关心响应内容 |
| `header` | 比较关键响应头（如 Content-Type）是否一致 | 确保响应类型相同 |
| `content` | 逐字节比较响应体内容 | 要求响应内容完全一致 |
| `similarity` | 计算响应内容的相似度（默认阈值 80%） | 允许响应有轻微差异 |
| `line-count` | 比较响应体的行数 | 用于文本类响应的粗略匹配 |
| `keyword` | 检查响应中是否包含指定关键词 | 验证响应中是否存在特定内容 |
| `regex` | 使用正则表达式匹配响应内容 | 复杂的模式匹配需求 |
| `expr` | 使用表达式灵活定义匹配条件 | 需要多条件组合判断 |

### 模式详解

#### `structure`（默认）
最常用模式，判断响应的"结构"是否相同：
- 状态码必须相同（如都是 200）
- JSON 结构必须相同（如都有 `{code, data}` 字段）
- 不比较具体值（`{"id":1}` 和 `{"id":2}` 视为相同结构）

```bash
apix pure -m status curl https://api.example.com
```

#### `status`
只判断 HTTP 状态码：
- 200 与 200 → 匹配 ✓
- 200 与 404 → 不匹配 ✗

```bash
apix pure -m status curl https://api.example.com
```

#### `content`
逐字节精确比较：
- 响应内容必须完全一致
- 空格、换行差异都会导致不匹配

```bash
apix pure -m content curl https://api.example.com
```

#### `similarity`
相似度匹配（默认 80% 阈值）：
- 计算两个响应的字符集合相似度
- 适用于文本响应的模糊匹配

```bash
# 可通过参数调整阈值（暂需通过源码修改）
apix pure -m similarity curl https://api.example.com
```

#### `keyword`
关键词匹配：
- 检查响应中是否包含指定关键词
- 需配合 `--match-expr` 使用

```bash
apix pure -m keyword --match-expr "success" curl https://api.example.com
```

#### `regex`
正则表达式匹配：
- 使用正则表达式匹配响应内容
- 需配合 `--match-expr` 使用

```bash
apix pure -m regex --match-expr "\"id\":\s*\d+" curl https://api.example.com
```

#### `expr`
最灵活的表达式匹配，详见 [表达式匹配语法](#表达式匹配语法) 章节。

---

## 🛠️ 高级用法

### 串行 vs 并行模式

```bash
# 并行模式（默认）- 同时测试多个参数移除
apix pure curl https://api.example.com

# 串行模式 - 逐个测试，适合参数有依赖关系
apix pure -s curl https://api.example.com

# 控制并行度（降低并发避免限流）
apix pure --parallel 4 curl https://api.example.com
```

### 使用系统 curl

```bash
# 使用系统 curl 替代 Go HTTP 客户端
apix pure -c curl https://api.example.com

# 禁用缓存
apix pure -c --no-cache curl https://api.example.com
```

---

## 🗂️ 项目结构

```
apix/
├── locales/          # 多语言翻译文件
│   ├── zh.json      # 中文
│   ├── en.json      # 英文
│   ├── ja.json      # 日语
│   └── ...          # 其他 36 种语言
├── apix.go          # 主程序逻辑
├── cache.go         # 缓存管理
├── httpclient.go    # HTTP 客户端
├── matcher.go       # 匹配器实现
├── i18n.go          # 国际化支持
└── go.mod           # Go 模块定义
```

---

## 🤝 贡献

欢迎提交 Issue 和 PR！

### 添加新语言翻译

1. 复制 `locales/en.json` 为 `locales/新语言代码.json`
2. 翻译所有 `"other"` 字段的值
3. 技术术语（HTTP/JSON/curl/API/URL 等）保持英文
4. 代码示例保持原样
5. 提交 PR

---

## 📄 许可证

本项目采用 [MIT 许可证](LICENSE)。

---

## 🙏 致谢

- [cobra](https://github.com/spf13/cobra) - CLI 框架
- [go-i18n](https://github.com/nicksnyder/go-i18n) - 国际化库
- [go-locale](https://github.com/Xuanwo/go-locale) - 系统语言检测
- [resty](https://github.com/go-resty/resty) - HTTP 客户端
- [expr](https://github.com/expr-lang/expr) - 表达式引擎

---

<p align="center">Made with ❤️ by ejfkdev</p>
