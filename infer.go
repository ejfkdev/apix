package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	jsonschema "github.com/JLugagne/jsonschema-infer"
)

const (
	FormatOpenAPI    = "openapi"
	FormatOpenAPIDoc = "openapi-doc"
	FormatJSONSchema = "jsonschema"
)

const (
	AppName = "jsc"
)

const (
	openAPIVersion         = "3.1.0"
	openAPISchemaDialectID = "https://spec.openapis.org/oas/3.1/dialect/base"
)

type Options struct {
	Format           string
	SchemaName       string
	IncludeExamples  bool
	ExampleMaxLength int
	Indent           string
	KeepConst        bool
	AddDescriptions  bool
	StructureOnly    bool
}

func Infer(r io.Reader, opts Options) ([]byte, error) {
	if opts.Format == "" {
		opts.Format = FormatOpenAPI
	}
	if opts.SchemaName == "" {
		opts.SchemaName = "Response"
	}
	if opts.ExampleMaxLength <= 0 {
		opts.ExampleMaxLength = 200
	}

	genOpts := make([]jsonschema.Option, 0, 2)
	if opts.IncludeExamples {
		genOpts = append(genOpts, jsonschema.WithExamples())
	}

	generator := jsonschema.New(genOpts...)
	stats := newStatsCollector()

	dec := json.NewDecoder(bufio.NewReader(r))

	sampleCount := 0
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("invalid JSON input: %w", err)
		}
		sampleCount++
		stats.observe(v, nil)
		if err := generator.AddParsedSample(v); err != nil {
			return nil, fmt.Errorf("failed to add sample: %w", err)
		}
	}
	if sampleCount == 0 {
		return nil, errors.New("no JSON input found on stdin")
	}

	schemaJSON, err := generator.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %w", err)
	}

	var schema any
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return nil, fmt.Errorf("failed to parse generated schema: %w", err)
	}

	schema = normalizeSchema(schema, stats, opts)

	var out any
	switch opts.Format {
	case FormatOpenAPI:
		out = schema
	case FormatOpenAPIDoc:
		out = wrapOpenAPIDoc(schema, opts.SchemaName)
	case FormatJSONSchema:
		out = schema
	default:
		return nil, fmt.Errorf("unknown format: %s", opts.Format)
	}

	if opts.Indent != "" {
		return json.MarshalIndent(out, "", opts.Indent)
	}
	return json.Marshal(out)
}

func wrapOpenAPIDoc(schema any, schemaName string) map[string]any {
	return map[string]any{
		"openapi":           openAPIVersion,
		"jsonSchemaDialect": openAPISchemaDialectID,
		"info": map[string]any{
			"title":   AppName,
			"version": AppVersion,
		},
		"paths": map[string]any{},
		"components": map[string]any{
			"schemas": map[string]any{
				schemaName: schema,
			},
		},
	}
}

func normalizeSchema(schema any, stats *statsCollector, opts Options) any {
	m, ok := schema.(map[string]any)
	if ok {
		if opts.Format != FormatJSONSchema {
			delete(m, "$schema")
		}
	}
	normalizeNode(schema, opts.ExampleMaxLength, opts.KeepConst)
	addOptional(schema, nil, stats)
	if opts.StructureOnly {
		return simplifySchema(schema)
	}
	state := &applyState{}
	applyStats(schema, nil, stats, opts, state)
	return schema
}

func normalizeNode(node any, maxLen int, keepConst bool) {
	switch t := node.(type) {
	case map[string]any:
		if !keepConst {
			delete(t, "const")
		}
		delete(t, "required")
		if ex, ok := t["example"]; ok {
			ex = truncateExampleValue(ex, maxLen)
			t["examples"] = []any{ex}
			delete(t, "example")
		}
		if exs, ok := t["examples"]; ok {
			t["examples"] = normalizeExamplesValue(exs, maxLen)
		}
		for k, v := range t {
			if k == "example" || k == "examples" {
				continue
			}
			normalizeNode(v, maxLen, keepConst)
		}
	case []any:
		for i := range t {
			normalizeNode(t[i], maxLen, keepConst)
		}
	}
}

func simplifySchema(node any) any {
	switch t := node.(type) {
	case map[string]any:
		out := map[string]any{}
		if v, ok := t["type"]; ok {
			out["type"] = v
		}
		if v, ok := t["optional"]; ok {
			out["optional"] = v
		}
		if v, ok := t["properties"]; ok {
			if props, ok := v.(map[string]any); ok {
				newProps := map[string]any{}
				for k, val := range props {
					newProps[k] = simplifySchema(val)
				}
				if len(newProps) > 0 {
					out["properties"] = newProps
				}
			}
		}
		if v, ok := t["items"]; ok {
			out["items"] = simplifySchema(v)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, v := range t {
			out = append(out, simplifySchema(v))
		}
		return out
	default:
		return node
	}
}

func normalizeExamplesValue(exs any, maxLen int) any {
	switch v := exs.(type) {
	case []any:
		for i := range v {
			v[i] = truncateExampleValue(v[i], maxLen)
		}
		return v
	default:
		return []any{truncateExampleValue(v, maxLen)}
	}
}

func truncateExampleValue(v any, maxLen int) any {
	switch t := v.(type) {
	case string:
		return truncateString(t, maxLen)
	case []any:
		for i := range t {
			t[i] = truncateExampleValue(t[i], maxLen)
		}
		return t
	case map[string]any:
		for k := range t {
			t[k] = truncateExampleValue(t[k], maxLen)
		}
		return t
	default:
		return v
	}
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	rs := []rune(s)
	if len(rs) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(rs[:maxLen])
	}
	return string(rs[:maxLen-3]) + "..."
}

type statsCollector struct {
	nodes  map[string]*nodeStats
	arrays map[string]*arrayObjectStats
}

type nodeStats struct {
	totalCount int
	nullCount  int

	stringSeen        bool
	stringMin         int
	stringMax         int
	stringEmptyCount  int
	stringUnique      map[string]struct{}
	stringUniqueCount int
	stringUniqueCap   bool

	numberSeen        bool
	numberMin         float64
	numberMax         float64
	numberUnique      map[string]struct{}
	numberUniqueCount int
	numberUniqueCap   bool

	arraySeen    bool
	arrayMin     int
	arrayMax     int
	arrayUnique  bool
	arrayUniqMin int
	arrayUniqMax int
}

func newStatsCollector() *statsCollector {
	return &statsCollector{
		nodes:  make(map[string]*nodeStats),
		arrays: make(map[string]*arrayObjectStats),
	}
}

func (s *statsCollector) observe(value any, path []string) {
	st := s.node(path)
	st.totalCount++
	if value == nil {
		st.nullCount++
		return
	}
	switch v := value.(type) {
	case string:
		st.observeString(v)
	case float64:
		st.observeNumber(v)
	case []any:
		st.observeArray(v)
		for _, item := range v {
			if obj, ok := item.(map[string]any); ok {
				arr := s.arrayStats(path)
				arr.total++
				for k := range obj {
					arr.fieldCounts[k]++
				}
			}
			s.observe(item, append(path, "*"))
		}
	case map[string]any:
		for k, val := range v {
			s.observe(val, append(path, k))
		}
	case bool:
		// 不记录数值或长度统计。
	default:
		_ = v
	}
}

func (s *statsCollector) node(path []string) *nodeStats {
	key := pathKey(path)
	if st, ok := s.nodes[key]; ok {
		return st
	}
	st := &nodeStats{}
	s.nodes[key] = st
	return st
}

func (s *statsCollector) nodeByPath(path []string) *nodeStats {
	return s.nodes[pathKey(path)]
}

type arrayObjectStats struct {
	total       int
	fieldCounts map[string]int
}

func (s *statsCollector) arrayStats(path []string) *arrayObjectStats {
	key := pathKey(path)
	if st, ok := s.arrays[key]; ok {
		return st
	}
	st := &arrayObjectStats{
		fieldCounts: make(map[string]int),
	}
	s.arrays[key] = st
	return st
}

func (s *statsCollector) arrayByPath(path []string) *arrayObjectStats {
	return s.arrays[pathKey(path)]
}

func (st *nodeStats) observeString(v string) {
	l := len([]rune(v))
	if v == "" {
		st.stringEmptyCount++
	}
	if !st.stringSeen {
		st.stringSeen = true
		st.stringMin = l
		st.stringMax = l
		st.trackStringUnique(v)
		return
	}
	if l < st.stringMin {
		st.stringMin = l
	}
	if l > st.stringMax {
		st.stringMax = l
	}
	st.trackStringUnique(v)
}

func (st *nodeStats) observeNumber(v float64) {
	if !st.numberSeen {
		st.numberSeen = true
		st.numberMin = v
		st.numberMax = v
		st.trackNumberUnique(v)
		return
	}
	if v < st.numberMin {
		st.numberMin = v
	}
	if v > st.numberMax {
		st.numberMax = v
	}
	st.trackNumberUnique(v)
}

func (st *nodeStats) observeArray(v []any) {
	l := len(v)
	uniqCount := arrayUniqueCount(v)
	if !st.arraySeen {
		st.arraySeen = true
		st.arrayMin = l
		st.arrayMax = l
		st.arrayUnique = uniqCount == l
		st.arrayUniqMin = uniqCount
		st.arrayUniqMax = uniqCount
		return
	}
	if l < st.arrayMin {
		st.arrayMin = l
	}
	if l > st.arrayMax {
		st.arrayMax = l
	}
	if st.arrayUnique && uniqCount != l {
		st.arrayUnique = false
	}
	if uniqCount < st.arrayUniqMin {
		st.arrayUniqMin = uniqCount
	}
	if uniqCount > st.arrayUniqMax {
		st.arrayUniqMax = uniqCount
	}
}

func arrayUniqueCount(v []any) int {
	seen := make(map[string]struct{}, len(v))
	for _, item := range v {
		key, err := json.Marshal(item)
		if err != nil {
			continue
		}
		k := string(key)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
	}
	return len(seen)
}

const distinctCap = 100

func (st *nodeStats) trackStringUnique(v string) {
	if st.stringUniqueCap {
		return
	}
	if st.stringUnique == nil {
		st.stringUnique = make(map[string]struct{})
	}
	if _, ok := st.stringUnique[v]; ok {
		return
	}
	if len(st.stringUnique) >= distinctCap {
		st.stringUniqueCap = true
		st.stringUnique = nil
		return
	}
	st.stringUnique[v] = struct{}{}
	st.stringUniqueCount++
}

func (st *nodeStats) trackNumberUnique(v float64) {
	if st.numberUniqueCap {
		return
	}
	if st.numberUnique == nil {
		st.numberUnique = make(map[string]struct{})
	}
	key := formatNumber(v)
	if _, ok := st.numberUnique[key]; ok {
		return
	}
	if len(st.numberUnique) >= distinctCap {
		st.numberUniqueCap = true
		st.numberUnique = nil
		return
	}
	st.numberUnique[key] = struct{}{}
	st.numberUniqueCount++
}

type applyState struct {
	legendAdded bool
}

func applyStats(node any, path []string, stats *statsCollector, opts Options, state *applyState) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}

	st := stats.nodeByPath(path)
	if st != nil && opts.AddDescriptions {
		if !state.legendAdded && schemaType(m) == "array" && st.arraySeen && st.arrayMin >= 3 {
			if _, exists := m["description"]; !exists {
				m["description"] = descriptionLegend()
				state.legendAdded = true
			}
		}
	}

	if st != nil && opts.AddDescriptions && isArrayContext(path) && arrayEligibleFor(path, stats) {
		if _, exists := m["description"]; !exists {
			if desc := buildDescription(m, st); desc != "" {
				m["description"] = desc
			}
		}
	}

	if props, ok := m["properties"].(map[string]any); ok {
		for k, v := range props {
			applyStats(v, append(path, k), stats, opts, state)
		}
	}
	if items, ok := m["items"]; ok {
		switch it := items.(type) {
		case map[string]any:
			applyStats(it, append(path, "*"), stats, opts, state)
		case []any:
			for _, elem := range it {
				applyStats(elem, append(path, "*"), stats, opts, state)
			}
		}
	}
}

func buildDescription(m map[string]any, st *nodeStats) string {
	typ := schemaType(m)
	switch typ {
	case "string":
		if st.stringSeen {
			return formatShortDesc("str", st.stringMin, st.stringMax, st.stringUniqueCount, st.stringUniqueCap, st.stringEmptyCount, st.nullCount, st.totalCount)
		}
	case "integer":
		if st.numberSeen {
			min := math.Trunc(st.numberMin)
			max := math.Trunc(st.numberMax)
			return formatShortDesc("int", int(min), int(max), st.numberUniqueCount, st.numberUniqueCap, 0, st.nullCount, st.totalCount)
		}
	case "number":
		if st.numberSeen {
			return formatShortDescNumber("num", st.numberMin, st.numberMax, st.numberUniqueCount, st.numberUniqueCap, st.nullCount, st.totalCount)
		}
	case "array":
		if st.arraySeen {
			return formatArrayDesc(st)
		}
	}
	return ""
}

func schemaType(m map[string]any) string {
	switch t := m["type"].(type) {
	case string:
		return t
	case []any:
		if len(t) == 1 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func pathKey(path []string) string {
	if len(path) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range path {
		b.WriteByte('/')
		b.WriteString(escapeToken(p))
	}
	return b.String()
}

func escapeToken(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}

func formatNumber(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%g", v)
}

func addOptional(node any, path []string, stats *statsCollector) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}

	if schemaType(m) == "array" {
		if items, ok := m["items"].(map[string]any); ok && schemaType(items) == "object" {
			if opt := optionalFields(path, stats); len(opt) > 0 {
				m["optional"] = opt
			} else {
				delete(m, "optional")
			}
		}
	}

	if props, ok := m["properties"].(map[string]any); ok {
		for k, v := range props {
			addOptional(v, append(path, k), stats)
		}
	}
	if items, ok := m["items"]; ok {
		switch it := items.(type) {
		case map[string]any:
			addOptional(it, append(path, "*"), stats)
		case []any:
			for _, elem := range it {
				addOptional(elem, append(path, "*"), stats)
			}
		}
	}
}

func optionalFields(path []string, stats *statsCollector) []string {
	st := stats.arrayByPath(path)
	if st == nil || st.total == 0 {
		return nil
	}
	out := make([]string, 0)
	for k, count := range st.fieldCounts {
		if count < st.total {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func isArrayContext(path []string) bool {
	for _, p := range path {
		if p == "*" {
			return true
		}
	}
	return false
}

func arrayEligibleFor(path []string, stats *statsCollector) bool {
	idx := lastIndex(path, "*")
	if idx == -1 {
		return false
	}
	parent := path[:idx]
	st := stats.nodeByPath(parent)
	if st == nil || !st.arraySeen {
		return false
	}
	return st.arrayMin >= 3
}

func lastIndex(path []string, token string) int {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == token {
			return i
		}
	}
	return -1
}

func descriptionLegend() string {
	return "Legend: x..y=min..max; str len=length; int/num=range; arr=size; uniq=all all-unique; uniq=a..b distinct range; uniq=n distinct count; eN empty strings; nN/T nulls/total"
}

func formatShortDesc(prefix string, min, max int, uniq int, uniqCap bool, empty int, nulls int, total int) string {
	desc := fmt.Sprintf("%s %d..%d", prefix, min, max)
	desc = appendShortStats(desc, uniq, uniqCap, empty, nulls, total)
	return desc
}

func formatShortDescNumber(prefix string, min, max float64, uniq int, uniqCap bool, nulls int, total int) string {
	desc := fmt.Sprintf("%s %s..%s", prefix, formatNumber(min), formatNumber(max))
	desc = appendShortStats(desc, uniq, uniqCap, 0, nulls, total)
	return desc
}

func formatArrayDesc(st *nodeStats) string {
	desc := fmt.Sprintf("arr %d..%d", st.arrayMin, st.arrayMax)
	if st.arrayUniqMin > 0 || st.arrayUniqMax > 0 {
		desc = desc + fmt.Sprintf(" uniq=%d..%d", st.arrayUniqMin, st.arrayUniqMax)
	}
	if st.arrayUnique {
		desc = desc + " uniq=all"
	}
	desc = appendShortStats(desc, 0, false, 0, st.nullCount, st.totalCount)
	return desc
}

func appendShortStats(desc string, uniq int, uniqCap bool, empty int, nulls int, total int) string {
	if uniq > 0 {
		if uniqCap {
			desc = desc + fmt.Sprintf(" uniq>=%d", uniq)
		} else {
			desc = desc + fmt.Sprintf(" uniq=%d", uniq)
		}
	}
	if empty > 0 {
		desc = desc + fmt.Sprintf(" e%d", empty)
	}
	if total > 0 && nulls > 0 {
		desc = desc + fmt.Sprintf(" n%d/%d", nulls, total)
	}
	return desc
}
