package db

import (
	"path/filepath"
	"testing"
	"time"
)

func initCallLogTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "call_log.db")
	if err := Init(dbPath); err != nil {
		t.Fatalf("Init() error=%v", err)
	}
	t.Cleanup(func() { DB = nil })
}

func TestCreateRingingCallLogUnreadFlag(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	in, err := CreateRingingCallLog("dev-1", "iccid-1", "+8613800000000", CallDirectionIn, base)
	if err != nil {
		t.Fatalf("CreateRingingCallLog(in) error=%v", err)
	}
	if !in.Unread {
		t.Fatal("inbound call log Unread=false want=true")
	}

	out, err := CreateRingingCallLog("dev-1", "iccid-1", "+8613900000000", CallDirectionOut, base)
	if err != nil {
		t.Fatalf("CreateRingingCallLog(out) error=%v", err)
	}
	if out.Unread {
		t.Fatal("outbound call log Unread=true want=false")
	}
}

func TestMarkCallLogConnectedThenEndedIsAnswered(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	log, err := CreateRingingCallLog("dev-1", "iccid-1", "+8613800000000", CallDirectionIn, base)
	if err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if err := MarkCallLogConnected(log.ID, base.Add(2*time.Second)); err != nil {
		t.Fatalf("MarkCallLogConnected() error=%v", err)
	}
	if err := MarkCallLogEnded(log.ID, base.Add(30*time.Second)); err != nil {
		t.Fatalf("MarkCallLogEnded() error=%v", err)
	}

	var got CallLog
	if err := DB.First(&got, log.ID).Error; err != nil {
		t.Fatalf("First() error=%v", err)
	}
	if got.Outcome != CallOutcomeAnswered {
		t.Fatalf("Outcome=%q want=%q", got.Outcome, CallOutcomeAnswered)
	}
	if got.AnsweredAt == nil || got.EndedAt == nil {
		t.Fatal("AnsweredAt/EndedAt should be set")
	}
}

func TestMarkCallLogEndedWithoutConnectIsMissed(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	log, err := CreateRingingCallLog("dev-1", "iccid-1", "+8613800000000", CallDirectionIn, base)
	if err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if err := MarkCallLogEnded(log.ID, base.Add(10*time.Second)); err != nil {
		t.Fatalf("MarkCallLogEnded() error=%v", err)
	}

	var got CallLog
	if err := DB.First(&got, log.ID).Error; err != nil {
		t.Fatalf("First() error=%v", err)
	}
	if got.Outcome != CallOutcomeMissed {
		t.Fatalf("Outcome=%q want=%q", got.Outcome, CallOutcomeMissed)
	}
}

func TestMarkCallLogEndedOutboundNeverConnectedIsAnswered(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	log, err := CreateRingingCallLog("dev-1", "iccid-1", "+8613800000000", CallDirectionOut, base)
	if err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if err := MarkCallLogEnded(log.ID, base.Add(10*time.Second)); err != nil {
		t.Fatalf("MarkCallLogEnded() error=%v", err)
	}

	var got CallLog
	if err := DB.First(&got, log.ID).Error; err != nil {
		t.Fatalf("First() error=%v", err)
	}
	if got.Outcome != CallOutcomeAnswered {
		t.Fatalf("Outcome=%q want=%q (unanswered outbound calls should not be reported as missed)", got.Outcome, CallOutcomeAnswered)
	}
}

func TestCountAndListUnreadCalls(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	if _, err := CreateRingingCallLog("dev-1", "iccid-1", "+861111", CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if _, err := CreateRingingCallLog("dev-1", "iccid-1", "+862222", CallDirectionIn, base.Add(time.Minute)); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if _, err := CreateRingingCallLog("dev-1", "iccid-1", "+863333", CallDirectionOut, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}

	count, err := CountUnreadCalls()
	if err != nil {
		t.Fatalf("CountUnreadCalls() error=%v", err)
	}
	if count != 2 {
		t.Fatalf("CountUnreadCalls()=%d want=2", count)
	}

	list, err := ListRecentUnreadCalls(10)
	if err != nil {
		t.Fatalf("ListRecentUnreadCalls() error=%v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list)=%d want=2", len(list))
	}
	if list[0].Number != "+862222" {
		t.Fatalf("list[0].Number=%q want=+862222 (expected desc by started_at)", list[0].Number)
	}
}

func TestMarkCallLogsReadAndMarkAllForDevice(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	a, err := CreateRingingCallLog("dev-1", "iccid-1", "+861111", CallDirectionIn, base)
	if err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	b, err := CreateRingingCallLog("dev-1", "iccid-1", "+862222", CallDirectionIn, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	c, err := CreateRingingCallLog("dev-2", "iccid-2", "+863333", CallDirectionIn, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}

	affected, err := MarkCallLogsRead([]uint{a.ID})
	if err != nil {
		t.Fatalf("MarkCallLogsRead() error=%v", err)
	}
	if affected != 1 {
		t.Fatalf("affected=%d want=1", affected)
	}

	affected, err = MarkAllCallLogsReadForDevice("dev-1")
	if err != nil {
		t.Fatalf("MarkAllCallLogsReadForDevice() error=%v", err)
	}
	if affected != 1 {
		t.Fatalf("affected=%d want=1 (only b should remain unread on dev-1)", affected)
	}

	count, err := CountUnreadCalls()
	if err != nil {
		t.Fatalf("CountUnreadCalls() error=%v", err)
	}
	if count != 1 {
		t.Fatalf("CountUnreadCalls()=%d want=1 (only dev-2's call c remains unread)", count)
	}

	var gotC CallLog
	if err := DB.First(&gotC, c.ID).Error; err != nil {
		t.Fatalf("First(c) error=%v", err)
	}
	if !gotC.Unread {
		t.Fatal("c.Unread=false want=true")
	}
	_ = b
}

func TestMarkAllCallLogsRead(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	if _, err := CreateRingingCallLog("dev-1", "iccid-1", "+861111", CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if _, err := CreateRingingCallLog("dev-2", "iccid-2", "+862222", CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if _, err := CreateRingingCallLog("dev-2", "iccid-2", "+863333", CallDirectionOut, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}

	affected, err := MarkAllCallLogsRead()
	if err != nil {
		t.Fatalf("MarkAllCallLogsRead() error=%v", err)
	}
	if affected != 2 {
		t.Fatalf("affected=%d want=2 (outbound call was never unread)", affected)
	}

	count, err := CountUnreadCalls()
	if err != nil {
		t.Fatalf("CountUnreadCalls() error=%v", err)
	}
	if count != 0 {
		t.Fatalf("CountUnreadCalls()=%d want=0", count)
	}
}

func TestListCallLogsByDevice(t *testing.T) {
	initCallLogTestDB(t)
	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)

	if _, err := CreateRingingCallLog("dev-1", "iccid-1", "+861111", CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if _, err := CreateRingingCallLog("dev-2", "iccid-2", "+862222", CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}

	list, err := ListCallLogsByDevice("dev-1", 10)
	if err != nil {
		t.Fatalf("ListCallLogsByDevice() error=%v", err)
	}
	if len(list) != 1 || list[0].Number != "+861111" {
		t.Fatalf("unexpected list=%+v", list)
	}
}
