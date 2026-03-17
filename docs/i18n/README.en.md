# apix - AI-Friendly HTTP Request Minimization Tool

[![Release](https://img.shields.io/github/v/release/ejfkdev/apix?include_prereleases&style=flat-square)](https://github.com/ejfkdev/apix/releases)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue?style=flat-square)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)
[![Languages](https://img.shields.io/badge/languages-39-blueviolet?style=flat-square)](../../README.md#-multi-language-support)
[![Go Report Card](https://goreportcard.com/badge/github.com/ejfkdev/apix?style=flat-square)](https://goreportcard.com/report/github.com/ejfkdev/apix)

Automatically minimize HTTP requests by removing redundant parameters and generating minimal reproducible curl commands. Effectively reduces token consumption in LLM contexts, allowing AI to focus on core logic rather than lengthy requests. Supports response structure analysis, JSON Schema extraction, and expression matching validation. Ideal for AI-assisted debugging, API documentation generation, automated testing, and issue reproduction scenarios.

[简体中文](../../README.md) | English

---

## 🚀 Features

- **🧹 Request Minimization** - Automatically identify and remove headers, query, body, cookie parameters that don't affect the response
- **📊 Response Analysis** - Generate JSON Schema structures, get condensed response data
- **🌍 Multi-language Support** - Support for 39 languages with automatic system language detection
- **⚡ High Performance** - Parallel processing support with built-in caching acceleration
- **🎯 Expression Matching** - Use expr expressions for custom matching conditions
- **🔄 Multi-client** - Support for Go HTTP client or system curl

---

## 📦 Installation

### Install from Source

```bash
go install github.com/ejfkdev/apix@latest
```

### Binary Download

Download binaries for your platform from the [Releases](https://github.com/ejfkdev/apix/releases) page.

### Verify Installation

```bash
apix version
```

---

## 🚀 Quick Start

### 1. Minimize HTTP Requests

```bash
# Basic minimization - automatically remove redundant parameters
apix pure curl https://api.example.com -H "Authorization: Bearer token" -H "X-Custom: value"

# Serial mode - test parameters one by one (suitable for parameters with dependencies)
apix pure -s curl https://api.example.com -H "X-A: 1" -H "X-B: 2"

# Output HTTP protocol format
apix pure -t curl https://api.example.com

# Output JSON format
apix pure -j curl https://api.example.com
```

### 2. Get Response Schema

```bash
# Get Schema from HTTP
apix schema curl https://api.example.com

# Output YAML format
apix schema -o yaml curl https://api.example.com

# Read JSON from stdin
echo '{"id":1,"name":"test"}' | apix schema -p
```

### 3. Get Condensed Response

```bash
# Get condensed JSON response (long strings truncated, arrays keep only first and last)
apix mini curl https://api.example.com

# Process from stdin
cat data.json | apix mini
```

---

## 🌍 Multi-language Support

apix supports **39 languages** with automatic system language detection:

```bash
# Auto-detect system language
apix --help

# Force specific language
LANG=zh apix --help      # Chinese
```

---

## 📖 Command Reference

### `apix pure` - Minimize HTTP Requests

Execute curl request and purify, output minimized request.

```bash
apix pure [options] curl [curl args...]
```

**Options:**
- `-j, --json` - Output JSON format
- `-t, --text` - Output HTTP protocol text format
- `-s, --serial` - Use serial mode (minimize one by one)
- `--parallel N` - Set parallelism (default 8)
- `--no-cache` - Disable cache
- `-c, --use-curl` - Use system curl command
- `-m, --match-mode` - Match mode (structure/status/expr/...)
- `--match-expr` - Expression match condition

**Examples:**

```bash
# Default minimization
apix pure curl https://httpbin.org/get

# Serial mode (suitable for parameters with dependencies)
apix pure -s curl https://httpbin.org/get -H "X-A: 1" -H "X-B: 2"

# Expression match: status 200 and JSON code equals 0
apix pure -m expr --match-expr "status==200 && $.code==0" curl https://api.example.com

# Match Content-Type containing json
apix pure -m expr --match-expr "headers['content-type'].contains('json')" curl https://httpbin.org/get

# POST form data minimization
apix pure curl -X POST -d "a=1&b=2&c=3" https://httpbin.org/post

# Cookie minimization (test each cookie individually)
apix pure curl https://httpbin.org/cookies -b "session=abc;tracking=xyz"
```

### `apix mini` - Get Condensed JSON Data

Get condensed JSON response data. Long strings are automatically truncated (>30 chars), arrays keep only first and last elements.

```bash
# Get from HTTP
apix mini curl https://api.example.com

# Output YAML format
apix mini -o yaml curl https://api.example.com

# Read from stdin
echo '{"key":"value"}' | apix mini
cat data.json | apix mini -p
```

### `apix schema` - Get Response Schema

Get the JSON Schema structure of the response.

```bash
# Get Schema from HTTP
apix schema curl https://api.example.com

# Output YAML format
apix schema -o yaml curl https://api.example.com

# Generate Schema from stdin
echo '{"id":1,"name":"test"}' | apix schema -p
```

### `apix clean` - Clean Cache

Clean all HTTP response cache files stored in the temporary directory.

```bash
apix clean
```

---

## 🎯 Expression Matching Syntax

Use `-m expr --match-expr "expression"` for custom matching conditions.

### Available Variables

| Variable | Type | Description |
|----------|------|-------------|
| `status` | int | HTTP status code |
| `content_length` | int | Response body length |
| `line_count` | int | Response body line count |
| `headers` | map | Response headers (case-insensitive access) |
| `body` | any | Response body JSON data |
| `$.field` | any | Shortcut for body.field |

### Operators

- Comparison: `==` `!=` `<` `>` `<=` `>=`
- Logic: `&&` `||` `!`
- Methods: `contains()`

### Examples

```bash
# Status code is 200
status == 200

# Status 200 and JSON code equals 0
status == 200 && $.code == 0

# Content-Type contains json (headers access is case-insensitive)
headers['content-type'].contains('json')

# Status 200 and line count less than 50
status == 200 && line_count < 50

# Check if field exists
$.data != nil
```

---

## ⚙️ Match Mode Details

| Mode | Logic | Use Case |
|------|-------|----------|
| `structure` (default) | Compare HTTP status code and JSON structure (ignore values) | Ensure response structure is consistent, don't care about specific values |
| `status` | Only compare HTTP status code (e.g., 200, 404) | Only care if request succeeds, not response content |
| `header` | Compare key response headers (e.g., Content-Type) | Ensure response type is the same |
| `content` | Byte-by-byte comparison of response body | Require response content to be exactly the same |
| `similarity` | Calculate similarity (default threshold 80%) | Allow minor differences in response |
| `line-count` | Compare number of lines in response | Rough matching for text responses |
| `keyword` | Check if response contains specified keyword | Verify specific content exists in response |
| `regex` | Use regular expression matching | Complex pattern matching needs |
| `expr` | Use flexible expressions | Multi-condition combined judgment |

### Mode Details

#### `structure` (Default)
Most commonly used mode, checks if response "structure" is the same:
- Status codes must match (e.g., both 200)
- JSON structure must match (e.g., both have `{code, data}` fields)
- Does not compare specific values (`{"id":1}` and `{"id":2}` are considered same structure)

#### `status`
Only checks HTTP status code:
- 200 vs 200 → Match ✓
- 200 vs 404 → No match ✗

#### `content`
Byte-by-byte exact comparison:
- Response content must be exactly the same
- Spaces, newline differences will cause mismatch

#### `keyword`
Keyword matching:
- Check if response contains specified keyword
- Use with `--match-expr`

#### `regex`
Regular expression matching:
- Use regex to match response content
- Use with `--match-expr`

---

## 🛠️ Advanced Usage

### Serial vs Parallel Mode

```bash
# Parallel mode (default) - test multiple parameter removals simultaneously
apix pure curl https://api.example.com

# Serial mode - test one by one, suitable for parameters with dependencies
apix pure -s curl https://api.example.com

# Control parallelism (reduce concurrency to avoid rate limiting)
apix pure --parallel 4 curl https://api.example.com
```

### Use System curl

```bash
# Use system curl instead of Go HTTP client
apix pure -c curl https://api.example.com

# Disable cache
apix pure -c --no-cache curl https://api.example.com
```

---

## 📄 License

This project is licensed under the [MIT License](../../LICENSE).

---

## 🙏 Acknowledgments

- [cobra](https://github.com/spf13/cobra) - CLI framework
- [go-i18n](https://github.com/nicksnyder/go-i18n) - Internationalization library
- [go-locale](https://github.com/Xuanwo/go-locale) - System language detection
- [resty](https://github.com/go-resty/resty) - HTTP client
- [expr](https://github.com/expr-lang/expr) - Expression engine

---

<p align="center">Made with ❤️ by ejfkdev</p>
