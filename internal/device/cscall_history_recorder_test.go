package device

import (
	"testing"

	"github.com/travislee89/vohive/internal/cscall"
	"github.com/travislee89/vohive/internal/db"
)

func TestCSCallHistoryRecorderIncomingAnswered(t *testing.T) {
	openDeviceTestDB(t)

	r := csCallHistoryRecorder{worker: &Worker{ID: "dev-1"}}
	ch := make(chan cscall.Event, 8)
	ch <- cscall.Event{Type: cscall.EventIncoming, CallID: "at", Number: "+8613800000000"}
	ch <- cscall.Event{Type: cscall.EventConnected, CallID: "at"}
	ch <- cscall.Event{Type: cscall.EventHangup, CallID: "at"}
	close(ch)

	r.run(ch, func() {})

	logs, err := db.ListCallLogsByDevice("dev-1", 10)
	if err != nil {
		t.Fatalf("ListCallLogsByDevice() error=%v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs)=%d want=1", len(logs))
	}
	got := logs[0]
	if got.Direction != db.CallDirectionIn {
		t.Fatalf("Direction=%q want=%q", got.Direction, db.CallDirectionIn)
	}
	if got.Number != "+8613800000000" {
		t.Fatalf("Number=%q want=+8613800000000", got.Number)
	}
	if got.Outcome != db.CallOutcomeAnswered {
		t.Fatalf("Outcome=%q want=%q", got.Outcome, db.CallOutcomeAnswered)
	}
	if !got.Unread {
		t.Fatal("Unread=false want=true (answered inbound calls still badge until viewed; only mark-read clears them)")
	}
}

func TestCSCallHistoryRecorderIncomingMissedStaysUnread(t *testing.T) {
	openDeviceTestDB(t)

	r := csCallHistoryRecorder{worker: &Worker{ID: "dev-1"}}
	ch := make(chan cscall.Event, 8)
	ch <- cscall.Event{Type: cscall.EventIncoming, CallID: "at", Number: "+8613800000000"}
	ch <- cscall.Event{Type: cscall.EventHangup, CallID: "at"}
	close(ch)

	r.run(ch, func() {})

	count, err := db.CountUnreadCalls()
	if err != nil {
		t.Fatalf("CountUnreadCalls() error=%v", err)
	}
	if count != 1 {
		t.Fatalf("CountUnreadCalls()=%d want=1", count)
	}

	logs, err := db.ListCallLogsByDevice("dev-1", 10)
	if err != nil {
		t.Fatalf("ListCallLogsByDevice() error=%v", err)
	}
	if len(logs) != 1 || logs[0].Outcome != db.CallOutcomeMissed {
		t.Fatalf("unexpected logs=%+v", logs)
	}
}

func TestCSCallHistoryRecorderOutboundDialingNeverUnread(t *testing.T) {
	openDeviceTestDB(t)

	r := csCallHistoryRecorder{worker: &Worker{ID: "dev-1"}}
	ch := make(chan cscall.Event, 8)
	ch <- cscall.Event{Type: cscall.EventDialing, CallID: "at", Number: "+8613900000000"}
	ch <- cscall.Event{Type: cscall.EventConnected, CallID: "at"}
	ch <- cscall.Event{Type: cscall.EventHangup, CallID: "at"}
	close(ch)

	r.run(ch, func() {})

	count, err := db.CountUnreadCalls()
	if err != nil {
		t.Fatalf("CountUnreadCalls() error=%v", err)
	}
	if count != 0 {
		t.Fatalf("CountUnreadCalls()=%d want=0 (outbound calls must never badge)", count)
	}

	logs, err := db.ListCallLogsByDevice("dev-1", 10)
	if err != nil {
		t.Fatalf("ListCallLogsByDevice() error=%v", err)
	}
	if len(logs) != 1 || logs[0].Direction != db.CallDirectionOut {
		t.Fatalf("unexpected logs=%+v", logs)
	}
}
