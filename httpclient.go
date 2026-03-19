package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"resty.dev/v3"
)

// HTTPClient HTTP 客户端接口
type HTTPClient interface {
	Do(args []string) (*HTTPResponse, error)
	SetCacheEnabled(enabled bool)
}

// HTTPResponse HTTP 响应
type HTTPResponse struct {
	Status  int
	Body    []byte
	Headers map[string]string
}

// CachedRestyClient 带缓存的 Resty 客户端
type CachedRestyClient struct {
	client      *resty.Client
	cache       *CacheManager
	cacheEnabled bool
}

// NewCachedRestyClient 创建新的带缓存的 resty 客户端
func NewCachedRestyClient() *CachedRestyClient {
	client := resty.New()
	client.SetTimeout(30 * time.Second)

	return &CachedRestyClient{
		client:       client,
		cache:        NewCacheManager(),
		cacheEnabled: false,
	}
}

// SetCacheEnabled 设置是否启用缓存
func (c *CachedRestyClient) SetCacheEnabled(enabled bool) {
	c.cacheEnabled = enabled
}

// Do 执行 HTTP 请求（带缓存支持）
func (c *CachedRestyClient) Do(args []string) (*HTTPResponse, error) {
	// 如果启用缓存，先检查缓存
	if c.cacheEnabled {
		cacheKey := extractCacheKeyFromArgs(args)
		if cacheKey != "" && c.cache.Exists(cacheKey) {
			entry, err := c.cache.Get(cacheKey)
			if err == nil {
				// 缓存命中，返回缓存的响应
				return &HTTPResponse{
					Status:  entry.Status,
					Body:    entry.RespBody,
					Headers: entry.RespHeaders,
				}, nil
			}
		}
	}
	
	// 执行实际请求
	resp, err := c.doRequest(args)
	if err != nil {
		return nil, err
	}
	
	// 如果启用缓存，保存响应
	if c.cacheEnabled {
		entry, cacheKey := argsToCacheEntry(args, resp)
		if entry != nil {
			c.cache.Set(cacheKey, entry)
		}
	}
	
	return resp, nil
}

// doRequest 执行实际的 HTTP 请求
func (c *CachedRestyClient) doRequest(args []string) (*HTTPResponse, error) {
	// 解析 curl 参数
	reqConfig, err := parseCurlArgs(args)
	if err != nil {
		return nil, err
	}

	// 根据配置创建新的客户端（支持每个请求不同的 TLS 和重定向配置）
	client := resty.New()
	client.SetTimeout(30 * time.Second)

	// 设置方法
	method := strings.ToUpper(reqConfig.method)
	if method == "" {
		method = "GET"
	}

	// 设置 insecure（跳过证书验证）
	if reqConfig.insecure {
		client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	// 设置重定向策略
	if reqConfig.followRedirect {
		if reqConfig.maxRedirects > 0 {
			client.SetRedirectPolicy(resty.FlexibleRedirectPolicy(reqConfig.maxRedirects))
		}
		// 默认 resty 会跟随重定向，不需要额外设置
	} else {
		// 不跟随重定向
		client.SetRedirectPolicy(resty.NoRedirectPolicy())
	}

	// 设置自定义域名解析 (--resolve)
	if len(reqConfig.resolve) > 0 {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: reqConfig.insecure},
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// 检查是否有自定义解析
				if ip, ok := reqConfig.resolve[addr]; ok {
					// 使用自定义 IP，但保持端口不变
					_, port, _ := net.SplitHostPort(addr)
					if port == "" {
						port = "443" // 默认 HTTPS 端口
					}
					newAddr := net.JoinHostPort(ip, port)
					return net.Dial(network, newAddr)
				}
				// 默认解析
				return net.Dial(network, addr)
			},
		}
		client.SetTransport(transport)
	}

	// 创建请求
	req := client.R()

	// 设置 headers
	for key, values := range reqConfig.headers {
		for _, value := range values {
			req.SetHeader(key, value)
		}
	}

	// 设置 cookies
	for key, value := range reqConfig.cookies {
		req.SetCookie(&http.Cookie{
			Name:  key,
			Value: value,
		})
	}

	// 设置查询参数
	for key, values := range reqConfig.queryParams {
		for _, value := range values {
			req.SetQueryParam(key, value)
		}
	}

	// 设置 body
	if reqConfig.body != "" {
		req.SetBody(reqConfig.body)
	}

	// 设置代理
	if reqConfig.proxy != "" {
		client.SetProxy(reqConfig.proxy)
	}

	// 设置超时
	if reqConfig.timeout > 0 {
		client.SetTimeout(time.Duration(reqConfig.timeout) * time.Second)
	}

	// 执行请求
	var resp *resty.Response
	switch method {
	case "GET":
		resp, err = req.Get(reqConfig.url)
	case "POST":
		resp, err = req.Post(reqConfig.url)
	case "PUT":
		resp, err = req.Put(reqConfig.url)
	case "PATCH":
		resp, err = req.Patch(reqConfig.url)
	case "DELETE":
		resp, err = req.Delete(reqConfig.url)
	case "HEAD":
		resp, err = req.Head(reqConfig.url)
	case "OPTIONS":
		resp, err = req.Options(reqConfig.url)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}

	if err != nil {
		return nil, err
	}

	// 构建响应
	headers := make(map[string]string)
	for key, values := range resp.Header() {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	return &HTTPResponse{
		Status:  resp.StatusCode(),
		Body:    resp.Bytes(),
		Headers: headers,
	}, nil
}

// CachedCurlExecutor 带缓存的系统 curl 执行器
type CachedCurlExecutor struct {
	cache        *CacheManager
	cacheEnabled bool
}

// NewCachedCurlExecutor 创建新的带缓存的 curl 执行器
func NewCachedCurlExecutor() *CachedCurlExecutor {
	return &CachedCurlExecutor{
		cache:        NewCacheManager(),
		cacheEnabled: false,
	}
}

// SetCacheEnabled 设置是否启用缓存
func (e *CachedCurlExecutor) SetCacheEnabled(enabled bool) {
	e.cacheEnabled = enabled
}

// Do 执行 curl 命令（带缓存支持）
func (e *CachedCurlExecutor) Do(args []string) (*HTTPResponse, error) {
	// 如果启用缓存，先检查缓存
	if e.cacheEnabled {
		cacheKey := extractCacheKeyFromArgs(args)
		if cacheKey != "" && e.cache.Exists(cacheKey) {
			entry, err := e.cache.Get(cacheKey)
			if err == nil {
				// 缓存命中，返回缓存的响应
				return &HTTPResponse{
					Status:  entry.Status,
					Body:    entry.RespBody,
					Headers: entry.RespHeaders,
				}, nil
			}
		}
	}
	
	// 执行实际请求
	res, err := Run(args, RunOptions{ForceSilent: true})
	if err != nil {
		return nil, err
	}

	resp := &HTTPResponse{
		Status: res.Status,
		Body:   res.Body,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
	
	// 如果启用缓存，保存响应
	if e.cacheEnabled {
		entry, cacheKey := argsToCacheEntry(args, resp)
		if entry != nil {
			e.cache.Set(cacheKey, entry)
		}
	}
	
	return resp, nil
}

// CurlRequestConfig curl 请求配置
type CurlRequestConfig struct {
	url            string
	method         string
	headers        map[string][]string
	cookies        map[string]string
	queryParams    map[string][]string
	body           string
	proxy          string
	timeout        int
	insecure       bool
	silent         bool
	userAgent      string
	followRedirect bool
	maxRedirects   int
	compressed     bool
	resolve        map[string]string // 自定义域名解析: host:port -> ip
}

// parseCurlArgs 解析 curl 参数（改进版）
func parseCurlArgs(args []string) (*CurlRequestConfig, error) {
	config := &CurlRequestConfig{
		headers:     make(map[string][]string),
		cookies:     make(map[string]string),
		queryParams: make(map[string][]string),
		resolve:     make(map[string]string),
	}

	// 解析参数
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// 处理长短选项
		switch {
		// URL
		case !strings.HasPrefix(arg, "-") && (strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://")):
			config.url = arg

		// 方法 -X, --request
		case arg == "-X" || arg == "--request":
			if i+1 < len(args) {
				config.method = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "-X"):
			config.method = arg[2:]
		case strings.HasPrefix(arg, "--request="):
			config.method = arg[10:]

		// Header -H, --header
		case arg == "-H" || arg == "--header":
			if i+1 < len(args) {
				if err := parseHeader(args[i+1], config.headers); err != nil {
					return nil, err
				}
				i++
			}
		case strings.HasPrefix(arg, "-H"):
			if err := parseHeader(arg[2:], config.headers); err != nil {
				return nil, err
			}
		case strings.HasPrefix(arg, "--header="):
			if err := parseHeader(arg[9:], config.headers); err != nil {
				return nil, err
			}

		// Cookie -b, --cookie
		case arg == "-b" || arg == "--cookie":
			if i+1 < len(args) {
				if err := parseCookie(args[i+1], config.cookies); err != nil {
					return nil, err
				}
				i++
			}
		case strings.HasPrefix(arg, "-b"):
			if err := parseCookie(arg[2:], config.cookies); err != nil {
				return nil, err
			}
		case strings.HasPrefix(arg, "--cookie="):
			if err := parseCookie(arg[9:], config.cookies); err != nil {
				return nil, err
			}

		// Data -d, --data
		case arg == "-d" || arg == "--data" || arg == "--data-raw":
			if i+1 < len(args) {
				config.body = args[i+1]
				if config.method == "" {
					config.method = "POST"
				}
				i++
			}
		case strings.HasPrefix(arg, "-d"):
			config.body = arg[2:]
			if config.method == "" {
				config.method = "POST"
			}
		case strings.HasPrefix(arg, "--data="):
			config.body = arg[7:]
			if config.method == "" {
				config.method = "POST"
			}

		// 代理 -x, --proxy
		case arg == "-x" || arg == "--proxy":
			if i+1 < len(args) {
				config.proxy = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "-x"):
			config.proxy = arg[2:]
		case strings.HasPrefix(arg, "--proxy="):
			config.proxy = arg[8:]

		// 超时 --connect-timeout
		case arg == "--connect-timeout":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &config.timeout)
				i++
			}
		case strings.HasPrefix(arg, "--connect-timeout="):
			fmt.Sscanf(arg[18:], "%d", &config.timeout)

		// 不安全的 -k, --insecure
		case arg == "-k" || arg == "--insecure":
			config.insecure = true

		// User Agent -A, --user-agent
		case arg == "-A" || arg == "--user-agent":
			if i+1 < len(args) {
				config.userAgent = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "-A"):
			config.userAgent = arg[2:]
		case strings.HasPrefix(arg, "--user-agent="):
			config.userAgent = arg[13:]

		// 跟随重定向 -L, --location
		case arg == "-L" || arg == "--location":
			config.followRedirect = true

		// 最大重定向次数 --max-redirs
		case arg == "--max-redirs":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &config.maxRedirects)
				i++
			}
		case strings.HasPrefix(arg, "--max-redirs="):
			fmt.Sscanf(arg[13:], "%d", &config.maxRedirects)

		// 压缩 --compressed
		case arg == "--compressed":
			config.compressed = true

		// 自定义域名解析 --resolve
		case arg == "--resolve":
			if i+1 < len(args) {
				parseResolve(args[i+1], config.resolve)
				i++
			}
		case strings.HasPrefix(arg, "--resolve="):
			parseResolve(arg[10:], config.resolve)

		// 静默模式 -s, --silent
		case arg == "-s" || arg == "--silent":
			config.silent = true

		// URL 参数
		case arg == "--url":
			if i+1 < len(args) {
				config.url = args[i+1]
				i++
			}
		}
	}

	// 设置默认 User-Agent
	if config.userAgent != "" {
		config.headers["User-Agent"] = []string{config.userAgent}
	}

	return config, nil
}

// parseHeader 解析 header 字符串
func parseHeader(header string, headers map[string][]string) error {
	parts := strings.SplitN(header, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid header format: %s", header)
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	headers[key] = append(headers[key], value)
	return nil
}

// parseCookie 解析 cookie 字符串
func parseCookie(cookie string, cookies map[string]string) error {
	parts := strings.SplitN(cookie, "=", 2)
	if len(parts) == 2 {
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// 清理 cookie 值中的非法字符（根据 RFC 6265）
		value = sanitizeCookieValue(value)
		cookies[key] = value
	} else {
		cookies[cookie] = ""
	}
	return nil
}


// sanitizeCookieValue 清理 cookie 值中的非法字符 (RFC 6265)
func sanitizeCookieValue(value string) string {
	var result strings.Builder
	for _, r := range value {
		switch r {
		case ';', ',', ' ', '\t', '\n', '\r':
			continue
		default:
			if r >= 0x20 && r <= 0x7E || r > 0x7F {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}

// parseResolve 解析 --resolve 参数
func parseResolve(resolve string, resolveMap map[string]string) {
	// 格式: host:port:addr 或 host:addr
	parts := strings.Split(resolve, ":")
	if len(parts) >= 2 {
		host := parts[0]
		var port, addr string
		if len(parts) == 2 {
			// host:addr (默认端口 443)
			port = "443"
			addr = parts[1]
		} else {
			// host:port:addr
			port = parts[1]
			addr = parts[2]
		}
		resolveMap[host+":"+port] = addr
	}
}
