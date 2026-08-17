package device

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/travislee89/vohive/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAddWorkerQMIManagedRebindsByIMEIWhenControlDeviceGone(t *testing.T) {
	// QMI 托管设备:配置 control_device 指向不存在节点,但配置了正确 IMEI;
	// 注入一块带该 IMEI 的新路径 QMI 硬件。bootstrap 应按 IMEI 取回新路径并采纳。

	originalDiscover := discoverQMIDevicesFn
	defer func() { discoverQMIDevicesFn = originalDiscover }()
	discoverQMIDevicesFn = func() ([]QMIDevice, error) {
		return []QMIDevice{
			{
				ControlPath:  "/dev/cdc-wdm-new-qmi",
				NetInterface: "wwan-new",
				USBPath:      "1-2.3",
				ATPort:       "/dev/ttyUSB-new",
			},
		}, nil
	}

	originalResolveQMI := resolveDiscoveredQMIDeviceFn
	defer func() { resolveDiscoveredQMIDeviceFn = originalResolveQMI }()
	resolveDiscoveredQMIDeviceFn = func(dev QMIDevice, timeout time.Duration, allowProbe bool) (QMIDevice, string) {
		if dev.ControlPath == "/dev/cdc-wdm-new-qmi" {
			return dev, "123456789012345"
		}
		return dev, ""
	}

	// 初始化 Pool
	p := NewPool(&config.Config{})
	defer p.cancel()

	devCfg := config.DeviceConfig{
		ID:             "dev-qmi-1",
		DeviceBackend:  "qmi",
		ModemIMEI:      "123456789012345",
		ControlDevice:  "/dev/nonexistent-control-old",
		Interface:      "wwan-old",
		USBPath:        "1-9.9",
		NetworkEnabled: true, // hasManagedQMINetwork 的条件
	}

	// 此时 /dev/nonexistent-control-old 不存在，controlDeviceStatErr != nil。
	// 但 shouldDiscoverQMIManagedBootstrapByIMEI 会返回 true，它会用 discovery
	// 取回 /dev/cdc-wdm-new-qmi 并采纳新的 QMI attachment。
	//
	// /dev/cdc-wdm-new-qmi 是虚构路径，没有真实硬件，真正打开 QMI 设备终究会失败；
	// 但 startQMICoreWithStartupBudget 把"打开失败/超时"当作可重试的瞬时故障处理
	// （转入后台重试循环），并不会让 AddWorkerFromConfig 返回 error。这一分类取决
	// 于失败在 1.5s 预算内被判定为 abort（快速失败，如无 qmi-proxy 时 dial 立即被拒）
	// 还是 retry（如本机装有 qmi-proxy，fork 后在预算内反复重试拨号直至超时，被归为
	// 可重试的 DeadlineExceeded）——因此是否安装/存在 qmi-proxy 会决定走哪条分支。
	// 因此这里对成功和失败两种结果分别断言 rebind 逻辑是否生效，而不是假设必然出错。
	w, err := p.AddWorkerFromConfig(devCfg)
	if err != nil {
		// 错误不能是"静态控制口不存在"的早退错误（即没有在读取旧的
		// /dev/nonexistent-control-old 时就直接放弃），说明流程确实基于 IMEI
		// 匹配换成了新发现的设备后才失败。
		require.NotContains(t, err.Error(), "设备控制口 /dev/nonexistent-control-old 不存在，可能模块尚未重新枚举")
		return
	}
	require.NotNil(t, w)
	require.Equal(t, "/dev/cdc-wdm-new-qmi", w.Config.ControlDevice)
	require.Equal(t, "wwan-new", w.Config.Interface)
}

// TestPoolAddWorkerFromConfigKeepsExistingDeviceErrorBeforeLimitError 测试尝试添加一个已存在的同名设备时，应该返回“设备已存在”错误
func TestPoolAddWorkerFromConfigKeepsExistingDeviceErrorBeforeLimitError(t *testing.T) {
	p := NewPool(&config.Config{})
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("dev%d", i)
		p.workers[id] = &Worker{ID: id, Config: config.DeviceConfig{ID: id}}
	}

	_, err := p.AddWorkerFromConfig(config.DeviceConfig{ID: "dev1"})
	if err == nil {
		t.Fatal("AddWorkerFromConfig() error = nil, want existing device error")
	}
	if !strings.Contains(err.Error(), "设备已存在") {
		t.Fatalf("AddWorkerFromConfig() error = %q, want existing device error", err.Error())
	}
}

// TestRemoveWorkerWaitsForInProgressInitialization 测试移除正在初始化中的设备时，应该同步等待其初始化完成后再执行销毁流程
func TestRemoveWorkerWaitsForInProgressInitialization(t *testing.T) {
	p := NewPool(&config.Config{})
	p.rebuilding["dev1"] = true

	go func() {
		time.Sleep(20 * time.Millisecond)
		p.mu.Lock()
		p.workers["dev1"] = &Worker{
			ID:     "dev1",
			Config: config.DeviceConfig{ID: "dev1"},
			stop:   make(chan struct{}),
		}
		delete(p.rebuilding, "dev1")
		p.mu.Unlock()
	}()

	if err := p.RemoveWorker("dev1"); err != nil {
		t.Fatalf("RemoveWorker() error = %v, want nil after in-progress init finishes", err)
	}
	if worker := p.GetWorker("dev1"); worker != nil {
		t.Fatalf("worker still exists after RemoveWorker: %#v", worker)
	}
}

// TestBeginRebuildAttemptLockedIncrementsMonotonically 测试同一设备连续两次进入启动流程时 token 单调递增
func TestBeginRebuildAttemptLockedIncrementsMonotonically(t *testing.T) {
	p := NewPool(&config.Config{})
	p.mu.Lock()
	first := p.beginRebuildAttemptLocked("dev1")
	second := p.beginRebuildAttemptLocked("dev1")
	p.mu.Unlock()

	if first != 1 {
		t.Fatalf("first attempt token = %d, want 1", first)
	}
	if second != 2 {
		t.Fatalf("second attempt token = %d, want 2", second)
	}
}

// TestEndRebuildAttemptIfCurrentOnlyClearsMatchingToken 测试只有 token 与最新一次尝试匹配时才会清除 rebuilding 标记，
// 避免滞后完成的旧启动流程误清新一轮尝试的状态
func TestEndRebuildAttemptIfCurrentOnlyClearsMatchingToken(t *testing.T) {
	p := NewPool(&config.Config{})
	p.mu.Lock()
	p.rebuilding["dev1"] = true
	p.rebuildAttempt["dev1"] = 2
	p.mu.Unlock()

	p.endRebuildAttemptIfCurrent("dev1", 1)
	p.mu.RLock()
	stillRebuilding := p.rebuilding["dev1"]
	p.mu.RUnlock()
	if !stillRebuilding {
		t.Fatal("stale token cleared rebuilding flag, want untouched")
	}

	p.endRebuildAttemptIfCurrent("dev1", 2)
	p.mu.RLock()
	stillRebuilding = p.rebuilding["dev1"]
	p.mu.RUnlock()
	if stillRebuilding {
		t.Fatal("current token failed to clear rebuilding flag")
	}
}

// TestStartBootstrapWatchdogForceClearsRebuildingAfterDeadline 测试启动看门狗在截止时间到达后，
// 如果该尝试仍是设备最新一次尝试，会强制释放 rebuilding 标记
func TestStartBootstrapWatchdogForceClearsRebuildingAfterDeadline(t *testing.T) {
	p := NewPool(&config.Config{})
	defer p.cancel()
	p.mu.Lock()
	p.rebuilding["dev1"] = true
	p.rebuildAttempt["dev1"] = 1
	p.mu.Unlock()

	stop := p.startBootstrapWatchdog("dev1", 1, 20*time.Millisecond)
	defer close(stop)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.RLock()
		cleared := !p.rebuilding["dev1"]
		p.mu.RUnlock()
		if cleared {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("watchdog did not clear rebuilding flag after deadline")
}

// TestStartBootstrapWatchdogIgnoresSupersededAttempt 测试看门狗触发时如果设备已经进入更新一轮尝试，
// 不应该误清新一轮尝试的 rebuilding 标记
func TestStartBootstrapWatchdogIgnoresSupersededAttempt(t *testing.T) {
	p := NewPool(&config.Config{})
	defer p.cancel()
	p.mu.Lock()
	p.rebuilding["dev1"] = true
	p.rebuildAttempt["dev1"] = 2 // 一次更新的尝试已经在进行
	p.mu.Unlock()

	stop := p.startBootstrapWatchdog("dev1", 1, 20*time.Millisecond)
	defer close(stop)

	time.Sleep(100 * time.Millisecond)

	p.mu.RLock()
	stillRebuilding := p.rebuilding["dev1"]
	p.mu.RUnlock()
	if !stillRebuilding {
		t.Fatal("watchdog cleared rebuilding flag for a superseded attempt, want untouched")
	}
}

// TestStartBootstrapWatchdogStopsWhenSignaled 测试正常完成路径 close(stop) 后看门狗不应该再触发
func TestStartBootstrapWatchdogStopsWhenSignaled(t *testing.T) {
	p := NewPool(&config.Config{})
	defer p.cancel()
	p.mu.Lock()
	p.rebuilding["dev1"] = true
	p.rebuildAttempt["dev1"] = 1
	p.mu.Unlock()

	stop := p.startBootstrapWatchdog("dev1", 1, 30*time.Millisecond)
	close(stop)

	time.Sleep(100 * time.Millisecond)

	p.mu.RLock()
	stillRebuilding := p.rebuilding["dev1"]
	p.mu.RUnlock()
	if !stillRebuilding {
		t.Fatal("watchdog fired after being stopped, want rebuilding flag untouched")
	}
}
