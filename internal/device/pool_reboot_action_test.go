package device

import (
	"testing"

	"github.com/travislee89/vohive/internal/backend"
)

// TestShouldUseATFirstRebootSkipsForQMIBackend 测试 QMI 模式设备重启不应优先走 AT+CFUN，
// 应直接使用 QMI ModeReset（backend.Reboot），仅 AT 模式设备才保留 AT 优先路径。
func TestShouldUseATFirstRebootSkipsForQMIBackend(t *testing.T) {
	if shouldUseATFirstReboot(backend.BackendQMI) {
		t.Fatal("shouldUseATFirstReboot(qmi) = true, want false — QMI 模式应直接走 QMI ModeReset")
	}
	if !shouldUseATFirstReboot(backend.BackendAT) {
		t.Fatal("shouldUseATFirstReboot(at) = false, want true — AT 模式应保留 AT 优先路径")
	}
}
