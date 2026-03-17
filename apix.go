package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/hjson/hjson-go"
	"github.com/spf13/cobra"
)

// Response 表示 HTTP 响应
type Response struct {
	Status  int            `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte         `json:"-"`
	BodyMap map[string]any `json:"body,omitempty"`
}

// Matcher 接口
type Matcher interface {
	Match(baseline, current Response) bool
}

// StructureMatcher 结构匹配器
type StructureMatcher struct{}

func (m *StructureMatcher) Match(baseline, current Response) bool {
	if baseline.Status != current.Status {
		return false
	}
	return schemaDigestFromBody(baseline.Body) == schemaDigestFromBody(current.Body)
}

func schemaDigestFromBody(body []byte) string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	return schemaDigest(v)
}

func buildResponse(result RunResult) Response {
	r := Response{
		Status: result.Status,
		Body:   result.Body,
	}
	var bodyMap map[string]any
	if err := json.Unmarshal(result.Body, &bodyMap); err == nil {
		r.BodyMap = bodyMap
	}
	return r
}

// createMatcher 创建匹配器（兼容旧接口）
func createMatcher(matchMode, matchExpr string, matchArgs ...string) Matcher {
	config := MatcherConfig{
		Mode: matchMode,
		Expr: matchExpr,
		Args: matchArgs,
	}
	
	inner, err := CreateMatcher(config)
	if err != nil {
		// 如果创建失败，使用默认的结构匹配器
		inner = &StructureMatcherV2{}
	}
	
	return NewLegacyMatcher(inner)
}

// 全局变量
var (
	httpClient   HTTPClient
	restyClient  *CachedRestyClient
	curlExecutor *CachedCurlExecutor
	clientOnce   sync.Once
)

// getHTTPClient 获取 HTTP 客户端（线程安全）
func getHTTPClient(useCurl bool) HTTPClient {
	clientOnce.Do(func() {
		restyClient = NewCachedRestyClient()
		curlExecutor = NewCachedCurlExecutor()
	})
	
	if useCurl {
		return curlExecutor
	}
	return restyClient
}

// rootCmd
var rootCmd = &cobra.Command{
	Use:   "apix",
	Short: T("root.short"),
	Long:  T("root.long"),
	Run: func(cmd *cobra.Command, args []string) {
		runStdinMode()
	},
}

// pureCmd - 精简 curl 请求，输出最小化 curl 命令
var pureCmd = &cobra.Command{
	Use:     "pure",
	Short:   T("pure.short"),
	Long:    T("pure.long"),
	Example: T("pure.example"),
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 检查 help 请求
		for _, arg := range args {
			if arg == "--help" || arg == "-h" {
				fmt.Println(cmd.Long)
				fmt.Println("\n" + T("msg.usage"))
				fmt.Println("  " + T("usage.pure"))
				fmt.Println("\n" + T("msg.examples"))
				fmt.Println(cmd.Example)
				fmt.Println("\n" + T("msg.params"))
				fmt.Println("  -j, --json              " + T("flag.json"))
				fmt.Println("  -t, --text              " + T("flag.text"))
				fmt.Println("  -s, --serial            " + T("flag.serial"))
				fmt.Println("      --resolve string    " + T("flag.resolve"))
				fmt.Println("      --parallel int      " + T("flag.parallel"))
				fmt.Println("      --no-cache          " + T("flag.no-cache"))
				fmt.Println("  -m, --match-mode string " + T("flag.match-mode"))
				fmt.Println("      --match-expr string " + T("flag.match-expr"))
				return nil
			}
		}

		// 查找 "curl" 标记的位置
		curlIdx := -1
		for i, arg := range args {
			if arg == "curl" {
				curlIdx = i
				break
			}
		}

		// 提取 curl 参数
		var curlArgs []string
		if curlIdx >= 0 && curlIdx < len(args)-1 {
			curlArgs = args[curlIdx+1:]
		} else {
			return fmt.Errorf("用法: apix pure [全局参数] curl [curl参数...]\n示例: apix pure curl https://httpbin.org/get")
		}

		// 解析子命令参数（curl 标记之前的参数）
		outFormat := "curl" // 默认输出格式
		serialFlag := false // 串行模式标志
		for i := 0; i < curlIdx; i++ {
			arg := args[i]
			switch {
			case arg == "-j" || arg == "--json":
				outFormat = "json"
			case arg == "-t" || arg == "--text":
				outFormat = "http"
			case arg == "-s" || arg == "--serial":
				serialFlag = true
			}
		}

		// 解析全局参数
		globalFlags := parseGlobalFlags(args[:curlIdx])
		matchModeFlag := globalFlags.matchMode
		matchExprFlag := globalFlags.matchExpr
		noCache := globalFlags.noCache
		useCurlFlag := globalFlags.useCurl
		parallelFlag := globalFlags.parallel

		// 如果 parallel 设置为 1，强制使用串行模式
		if parallelFlag == 1 {
			serialFlag = true
		}

		return runPureMode(curlArgs, parallelFlag, matchModeFlag, matchExprFlag, outFormat, !noCache, serialFlag, useCurlFlag)
	},
}

// miniCmd - 获取精简 JSON 数据
var miniCmd = &cobra.Command{
	Use:     "mini",
	Short:   T("mini.short"),
	Long:    T("mini.long"),
	Example: T("mini.example"),
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 检查 help 请求
		for _, arg := range args {
			if arg == "--help" || arg == "-h" {
				fmt.Println(cmd.Long)
				fmt.Println("\n" + T("msg.usage"))
				fmt.Println("  " + T("usage.mini"))
				fmt.Println("\n" + T("msg.examples"))
				fmt.Println(cmd.Example)
				fmt.Println("\n" + T("msg.params"))
				fmt.Println("  -o, --out string    " + T("flag.out"))
				fmt.Println("  -p, --pretty        " + T("flag.pretty"))
				fmt.Println("      --resolve       " + T("flag.resolve"))
				fmt.Println("      --no-cache      " + T("flag.no-cache"))
				fmt.Println("  -c, --use-curl      " + T("flag.use-curl"))
				return nil
			}
		}

		// 查找 "curl" 标记
		curlIdx := -1
		for i, arg := range args {
			if arg == "curl" {
				curlIdx = i
				break
			}
		}

		// 解析全局参数
		var checkArgs []string
		if curlIdx >= 0 {
			checkArgs = args[:curlIdx]
		} else {
			checkArgs = args
		}
		globalFlags := parseGlobalFlags(checkArgs)
		noCache := globalFlags.noCache
		useCurlFlag := globalFlags.useCurl
		outFormatFlag := globalFlags.outFormat
		prettyFlag := globalFlags.pretty
		indentFlag := globalFlags.indent

		normalizedOut, err := normalizeOutFormat(outFormatFlag)
		if err != nil {
			return err
		}

		// curl 模式：从 HTTP 获取
		if curlIdx >= 0 && curlIdx < len(args)-1 {
			curlArgs := args[curlIdx+1:]
			return runMiniModeFromHTTP(curlArgs, !noCache, normalizedOut, prettyFlag, indentFlag, useCurlFlag)
		}

		// stdin 模式
		return runSampleMode(prettyFlag, indentFlag, normalizedOut)
	},
}

// schemaCmd - 获取响应 Schema
var schemaCmd = &cobra.Command{
	Use:     "schema",
	Short:   T("schema.short"),
	Long:    T("schema.long"),
	Example: T("schema.example"),
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 检查 help 请求
		for _, arg := range args {
			if arg == "--help" || arg == "-h" {
				fmt.Println(cmd.Long)
				fmt.Println("\n" + T("msg.usage"))
				fmt.Println("  " + T("usage.schema"))
				fmt.Println("\n" + T("msg.examples"))
				fmt.Println(cmd.Example)
				fmt.Println("\n" + T("msg.params"))
				fmt.Println("  -o, --out string    " + T("flag.out"))
				fmt.Println("  -p, --pretty        " + T("flag.pretty"))
				fmt.Println("      --resolve       " + T("flag.resolve"))
				fmt.Println("      --no-cache      " + T("flag.no-cache"))
				fmt.Println("  -c, --use-curl      " + T("flag.use-curl"))
				return nil
			}
		}

		// 查找 "curl" 标记
		curlIdx := -1
		for i, arg := range args {
			if arg == "curl" {
				curlIdx = i
				break
			}
		}

		// 解析全局参数
		var checkArgs []string
		if curlIdx >= 0 {
			checkArgs = args[:curlIdx]
		} else {
			checkArgs = args
		}
		globalFlags := parseGlobalFlags(checkArgs)
		noCache := globalFlags.noCache
		useCurlFlag := globalFlags.useCurl
		outFormatFlag := globalFlags.outFormat
		prettyFlag := globalFlags.pretty
		indentFlag := globalFlags.indent

		// curl 模式：从 HTTP 获取
		if curlIdx >= 0 && curlIdx < len(args)-1 {
			curlArgs := args[curlIdx+1:]
			return runSchemaModeFromHTTP(curlArgs, !noCache, outFormatFlag, prettyFlag, indentFlag, useCurlFlag)
		}

		// stdin 模式
		return runSchemaModeFromStdin(outFormatFlag, prettyFlag, indentFlag)
	},
}

// versionCmd
var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   T("version.short"),
	Long:    T("version.long"),
	Example: T("version.example"),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("apix version %s", AppVersion)
		if GitCommit != "unknown" {
			fmt.Printf(" (git: %s)", GitCommit[:7])
		}
		fmt.Println()
		if BuildTime != "unknown" {
			fmt.Printf("Build time: %s\n", BuildTime)
		}
	},
}

// cleanCmd - 清理缓存
var cleanCmd = &cobra.Command{
	Use:     "clean",
	Short:   T("clean.short"),
	Long:    T("clean.long"),
	Example: T("clean.example"),
	RunE: func(cmd *cobra.Command, args []string) error {
		cache := NewCacheManager()
		if err := cache.Clear(); err != nil {
			return fmt.Errorf("清理缓存失败: %w", err)
		}
		fmt.Println(T("msg.cache_cleaned"))
		return nil
	},
}

// globalFlags 全局参数结构
type globalFlags struct {
	outFormat  string
	indent     int
	pretty     bool
	matchMode  string
	matchExpr  string
	parallel   int
	useCurl    bool
	noCache    bool
	matchArgs  []string // 匹配模式的额外参数
}

// parseGlobalFlags 手动解析全局参数
func parseGlobalFlags(args []string) globalFlags {
	flags := globalFlags{
		outFormat: "json",
		indent:    0,
		pretty:    false,
		matchMode: "structure",
		matchExpr: "",
		parallel:  8,
		useCurl:   false,
		noCache:   false,
		matchArgs: []string{},
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-o" || arg == "--out":
			if i+1 < len(args) {
				flags.outFormat = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--out="):
			flags.outFormat = strings.TrimPrefix(arg, "--out=")
		case arg == "-i" || arg == "--indent":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &flags.indent)
				i++
			}
		case strings.HasPrefix(arg, "--indent="):
			fmt.Sscanf(strings.TrimPrefix(arg, "--indent="), "%d", &flags.indent)
		case arg == "-p" || arg == "--pretty":
			flags.pretty = true
		case arg == "-m" || arg == "--match-mode":
			if i+1 < len(args) {
				flags.matchMode = args[i+1]
				// 检查是否有额外参数（如 -m keyword success）
				for j := i + 2; j < len(args); j++ {
					nextArg := args[j]
					if strings.HasPrefix(nextArg, "-") {
						break
					}
					flags.matchArgs = append(flags.matchArgs, nextArg)
					i++
				}
				i++
			}
		case strings.HasPrefix(arg, "--match-mode="):
			flags.matchMode = strings.TrimPrefix(arg, "--match-mode=")
		case arg == "--match-expr":
			if i+1 < len(args) {
				flags.matchExpr = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--match-expr="):
			flags.matchExpr = strings.TrimPrefix(arg, "--match-expr=")
		case arg == "--parallel":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &flags.parallel)
				i++
			}
		case strings.HasPrefix(arg, "--parallel="):
			fmt.Sscanf(strings.TrimPrefix(arg, "--parallel="), "%d", &flags.parallel)
		case arg == "-c" || arg == "--use-curl":
			flags.useCurl = true
		case arg == "--no-cache":
			flags.noCache = true
		}
	}

	return flags
}

func init() {
	// 初始化 i18n
	initI18n()

	// 更新命令的翻译（因为命令定义在包初始化时，i18n 尚未加载）
	rootCmd.Short = T("root.short")
	rootCmd.Long = T("root.long")
	pureCmd.Short = T("pure.short")
	pureCmd.Long = T("pure.long")
	pureCmd.Example = T("pure.example")
	miniCmd.Short = T("mini.short")
	miniCmd.Long = T("mini.long")
	miniCmd.Example = T("mini.example")
	schemaCmd.Short = T("schema.short")
	schemaCmd.Long = T("schema.long")
	schemaCmd.Example = T("schema.example")
	versionCmd.Short = T("version.short")
	versionCmd.Long = T("version.long")
	versionCmd.Example = T("version.example")
	cleanCmd.Short = T("clean.short")
	cleanCmd.Long = T("clean.long")
	cleanCmd.Example = T("clean.example")

	// 全局 Persistent Flags - 可以在子命令前使用
	rootCmd.PersistentFlags().StringP("out", "o", "json", T("flag.out"))
	rootCmd.PersistentFlags().IntP("indent", "i", 0, T("flag.indent"))
	rootCmd.PersistentFlags().BoolP("pretty", "p", false, T("flag.pretty"))
	rootCmd.PersistentFlags().StringP("match-mode", "m", "structure", T("flag.match-mode"))
	rootCmd.PersistentFlags().String("match-expr", "", T("flag.match-expr"))
	rootCmd.PersistentFlags().Int("parallel", 8, T("flag.parallel"))
	rootCmd.PersistentFlags().BoolP("use-curl", "c", false, T("flag.use-curl"))
	rootCmd.PersistentFlags().Bool("no-cache", false, T("flag.no-cache"))

	rootCmd.AddCommand(pureCmd)
	rootCmd.AddCommand(miniCmd)
	rootCmd.AddCommand(schemaCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(versionCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runStdinMode 原来的 stdin 模式
func runStdinMode() {
	input, hasInput, err := readStdinMaybe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "apix:", err)
		os.Exit(1)
	}
	if !hasInput {
		printUsage()
		return
	}

	indentStr := indentString(0, false)

	schemaOutBytes, err := Infer(bytes.NewReader(input), Options{
		Format:          "openapi",
		SchemaName:      "Response",
		IncludeExamples: false,
		Indent:          "",
		KeepConst:       false,
		AddDescriptions: true,
	})
	if err != nil {
		trimmed := stripToJSONStart(input)
		if !bytes.Equal(trimmed, input) {
			schemaOutBytes, err = Infer(bytes.NewReader(trimmed), Options{
				Format:          "openapi",
				SchemaName:      "Response",
				IncludeExamples: false,
				Indent:          "",
				KeepConst:       false,
				AddDescriptions: true,
			})
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "apix:", err)
			os.Exit(1)
		}
	}

	var schema any
	if err := json.Unmarshal(schemaOutBytes, &schema); err != nil {
		fmt.Fprintln(os.Stderr, "apix:", err)
		os.Exit(1)
	}

	out, err := encodeOutput(schema, "json", indentStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "apix:", err)
		os.Exit(1)
	}

	os.Stdout.Write(out)
	fmt.Fprintln(os.Stdout)
}

// runSchemaModeFromStdin 从 stdin 读取 JSON 生成 schema
func runSchemaModeFromStdin(outFormat string, pretty bool, indent int) error {
	input, hasInput, err := readStdinMaybe()
	if err != nil {
		return err
	}
	if !hasInput {
		return fmt.Errorf("需要从 stdin 提供 JSON 数据，或使用 'apix schema curl <url>' 从 HTTP 请求获取")
	}

	indentStr := indentString(indent, pretty)

	schemaOutBytes, err := Infer(bytes.NewReader(input), Options{
		Format:          FormatOpenAPI,
		IncludeExamples: false,
		Indent:          "",
		KeepConst:       false,
		AddDescriptions: true,
	})
	if err != nil {
		return err
	}

	var schema any
	if err := json.Unmarshal(schemaOutBytes, &schema); err != nil {
		return err
	}

	enc, err := encodeOutput(schema, outFormat, indentStr)
	if err != nil {
		return err
	}

	os.Stdout.Write(enc)
	if indentStr != "" {
		fmt.Fprintln(os.Stdout)
	}

	return nil
}

// runSchemaModeFromHTTP 从 HTTP 请求获取 schema
// 注意：缓存现在在 HTTP client 层处理
func runSchemaModeFromHTTP(args []string, useCache bool, outFormat string, pretty bool, indent int, useCurl bool) error {
	// 启用/禁用 HTTP client 缓存
	httpClient = getHTTPClient(useCurl)
	restyClient.SetCacheEnabled(useCache)
	curlExecutor.SetCacheEnabled(useCache)

	// 执行 HTTP 请求
	res, err := httpClient.Do(args)
	if err != nil {
		return err
	}

	// 生成 schema
	schemaOutBytes, err := Infer(bytes.NewReader(res.Body), Options{
		Format:          FormatOpenAPI,
		IncludeExamples: false,
		Indent:          "",
		KeepConst:       false,
		AddDescriptions: true,
	})
	if err != nil {
		return err
	}

	var schema any
	if err := json.Unmarshal(schemaOutBytes, &schema); err != nil {
		return err
	}

	return outputSchema(schema, outFormat, pretty, indent)
}

// outputSchema 输出 schema
func outputSchema(schema any, outFormat string, pretty bool, indent int) error {
	indentStr := indentString(indent, pretty)
	enc, err := encodeOutput(schema, outFormat, indentStr)
	if err != nil {
		return err
	}
	os.Stdout.Write(enc)
	if indentStr != "" {
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

// runSampleMode sample 模式
func runSampleMode(pretty bool, indent int, outFormat string) error {
	input, hasInput, err := readStdinMaybe()
	if err != nil {
		return err
	}
	if !hasInput {
		return fmt.Errorf("需要从 stdin 提供 JSON 数据")
	}

	indentStr := indentString(indent, pretty)

	var v any
	if err := json.Unmarshal(input, &v); err != nil {
		trimmed := stripToJSONStart(input)
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return err
		}
	}

	sampleObj := simplifyJSON(v)

	enc, err := encodeOutput(sampleObj, outFormat, indentStr)
	if err != nil {
		return err
	}

	os.Stdout.Write(enc)
	if indentStr != "" {
		fmt.Fprintln(os.Stdout)
	}

	return nil
}

// runCurlMode curl 模式
func runCurlMode(args []string, indent, thread int, sample, schemaOut, pretty bool, outFormat, matchMode string) error {
	if thread < 1 {
		thread = 1
	}

	matcher := createMatcher(matchMode, "")

	baseline, err := fetchStructure(args)
	if err != nil {
		return err
	}

	current, err := minimizeAll(args, baseline, thread, matcher)
	if err != nil {
		return err
	}

	final, err := fetchStructure(current)
	if err != nil {
		return err
	}

	indentStr := indentString(indent, pretty)

	fmt.Fprintln(os.Stdout, "curl "+shellJoin(current))
	fmt.Fprintln(os.Stdout)

	if sample {
		if schemaOut {
			schemaBytes, _ := encodeOutput(final.Schema, outFormat, indentStr)
			os.Stdout.Write(schemaBytes)
			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout)
		}
		sampleObj, _ := simplifiedSample(final.Body)
		enc, _ := encodeOutput(sampleObj, outFormat, indentStr)
		os.Stdout.Write(enc)
	} else {
		enc, _ := encodeOutput(final.Schema, outFormat, indentStr)
		os.Stdout.Write(enc)
	}

	if indentStr != "" {
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

// outputRawResponse 直接输出原始响应
func outputRawResponse(body []byte, raw bool, outFormat string, pretty bool, indent int) error {
	indentStr := indentString(indent, pretty)

	var outputData any
	if raw {
		var bodyMap map[string]any
		if err := json.Unmarshal(body, &bodyMap); err == nil {
			outputData = bodyMap
		} else {
			outputData = string(body)
		}
	} else {
		sampleObj, _ := simplifiedSample(body)
		outputData = sampleObj
	}

	enc, err := encodeOutput(outputData, outFormat, indentStr)
	if err != nil {
		return err
	}
	os.Stdout.Write(enc)

	if indentStr != "" || pretty {
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

// runPureMode pure 模式 - 精简请求并输出最小化请求
// 注意：缓存现在在 HTTP client 层处理，不在业务层处理
func runPureMode(args []string, parallel int, matchMode, matchExpr, outFormat string, useCache bool, serial bool, useCurl bool) error {
	if parallel < 1 {
		parallel = 1
	}

	// 获取 HTTP 客户端并设置缓存
	httpClient = getHTTPClient(useCurl)
	restyClient.SetCacheEnabled(useCache)
	curlExecutor.SetCacheEnabled(useCache)

	matcher := createMatcher(matchMode, matchExpr)

	baseline, err := fetchStructure(args)
	if err != nil {
		return err
	}

	var current []string
	if serial {
		// 串行模式：逐个尝试删除参数
		current, err = minimizeAllSerial(args, baseline, matcher)
	} else {
		// 并行模式（默认）：批量并行删除参数
		current, err = minimizeAll(args, baseline, parallel, matcher)
	}
	if err != nil {
		return err
	}

	// 根据输出格式输出结果
	switch outFormat {
	case "json":
		return outputPureJSON(current)
	case "http":
		return outputPureHTTP(current)
	default: // curl
		fmt.Fprintln(os.Stdout, "curl "+shellJoin(current))
		return nil
	}
}

// outputPureJSON 输出 JSON 格式（类似 HAR）
func outputPureJSON(args []string) error {
	config, err := parseCurlArgs(args)
	if err != nil {
		return err
	}

	// 解析 URL
	parsedURL, _ := url.Parse(config.url)
	if parsedURL == nil {
		parsedURL = &url.URL{}
	}

	// 构建 headers 数组
	headers := []map[string]string{}
	for name, values := range config.headers {
		for _, value := range values {
			headers = append(headers, map[string]string{
				"name":  name,
				"value": value,
			})
		}
	}

	// 构建 cookies 数组
	cookies := []map[string]string{}
	for name, value := range config.cookies {
		cookies = append(cookies, map[string]string{
			"name":  name,
			"value": value,
		})
	}

	// 确定方法，默认为 GET
	method := strings.ToUpper(config.method)
	if method == "" {
		method = "GET"
	}

	// 构建类似 HAR 的结构
	result := map[string]any{
		"method":  method,
		"url":     config.url,
		"path":    parsedURL.Path,
		"query":   parsedURL.RawQuery,
		"headers": headers,
		"cookies": cookies,
	}

	if config.body != "" {
		result["body"] = config.body
	}

	enc, _ := json.MarshalIndent(result, "", "  ")
	os.Stdout.Write(enc)
	fmt.Fprintln(os.Stdout)
	return nil
}

// outputPureHTTP 输出 HTTP 协议消息格式
func outputPureHTTP(args []string) error {
	config, err := parseCurlArgs(args)
	if err != nil {
		return err
	}

	// 解析 URL
	parsedURL, _ := url.Parse(config.url)
	if parsedURL == nil {
		parsedURL = &url.URL{}
	}

	// 确定方法
	method := strings.ToUpper(config.method)
	if method == "" {
		method = "GET"
	}

	// 构建请求路径
	path := parsedURL.Path
	if path == "" {
		path = "/"
	}
	if parsedURL.RawQuery != "" {
		path += "?" + parsedURL.RawQuery
	}

	// 输出请求行
	fmt.Fprintf(os.Stdout, "%s %s HTTP/1.1\n", method, path)

	// 输出 Host header
	if parsedURL.Host != "" {
		fmt.Fprintf(os.Stdout, "Host: %s\n", parsedURL.Host)
	}

	// 输出其他 headers
	for name, values := range config.headers {
		for _, value := range values {
			fmt.Fprintf(os.Stdout, "%s: %s\n", name, value)
		}
	}

	// 输出 cookies
	if len(config.cookies) > 0 {
		cookieParts := []string{}
		for name, value := range config.cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", name, value))
		}
		fmt.Fprintf(os.Stdout, "Cookie: %s\n", strings.Join(cookieParts, "; "))
	}

	// 输出 body
	if config.body != "" {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, config.body)
	}

	return nil
}

// runMiniModeFromHTTP mini 模式 - 从 HTTP 获取精简 JSON
func runMiniModeFromHTTP(args []string, useCache bool, outFormat string, pretty bool, indent int, useCurl bool) error {
	// 启用/禁用 HTTP client 缓存
	httpClient = getHTTPClient(useCurl)
	restyClient.SetCacheEnabled(useCache)
	curlExecutor.SetCacheEnabled(useCache)

	// 执行 HTTP 请求
	res, err := httpClient.Do(args)
	if err != nil {
		return err
	}

	// 生成精简 sample
	sampleObj, err := simplifiedSample(res.Body)
	if err != nil {
		// 如果 JSON 解析失败，返回原始 body
		return outputMini(string(res.Body), outFormat, pretty, indent)
	}

	return outputMini(sampleObj, outFormat, pretty, indent)
}

// outputMini 输出 mini 结果
func outputMini(sample any, outFormat string, pretty bool, indent int) error {
	indentStr := indentString(indent, pretty)
	enc, err := encodeOutput(sample, outFormat, indentStr)
	if err != nil {
		return err
	}
	os.Stdout.Write(enc)
	if indentStr != "" || pretty {
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

// minimizeAll 精简请求
func minimizeAll(args []string, baseline structureResult, parallel int, matcher Matcher) ([]string, error) {
	// 阶段1: 贪心批量精简
	current := append([]string{}, args...)
	for {
		candidates := buildCandidates(current)
		if len(candidates) == 0 {
			break
		}
		removable := probeCandidates(candidates, baseline, parallel, matcher)
		if len(removable) == 0 {
			break
		}

		batch := filterCandidates(candidates, removable)
		batchArgs := applyRemovals(current, batch)
		ok, _ := probeSameWithMatcher(batchArgs, baseline, matcher)
		if ok {
			if sameArgs(batchArgs, current) {
				break
			}
			current = batchArgs
			continue
		}

		changed := false
		for _, c := range batch {
			testArgs := applyRemoval(current, c)
			ok, _ := probeSameWithMatcher(testArgs, baseline, matcher)
			if ok && !sameArgs(testArgs, current) {
				current = testArgs
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// 阶段2: 验证最终结果
	// 如果贪心结果不满足条件，说明存在参数依赖（如Auth和Cookie互斥）
	ok, err := probeSameWithMatcher(current, baseline, matcher)
	if err != nil {
		return nil, err
	}
	if ok {
		return current, nil
	}

	// 阶段3: 恢复模式 - 尝试加回被删除的参数
	// 构建被删除的候选列表（与原始请求相比）
	removedCandidates := buildRemovedCandidates(args, current)
	if len(removedCandidates) > 0 {
		// 并行尝试加回每个参数，找到一个能工作的即可
		restored := probeRestoreCandidates(current, removedCandidates, baseline, parallel, matcher)
		if restored != nil {
			// 加回一个参数后验证
			ok, _ := probeSameWithMatcher(restored, baseline, matcher)
			if ok {
				return restored, nil
			}
		}
	}

	// 阶段4: 保守模式 - 回退到原始请求，串行逐个精简
	return minimizeAllSequential(args, baseline, matcher)
}

// buildRemovedCandidates 构建被删除的候选列表（通用实现）
// 通过比较原始请求和精简后的请求，找出被删除的参数
func buildRemovedCandidates(original, minimized []string) []candidate {
	// 从原始请求构建所有候选
	origCandidates := buildCandidates(original)
	
	// 从精简请求构建候选集合（用于快速查找）
	minCandidates := buildCandidates(minimized)
	minCandidateSet := make(map[string]bool)
	for _, c := range minCandidates {
		minCandidateSet[c.kind+":"+c.key] = true
	}
	
	var removed []candidate
	
	// 找出被删除的候选（存在于原始请求但不在精简请求中）
	for _, c := range origCandidates {
		if !minCandidateSet[c.kind+":"+c.key] {
			// 这个候选被删除了，构建恢复后的请求
			// 恢复 = 当前精简请求 + 这个被删除的参数
			restoredArgs := mergeCandidateBack(minimized, c)
			if restoredArgs != nil {
				removed = append(removed, candidate{
					kind: c.kind,
					key:  c.key,
					args: restoredArgs,
				})
			}
		}
	}
	
	return removed
}

// mergeCandidateBack 将被删除的参数合并回精简后的请求
func mergeCandidateBack(minimized []string, c candidate) []string {
	// 对于 header: 使用 AddHeader
	if c.kind == "header" {
		return AddHeader(minimized, c.key, getOriginalHeaderValue(c.args, c.key))
	}
	
	// 对于 query: 从原始候选的 args 中提取 URL，合并到当前请求
	if c.kind == "query" {
		return mergeQueryBack(minimized, c)
	}
	
	// 对于 post body: 从原始候选的 args 中提取 body，合并到当前请求
	if c.kind == "post" {
		return mergePostBodyBack(minimized, c)
	}
	
	// 对于 cookie: 合并 cookie 到 Cookie header
	if c.kind == "cookie" {
		return mergeCookieBack(minimized, c)
	}
	
	return nil
}

// getOriginalHeaderValue 从原始请求中获取 header 值
func getOriginalHeaderValue(original []string, key string) string {
	headers := ExtractHeaders(original)
	for _, h := range headers {
		if strings.EqualFold(h.Key, key) {
			if idx := strings.Index(h.Raw, ":"); idx != -1 {
				return strings.TrimSpace(h.Raw[idx+1:])
			}
		}
	}
	return ""
}

// mergeQueryBack 将 query 参数合并回请求
func mergeQueryBack(minimized []string, c candidate) []string {
	// 从候选的 args 中提取被删除的 query 值
	origURL, _ := FindFirstURL(c.args)
	minURL, minIdx := FindFirstURL(minimized)
	
	if origURL == "" || minURL == "" || minIdx == -1 {
		return nil
	}
	
	origParsed, _ := url.Parse(origURL)
	minParsed, _ := url.Parse(minURL)
	
	if origParsed == nil || minParsed == nil {
		return nil
	}
	
	// 获取被删除的 query 值
	origValues := origParsed.Query()
	minValues := minParsed.Query()
	
	// 构建新的 query，包含精简后的所有 query 加上被删除的这个
	newQuery := url.Values{}
	for k, vs := range minValues {
		newQuery[k] = vs
	}
	// 加上被删除的（存在于 orig 但不在 min 中的）
	for k, vs := range origValues {
		if _, exists := minValues[k]; !exists && len(vs) > 0 {
			newQuery[k] = vs
			break // 只加回这一个
		}
	}
	
	newParsed := cloneURL(minParsed)
	newParsed.RawQuery = newQuery.Encode()
	
	newArgs := append([]string{}, minimized...)
	newArgs[minIdx] = newParsed.String()
	return newArgs
}

// mergePostBodyBack 将 POST body 字段合并回请求
func mergePostBodyBack(minimized []string, c candidate) []string {
	// 从候选的 args 和当前请求中提取 body
	origRef, origOk := FindFirstDataArg(c.args)
	minRef, minOk := FindFirstDataArg(minimized)
	
	if !origOk || origRef.Value == "" {
		return nil
	}
	
	origTrim := strings.TrimSpace(origRef.Value)
	minTrim := ""
	if minOk {
		minTrim = strings.TrimSpace(minRef.Value)
	}
	
	if !strings.HasPrefix(origTrim, "{") {
		return nil
	}
	
	var origPayload map[string]any
	if err := json.Unmarshal([]byte(origTrim), &origPayload); err != nil {
		return nil
	}
	
	var minPayload map[string]any
	if minTrim != "" && strings.HasPrefix(minTrim, "{") {
		json.Unmarshal([]byte(minTrim), &minPayload)
	}
	
	// 构建新的 payload，包含精简后的所有字段加上被删除的字段
	newPayload := make(map[string]any)
	for k, v := range minPayload {
		newPayload[k] = v
	}
	// 加上被删除的（存在于 orig 但不在 min 中的）
	for k, v := range origPayload {
		if _, exists := minPayload[k]; !exists {
			newPayload[k] = v
			break // 只加回这一个
		}
	}
	
	body, _ := json.Marshal(newPayload)
	if minOk {
		return ReplaceDataArg(minimized, minRef, string(body))
	}
	// 当前请求没有 body，需要添加
	return append(minimized, "-d", string(body))
}

// mergeCookieBack 将 cookie 合并回 Cookie header
func mergeCookieBack(minimized []string, c candidate) []string {
	// 从候选的 args 中提取 cookie 值
	origValue := getCookieHeaderValue(c.args)
	minValue := getCookieHeaderValue(minimized)
	
	origCookies := parseCookieHeader(origValue)
	minCookies := parseCookieHeader(minValue)
	
	// 构建新的 cookies，包含精简后的所有 cookies 加上被删除的这个
	newCookies := make(map[string]string)
	for k, v := range minCookies {
		newCookies[k] = v
	}
	// 加上被删除的（存在于 orig 但不在 min 中的）
	for k, v := range origCookies {
		if _, exists := minCookies[k]; !exists {
			newCookies[k] = v
			break // 只加回这一个
		}
	}
	
	newCookieValue := buildCookieHeader(newCookies)
	return ReplaceHeaderValue(minimized, "Cookie", newCookieValue)
}

// getCookieHeaderValue 从请求中提取 Cookie header 值
func getCookieHeaderValue(args []string) string {
	headers := ExtractHeaders(args)
	for _, h := range headers {
		if strings.EqualFold(h.Key, "Cookie") && h.Raw != "" {
			cookieValue := h.Raw
			if idx := strings.Index(cookieValue, ":"); idx != -1 {
				return strings.TrimSpace(cookieValue[idx+1:])
			}
		}
	}
	return ""
}

// AddHeader 添加 header 到请求
func AddHeader(args []string, key, value string) []string {
	out := append([]string{}, args...)
	out = append(out, "-H", key+": "+value)
	return out
}

// probeRestoreCandidates 并行尝试加回参数，返回第一个能工作的
func probeRestoreCandidates(base []string, candidates []candidate, baseline structureResult, parallel int, matcher Matcher) []string {
	if len(candidates) == 0 {
		return nil
	}
	
	type job struct {
		idx  int
		args []string
	}
	jobs := make(chan job, len(candidates))
	results := make(chan int, len(candidates))
	var wg sync.WaitGroup
	
	workers := parallel
	if workers < 1 {
		workers = 1
	}
	if workers > len(candidates) {
		workers = len(candidates)
	}
	
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				ok, _ := probeSameWithMatcher(j.args, baseline, matcher)
				if ok {
					results <- j.idx
				}
			}
		}()
	}
	
	go func() {
		wg.Wait()
		close(results)
	}()
	
	for i, c := range candidates {
		jobs <- job{idx: i, args: c.args}
	}
	close(jobs)
	
	// 返回第一个成功的
	for idx := range results {
		return candidates[idx].args
	}
	
	return nil
}

// minimizeAllSequential 串行逐个精简（保守模式）
func minimizeAllSequential(args []string, baseline structureResult, matcher Matcher) ([]string, error) {
	current := append([]string{}, args...)
	
	for {
		candidates := buildCandidates(current)
		if len(candidates) == 0 {
			return current, nil
		}
		
		changed := false
		for _, c := range candidates {
			testArgs := applyRemoval(current, c)
			ok, _ := probeSameWithMatcher(testArgs, baseline, matcher)
			if ok && !sameArgs(testArgs, current) {
				current = testArgs
				changed = true
				// 一次只删除一个，然后重新构建候选
				break
			}
		}
		
		if !changed {
			return current, nil
		}
	}
}

// minimizeAllSerial 串行精简模式（用户显式指定）
// 特点：基于当前状态逐个精简，每次精简后更新当前状态
// 例如：[a,b,c,d] -> 删除 a 成功 -> 当前 [b,c,d] -> 尝试删除 b -> [c,d] ...
// 这与并行模式不同，并行模式始终基于原始请求进行测试
func minimizeAllSerial(args []string, baseline structureResult, matcher Matcher) ([]string, error) {
	current := append([]string{}, args...)
	
	for {
		candidates := buildCandidates(current)
		if len(candidates) == 0 {
			return current, nil
		}
		
		found := false
		for _, c := range candidates {
			// 基于当前状态删除一个参数
			testArgs := applyRemoval(current, c)
			if sameArgs(testArgs, current) {
				continue
			}
			
			// 验证删除后的请求
			ok, err := probeSameWithMatcher(testArgs, baseline, matcher)
			if err != nil {
				return nil, err
			}
			
			if ok {
				// 删除成功，更新当前状态并从头开始
				current = testArgs
				found = true
				break
			}
		}
		
		// 如果一轮下来没有删除任何参数，说明已经最小化
		if !found {
			return current, nil
		}
	}
}

func probeSameWithMatcher(args []string, baseline structureResult, matcher Matcher) (bool, error) {
	// 使用 httpClient 执行请求（确保已初始化）
	if httpClient == nil {
		httpClient = getHTTPClient(false) // 默认使用 resty
	}
	
	res, err := httpClient.Do(args)
	if err != nil {
		return false, err
	}
	current := buildHTTPResponse(res)
	baseResp := Response{Status: baseline.Status, Headers: baseline.Headers, Body: baseline.Body}
	return matcher.Match(baseResp, current), nil
}

func buildHTTPResponse(res *HTTPResponse) Response {
	r := Response{
		Status:  res.Status,
		Headers: res.Headers,
		Body:    res.Body,
	}
	var bodyMap map[string]any
	if err := json.Unmarshal(res.Body, &bodyMap); err == nil {
		r.BodyMap = bodyMap
	}
	return r
}

func probeCandidates(candidates []candidate, baseline structureResult, parallel int, matcher Matcher) map[string]bool {
	out := make(map[string]bool)
	if len(candidates) == 0 {
		return out
	}

	type job struct {
		id   string
		args []string
	}
	jobs := make(chan job, len(candidates))
	results := make(chan string, len(candidates))
	var wg sync.WaitGroup

	workers := parallel
	if workers < 1 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				ok, _ := probeSameWithMatcher(j.args, baseline, matcher)
				if ok {
					results <- j.id
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for _, c := range candidates {
		jobs <- job{id: c.kind + ":" + c.key, args: c.args}
	}
	close(jobs)

	for id := range results {
		out[id] = true
	}
	return out
}

func filterCandidates(candidates []candidate, removable map[string]bool) []candidate {
	var out []candidate
	for _, c := range candidates {
		if removable[c.kind+":"+c.key] {
			out = append(out, c)
		}
	}
	return out
}

func applyRemovals(args []string, candidates []candidate) []string {
	out := append([]string{}, args...)
	for _, c := range candidates {
		out = applyRemoval(out, c)
	}
	return out
}

func applyRemoval(args []string, c candidate) []string {
	switch c.kind {
	case "header":
		return RemoveHeaders(args, map[string]bool{c.key: true})
	case "query", "post", "cookie":
		return c.args
	default:
		return args
	}
}

func sameArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func shellJoin(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	needs := strings.ContainsAny(s, " \t\n'\"\\$`")
	if !needs {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func indentString(indent int, pretty bool) string {
	if indent > 0 {
		return strings.Repeat(" ", indent)
	}
	if pretty {
		return "  "
	}
	return ""
}

func stripToJSONStart(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	for i, c := range b {
		if c == '{' || c == '[' {
			return b[i:]
		}
	}
	return b
}

func readStdinMaybe() ([]byte, bool, error) {
	if stdinIsTTY() {
		return nil, false, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, false, err
	}
	return data, len(data) > 0, nil
}

func printUsage() {
	msg := `apix - API 请求净化与响应分析工具

用法:
  apix [子命令] [参数]

子命令:
  pure    精简 HTTP 请求，输出最小化请求
  mini    获取精简后的 JSON 数据
  schema  获取响应 JSON 的数据结构
  version 打印版本号

示例:
  # 精简请求
  apix pure curl https://httpbin.org/get
  
  # 获取精简响应
  apix mini curl https://httpbin.org/get
  
  # 获取响应 Schema
  apix schema curl https://httpbin.org/get
  
  # 从 stdin 处理 JSON
  cat resp.json | apix mini
  cat resp.json | apix schema

使用 "apix [子命令] --help" 查看子命令详细帮助。
`
	fmt.Fprintln(os.Stdout, msg)
}

func simplifyArray(arr []any) any {
	if len(arr) == 0 {
		return []any{}
	}
	if len(arr) >= 2 {
		return []any{simplifyJSON(arr[0]), simplifyJSON(arr[len(arr)-1])}
	}
	return []any{simplifyJSON(arr[0])}
}

func truncateText(s string) string {
	r := []rune(s)
	if len(r) <= 30 {
		return s
	}
	truncated := len(r) - 20
	head := string(r[:10])
	tail := string(r[len(r)-10:])
	return fmt.Sprintf("%s[...truncated:%d chars...]%s", head, truncated, tail)
}

func normalizeOutFormat(s string) (string, error) {
	if s == "" {
		return "json", nil
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "json":
		return "json", nil
	case "yaml", "yml":
		return "yaml", nil
	case "hjson":
		return "hjson", nil
	case "edn":
		return "edn", nil
	case "kdl":
		return "kdl", nil
	case "hcl":
		return "hcl", nil
	default:
		return "", fmt.Errorf("unsupported output format: %s", s)
	}
}

// encodeOutput 支持多种输出格式
func encodeOutput(data any, format, indent string) ([]byte, error) {
	switch format {
	case "yaml", "yml":
		return encodeYAML(data, indent)
	case "hjson":
		return encodeHJSON(data, indent)
	case "json":
		if indent != "" {
			return json.MarshalIndent(data, "", indent)
		}
		return json.Marshal(data)
	default:
		// 默认使用 json
		if indent != "" {
			return json.MarshalIndent(data, "", indent)
		}
		return json.Marshal(data)
	}
}

// encodeYAML 编码为 YAML
func encodeYAML(data any, indent string) ([]byte, error) {
	return yaml.Marshal(data)
}

// encodeHJSON 编码为 HJSON
func encodeHJSON(data any, indent string) ([]byte, error) {
	return hjson.Marshal(data)
}

func simplifiedSample(body []byte) (any, error) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return simplifyJSON(v), nil
}

func simplifyJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, v := range t {
			out[k] = simplifyJSON(v)
		}
		return out
	case []any:
		return simplifyArray(t)
	case string:
		return truncateText(t)
	default:
		return v
	}
}

func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// 版本信息，在构建时通过 -ldflags 注入
var (
	AppVersion = "dev"
	GitCommit  = "unknown"
	BuildTime  = "unknown"
)

// structureResult 结构结果
type structureResult struct {
	Schema  any
	Digest  string
	Status  int
	Headers map[string]string
	Body    []byte
}

// candidate 候选项
type candidate struct {
	kind string
	key  string
	args []string
}

// fetchStructure 获取结构
func fetchStructure(args []string) (structureResult, error) {
	// 使用 httpClient 执行请求（确保已初始化）
	if httpClient == nil {
		httpClient = getHTTPClient(false) // 默认使用 resty
	}
	
	res, err := httpClient.Do(args)
	if err != nil {
		return structureResult{}, err
	}
	if res.Status < 200 || res.Status >= 400 {
		return structureResult{}, fmt.Errorf("unexpected status code: %d", res.Status)
	}

	out, err := Infer(bytes.NewReader(res.Body), Options{
		Format:          FormatOpenAPI,
		IncludeExamples: false,
		Indent:          "",
		KeepConst:       false,
		AddDescriptions: false,
		StructureOnly:   true,
	})
	if err != nil {
		return structureResult{}, err
	}

	var schema any
	if err := json.Unmarshal(out, &schema); err != nil {
		return structureResult{}, err
	}
	digest := schemaDigest(schema)
	return structureResult{Schema: schema, Digest: digest, Status: res.Status, Headers: res.Headers, Body: res.Body}, nil
}

// buildCandidates 构建候选项
func buildCandidates(args []string) []candidate {
	var out []candidate

	// Headers
	headers := ExtractHeaders(args)
	seenKeys := make(map[string]bool)
	for _, h := range headers {
		if h.Key == "" || seenKeys[h.Key] {
			continue
		}
		seenKeys[h.Key] = true
		out = append(out, candidate{
			kind: "header",
			key:  h.Key,
			args: RemoveHeaders(args, map[string]bool{h.Key: true}),
		})
	}

	// Query 参数
	if rawURL, idx := FindFirstURL(args); idx != -1 {
		if parsed, err := url.Parse(rawURL); err == nil && parsed.RawQuery != "" {
			values := parsed.Query()
			keys := make([]string, 0, len(values))
			for k := range values {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				next := cloneURL(parsed)
				q := next.Query()
				q.Del(key)
				next.RawQuery = q.Encode()
				newArgs := append([]string{}, args...)
				newArgs[idx] = next.String()
				out = append(out, candidate{
					kind: "query",
					key:  key,
					args: newArgs,
				})
			}
		}
	}

	// POST body JSON 字段
	if ref, ok := FindFirstDataArg(args); ok && ref.Value != "" && !strings.HasPrefix(ref.Value, "@") {
		trim := strings.TrimSpace(ref.Value)
		if strings.HasPrefix(trim, "{") {
			var payload map[string]any
			if err := json.Unmarshal([]byte(trim), &payload); err == nil && len(payload) > 0 {
				keys := make([]string, 0, len(payload))
				for k := range payload {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, key := range keys {
					next := make(map[string]any, len(payload)-1)
					for k, v := range payload {
						if k != key {
							next[k] = v
						}
					}
					body, _ := json.Marshal(next)
					out = append(out, candidate{
						kind: "post",
						key:  key,
						args: ReplaceDataArg(args, ref, string(body)),
					})
				}
			}
		}
	}

	// Cookie - 处理 Cookie header 中的各个 key
	for _, h := range headers {
		if strings.EqualFold(h.Key, "Cookie") && h.Raw != "" {
			// 提取 Cookie header 的值（去掉 "Cookie: " 前缀）
			cookieValue := h.Raw
			if idx := strings.Index(cookieValue, ":"); idx != -1 {
				cookieValue = strings.TrimSpace(cookieValue[idx+1:])
			}
			cookies := parseCookieHeader(cookieValue)
			if len(cookies) > 0 {
				keys := make([]string, 0, len(cookies))
				for k := range cookies {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, key := range keys {
					// 构建删除该 cookie key 后的新 Cookie header 值
					newCookies := make(map[string]string)
					for k, v := range cookies {
						if k != key {
							newCookies[k] = v
						}
					}
					newCookieValue := buildCookieHeader(newCookies)
					out = append(out, candidate{
						kind: "cookie",
						key:  key,
						args: ReplaceHeaderValue(args, h.Key, newCookieValue),
					})
				}
			}
		}
	}

	return out
}

// parseCookieHeader 解析 Cookie header 值
func parseCookieHeader(headerValue string) map[string]string {
	cookies := make(map[string]string)
	parts := strings.Split(headerValue, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		keyVal := strings.SplitN(part, "=", 2)
		if len(keyVal) >= 1 {
			key := keyVal[0]
			val := ""
			if len(keyVal) == 2 {
				val = keyVal[1]
			}
			cookies[key] = val
		}
	}
	return cookies
}

// buildCookieHeader 构建 Cookie header 值
func buildCookieHeader(cookies map[string]string) string {
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for k, v := range cookies {
		if v == "" {
			parts = append(parts, k)
		} else {
			parts = append(parts, k+"="+v)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func cloneURL(u *url.URL) *url.URL {
	clone := *u
	return &clone
}

// schemaDigest 计算 schema digest
func schemaDigest(schema any) string {
	var b strings.Builder
	writeDigest(&b, schema)
	return b.String()
}

func writeDigest(b *strings.Builder, v any) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("{")
		for _, k := range keys {
			b.WriteString(k)
			b.WriteByte(':')
			writeDigest(b, t[k])
			b.WriteByte(',')
		}
		b.WriteString("}")
	case []any:
		b.WriteString("[")
		for _, v := range t {
			writeDigest(b, v)
			b.WriteByte(',')
		}
		b.WriteString("]")
	case string:
		b.WriteString(strconv.Quote(t))
	case float64:
		b.WriteString(strconv.FormatFloat(t, 'g', -1, 64))
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case nil:
		b.WriteString("null")
	default:
		b.WriteString(strconv.Quote(fmt.Sprintf("%v", t)))
	}
}
