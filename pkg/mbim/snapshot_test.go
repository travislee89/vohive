package mbim

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestMonitorAppliesIndications(t *testing.T) {
	m := &Monitor{}

	sig := make([]byte, 20)
	le.PutUint32(sig[0:], 20)
	m.apply(Indication{Service: UUIDBasicConnect, CID: CIDBasicConnectSignalState, InfoBuffer: sig})

	providerID := encodeUTF16("46000")
	reg := make([]byte, registerFixedLen+len(providerID))
	le.PutUint32(reg[4:], 3)
	le.PutUint32(reg[20:], registerFixedLen)
	le.PutUint32(reg[24:], uint32(len(providerID)))
	copy(reg[registerFixedLen:], providerID)
	m.apply(Indication{Service: UUIDBasicConnect, CID: CIDBasicConnectRegisterState, InfoBuffer: reg})

	snap := m.Snapshot()
	if snap.SignalDBM != -73 {
		t.Fatalf("SignalDBM = %d, want -73", snap.SignalDBM)
	}
	if snap.MCC != "460" || snap.MNC != "00" {
		t.Fatalf("MCC/MNC = %s/%s", snap.MCC, snap.MNC)
	}
	if snap.RegisterState != 3 {
		t.Fatalf("RegisterState = %d", snap.RegisterState)
	}
}

func TestMonitorRunConsumesDeviceIndications(t *testing.T) {
	ft := newFakeTransport()
	ft.reply = func(w []byte) ([]byte, bool) {
		h, _ := decodeHeader(w)
		if h.Type == MessageTypeOpen {
			return openDoneMsg(h.TransactionID), true
		}
		return nil, false
	}
	d := newDevice(ft)
	if err := d.Open(context.Background(), 4096); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	m := NewMonitor(d)
	go m.Run()
	defer m.Stop()

	sig := make([]byte, 20)
	le.PutUint32(sig[0:], 31)
	ft.toRead <- indicateStatusMsg(0, UUIDBasicConnect, CIDBasicConnectSignalState, sig)

	deadline := time.After(time.Second)
	for {
		if m.Snapshot().SignalDBM == -51 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("snapshot was not updated before timeout")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestMonitorSMSCallback(t *testing.T) {
	ft := newFakeTransport()
	ft.reply = func(w []byte) ([]byte, bool) {
		h, _ := decodeHeader(w)
		if h.Type == MessageTypeOpen {
			return openDoneMsg(h.TransactionID), true
		}
		return nil, false
	}
	d := newDevice(ft)
	if err := d.Open(context.Background(), 4096); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	m := NewMonitor(d)
	got := make(chan struct{}, 1)
	m.SetOnSMS(func() { got <- struct{}{} })
	go m.Run()
	defer m.Stop()

	m.apply(Indication{Service: UUIDSMS, CID: CIDSMSRead, InfoBuffer: nil})

	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("SMS 回调未触发")
	}
}

func TestMonitorUSSDCallback(t *testing.T) {
	m := &Monitor{}
	got := make(chan USSDResponse, 1)
	m.SetOnUSSD(func(resp USSDResponse) { got <- resp })

	payload := []byte{0xC8, 0x34}
	info := make([]byte, 20+len(payload))
	le.PutUint32(info[0:], USSDRespNoActionRequired)
	le.PutUint32(info[4:], 1)
	le.PutUint32(info[8:], 0x0F)
	le.PutUint32(info[12:], 20)
	le.PutUint32(info[16:], uint32(len(payload)))
	copy(info[20:], payload)

	m.apply(Indication{Service: UUIDUSSD, CID: CIDUSSD, InfoBuffer: info})

	select {
	case resp := <-got:
		if resp.Response != USSDRespNoActionRequired || resp.SessionState != 1 || resp.DCS != 0x0F {
			t.Fatalf("USSD response = %+v", resp)
		}
		if !bytes.Equal(resp.Payload, payload) {
			t.Fatalf("Payload = %x, want %x", resp.Payload, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("USSD callback was not invoked")
	}
}

func TestMonitorFiresOnSubscriberReadyWhenStateChanges(t *testing.T) {
	m := &Monitor{}
	got := make(chan Snapshot, 1)
	m.SetOnSubscriberReady(func(s Snapshot) { got <- s })

	info := buildSubscriberBuf("460001234567890", "89860012345678901234", "")
	m.apply(Indication{Service: UUIDBasicConnect, CID: CIDBasicConnectSubscriberReadyStatus, InfoBuffer: info})

	select {
	case s := <-got:
		if s.ICCID != "89860012345678901234" {
			t.Fatalf("ICCID = %q, want 89860012345678901234", s.ICCID)
		}
		if s.ReadyState != 1 {
			t.Fatalf("ReadyState = %d, want 1", s.ReadyState)
		}
	case <-time.After(time.Second):
		t.Fatal("OnSubscriberReady callback was not invoked")
	}
}

func TestMonitorDoesNotFireOnSubscriberReadyWhenUnchanged(t *testing.T) {
	m := &Monitor{}
	m.snap.ReadyState = 1
	m.snap.ICCID = "89860012345678901234"
	fired := false
	m.SetOnSubscriberReady(func(Snapshot) { fired = true })

	info := buildSubscriberBuf("460001234567890", "89860012345678901234", "")
	m.apply(Indication{Service: UUIDBasicConnect, CID: CIDBasicConnectSubscriberReadyStatus, InfoBuffer: info})

	if fired {
		t.Fatal("OnSubscriberReady should not fire when ReadyState/ICCID unchanged")
	}
}

func TestMonitorSlotInfoStatusCallback(t *testing.T) {
	m := &Monitor{}
	got := make(chan SlotInfoStatus, 1)
	m.SetOnSlotInfoStatus(func(s SlotInfoStatus) { got <- s })

	info := make([]byte, 8)
	le.PutUint32(info[0:], 0)
	le.PutUint32(info[4:], UICCSlotStateActiveEsim)
	m.apply(Indication{Service: UUIDMSBasicConnectExtensions, CID: CIDMSBasicConnectExtSlotInfoStatus, InfoBuffer: info})

	select {
	case s := <-got:
		if s.SlotIndex != 0 || s.State != UICCSlotStateActiveEsim {
			t.Fatalf("slot info = %+v", s)
		}
	case <-time.After(time.Second):
		t.Fatal("OnSlotInfoStatus callback was not invoked")
	}
}

// TestMonitorFiresOnRegisterStateChange 验证 REGISTER_STATE indication 变化时触发回调（如归属地→漫游）。
func TestMonitorFiresOnRegisterStateChange(t *testing.T) {
	m := &Monitor{}
	got := make(chan Snapshot, 1)
	m.SetOnRegisterState(func(s Snapshot) { got <- s })

	// 先设一个初始注册状态（home=3）
	providerID := encodeUTF16("46000")
	reg := make([]byte, registerFixedLen+len(providerID))
	le.PutUint32(reg[4:], 3) // home
	le.PutUint32(reg[20:], registerFixedLen)
	le.PutUint32(reg[24:], uint32(len(providerID)))
	copy(reg[registerFixedLen:], providerID)
	m.apply(Indication{Service: UUIDBasicConnect, CID: CIDBasicConnectRegisterState, InfoBuffer: reg})

	// 第一次 apply 不应触发（没有"变化"——初始为 0→3 也算变化，应触发）
	select {
	case <-got:
		// 初始 0→3 属于变化，触发是合理的
	case <-time.After(time.Second):
		t.Fatal("RegisterState 从 0 变到 3 应触发回调")
	}

	// 再次用相同状态 apply，不应触发
	m.apply(Indication{Service: UUIDBasicConnect, CID: CIDBasicConnectRegisterState, InfoBuffer: reg})
	select {
	case <-got:
		t.Fatal("RegisterState 未变化(3→3)时不应触发回调")
	case <-time.After(50 * time.Millisecond):
	}

	// 变为漫游(4)应触发
	reg2 := make([]byte, registerFixedLen+len(providerID))
	le.PutUint32(reg2[4:], 4) // roaming
	le.PutUint32(reg2[20:], registerFixedLen)
	le.PutUint32(reg2[24:], uint32(len(providerID)))
	copy(reg2[registerFixedLen:], providerID)
	m.apply(Indication{Service: UUIDBasicConnect, CID: CIDBasicConnectRegisterState, InfoBuffer: reg2})

	select {
	case s := <-got:
		if s.RegisterState != 4 {
			t.Fatalf("RegisterState = %d, want 4", s.RegisterState)
		}
	case <-time.After(time.Second):
		t.Fatal("RegisterState 从 3 变到 4 应触发回调")
	}
}
