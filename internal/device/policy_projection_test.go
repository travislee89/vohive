package device

import (
	"context"
	"testing"
	"time"

	"github.com/travislee89/vohive/internal/backend"
	"github.com/travislee89/vohive/internal/cardpolicy"
	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/internal/vowifihost"
)

func TestApplyPolicyProjectsFields(t *testing.T) {
	w := &Worker{ID: "wwan0"}
	applyPolicyToWorker(w, cardpolicy.Policy{
		ICCID: "x", NetworkEnabled: true, VoWiFiEnabled: true,
		AirplaneEnabled: true, IPVersion: "v4v6", APN: "ims",
	})
	if !w.Config.NetworkEnabled || !w.Config.VoWiFiEnabled || !w.Config.AirplaneEnabled {
		t.Fatalf("开关未投影: %+v", w.Config)
	}
	if w.Config.IPVersion != "v4v6" || w.Config.APN != "ims" {
		t.Fatalf("ip/apn 未投影: %+v", w.Config)
	}
	if !w.Config.SMSEnabled {
		t.Fatal("SMS 应恒为 true")
	}
}

// 投影时按策略真正进入飞行模式：当前在线 ⇒ 切 RFOff。
func TestProjectionEntersAirplaneMode(t *testing.T) {
	p := &Pool{ctx: context.Background()}
	stub := &workerStatusBackendStub{opMode: backend.ModeOnline}
	w := &Worker{ID: "wwan0", Backend: stub}

	p.enterAirplaneModeFromPolicy(w, "test")

	if len(stub.setOpModeCalls) != 1 || stub.setOpModeCalls[0] != backend.ModeRFOff {
		t.Fatalf("应切到 RFOff: %+v", stub.setOpModeCalls)
	}
}

// 幂等：已在飞行模式时不重复下发 SetOperatingMode。
func TestProjectionEnterAirplaneIdempotent(t *testing.T) {
	p := &Pool{ctx: context.Background()}
	stub := &workerStatusBackendStub{opMode: backend.ModeRFOff}
	w := &Worker{ID: "wwan0", Backend: stub}

	p.enterAirplaneModeFromPolicy(w, "test")

	if len(stub.setOpModeCalls) != 0 {
		t.Fatalf("已在飞行不应重复切: %+v", stub.setOpModeCalls)
	}
}

// 投影时按策略退出飞行：当前 RFOff 且策略不要求飞行 ⇒ 切回 Online。
func TestProjectionExitsAirplaneMode(t *testing.T) {
	p := &Pool{ctx: context.Background()}
	stub := &workerStatusBackendStub{opMode: backend.ModeRFOff}
	w := &Worker{ID: "wwan0", Backend: stub}

	p.exitAirplaneModeIfNeeded(w, "test")

	if len(stub.setOpModeCalls) != 1 || stub.setOpModeCalls[0] != backend.ModeOnline {
		t.Fatalf("应切回 Online: %+v", stub.setOpModeCalls)
	}
}

// 已在线时退出飞行是 no-op。
func TestProjectionExitAirplaneSkipsWhenOnline(t *testing.T) {
	p := &Pool{ctx: context.Background()}
	stub := &workerStatusBackendStub{opMode: backend.ModeOnline}
	w := &Worker{ID: "wwan0", Backend: stub}

	p.exitAirplaneModeIfNeeded(w, "test")

	if len(stub.setOpModeCalls) != 0 {
		t.Fatalf("已在线不应切: %+v", stub.setOpModeCalls)
	}
}

type stubPolicyResolver struct {
	pol cardpolicy.Policy
	err error
}

func (s *stubPolicyResolver) Resolve(iccid string) (cardpolicy.Policy, error) {
	return s.pol, s.err
}

func TestResolveAndApplyPolicy_EmptyICCID(t *testing.T) {
	p := &Pool{}
	w := &Worker{ID: "wwan0"}
	p.SetPolicyResolver(&stubPolicyResolver{})

	res := p.resolveAndApplyPolicy(w, "test")
	if res.Applied || res.Reason != "iccid_empty" {
		t.Fatalf("空 ICCID 应返回 iccid_empty: %+v", res)
	}
}

func TestResolveAndApplyPolicy_ResolvesAndProjects(t *testing.T) {
	p := &Pool{ctx: context.Background()}
	p.SetPolicyResolver(&stubPolicyResolver{
		pol: cardpolicy.Policy{ICCID: "123", AirplaneEnabled: true},
	})
	stub := &workerStatusBackendStub{opMode: backend.ModeOnline}
	w := &Worker{ID: "wwan0", Backend: stub}
	w.state.Identity.ICCID = "123"

	res := p.resolveAndApplyPolicy(w, "test")
	if !res.Applied {
		t.Fatalf("应成功应用: %+v", res)
	}
	if !w.Config.AirplaneEnabled {
		t.Fatal("策略投影失败")
	}
	if len(stub.setOpModeCalls) != 1 || stub.setOpModeCalls[0] != backend.ModeRFOff {
		t.Fatalf("应切入飞行模式: %+v", stub.setOpModeCalls)
	}
}

func TestResolveAndApplyPolicyDoesNotRecoverVoWiFiWhenCardPolicyDisabled(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()
	p.SetPolicyResolver(&stubPolicyResolver{
		pol: cardpolicy.Policy{ICCID: "123", VoWiFiEnabled: false},
	})
	commands := make(chan vowifihost.LifecycleCommand, 1)
	p.voWiFiHost().LifecycleControllerForTest().TestRun = func(ctx context.Context, cmd vowifihost.LifecycleCommand) error {
		commands <- cmd
		return nil
	}
	w := &Worker{ID: "wwan0"}
	w.state.Identity.ICCID = "123"
	w.state.Identity.IMSI = "001010000000001"

	res := p.resolveAndApplyPolicy(w, "test")
	if !res.Applied {
		t.Fatalf("应成功应用: %+v", res)
	}

	select {
	case cmd := <-commands:
		t.Fatalf("卡策略未开启 VoWiFi 时不应调度恢复: %+v", cmd)
	case <-time.After(120 * time.Millisecond):
	}
}

func TestRefreshIdentityAndApplyCardPolicyDoesNotFallbackToConfiguredNetworkWithoutResolver(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()

	ctrl := &fakeController{}
	w := &Worker{
		ID: "wwan0",
		Config: config.DeviceConfig{
			ID:             "wwan0",
			NetworkEnabled: true,
		},
		Backend: &workerStartupIdentityBackendStub{
			liveICCID: "898600000000000001",
			liveIMSI:  "460001234567890",
		},
		netOverride: ctrl,
	}

	result, err := p.refreshIdentityAndApplyCardPolicy(w, "startup_post_apply")
	if err != nil {
		t.Fatalf("refreshIdentityAndApplyCardPolicy() error=%v", err)
	}
	if result.ICCID != "898600000000000001" || result.IMSI != "460001234567890" {
		t.Fatalf("live identity result mismatch: %+v", result)
	}
	if ctrl.connected {
		t.Fatal("无 policy resolver 时不应回退到旧 worker.Config 连接数据网络")
	}
}

// 漫游且未开启「漫游数据」开关时，应跳过开网（reason=roaming_blocked），不调用 Connect。
func TestResolveAndApplyPolicy_RoamingBlocksNetworkWhenDisabled(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()
	p.SetPolicyResolver(&stubPolicyResolver{
		pol: cardpolicy.Policy{
			ICCID:              "123",
			NetworkEnabled:     true,
			RoamingDataEnabled: false,
		},
	})
	fc := &fakeController{connected: false}
	w := &Worker{
		ID:         "wwan0",
		netOverride: fc,
		Backend:    &workerStatusBackendStub{opMode: backend.ModeOnline},
	}
	w.state.Identity.ICCID = "123"
	// 模拟漫游注册状态（RegStatus=5）
	w.state.Runtime.RegStatus = regStatusRoaming

	res := p.resolveAndApplyPolicy(w, "test")
	if !res.Applied {
		t.Fatalf("应标记 Applied: %+v", res)
	}
	if res.Reason != "roaming_blocked" {
		t.Fatalf("reason 应为 roaming_blocked: %q", res.Reason)
	}
	if fc.connected {
		t.Fatal("漫游且未开启漫游数据时不应开网")
	}
}

// 漫游但已开启「漫游数据」开关时，应正常开网。
func TestResolveAndApplyPolicy_RoamingAllowsNetworkWhenEnabled(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()
	p.SetPolicyResolver(&stubPolicyResolver{
		pol: cardpolicy.Policy{
			ICCID:              "123",
			NetworkEnabled:     true,
			RoamingDataEnabled: true,
		},
	})
	fc := &fakeController{connected: false}
	w := &Worker{
		ID:         "wwan0",
		netOverride: fc,
		Backend:    &workerStatusBackendStub{opMode: backend.ModeOnline},
	}
	w.state.Identity.ICCID = "123"
	w.state.Runtime.RegStatus = regStatusRoaming

	res := p.resolveAndApplyPolicy(w, "test")
	if !res.Applied {
		t.Fatalf("应标记 Applied: %+v", res)
	}
	if res.Reason == "roaming_blocked" {
		t.Fatal("已开启漫游数据时不应被拦截")
	}
}

// 非漫游状态（归属地注册 RegStatus=1）时，无论漫游数据开关如何，都应正常开网。
func TestResolveAndApplyPolicy_HomeNotBlocked(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()
	p.SetPolicyResolver(&stubPolicyResolver{
		pol: cardpolicy.Policy{
			ICCID:              "123",
			NetworkEnabled:     true,
			RoamingDataEnabled: false,
		},
	})
	fc := &fakeController{connected: false}
	w := &Worker{
		ID:         "wwan0",
		netOverride: fc,
		Backend:    &workerStatusBackendStub{opMode: backend.ModeOnline},
	}
	w.state.Identity.ICCID = "123"
	// 归属地注册
	w.state.Runtime.RegStatus = 1

	res := p.resolveAndApplyPolicy(w, "test")
	if !res.Applied {
		t.Fatalf("应标记 Applied: %+v", res)
	}
	if res.Reason == "roaming_blocked" {
		t.Fatal("归属地注册不应被漫游拦截")
	}
}

// 漫游时已连网，应用策略后应主动停网。
func TestResolveAndApplyPolicy_RoamingStopsConnectedNetwork(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()
	p.SetPolicyResolver(&stubPolicyResolver{
		pol: cardpolicy.Policy{
			ICCID:              "123",
			NetworkEnabled:     true,
			RoamingDataEnabled: false,
		},
	})
	fc := &fakeController{connected: true} // 模拟当前已连网
	w := &Worker{
		ID:         "wwan0",
		netOverride: fc,
		Backend:    &workerStatusBackendStub{opMode: backend.ModeOnline},
	}
	w.state.Identity.ICCID = "123"
	w.state.Runtime.RegStatus = regStatusRoaming

	res := p.resolveAndApplyPolicy(w, "test")
	if res.Reason != "roaming_blocked" {
		t.Fatalf("reason 应为 roaming_blocked: %q", res.Reason)
	}
	if fc.connected {
		t.Fatal("漫游拦截时应主动断开已连网络")
	}
}

// isWorkerRoaming 辅助函数测试
func TestIsWorkerRoaming(t *testing.T) {
	if isWorkerRoaming(nil) {
		t.Fatal("nil worker 不应判为漫游")
	}
	w := &Worker{ID: "wwan0"}
	if isWorkerRoaming(w) {
		t.Fatal("未设置注册状态的 worker 不应判为漫游")
	}
	w.state.Runtime.RegStatus = 1
	if isWorkerRoaming(w) {
		t.Fatal("归属地注册(1)不应判为漫游")
	}
	w.state.Runtime.RegStatus = regStatusRoaming
	if !isWorkerRoaming(w) {
		t.Fatal("RegStatus=5 应判为漫游")
	}
}
