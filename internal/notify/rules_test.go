package notify

import (
	"testing"
	"time"

	"github.com/boa-z/vohive/internal/db"
)

func TestRenderPlaceholders(t *testing.T) {
	got := RenderPlaceholders("[{{device_label}}] {{content}} ({{unknown}})", map[string]string{
		"device_label": "wwan0",
		"content":      "hello",
	})
	want := "[wwan0] hello ({{unknown}})"
	if got != want {
		t.Fatalf("RenderPlaceholders() = %q, want %q", got, want)
	}
}

func TestMatchesRule(t *testing.T) {
	ctx := NotificationContext{
		Text: "收到新短信 / 蜂窝\n内容  验证码 123456",
		SMS: &SMSContext{
			Sender:  "+8613800000000",
			Content: "您的验证码是 123456，5 分钟内有效",
		},
	}

	cases := []struct {
		name string
		rule db.NotifyRule
		want bool
	}{
		{"all matches always", db.NotifyRule{MatchMethod: MatchAll}, true},
		{"empty method matches always", db.NotifyRule{}, true},
		{"contains hit", db.NotifyRule{MatchField: "content", MatchMethod: MatchContains, MatchContent: "验证码"}, true},
		{"contains miss", db.NotifyRule{MatchField: "content", MatchMethod: MatchContains, MatchContent: "订单"}, false},
		{"not_contains hit", db.NotifyRule{MatchField: "content", MatchMethod: MatchNotContains, MatchContent: "订单"}, true},
		{"not_contains miss", db.NotifyRule{MatchField: "content", MatchMethod: MatchNotContains, MatchContent: "验证码"}, false},
		{"equals hit on sender", db.NotifyRule{MatchField: "sender", MatchMethod: MatchEquals, MatchContent: "+8613800000000"}, true},
		{"equals miss on sender", db.NotifyRule{MatchField: "sender", MatchMethod: MatchEquals, MatchContent: "+8613900000000"}, false},
		{"regex hit", db.NotifyRule{MatchField: "content", MatchMethod: MatchRegex, MatchContent: `\d{6}`}, true},
		{"regex miss", db.NotifyRule{MatchField: "content", MatchMethod: MatchRegex, MatchContent: `^订单`}, false},
		{"regex invalid pattern fails closed", db.NotifyRule{MatchField: "content", MatchMethod: MatchRegex, MatchContent: `(`}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesRule(tc.rule, ctx); got != tc.want {
				t.Fatalf("matchesRule() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluateSMSRulesFirstMatchWins(t *testing.T) {
	initNotifyRuleTestDB(t)

	specific := db.NotifyRule{
		MessageType:  "sms",
		Name:         "specific",
		Enabled:      true,
		Priority:     0,
		MatchField:   "content",
		MatchMethod:  MatchContains,
		MatchContent: "验证码",
	}
	specific.SetChannels([]string{"telegram"})
	saved, err := db.UpsertNotifyRule(specific)
	if err != nil {
		t.Fatalf("UpsertNotifyRule(specific) error=%v", err)
	}
	specific = saved

	fallback := db.NotifyRule{
		MessageType: "sms",
		Name:        "fallback",
		Enabled:     true,
		Priority:    1,
		MatchField:  "any",
		MatchMethod: MatchAll,
	}
	fallback.SetChannels([]string{"webhook"})
	if _, err := db.UpsertNotifyRule(fallback); err != nil {
		t.Fatalf("UpsertNotifyRule(fallback) error=%v", err)
	}

	ctx := NotificationContext{
		Event:     "sms_received",
		Text:      "收到新短信",
		Timestamp: time.Now(),
		SMS:       &SMSContext{Content: "您的验证码是 123456"},
	}
	rule, err := evaluateSMSRules(ctx)
	if err != nil {
		t.Fatalf("evaluateSMSRules() error=%v", err)
	}
	if rule == nil || rule.Name != "specific" {
		t.Fatalf("evaluateSMSRules() matched=%v, want specific", rule)
	}

	// 禁用/删除后应回退到下一条按 priority 排列的规则。
	specific.Enabled = false
	if _, err := db.UpsertNotifyRule(specific); err != nil {
		t.Fatalf("disable specific rule: %v", err)
	}
	rule, err = evaluateSMSRules(ctx)
	if err != nil {
		t.Fatalf("evaluateSMSRules() error=%v", err)
	}
	if rule == nil || rule.Name != "fallback" {
		t.Fatalf("evaluateSMSRules() after disable = %v, want fallback", rule)
	}
}

func TestEvaluateSMSRulesNoMatch(t *testing.T) {
	initNotifyRuleTestDB(t)

	rule := db.NotifyRule{
		MessageType:  "sms",
		Name:         "orders-only",
		Enabled:      true,
		MatchField:   "content",
		MatchMethod:  MatchContains,
		MatchContent: "订单",
	}
	rule.SetChannels([]string{"telegram"})
	if _, err := db.UpsertNotifyRule(rule); err != nil {
		t.Fatalf("UpsertNotifyRule() error=%v", err)
	}

	ctx := NotificationContext{
		Event: "sms_received",
		Text:  "收到新短信",
		SMS:   &SMSContext{Content: "您的验证码是 123456"},
	}
	got, err := evaluateSMSRules(ctx)
	if err != nil {
		t.Fatalf("evaluateSMSRules() error=%v", err)
	}
	if got != nil {
		t.Fatalf("evaluateSMSRules() = %v, want nil", got)
	}
}
