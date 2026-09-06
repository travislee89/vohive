package device

import (
	"encoding/hex"
	"time"

	"github.com/travislee89/vohive/internal/backend"
	"github.com/travislee89/vohive/internal/db"
	"github.com/travislee89/vohive/pkg/logger"
	"github.com/travislee89/vohive/pkg/smscodec"
)

// applySMSDeliveryReportsPolicy 按设备当前短信通道，把"是否请求送达报告"的策略结果落到
// 传输层：AT 切换 +CNMI 的 <ds> 参数；QMI 切换 WMS 路由的 TransferStatusReportToClient；
// MBIM 没有对应的显式开关（是否出现在消息存储里取决于基带固件），此处不做任何调用。
func (p *Pool) applySMSDeliveryReportsPolicy(w *Worker, enabled bool) {
	if w == nil {
		return
	}
	if w.Modem != nil && (w.Backend == nil || w.Backend.Mode() == backend.BackendAT) {
		if err := w.Modem.SetSMSDeliveryReportsEnabled(enabled); err != nil {
			logger.Warn("设置 AT 短信送达报告上报失败", "device", w.ID, "enabled", enabled, "err", err)
		}
		return
	}
	if w.Backend != nil && w.Backend.Mode() == backend.BackendQMI {
		p.applyQMIStatusReportRoutingForWorker(w, enabled)
	}
}

// handleIncomingStatusReport 是跨传输（AT/QMI/MBIM）通用的送达报告落库入口。
// 未能匹配到对应待确认记录（例如收到了无法关联的报告，或该功能此前未请求过）时，
// 仅记日志，不视为错误——这是预期会发生的正常情况（比如 MR 早已从循环计数器绕回）。
func (w *Worker) handleIncomingStatusReport(sr smscodec.StatusReportInfo, rawPDU []byte) {
	if w == nil {
		return
	}
	row, err := db.MatchAndUpdateSMSStatusReport(w.ID, int(sr.MR), int(sr.Status), sr.DischargeAt, time.Now(), hex.EncodeToString(rawPDU))
	if err != nil {
		logger.Debug("收到送达报告但未匹配到待确认记录", "device", w.ID, "tp_mr", sr.MR, "tp_status", sr.Status, "err", err)
		return
	}
	logger.Info("短信送达报告已更新", "device", w.ID, "sms_id", row.SMSID, "tp_mr", sr.MR, "state", row.State)
}
