package device

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/travislee89/vohive/internal/backend"
	"github.com/travislee89/vohive/pkg/logger"
	"github.com/travislee89/vohive/pkg/smscodec"
)

// mbimTPDUFromHex 解析 MBIM SMS 存储返回的十六进制 PDU，跳过 SMSC 前缀，返回裸 TPDU 字节。
func mbimTPDUFromHex(pduHex string) ([]byte, error) {
	raw, err := hex.DecodeString(pduHex)
	if err != nil {
		return nil, fmt.Errorf("pdu hex 解析失败: %w", err)
	}
	if len(raw) < 1 {
		return nil, fmt.Errorf("pdu 为空")
	}
	scLen := int(raw[0])
	off := 1 + scLen
	if off > len(raw) {
		return nil, fmt.Errorf("SMSC 长度越界: scLen=%d len=%d", scLen, len(raw))
	}
	return raw[off:], nil
}

func decodeMBIMDeliverPDUHex(pduHex string) (sender, text string, ts time.Time, concat smscodec.ConcatInfo, err error) {
	tpduBytes, err := mbimTPDUFromHex(pduHex)
	if err != nil {
		return "", "", time.Time{}, smscodec.ConcatInfo{}, err
	}
	sender, text, ts, concat, err = smscodec.DecodeDeliverTPDU(tpduBytes)
	if err != nil {
		return "", "", time.Time{}, smscodec.ConcatInfo{}, fmt.Errorf("DELIVER 解码失败: %w", err)
	}
	return sender, text, ts, concat, nil
}

// decodeMBIMStatusReportPDUHex 尝试把 MBIM SMS 存储中的一条记录解码为 SMS-STATUS-REPORT。
// ok=false 表示该记录不是状态报告（调用方应回退到 decodeMBIMDeliverPDUHex）。
func decodeMBIMStatusReportPDUHex(pduHex string) (info smscodec.StatusReportInfo, ok bool) {
	tpduBytes, err := mbimTPDUFromHex(pduHex)
	if err != nil {
		return smscodec.StatusReportInfo{}, false
	}
	info, ok, err = smscodec.DecodeStatusReportTPDU(tpduBytes)
	if err != nil || !ok {
		return smscodec.StatusReportInfo{}, false
	}
	return info, true
}

func (w *Worker) handleNewSMSMBIM(reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	w.drainMBIMInbox(ctx, reason)
}

func (w *Worker) drainMBIMInbox(ctx context.Context, reason string) {
	lister, ok := w.Backend.(interface {
		ListSMS(context.Context) ([]backend.SMSSummary, error)
		ReadSMS(context.Context, int) (*backend.SMS, error)
		DeleteSMS(context.Context, int) error
	})
	if !ok {
		return
	}

	summaries, err := lister.ListSMS(ctx)
	if err != nil {
		logger.Warn(fmt.Sprintf("[%s] MBIM 列举短信失败", w.ID), "reason", reason, "err", err)
		return
	}
	for _, summary := range summaries {
		msg, err := lister.ReadSMS(ctx, summary.Index)
		if err != nil || msg == nil {
			logger.Warn(fmt.Sprintf("[%s] MBIM 读取短信失败", w.ID), "index", summary.Index, "err", err)
			continue
		}
		if sr, ok := decodeMBIMStatusReportPDUHex(msg.Content); ok {
			if raw, err := hex.DecodeString(msg.Content); err == nil {
				w.handleIncomingStatusReport(sr, raw)
			}
			if err := lister.DeleteSMS(ctx, summary.Index); err != nil {
				logger.Debug(fmt.Sprintf("[%s] MBIM 删除已读送达报告失败", w.ID), "index", summary.Index, "err", err)
			}
			continue
		}

		sender, text, ts, concat, err := decodeMBIMDeliverPDUHex(msg.Content)
		if err != nil {
			logger.Warn(fmt.Sprintf("[%s] MBIM 短信解码失败", w.ID), "index", summary.Index, "err", err)
			continue
		}
		if concat.IsConcat {
			logger.Debug(fmt.Sprintf("[%s] 收到 MBIM 短信分片", w.ID), "ref", concat.Ref, "seq", concat.Seq, "total", concat.Total)
		}
		if complete, full := w.reassembler.Add(sender, concat, text); complete {
			if ts.IsZero() {
				ts = time.Now()
			}
			w.processSMS(sender, full, ts)
		}
		if err := lister.DeleteSMS(ctx, summary.Index); err != nil {
			logger.Debug(fmt.Sprintf("[%s] MBIM 删除已读短信失败", w.ID), "index", summary.Index, "err", err)
		}
	}
}
