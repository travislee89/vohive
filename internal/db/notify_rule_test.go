package db

import (
	"path/filepath"
	"testing"
)

func initNotifyTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "notify.db")
	if err := Init(dbPath); err != nil {
		t.Fatalf("Init() error=%v", err)
	}
	t.Cleanup(func() { DB = nil })
}

func TestUpsertNotifyRuleGeneratesIDAndRoundTrips(t *testing.T) {
	initNotifyTestDB(t)

	rule := NotifyRule{
		MessageType: "sms",
		Name:        "test rule",
		Enabled:     true,
		MatchField:  "content",
		MatchMethod: "contains",
	}
	rule.SetChannels([]string{"telegram", "webhook"})

	saved, err := UpsertNotifyRule(rule)
	if err != nil {
		t.Fatalf("UpsertNotifyRule() error=%v", err)
	}
	if saved.ID == "" {
		t.Fatal("expected generated ID")
	}

	got, err := GetNotifyRule(saved.ID)
	if err != nil {
		t.Fatalf("GetNotifyRule() error=%v", err)
	}
	if got == nil {
		t.Fatal("GetNotifyRule() = nil")
	}
	if got.Name != "test rule" {
		t.Fatalf("Name=%q, want %q", got.Name, "test rule")
	}
	if channels := got.Channels(); len(channels) != 2 || channels[0] != "telegram" || channels[1] != "webhook" {
		t.Fatalf("Channels()=%v, want [telegram webhook]", channels)
	}
}

func TestDeleteNotifyRule(t *testing.T) {
	initNotifyTestDB(t)

	rule := NotifyRule{MessageType: "sms", Name: "to-delete", MatchMethod: "all"}
	saved, err := UpsertNotifyRule(rule)
	if err != nil {
		t.Fatalf("UpsertNotifyRule() error=%v", err)
	}
	if err := DeleteNotifyRule(saved.ID); err != nil {
		t.Fatalf("DeleteNotifyRule() error=%v", err)
	}
	got, err := GetNotifyRule(saved.ID)
	if err != nil {
		t.Fatalf("GetNotifyRule() error=%v", err)
	}
	if got != nil {
		t.Fatalf("GetNotifyRule() after delete = %v, want nil", got)
	}
}

func TestCountNotifyRulesByType(t *testing.T) {
	initNotifyTestDB(t)

	for i, enabled := range []bool{true, true, false} {
		rule := NotifyRule{MessageType: "sms", Name: "r", Enabled: enabled, MatchMethod: "all", Priority: i}
		if _, err := UpsertNotifyRule(rule); err != nil {
			t.Fatalf("UpsertNotifyRule() error=%v", err)
		}
	}
	enabled, total, err := CountNotifyRulesByType("sms")
	if err != nil {
		t.Fatalf("CountNotifyRulesByType() error=%v", err)
	}
	if enabled != 2 || total != 3 {
		t.Fatalf("enabled=%d total=%d, want 2 3", enabled, total)
	}
}
