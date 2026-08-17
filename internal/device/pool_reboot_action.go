package device

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/travislee89/vohive/internal/backend"
	"github.com/travislee89/vohive/internal/config"
)

// ErrWorkerNotFound 表示指定 id 未找到对应的设备 worker。
var ErrWorkerNotFound = errors.New("设备未找到")

// ErrIdentityConflict 表示重启前的 IMEI 校验发现设备路径已漂移，为安全起见拒绝重启。
var ErrIdentityConflict = errors.New("设备路径已漂移")

// shouldUseATFirstReboot 判断重启时是否应优先尝试 AT+CFUN=1,1。
// QMI 模式设备直接走 QMI ModeReset（backend.Reboot）；AT 优先路径仅保留给 AT 模式设备，
// 原先"QMI 模式也优先走 AT"是为了规避部分模组 QMI ModeReset 假死的历史问题，
// 现已实测确认本机型号 QMI ModeReset 正常工作，因此 QMI 模式不再绕道 AT。
func shouldUseATFirstReboot(backendMode string) bool {
	return backendMode != backend.BackendQMI
}

// validateRebootWorkerIdentity 在重启前校验控制面当前 IMEI 是否仍与配置一致，防止设备路径漂移后误重启到另一张卡上。
func validateRebootWorkerIdentity(ctx context.Context, worker *Worker) error {
	if worker == nil || worker.Backend == nil {
		return nil
	}
	expectedIMEI := strings.TrimSpace(worker.Config.ModemIMEI)
	if expectedIMEI == "" {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	currentIMEI, err := worker.Backend.GetIMEI(probeCtx)
	if err != nil || strings.TrimSpace(currentIMEI) == "" {
		return nil
	}
	currentIMEI = strings.TrimSpace(currentIMEI)
	if !config.IMEIMatches(currentIMEI, expectedIMEI) {
		return fmt.Errorf("%w：当前控制面 IMEI=%s，不匹配配置 IMEI=%s，请先重新扫描/重新绑定后再重启", ErrIdentityConflict, currentIMEI, expectedIMEI)
	}
	return nil
}

// RebootWorkerAction 执行模组重启（QMI 模式走 QMI ModeReset，AT 模式走 AT+CFUN=1,1），
// 并在指令送达后启动重启恢复跟踪。手动重启（HTTP /actions/reboot）与自动化任务重启共用同一路径。
func (p *Pool) RebootWorkerAction(ctx context.Context, id string) error {
	worker := p.GetWorker(id)
	if worker == nil {
		return ErrWorkerNotFound
	}

	if err := validateRebootWorkerIdentity(ctx, worker); err != nil {
		return err
	}

	rebootSent := false
	useATFirst := worker.Backend == nil || shouldUseATFirstReboot(worker.Backend.Mode())

	// AT 模式设备优先尝试使用 AT 端口软重启；QMI 模式设备直接走 QMI ModeReset（见下方 fallback）
	if useATFirst && worker.Modem != nil && worker.Modem.HasATPort() && worker.Modem.CanExecuteAT() {
		_, err := worker.Modem.ExecuteAT("AT+CFUN=1,1", 20*time.Second)
		if err == nil {
			rebootSent = true
		} else {
			// 如果发送后立刻断开，可能会报错，视同成功发送
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "timeout") || strings.Contains(msg, "eof") || strings.Contains(msg, "closed") || strings.Contains(msg, "no such file") {
				rebootSent = true
			}
		}
	}

	// QMI 模式设备的主路径；AT 模式设备在 AT 端口不可用/发送失败时的降级路径
	if !rebootSent && worker.Backend != nil {
		if err := worker.Backend.Reboot(ctx); err != nil {
			return fmt.Errorf("重启指令失败: %w", err)
		}
		rebootSent = true
	}

	if !rebootSent {
		return errors.New("无法发送重启指令，无可用通道")
	}

	p.MarkLifecycleRecovery(id, LifecyclePhaseRebooting, "manual_reboot", 3*time.Minute)
	p.ScheduleModemRebootRecovery(id, "manual_reboot")
	return nil
}
