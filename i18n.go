package main

import (
	"embed"
	"encoding/json"
	"os"
	"strings"

	"github.com/Xuanwo/go-locale"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localeFS embed.FS

// 支持的语言列表（按优先级分组）
var supportedLangs = []string{
	// Tier 1: 核心与高频技术语言
	"en", "zh", "ja", "ko", "es", "fr", "de", "ru", "pt-BR",
	// Tier 2: 活跃开发者市场
	"pl", "tr", "vi", "id", "it", "nl", "uk", "th", "pt",
	// Tier 3: 欧洲与高净值市场
	"sv", "da", "fi", "no", "cs", "hu", "el", "ro", "ca",
	// Tier 4: 潜力与中亚/中东市场
	"ar", "he", "fa", "kk", "hi", "bn", "bg", "sk", "sl", "lt", "lv", "et",
}

// 语言代码映射（处理变体）
var langMappings = map[string]string{
	// 中文变体
	"zh-CN": "zh",
	"zh-Hans": "zh",
	"zh-TW": "zh",
	"zh-Hant": "zh",
	"zh-HK": "zh",
	"zh-MO": "zh",
	"zh-SG": "zh",
	// 葡萄牙语变体
	"pt-PT": "pt",
}

var (
	// 全局 i18n bundle 和 localizer
	i18nBundle    *i18n.Bundle
	i18nLocalizer *i18n.Localizer
	currentLang   string
)

// initI18n 初始化国际化支持
func initI18n() {
	// 创建 bundle，默认英语
	i18nBundle = i18n.NewBundle(language.English)
	i18nBundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	// 加载所有语言文件
	for _, lang := range supportedLangs {
		filename := "locales/" + lang + ".json"
		if data, err := localeFS.ReadFile(filename); err == nil {
			i18nBundle.MustParseMessageFileBytes(data, filename)
		}
	}

	// 检测系统语言
	currentLang = detectSystemLang()

	// 创建 localizer（按优先级：当前语言 -> 英语）
	i18nLocalizer = i18n.NewLocalizer(i18nBundle, currentLang, "en")
}

// detectSystemLang 检测系统语言
// 优先级: LANG 环境变量 > go-locale 自动检测 > 默认英语
func detectSystemLang() string {
	// 优先检查 LANG 环境变量
	if envLang := os.Getenv("LANG"); envLang != "" {
		// 处理类似 zh_CN.UTF-8 的格式
		lang := strings.Split(envLang, ".")[0]
		lang = strings.Split(lang, "@")[0]
		lang = strings.ReplaceAll(lang, "_", "-")
		
		normalized := normalizeLangCode(lang)
		if normalized != "en" || strings.HasPrefix(strings.ToLower(lang), "en") {
			return normalized
		}
	}

	// 使用 go-locale 自动检测
	tag, err := locale.Detect()
	if err != nil {
		return "en" // 检测失败默认英语
	}

	return normalizeLangCode(tag.String())
}

// normalizeLangCode 标准化语言代码
func normalizeLangCode(rawLang string) string {
	rawLang = strings.ToLower(rawLang)

	// 检查映射表（处理带区域的代码如 zh-CN, pt-BR）
	if mapped, ok := langMappings[strings.ToLower(rawLang)]; ok {
		rawLang = mapped
	}

	// 提取基础语言代码（去掉区域后缀）
	parts := strings.Split(rawLang, "-")
	baseLang := parts[0]

	// 特殊处理巴西葡萄牙语
	if len(parts) >= 2 && baseLang == "pt" && strings.ToLower(parts[1]) == "br" {
		baseLang = "pt-BR"
	}

	// 检查是否在支持列表中
	for _, lang := range supportedLangs {
		if baseLang == lang {
			return baseLang
		}
	}

	return "en" // 默认英语
}

// T 翻译字符串
func T(key string, data ...map[string]interface{}) string {
	if i18nLocalizer == nil {
		initI18n()
	}

	config := &i18n.LocalizeConfig{
		MessageID: key,
	}
	if len(data) > 0 {
		config.TemplateData = data[0]
	}

	msg, err := i18nLocalizer.Localize(config)
	if err != nil {
		return key // 翻译失败返回 key
	}
	return msg
}

// GetLang 获取当前语言
func GetLang() string {
	if currentLang == "" {
		initI18n()
	}
	return currentLang
}

// SetLang 设置语言（用于测试或强制指定）
func SetLang(lang string) {
	currentLang = normalizeLangCode(lang)
	if i18nBundle != nil {
		i18nLocalizer = i18n.NewLocalizer(i18nBundle, currentLang, "en")
	}
}

// GetSupportedLangs 获取支持的语言列表
func GetSupportedLangs() []string {
	return supportedLangs
}
