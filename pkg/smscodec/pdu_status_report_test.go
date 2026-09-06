package smscodec

import (
	"testing"

	"github.com/warthog618/sms/encoding/tpdu"
)

// fixedCounter 是一个测试用的 tpdu.Counter：每次调用 Count() 递增并返回。
type fixedCounter struct{ n int }

func (c *fixedCounter) Count() int {
	c.n++
	return c.n
}

func TestBuildSubmitTPDUsWithOptionsSetsSRRBit(t *testing.T) {
	tpdus, _, mrs, err := BuildSubmitTPDUsWithOptions("10086", "hello", SubmitOptions{RequestStatusReport: true})
	if err != nil {
		t.Fatalf("BuildSubmitTPDUsWithOptions() error = %v", err)
	}
	if len(tpdus) != 1 || len(mrs) != 1 {
		t.Fatalf("parts=%d mrs=%d want 1/1", len(tpdus), len(mrs))
	}

	pdu := &tpdu.TPDU{Direction: tpdu.MO}
	if err := pdu.UnmarshalBinary(tpdus[0]); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if !pdu.FirstOctet.SRR() {
		t.Fatalf("TP-SRR bit not set, FirstOctet=0x%02x", byte(pdu.FirstOctet))
	}
}

func TestBuildSubmitTPDUsWithOptionsWithoutSRRDoesNotSetBit(t *testing.T) {
	tpdus, _, _, err := BuildSubmitTPDUsWithOptions("10086", "hello", SubmitOptions{})
	if err != nil {
		t.Fatalf("BuildSubmitTPDUsWithOptions() error = %v", err)
	}
	pdu := &tpdu.TPDU{Direction: tpdu.MO}
	if err := pdu.UnmarshalBinary(tpdus[0]); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if pdu.FirstOctet.SRR() {
		t.Fatalf("TP-SRR bit should not be set by default")
	}
}

func TestBuildSubmitTPDUsWithOptionsUsesProvidedMRGenerator(t *testing.T) {
	counter := &fixedCounter{n: 40}
	_, _, mrs1, err := BuildSubmitTPDUsWithOptions("10086", "a", SubmitOptions{RequestStatusReport: true, MRGenerator: counter})
	if err != nil {
		t.Fatalf("first send error = %v", err)
	}
	_, _, mrs2, err := BuildSubmitTPDUsWithOptions("10086", "b", SubmitOptions{RequestStatusReport: true, MRGenerator: counter})
	if err != nil {
		t.Fatalf("second send error = %v", err)
	}
	if len(mrs1) != 1 || len(mrs2) != 1 {
		t.Fatalf("unexpected part counts: %d %d", len(mrs1), len(mrs2))
	}
	if mrs1[0] != 41 {
		t.Fatalf("first MR = %d want 41", mrs1[0])
	}
	if mrs2[0] != 42 {
		t.Fatalf("second MR = %d want 42 (must not reset to 1 across calls)", mrs2[0])
	}
}

// statusReportFixture 取自 github.com/warthog618/sms 的 "SmsStatusReport full" 测试用例：
// MR=0x42, RA="6391", ST=0xab（属于 3GPP 永久失败类别）。
var statusReportFixture = []byte{
	0x06, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
	0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x07,
	0x89, 0x04, 0x06, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74,
}

func TestDecodeStatusReportTPDU(t *testing.T) {
	info, ok, err := DecodeStatusReportTPDU(statusReportFixture)
	if err != nil {
		t.Fatalf("DecodeStatusReportTPDU() error = %v", err)
	}
	if !ok {
		t.Fatal("DecodeStatusReportTPDU() ok = false, want true")
	}
	if info.MR != 0x42 {
		t.Fatalf("MR = 0x%02x want 0x42", info.MR)
	}
	if info.RA != "+6391" {
		t.Fatalf("RA = %q want %q", info.RA, "+6391")
	}
	if info.Status != 0xab {
		t.Fatalf("Status = 0x%02x want 0xab", info.Status)
	}
	if info.SCTS.IsZero() || info.DischargeAt.IsZero() {
		t.Fatal("SCTS/DischargeAt should not be zero")
	}
}

func TestDecodeStatusReportTPDURejectsNonStatusReport(t *testing.T) {
	// screenshotPDUShort (定义于 pdu_trim_test.go) 是一条真实的 SMS-DELIVER（MT 方向）TPDU，
	// 用它验证 DecodeStatusReportTPDU 在遇到非状态报告类型时返回 ok=false 而非 error。
	b, err := hexStringToBytesForTest(screenshotPDUShort)
	if err != nil {
		t.Fatal(err)
	}
	smscLen := int(b[0])
	tpduBytes := b[1+smscLen:]

	_, ok, err := DecodeStatusReportTPDU(tpduBytes)
	if err != nil {
		t.Fatalf("DecodeStatusReportTPDU() error = %v", err)
	}
	if ok {
		t.Fatal("DecodeStatusReportTPDU() ok = true for a SMS-DELIVER TPDU, want false")
	}
}

func TestLooksLikeStatusReportTPDU(t *testing.T) {
	if !LooksLikeStatusReportTPDU(statusReportFixture) {
		t.Fatal("expected fixture to look like a status report")
	}
	tpdus, _, _, err := BuildSubmitTPDUsWithOptions("10086", "hello", SubmitOptions{})
	if err != nil {
		t.Fatalf("BuildSubmitTPDUsWithOptions() error = %v", err)
	}
	if LooksLikeStatusReportTPDU(tpdus[0]) {
		t.Fatal("SMS-SUBMIT TPDU should not look like a status report")
	}
}

func TestClassifyTPStatusRanges(t *testing.T) {
	// ClassifyTPStatus 定义在 internal/db，这里只验证 smscodec 侧的分流辅助函数存在且自洽。
	if !LooksLikeStatusReportTPDU([]byte{0x02}) {
		t.Fatal("TP-MTI=10 should look like a status report")
	}
	if LooksLikeStatusReportTPDU([]byte{0x00}) {
		t.Fatal("TP-MTI=00 (deliver) should not look like a status report")
	}
	if LooksLikeStatusReportTPDU([]byte{0x01}) {
		t.Fatal("TP-MTI=01 (submit) should not look like a status report")
	}
}
