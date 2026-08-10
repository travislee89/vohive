package device

import (
	"fmt"
	"strings"

	"github.com/boa-z/vohive/internal/backend"
	"github.com/boa-z/vohive/internal/cscall"
	"github.com/boa-z/vohive/internal/sipgw"
	"github.com/boa-z/vohive/pkg/logger"
)

func newCSCallManagerForWorker(w *Worker, r *sipgw.Registrar) *cscall.Manager {
	if w == nil || r == nil || w.Config.AudioDevice == "" {
		return nil
	}
	switch {
	case w.Backend != nil && w.Backend.Mode() == backend.BackendAT && w.Modem != nil:
		return cscall.NewManagerWithController(w.ID, w.Config.AudioDevice, cscall.NewATController(w.Modem), r)
	case w.Backend != nil && w.Backend.Mode() == backend.BackendQMI && w.QMICore != nil:
		return cscall.NewManagerWithController(w.ID, w.Config.AudioDevice, cscall.NewQMIController(w.QMICore), r)
	default:
		logger.Debug(fmt.Sprintf("[%s] 跳过 CS 域语音桥接：缺少可用控制面", w.ID),
			"backend", workerBackendMode(w),
			"audio_device", w.Config.AudioDevice,
			"has_modem", w.Modem != nil,
			"has_qmi_core", w.QMICore != nil)
		return nil
	}
}

// CSCallNotInitializedReason 返回指定设备 CSCallMgr 为 nil 的具体原因（人类可读）。
// 当 worker 不存在或 CSCallMgr 已初始化时返回空字符串。
// 诊断优先级：SIP 注册器未启用 > 未发现音频设备 > 后端模式不支持 > 控制面未就绪。
func (p *Pool) CSCallNotInitializedReason(deviceID string) string {
	w := p.GetWorker(deviceID)
	if w == nil || w.CSCallMgr != nil {
		return ""
	}
	if p.sipRegistrar == nil {
		return "SIP 注册器未启用。请在 config.yaml 中配置 voice_gateway.sip.listen（并配置 users）后重启服务，软电话才能接入并桥接 CS 域语音。"
	}
	if strings.TrimSpace(w.Config.AudioDevice) == "" {
		return "未发现可用的 USB 音频设备。请确认模组已暴露 USB 声卡节点（如 hw:CARD=...,DEV=0）并被系统识别后重启服务。"
	}
	mode := workerBackendMode(w)
	if mode != backend.BackendAT && mode != backend.BackendQMI {
		return fmt.Sprintf("当前后端模式 %q 暂不支持 CS 域语音桥接。请将 device_backend 改为 at 或 qmi 后重启服务。", mode)
	}
	return fmt.Sprintf("后端控制面未就绪（backend=%s, has_modem=%v, has_qmi_core=%v）。请检查设备启动日志中相关错误并重启服务。",
		mode, w.Modem != nil, w.QMICore != nil)
}
