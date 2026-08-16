package notify

import (
	"regexp"
	"strings"

	"github.com/boa-z/vohive/internal/db"
	"github.com/boa-z/vohive/pkg/logger"
)

// 匹配方式枚举，对应前端「匹配方式」下拉框。
const (
	MatchAll         = "all"          // 全部匹配
	MatchContains    = "contains"     // 包含
	MatchNotContains = "not_contains" // 不包含
	MatchEquals      = "equals"       // 等于
	MatchRegex       = "regex"        // 正则
)

// SendBodyModePlain 与 SendBodyModeCustomJSON 对应前端「纯文本/自定义请求体」两个 Tab。
const (
	SendBodyModePlain      = "plain"
	SendBodyModeCustomJSON = "custom_json"
)

// evaluateSMSRules 加载已启用的 sms 规则（按 priority 排序），返回第一条匹配的规则；
// 无匹配返回 (nil, nil)。首条命中即停止（first-match-wins）。
func evaluateSMSRules(ctx NotificationContext) (*db.NotifyRule, error) {
	rules, err := db.ListEnabledNotifyRules("sms")
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if matchesRule(rules[i], ctx) {
			return &rules[i], nil
		}
	}
	return nil, nil
}

// extractMatchValue 根据规则的 match_field 取出用于比较的原始值。
func extractMatchValue(rule db.NotifyRule, ctx NotificationContext) string {
	switch rule.MatchField {
	case "content":
		if ctx.SMS != nil {
			return ctx.SMS.Content
		}
	case "sender":
		if ctx.SMS != nil {
			return ctx.SMS.Sender
		}
	}
	return ctx.Text
}

func matchesRule(rule db.NotifyRule, ctx NotificationContext) bool {
	method := strings.TrimSpace(rule.MatchMethod)
	if method == "" || method == MatchAll {
		return true
	}
	value := extractMatchValue(rule, ctx)
	pattern := rule.MatchContent

	switch method {
	case MatchContains:
		return strings.Contains(value, pattern)
	case MatchNotContains:
		return !strings.Contains(value, pattern)
	case MatchEquals:
		return value == pattern
	case MatchRegex:
		re, err := regexp.Compile(pattern)
		if err != nil {
			logger.Warn("转发规则正则表达式编译失败", "rule", rule.Name, "pattern", pattern, "err", err)
			return false
		}
		return re.MatchString(value)
	default:
		return true
	}
}

// buildSMSTemplateValues 组装 SMS 场景下模板占位符可用的全部变量。
// 注意：没有验证码(OTP)提取能力，因此不提供 "code" 占位符，避免渲染出恒为空的误导值。
func buildSMSTemplateValues(ctx NotificationContext) map[string]string {
	values := map[string]string{
		"event":        strings.TrimSpace(ctx.Event),
		"timestamp":    ctx.Timestamp.Format("2006-01-02 15:04:05"),
		"device_id":    strings.TrimSpace(ctx.DeviceID),
		"device_name":  strings.TrimSpace(ctx.DeviceName),
		"device_label": ctx.DeviceLabel(),
		"text":         strings.TrimSpace(ctx.Text),
	}
	if ctx.SMS != nil {
		values["sender"] = strings.TrimSpace(ctx.SMS.Sender)
		values["content"] = strings.TrimSpace(ctx.SMS.Content)
		values["source"] = strings.TrimSpace(ctx.SMS.Source)
		values["local_phone"] = strings.TrimSpace(ctx.SMS.LocalPhone)
		values["operator"] = strings.TrimSpace(ctx.SMS.Operator)
	}
	return values
}
