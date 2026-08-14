package device

import (
	"context"
	"testing"

	"github.com/boa-z/vohive/internal/backend"
)

type smscResult struct {
	value string
	err   error
}

type workerSMSCBackendStub struct {
	workerStatusBackendStub
	seq   []smscResult
	calls int
}

func (s *workerSMSCBackendStub) GetSMSC(ctx context.Context) (string, error) {
	s.calls++
	if len(s.seq) == 0 {
		return "", nil
	}
	out := s.seq[0]
	s.seq = s.seq[1:]
	return out.value, out.err
}

func TestWorkerGetSMSCWithContextQMIRequiresSMSCProvider(t *testing.T) {
	w := &Worker{
		ID:      "dev-qmi",
		Backend: &workerStatusBackendStub{mode: backend.BackendQMI},
	}
	got, err := w.getSMSCWithContext(context.Background())
	if err == nil {
		t.Fatal("expected error for qmi backend without SMSCProvider")
	}
	if got != "" {
		t.Fatalf("getSMSCWithContext()=%q want empty", got)
	}
}

func TestWorkerGetSMSCWithContextATUsesProvider(t *testing.T) {
	b := &workerSMSCBackendStub{
		workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendAT},
		seq: []smscResult{
			{value: "+8613800250500"},
		},
	}
	w := &Worker{
		ID:      "dev-at",
		Backend: b,
	}
	got, err := w.getSMSCWithContext(context.Background())
	if err != nil {
		t.Fatalf("getSMSCWithContext() error=%v", err)
	}
	if got != "+8613800250500" {
		t.Fatalf("getSMSCWithContext()=%q want=%q", got, "+8613800250500")
	}
}

func TestWorkerRefreshRuntimeProjectsSMSCAndServingPLMN(t *testing.T) {
	// 验证 collectRuntimeStatus + mergeRuntimeStateLocked + ProjectDeviceStatus
	// 整条链路会把 SMSC 与当前驻留网络 MCC/MNC 透出到对外 modem.DeviceStatus。
	b := &workerSMSCBackendStub{
		workerStatusBackendStub: workerStatusBackendStub{
			mode: backend.BackendQMI,
			serving: &backend.ServingSystem{
				Operator: "Singtel",
				MCC:      525,
				MNC:      1,
			},
		},
		seq: []smscResult{
			{value: "+6596197777"},
		},
	}
	w := &Worker{
		ID:      "dev-qmi",
		Backend: b,
	}
	if err := w.RefreshRuntime(context.Background(), "test"); err != nil {
		t.Fatalf("RefreshRuntime() error=%v", err)
	}
	st := w.ProjectDeviceStatus()
	if st.SMSC != "+6596197777" {
		t.Fatalf("ProjectDeviceStatus().SMSC=%q want=%q", st.SMSC, "+6596197777")
	}
	if st.ServingMCC != "525" || st.ServingMNC != "1" {
		t.Fatalf("ProjectDeviceStatus().serving=%s/%s want 525/1", st.ServingMCC, st.ServingMNC)
	}
	if st.Operator != "Singtel" {
		t.Fatalf("ProjectDeviceStatus().Operator=%q want Singtel", st.Operator)
	}
}
