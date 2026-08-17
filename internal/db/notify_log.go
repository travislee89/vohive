package db

import (
	"strings"
	"time"
)

// NotifyLog 记录一次通知转发结果：匹配到规则后每个目标渠道一条，
// 或未匹配到任何规则时一条 Channel 为空、Status 为 "unmatched" 的记录。
type NotifyLog struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	MessageType     string    `gorm:"column:message_type;index" json:"message_type"`
	Status          string    `gorm:"column:status;index" json:"status"` // success | unmatched | failed
	ContentSummary  string    `gorm:"column:content_summary" json:"content_summary"`
	MatchedRuleID   *string   `gorm:"column:matched_rule_id" json:"matched_rule_id,omitempty"`
	MatchedRuleName string    `gorm:"column:matched_rule_name" json:"matched_rule_name,omitempty"`
	Channel         string    `gorm:"column:channel;index" json:"channel,omitempty"`
	ErrorDetail     string    `gorm:"column:error_detail" json:"error_detail,omitempty"`
	DeviceID        string    `gorm:"column:device_id;index" json:"device_id,omitempty"`
	Timestamp       time.Time `gorm:"column:timestamp;index:idx_notify_log_ts,sort:desc" json:"timestamp"`
}

func (NotifyLog) TableName() string { return "notify_logs" }

const (
	NotifyLogStatusSuccess   = "success"
	NotifyLogStatusUnmatched = "unmatched"
	NotifyLogStatusFailed    = "failed"
)

// notifyLogSummaryMaxRunes 内容摘要的最大字符数，避免日志表被超长文本撑大。
const notifyLogSummaryMaxRunes = 120

// SummarizeNotifyLogContent 截断长文本用于日志表的内容摘要列。
func SummarizeNotifyLogContent(text string) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= notifyLogSummaryMaxRunes {
		return text
	}
	return string(runes[:notifyLogSummaryMaxRunes]) + "…"
}

// CreateNotifyLog 写入一条转发日志（fire-and-forget，调用方应忽略/仅记录错误，不阻塞发送流程）。
func CreateNotifyLog(log NotifyLog) error {
	if DB == nil {
		return nil
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}
	return DB.Create(&log).Error
}
