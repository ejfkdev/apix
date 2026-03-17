package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// MatchContext 匹配上下文，包含响应信息和匹配参数
type MatchContext struct {
	Baseline Response
	Current  Response
	Args     []string // 额外参数，如 keyword、regex 模式等
}

// MatcherV2 新版匹配器接口
type MatcherV2 interface {
	Match(ctx MatchContext) bool
	Name() string
}

// StatusMatcher 状态码匹配器
type StatusMatcher struct{}

func (m *StatusMatcher) Name() string { return "status" }
func (m *StatusMatcher) Match(ctx MatchContext) bool {
	return ctx.Baseline.Status == ctx.Current.Status
}

// StructureMatcher 数据结构匹配器
type StructureMatcherV2 struct{}

func (m *StructureMatcherV2) Name() string { return "structure" }
func (m *StructureMatcherV2) Match(ctx MatchContext) bool {
	if ctx.Baseline.Status != ctx.Current.Status {
		return false
	}
	return schemaDigestFromBody(ctx.Baseline.Body) == schemaDigestFromBody(ctx.Current.Body)
}

// HeaderMatcher 响应头匹配器（检查关键头是否一致）
type HeaderMatcher struct{}

func (m *HeaderMatcher) Name() string { return "header" }
func (m *HeaderMatcher) Match(ctx MatchContext) bool {
	// 简化实现：只比较 Content-Type
	return true // 暂时返回 true，实际需要解析 header
}

// ContentMatcher 内容匹配器（完全匹配）
type ContentMatcher struct{}

func (m *ContentMatcher) Name() string { return "content" }
func (m *ContentMatcher) Match(ctx MatchContext) bool {
	return bytes.Equal(ctx.Baseline.Body, ctx.Current.Body)
}

// SimilarityMatcher 内容相似度匹配器
type SimilarityMatcher struct{}

func (m *SimilarityMatcher) Name() string { return "similarity" }
func (m *SimilarityMatcher) Match(ctx MatchContext) bool {
	threshold := 0.8 // 默认 80% 相似度
	if len(ctx.Args) > 0 {
		if t, err := strconv.ParseFloat(ctx.Args[0], 64); err == nil {
			threshold = t
		}
	}
	
	// 简化实现：计算 Jaccard 相似度
	// 实际使用时可引入更复杂的文本相似度算法
	similarity := calculateSimilarity(string(ctx.Baseline.Body), string(ctx.Current.Body))
	return similarity >= threshold
}

func calculateSimilarity(a, b string) float64 {
	// 简化的相似度计算
	if a == b {
		return 1.0
	}
	// 计算共同字符比例
	setA := make(map[rune]bool)
	for _, r := range a {
		setA[r] = true
	}
	
	common := 0
	setB := make(map[rune]bool)
	for _, r := range b {
		if setA[r] && !setB[r] {
			common++
		}
		setB[r] = true
	}
	
	union := len(setA) + len(setB) - common
	if union == 0 {
		return 1.0
	}
	return float64(common) / float64(union)
}

// LineCountMatcher 行数匹配器
type LineCountMatcher struct{}

func (m *LineCountMatcher) Name() string { return "line-count" }
func (m *LineCountMatcher) Match(ctx MatchContext) bool {
	baselineLines := strings.Count(string(ctx.Baseline.Body), "\n")
	currentLines := strings.Count(string(ctx.Current.Body), "\n")
	return baselineLines == currentLines
}

// KeywordMatcher 关键字匹配器
type KeywordMatcher struct{}

func (m *KeywordMatcher) Name() string { return "keyword" }
func (m *KeywordMatcher) Match(ctx MatchContext) bool {
	if len(ctx.Args) == 0 {
		return true
	}
	keyword := ctx.Args[0]
	return strings.Contains(string(ctx.Current.Body), keyword)
}

// RegexMatcher 正则匹配器
type RegexMatcher struct{}

func (m *RegexMatcher) Name() string { return "regex" }
func (m *RegexMatcher) Match(ctx MatchContext) bool {
	if len(ctx.Args) == 0 {
		return true
	}
	pattern := ctx.Args[0]
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.Match(ctx.Current.Body)
}

// ExprMatcherV2 表达式匹配器（使用 expr 库）
type ExprMatcherV2 struct {
	program *vm.Program
}

func NewExprMatcherV2(expression string) (*ExprMatcherV2, error) {
	// 预处理表达式
	exprStr := preprocessExpr(expression)
	
	program, err := expr.Compile(exprStr, expr.Env(map[string]any{}))
	if err != nil {
		return nil, err
	}
	
	return &ExprMatcherV2{program: program}, nil
}

func (m *ExprMatcherV2) Name() string { return "expr" }
func (m *ExprMatcherV2) Match(ctx MatchContext) bool {
	// 构建表达式执行环境
	env := buildExprEnv(ctx.Current)
	
	result, err := expr.Run(m.program, env)
	if err != nil {
		return false
	}
	
	if b, ok := result.(bool); ok {
		return b
	}
	return false
}

// preprocessExpr 预处理表达式
// 1. 支持 status==200 简写（status 自动映射）
// 2. 支持 $.code==200 简写（转为 body.code == 200）
// 3. 支持 $.data 表示存在字段（expr 内置）
// 4. 支持 !$.data 表示不存在字段
//
// 逻辑运算符使用 expr 标准语法：&& || !
// 示例: status==200 && $.code==0 && line-count<50
func preprocessExpr(expr string) string {
	expr = strings.TrimSpace(expr)
	
	// 处理 !$.field 表示字段不存在
	// 将 !$.field 替换为 (body.field == nil)
	if strings.Contains(expr, "!$.") {
		expr = handleNotExistField(expr)
	}
	
	// 处理 $.field 简写
	// $.code -> body.code
	// $.data.items -> body.data.items
	expr = regexp.MustCompile(`\$\.([a-zA-Z_][a-zA-Z0-9_]*)`).ReplaceAllString(expr, "body.$1")
	expr = regexp.MustCompile(`\$\['([^']+)'\]`).ReplaceAllString(expr, "body['$1']")
	
	// 处理 status、line-count、content-length 等字段
	// 如果前面没有 body. 前缀，则认为是顶层字段
	expr = handleTopLevelFields(expr)
	
	return expr
}

func handleNotExistField(expr string) string {
	// 简单实现：将 !$.field 替换为 (body.field == nil)
	re := regexp.MustCompile(`!\$\.([a-zA-Z_][a-zA-Z0-9_\.]*)`)
	return re.ReplaceAllString(expr, "($1 == nil)")
}

func handleTopLevelFields(expr string) string {
	// 定义顶层字段映射（将短横线替换为下划线）
	fieldMap := map[string]string{
		"line-count":     "line_count",
		"content-length": "content_length",
	}
	
	// 使用简单的单词边界替换（这些字段不会被 body. 前缀）
	for oldField, newField := range fieldMap {
		// 使用单词边界匹配完整的字段名
		pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(oldField))
		re := regexp.MustCompile(pattern)
		expr = re.ReplaceAllString(expr, newField)
	}
	
	return expr
}

func buildExprEnv(resp Response) map[string]any {
	// 构建 headers map（统一转为小写，实现不区分大小写访问）
	headers := make(map[string]string)
	for k, v := range resp.Headers {
		headers[strings.ToLower(k)] = v
	}
	
	env := map[string]any{
		"status":          resp.Status,
		"content_length":  len(resp.Body),
		"line_count":      strings.Count(string(resp.Body), "\n"),
		"headers":         headers,
		"body":            resp.BodyMap,
	}
	
	// 如果 BodyMap 为空，尝试解析
	if resp.BodyMap == nil && len(resp.Body) > 0 {
		var bodyMap any
		if err := json.Unmarshal(resp.Body, &bodyMap); err == nil {
			env["body"] = bodyMap
		} else {
			env["body"] = string(resp.Body)
		}
	}
	
	return env
}

// CreateMatcher 创建匹配器
type MatcherConfig struct {
	Mode string
	Args []string
	Expr string
}

func CreateMatcher(config MatcherConfig) (MatcherV2, error) {
	switch config.Mode {
	case "status":
		return &StatusMatcher{}, nil
	case "structure":
		return &StructureMatcherV2{}, nil
	case "header":
		return &HeaderMatcher{}, nil
	case "content":
		return &ContentMatcher{}, nil
	case "similarity":
		return &SimilarityMatcher{}, nil
	case "line-count":
		return &LineCountMatcher{}, nil
	case "keyword":
		return &KeywordMatcher{}, nil
	case "regex":
		return &RegexMatcher{}, nil
	case "expr":
		if config.Expr == "" {
			return nil, fmt.Errorf("expr mode requires expression")
		}
		return NewExprMatcherV2(config.Expr)
	default:
		// 默认使用 structure
		return &StructureMatcherV2{}, nil
	}
}

// LegacyMatcher 适配器，兼容旧的 Matcher 接口
type LegacyMatcher struct {
	inner MatcherV2
	ctx   MatchContext
}

func NewLegacyMatcher(inner MatcherV2) *LegacyMatcher {
	return &LegacyMatcher{inner: inner}
}

func (m *LegacyMatcher) Match(baseline, current Response) bool {
	m.ctx.Baseline = baseline
	m.ctx.Current = current
	return m.inner.Match(m.ctx)
}
