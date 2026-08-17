package device

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/travislee89/vohive/internal/db"
	"github.com/travislee89/vohive/pkg/smscodec"
)

// SendSMSResult 是一次短信发送动作的结果，供 HTTP 层与自动化任务共用。
type SendSMSResult struct {
	MessageID     string
	PartsTotal    int
	DeliveryState string
}

// SendSMSAction 向指定设备发送一条短信（自动选择 VoWiFi 或蜂窝/AT 通道），并写入发送历史。
// HTTP 层的 /api/sms/send 与自动化中心的定时短信任务共用同一路径；device_id/imsi 到具体 worker
// 的解析（HTTP 请求形态相关）由调用方完成，本函数只处理"给定一个已解析的 deviceID，发送并记录"。
func (p *Pool) SendSMSAction(ctx context.Context, deviceID, phone, message string, opts smscodec.SubmitOptions) (SendSMSResult, error) {
	worker := p.GetWorker(deviceID)
	if worker == nil {
		return SendSMSResult{}, ErrWorkerNotFound
	}

	imsi := worker.GetIMSI()
	result := SendSMSResult{PartsTotal: 1, DeliveryState: "acked"}

	if p.IsVoWiFiActive(deviceID) {
		// VoWiFi 模式下使用 IMS Core 发送；短信历史由宿主侧 runtime event / failure recorder 入库。
		outcome, err := p.SendVoWiFiSMSWithOptions(ctx, deviceID, phone, message, opts)
		if outcome.PartsTotal > 0 {
			result.PartsTotal = outcome.PartsTotal
		}
		if strings.TrimSpace(outcome.DeliveryState) != "" {
			result.DeliveryState = strings.TrimSpace(outcome.DeliveryState)
		}
		result.MessageID = strings.TrimSpace(outcome.MessageID)
		if err != nil {
			_ = RecordVoWiFiSMSSendFailure(p, deviceID, phone, message, time.Now())
			return result, fmt.Errorf("VoWiFi 短信发送失败: %w", err)
		}
		return result, nil
	}

	// 普通模式使用 AT 发送
	if err := worker.SendSMSWithOptions(phone, message, opts); err != nil {
		if imsi != "" {
			// 发送失败，入库记录（status=3）
			_, _ = db.SaveSMS(imsi, worker.ID, phone, message, 2, 3, time.Now())
		}
		return result, fmt.Errorf("发送失败: %w", err)
	}
	if imsi != "" {
		// 发送成功，入库记录（status=2）
		_, _ = db.SaveSMS(imsi, worker.ID, phone, message, 2, 2, time.Now())
	}
	return result, nil
}
