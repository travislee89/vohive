package db

import (
	"testing"
	"time"
)

func TestMarkSMSReadDecrementsContactUnread(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	first, err := SaveSMS("imsi-1", "+8613800000000", "", "hello", 1, 0, base)
	if err != nil {
		t.Fatalf("SaveSMS(first) error=%v", err)
	}
	second, err := SaveSMS("imsi-1", "+8613800000000", "", "again", 1, 0, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("SaveSMS(second) error=%v", err)
	}
	if got := unreadFor(t, "imsi-1", "+8613800000000"); got != 2 {
		t.Fatalf("unread before=%d want=2", got)
	}

	marked, err := MarkSMSRead(first.ID)
	if err != nil || !marked {
		t.Fatalf("MarkSMSRead(first) = %v, %v; want true, nil", marked, err)
	}
	if got := unreadFor(t, "imsi-1", "+8613800000000"); got != 1 {
		t.Fatalf("unread after first=%d want=1", got)
	}

	marked, err = MarkSMSRead(first.ID)
	if err != nil || marked {
		t.Fatalf("MarkSMSRead(already read) = %v, %v; want false, nil", marked, err)
	}
	if got := unreadFor(t, "imsi-1", "+8613800000000"); got != 1 {
		t.Fatalf("unread after duplicate=%d want=1", got)
	}

	if _, err := MarkSMSRead(second.ID); err != nil {
		t.Fatalf("MarkSMSRead(second) error=%v", err)
	}
	if got := unreadFor(t, "imsi-1", "+8613800000000"); got != 0 {
		t.Fatalf("unread final=%d want=0", got)
	}
}

func TestMarkSMSReadIgnoresOutgoingAndMissing(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	sent, err := SaveSMS("imsi-2", "", "+8613900000000", "out", 2, 2, base)
	if err != nil {
		t.Fatalf("SaveSMS(sent) error=%v", err)
	}
	if marked, err := MarkSMSRead(sent.ID); err != nil || marked {
		t.Fatalf("MarkSMSRead(outgoing) = %v, %v; want false, nil", marked, err)
	}
	if marked, err := MarkSMSRead(999999); err != nil || marked {
		t.Fatalf("MarkSMSRead(missing) = %v, %v; want false, nil", marked, err)
	}
}

func unreadFor(t *testing.T, imsi, peer string) int {
	t.Helper()
	var contact SMSContact
	if err := DB.Where("imsi = ? AND peer = ?", imsi, peer).First(&contact).Error; err != nil {
		t.Fatalf("load contact error=%v", err)
	}
	return contact.UnreadCount
}
