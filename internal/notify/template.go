package notify

import "regexp"

// placeholderPattern 匹配 "{{key}}" 形式的模板占位符，被 Webhook 文本模板与转发规则模板共用。
var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// RenderPlaceholders 将 tmpl 中形如 "{{key}}" 的占位符替换为 values[key]；
// 未知占位符原样保留，便于排查模板拼写错误。
func RenderPlaceholders(tmpl string, values map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(tmpl, func(token string) string {
		matches := placeholderPattern.FindStringSubmatch(token)
		if len(matches) != 2 {
			return token
		}
		if v, ok := values[matches[1]]; ok {
			return v
		}
		return token
	})
}
