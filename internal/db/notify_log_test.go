package db

import (
	"testing"
	"time"
)

func TestListNotifyLogsFiltersAndPaginates(t *testing.T) {
	initNotifyTestDB(t)

	base := time.Now().Add(-time.Hour)
	for i := 0; i < 3; i++ {
		if err := CreateNotifyLog(NotifyLog{
			MessageType:    "sms",
			Status:         NotifyLogStatusSuccess,
			ContentSummary: "hello",
			Channel:        "telegram",
			Timestamp:      base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("CreateNotifyLog() error=%v", err)
		}
	}
	if err := CreateNotifyLog(NotifyLog{
		MessageType:    "sms",
		Status:         NotifyLogStatusUnmatched,
		ContentSummary: "no match",
		Timestamp:      base,
	}); err != nil {
		t.Fatalf("CreateNotifyLog() error=%v", err)
	}

	logs, total, err := ListNotifyLogs(1, 2, "sms", NotifyLogStatusSuccess, "", nil, nil)
	if err != nil {
		t.Fatalf("ListNotifyLogs() error=%v", err)
	}
	if total != 3 {
		t.Fatalf("total=%d, want 3", total)
	}
	if len(logs) != 2 {
		t.Fatalf("page size=%d, want 2", len(logs))
	}

	logs, total, err = ListNotifyLogs(1, 20, "sms", NotifyLogStatusUnmatched, "", nil, nil)
	if err != nil {
		t.Fatalf("ListNotifyLogs() error=%v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("unmatched filter total=%d len=%d, want 1 1", total, len(logs))
	}
}

func TestDeleteNotifyLogsBefore(t *testing.T) {
	initNotifyTestDB(t)

	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-time.Minute)
	if err := CreateNotifyLog(NotifyLog{MessageType: "sms", Status: NotifyLogStatusSuccess, Timestamp: old}); err != nil {
		t.Fatalf("CreateNotifyLog() error=%v", err)
	}
	if err := CreateNotifyLog(NotifyLog{MessageType: "sms", Status: NotifyLogStatusSuccess, Timestamp: recent}); err != nil {
		t.Fatalf("CreateNotifyLog() error=%v", err)
	}

	if err := DeleteNotifyLogsBefore(time.Now().Add(-24 * time.Hour)); err != nil {
		t.Fatalf("DeleteNotifyLogsBefore() error=%v", err)
	}

	_, total, err := ListNotifyLogs(1, 20, "", "", "", nil, nil)
	if err != nil {
		t.Fatalf("ListNotifyLogs() error=%v", err)
	}
	if total != 1 {
		t.Fatalf("total after cleanup=%d, want 1", total)
	}
}

func TestSeedDefaultSMSRuleIfEmptyPersistsFlag(t *testing.T) {
	initNotifyTestDB(t)

	if err := SeedDefaultSMSRuleIfEmpty([]string{"telegram"}); err != nil {
		t.Fatalf("SeedDefaultSMSRuleIfEmpty() error=%v", err)
	}
	settings, err := GetNotifySettings()
	if err != nil {
		t.Fatalf("GetNotifySettings() error=%v", err)
	}
	if !settings.DefaultRuleSeeded {
		t.Fatal("expected DefaultRuleSeeded=true after seeding")
	}
}
