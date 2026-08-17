package device

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/boa-z/vohive/internal/config"
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
	// 但 shouldDiscoverQMIManagedBootstrapByIMEI 会返回 true。
	// 它会用 discovery 取回 /dev/cdc-wdm-new-qmi，并用新的 QMI attachment 启动 worker。
	w, err := p.AddWorkerFromConfig(devCfg)
	require.NoError(t, err)
	require.NotNil(t, w)
	t.Cleanup(func() {
		_ = p.RemoveWorker(w.ID)
		_ = p.Shutdown()
	})

	require.Equal(t, "/dev/cdc-wdm-new-qmi", w.Config.ControlDevice)
	require.Equal(t, "/dev/cdc-wdm-new-qmi", w.Config.QMIDevice)
	require.Equal(t, "wwan-new", w.Config.Interface)
	require.Equal(t, "1-2.3", w.Config.USBPath)
	require.Equal(t, "/dev/ttyUSB-new", w.Config.ATPort)
	require.Equal(t, "/dev/ttyUSB-new", w.Config.ManagePort)
	require.NotNil(t, w.QMICore)
	require.Equal(t, "/dev/cdc-wdm-new-qmi", w.QMICore.ControlDevice())
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
