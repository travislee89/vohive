package notify

import (
	"testing"
	"time"

	"github.com/boa-z/vohive/internal/db"
)

func TestBroadcastSMSWithRulesWritesUnmatchedLog(t *testing.T) {
	initNotifyRuleTestDB(t)
	capture := &captureChannel{}
	m := &Manager{channels: []Channel{capture}}

	m.NotifySMS("wwan0", "+8613800000000", "hello", time.Now())

	// 未匹配到任何规则：不应转发到渠道，但应写一条 unmatched 日志。
	time.Sleep(50 * time.Millisecond)
	if got := capture.Last(); got != "" {
		t.Fatalf("expected no forward, got %q", got)
	}

	var logs []db.NotifyLog
	if err := db.DB.Find(&logs).Error; err != nil {
		t.Fatalf("query notify_logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("notify_logs count=%d, want 1", len(logs))
	}
	if logs[0].Status != db.NotifyLogStatusUnmatched {
		t.Fatalf("status=%q, want unmatched", logs[0].Status)
	}
	if logs[0].Channel != "" {
		t.Fatalf("channel=%q, want empty", logs[0].Channel)
	}
}

func TestBroadcastSMSWithRulesWritesSuccessLogPerChannel(t *testing.T) {
	initNotifyRuleTestDB(t)
	seedCatchAllSMSRule(t, "capture")
	capture := &captureChannel{}
	m := &Manager{channels: []Channel{capture}}

	m.NotifySMS("wwan0", "+8613800000000", "hello", time.Now())

	waitUntil(t, time.Second, func() bool { return capture.Last() != "" })

	var logs []db.NotifyLog
	if err := db.DB.Find(&logs).Error; err != nil {
		t.Fatalf("query notify_logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("notify_logs count=%d, want 1", len(logs))
	}
	if logs[0].Status != db.NotifyLogStatusSuccess {
		t.Fatalf("status=%q, want success", logs[0].Status)
	}
	if logs[0].Channel != "capture" {
		t.Fatalf("channel=%q, want capture", logs[0].Channel)
	}
	if logs[0].MatchedRuleName != "test-catch-all" {
		t.Fatalf("matched_rule_name=%q, want test-catch-all", logs[0].MatchedRuleName)
	}
}

func TestSeedDefaultSMSRuleIfEmptySeedsOnce(t *testing.T) {
	initNotifyRuleTestDB(t)

	if err := db.SeedDefaultSMSRuleIfEmpty([]string{"telegram", "webhook"}); err != nil {
		t.Fatalf("SeedDefaultSMSRuleIfEmpty() error=%v", err)
	}
	rules, err := db.ListNotifyRules("sms")
	if err != nil {
		t.Fatalf("ListNotifyRules() error=%v", err)
	}
	if len(rules) != 1 || !rules[0].IsDefault {
		t.Fatalf("rules=%v, want exactly one default rule", rules)
	}
	if got := rules[0].Channels(); len(got) != 2 {
		t.Fatalf("seeded channels=%v, want [telegram webhook]", got)
	}

	// 用户主动删除默认规则后，再次调用不应把它种回来。
	if err := db.DeleteNotifyRule(rules[0].ID); err != nil {
		t.Fatalf("DeleteNotifyRule() error=%v", err)
	}
	if err := db.SeedDefaultSMSRuleIfEmpty([]string{"telegram"}); err != nil {
		t.Fatalf("SeedDefaultSMSRuleIfEmpty() (second call) error=%v", err)
	}
	rules, err = db.ListNotifyRules("sms")
	if err != nil {
		t.Fatalf("ListNotifyRules() error=%v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("rules=%v, want none (deleted default should not be reseeded)", rules)
	}
}
