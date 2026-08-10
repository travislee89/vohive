package device

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/boa-z/vohive/internal/config"
	"github.com/boa-z/vohive/internal/modem"
	"github.com/boa-z/vohive/pkg/logger"
)

// enableQuectelUACAndReboot 在独立 goroutine 中尝试通过一次性 AT 会话开启 Quectel 模组的
// USB Audio (UAC)。若配置发生更改，则在短暂延迟后自动重启模组，使声卡枚举生效。
// 模组重启恢复流程（runModemRebootRecovery → rescanAndReconnect）会重新发现 AudioDevice
// 并创建 CSCallMgr。该方法非阻塞，调用方无需等待。
//
// 触发条件（由调用方判断）：SIP 已就绪、未发现 AudioDevice、QMI 后端、AT 口可用。
func (p *Pool) enableQuectelUACAndReboot(w *Worker, atPort string) {
	deviceID := w.ID
	atPort = strings.TrimSpace(atPort)
	if atPort == "" {
		logger.Debug(fmt.Sprintf("[%s] 跳过 UAC 开启：AT 口为空", deviceID))
		return
	}
	if config.NormalizeModuleVendor(w.Config.ModuleVendor) != config.ModuleVendorQuectel {
		logger.Debug(fmt.Sprintf("[%s] 跳过 UAC 开启：非 Quectel 模组 (vendor=%q)", deviceID, w.Config.ModuleVendor))
		return
	}

	// 同一进程内对同一设备只尝试一次 UAC 开启。
	// 模组重启恢复期间会多次重建 worker 并重新进入 bootstrap，若无此去重，
	// 每次都会重复打开 AT 口（重启中 AT 口不可用会超时，恢复后 UAC 已开会重复查询）。
	p.mu.Lock()
	if p.uacAttempted[deviceID] {
		p.mu.Unlock()
		logger.Debug(fmt.Sprintf("[%s] 跳过 UAC 开启：本进程已尝试过", deviceID))
		return
	}
	p.uacAttempted[deviceID] = true
	p.mu.Unlock()

	logger.Info(fmt.Sprintf("[%s] 未发现 USB 音频设备，尝试通过 %s 开启 Quectel USB Audio (UAC)", deviceID, atPort))
	modified, err := tryEnableQuectelUAC(deviceID, atPort)
	if err != nil {
		logger.Warn(fmt.Sprintf("[%s] 开启 UAC 失败: %v", deviceID, err))
		return
	}
	if !modified {
		logger.Warn(fmt.Sprintf("[%s] UAC 已开启或模组不支持该指令，但仍未发现声卡，请手动检查模组 USB 配置或拔插 USB", deviceID))
		return
	}

	logger.Info(fmt.Sprintf("[%s] UAC 已开启，将在 3 秒后自动重启模组以使声卡枚举生效", deviceID))
	time.AfterFunc(3*time.Second, func() {
		if err := p.rebootWorkerForUAC(w); err != nil {
			logger.Warn(fmt.Sprintf("[%s] UAC 开启后重启模组失败: %v", deviceID, err))
			return
		}
		logger.Info(fmt.Sprintf("[%s] 已发送模组重启指令，恢复后将自动发现声卡并启用 CS 域语音桥接", deviceID))
		p.MarkLifecycleRecovery(deviceID, LifecyclePhaseRebooting, "uac_enabled", 3*time.Minute)
		p.ScheduleModemRebootRecovery(deviceID, "uac_enabled")
	})
}

// rebootWorkerForUAC 通过 worker 的后端发送重启指令。QMI 后端走 ModeReset。
// 发送后连接立即断开属正常现象，按成功处理。
func (p *Pool) rebootWorkerForUAC(w *Worker) error {
	if w.Backend == nil {
		return fmt.Errorf("backend 未初始化")
	}
	ctx, cancel := context.WithTimeout(p.ctx, 20*time.Second)
	defer cancel()
	if err := w.Backend.Reboot(ctx); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "timeout") || strings.Contains(msg, "eof") || strings.Contains(msg, "closed") || strings.Contains(msg, "no such file") {
			return nil
		}
		return err
	}
	return nil
}

// tryEnableQuectelUAC 通过一次性 AT 串口会话查询并把 Quectel USBCFG 的 UAC 位（最后一位）置 1。
// 返回 modified=true 表示本次确实更改了配置（需要重启模组才能生效）。
// 不依赖常驻 modem.Manager，适合在纯 QMI 模式（AT 管理器未启动）下使用。
func tryEnableQuectelUAC(deviceID, atPort string) (modified bool, err error) {
	sat, err := modem.NewSerialAT(atPort, 115200, 8, 1, "N")
	if err != nil {
		return false, fmt.Errorf("打开 %s: %w", atPort, err)
	}
	defer sat.Close()

	// 1. AT 握手，确认串口可用
	if _, err := sat.Execute("AT", time.Second); err != nil {
		return false, fmt.Errorf("AT 握手失败: %w", err)
	}

	// 2. 查询当前 USBCFG
	resp, err := sat.Execute(`AT+QCFG="USBCFG"?`, 2*time.Second)
	if err != nil {
		return false, fmt.Errorf("查询 USBCFG 失败: %w", err)
	}
	if strings.Contains(resp, "ERROR") {
		return false, fmt.Errorf("查询 USBCFG 返回 ERROR")
	}

	// 3. 解析并生成开启命令
	cmd, mod := modem.BuildUSBCFGEnableCmd(resp)
	if !mod {
		return false, nil
	}

	logger.Info(fmt.Sprintf("[%s] 通过 %s 写入 UAC 开启配置", deviceID, cmd))
	resp, err = sat.Execute(cmd, 3*time.Second)
	if err != nil {
		return false, fmt.Errorf("写入 USBCFG 失败: %w", err)
	}
	if strings.Contains(resp, "ERROR") {
		return false, fmt.Errorf("写入 USBCFG 返回 ERROR")
	}
	return true, nil
}
