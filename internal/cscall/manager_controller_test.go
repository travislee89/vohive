package cscall

import (
	"context"
	"testing"
)

func TestHasConnectedCallMatchesEmptyOrExactID(t *testing.T) {
	calls := []CallInfo{{ID: "7", State: CallStateConnected}}
	if !hasConnectedCall(calls, "7") {
		t.Fatal("hasConnectedCall() false for exact connected call")
	}
	if !hasConnectedCall(calls, "") {
		t.Fatal("hasConnectedCall() false for empty desired call")
	}
	if hasConnectedCall(calls, "8") {
		t.Fatal("hasConnectedCall() true for different call id")
	}
}

func TestManagerBeginIncomingCallSetsRingingState(t *testing.T) {
	mgr := &Manager{deviceID: "dev-1", state: CallStateIdle}
	sipCallID, shouldStart := mgr.beginIncomingCall("at", "+123")
	if !shouldStart {
		t.Fatal("shouldStart=false want true")
	}
	if sipCallID == "" {
		t.Fatal("sipCallID is empty")
	}
	if mgr.state != CallStateRinging {
		t.Fatalf("state=%v want ringing", mgr.state)
	}
	if mgr.callerID != "+123" {
		t.Fatalf("callerID=%q want +123", mgr.callerID)
	}
	if mgr.controllerCallID != "at" {
		t.Fatalf("controllerCallID=%q want at", mgr.controllerCallID)
	}
}

func TestManagerSubscribeReceivesBroadcastEvents(t *testing.T) {
	mgr := &Manager{deviceID: "dev-1", state: CallStateIdle, subscribers: make(map[chan Event]struct{})}

	ch, cancel := mgr.Subscribe()
	defer cancel()

	// 再订阅一个，验证多播
	ch2, cancel2 := mgr.Subscribe()
	defer cancel2()

	mgr.broadcastEvent(Event{Type: EventIncoming, CallID: "at", Number: "+123"})

	got := <-ch
	if got.Type != EventIncoming || got.Number != "+123" {
		t.Fatalf("subscriber1 event=%+v want incoming +123", got)
	}
	got2 := <-ch2
	if got2.Type != EventIncoming || got2.Number != "+123" {
		t.Fatalf("subscriber2 event=%+v want incoming +123", got2)
	}

	// 取消后不再收到新事件
	cancel()
	mgr.broadcastEvent(Event{Type: EventHangup, CallID: "at"})
	select {
	case <-ch:
		t.Fatal("subscriber received event after cancel")
	default:
	}
}

func TestManagerCallsReturnsNilWhenControllerMissing(t *testing.T) {
	mgr := &Manager{deviceID: "dev-1", state: CallStateIdle}
	if calls := mgr.Calls(context.Background()); calls != nil {
		t.Fatalf("Calls() with nil controller=%+v want nil", calls)
	}
}
