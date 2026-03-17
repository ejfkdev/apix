package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CacheEntry 缓存条目 - 存储完整的 HTTP 请求和响应
type CacheEntry struct {
	// 请求信息
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Cookies     map[string]string `json:"cookies"`
	Body        string            `json:"body"`
	
	// 响应信息
	Status      int               `json:"status"`
	RespHeaders map[string]string `json:"resp_headers"`
	RespBody    []byte            `json:"resp_body"`
	
	// 元数据
	Timestamp   int64             `json:"timestamp"`
	RequestSig  string            `json:"request_sig"`
}

// CacheManager 缓存管理器
type CacheManager struct {
	baseDir string
}

// NewCacheManager 创建新的缓存管理器
func NewCacheManager() *CacheManager {
	baseDir := filepath.Join(os.TempDir(), "apix_cache")
	os.MkdirAll(baseDir, 0755)
	return &CacheManager{baseDir: baseDir}
}

// GenerateCacheKey 生成缓存 key（基于完整请求信息）
// 这个 key 用于唯一标识一个 HTTP 请求，包含所有参数名和值
func GenerateCacheKey(method, reqURL string, headers, cookies map[string]string, body string) string {
	u, err := url.Parse(reqURL)
	if err != nil {
		u = &url.URL{Path: reqURL}
	}
	
	var parts []string
	parts = append(parts, strings.ToUpper(method))
	parts = append(parts, u.Scheme+"://"+u.Host+u.Path)
	
	// 包含查询参数（key 和 value）
	if u.RawQuery != "" {
		queryParts := strings.Split(u.RawQuery, "&")
		sort.Strings(queryParts)
		parts = append(parts, "query:"+strings.Join(queryParts, "&"))
	}
	
	// 包含 headers（key 和 value）
	if len(headers) > 0 {
		var headerParts []string
		var keys []string
		for k := range headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			headerParts = append(headerParts, k+"="+headers[k])
		}
		parts = append(parts, "headers:"+strings.Join(headerParts, "&"))
	}
	
	// 包含 cookies（key 和 value）
	if len(cookies) > 0 {
		var cookieParts []string
		var keys []string
		for k := range cookies {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			cookieParts = append(cookieParts, k+"="+cookies[k])
		}
		parts = append(parts, "cookies:"+strings.Join(cookieParts, "&"))
	}
	
	// 包含 body
	if body != "" {
		parts = append(parts, "body:"+body)
	}
	
	content := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])[:16] // 16 chars for uniqueness
}

// GenerateCachePath 生成缓存文件路径
func (c *CacheManager) GenerateCachePath(cacheKey string) string {
	return filepath.Join(c.baseDir, cacheKey+".json")
}

// Get 获取缓存条目
func (c *CacheManager) Get(cacheKey string) (*CacheEntry, error) {
	path := c.GenerateCachePath(cacheKey)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	
	return &entry, nil
}

// Set 设置缓存条目
func (c *CacheManager) Set(cacheKey string, entry *CacheEntry) error {
	path := c.GenerateCachePath(cacheKey)
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0644)
}

// Exists 检查缓存是否存在
func (c *CacheManager) Exists(cacheKey string) bool {
	path := c.GenerateCachePath(cacheKey)
	_, err := os.Stat(path)
	return err == nil
}

// Clear 清除所有缓存
func (c *CacheManager) Clear() error {
	return os.RemoveAll(c.baseDir)
}

// extractCacheKeyFromArgs 从 curl 参数中提取缓存 key
func extractCacheKeyFromArgs(args []string) string {
	method, url, headers, cookies, body := extractRequestInfo(args)
	if url == "" {
		return ""
	}
	return GenerateCacheKey(method, url, headers, cookies, body)
}

// argsToCacheEntry 将 curl 参数和响应转换为缓存条目
func argsToCacheEntry(args []string, resp *HTTPResponse) (*CacheEntry, string) {
	method, url, headers, cookies, body := extractRequestInfo(args)
	if url == "" {
		return nil, ""
	}
	
	cacheKey := GenerateCacheKey(method, url, headers, cookies, body)
	
	entry := &CacheEntry{
		Method:      method,
		URL:         url,
		Headers:     headers,
		Cookies:     cookies,
		Body:        body,
		Status:      resp.Status,
		RespHeaders: resp.Headers,
		RespBody:    resp.Body,
		Timestamp:   time.Now().Unix(),
		RequestSig:  cacheKey,
	}
	
	return entry, cacheKey
}

// extractRequestInfo 提取请求信息（公共逻辑提取）
func extractRequestInfo(args []string) (method, url string, headers, cookies map[string]string, body string) {
	config, err := parseCurlArgs(args)
	if err != nil {
		return "", "", nil, nil, ""
	}
	
	// 转换 headers 格式，并提取 Cookie header
	headers = make(map[string]string)
	cookies = make(map[string]string)
	
	// 复制 -b 参数中的 cookies
	for k, v := range config.cookies {
		cookies[k] = v
	}
	
	for k, values := range config.headers {
		if len(values) > 0 {
			headers[k] = values[0]
			// 如果是 Cookie header，解析其中的 cookies
			if strings.EqualFold(k, "Cookie") {
				for cookieKey, cookieVal := range parseCookieHeader(values[0]) {
					cookies[cookieKey] = cookieVal
				}
			}
		}
	}
	
	method = config.method
	if method == "" {
		method = "GET"
	}
	
	return method, config.url, headers, cookies, config.body
}


