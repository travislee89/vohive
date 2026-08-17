package device

import "time"

// Notifier 定义了设备池需要的事件通知接口，
// 用于解耦具体的通知实现（如 Telegram 或 Webhook）
type Notifier interface {
	NotifySMS(deviceID, sender, content string, timestamp time.Time)
	NotifyIPRotated(deviceID, oldIP, newIP string, duration time.Duration)
	NotifyRaw(msg string)
}

type SMSSourceNotifier interface {
	NotifySMSWithSource(deviceID, sender, content, source string, timestamp time.Time)
}

// SMSIDNotifier 是 Notifier 的可选扩展：调用方明确知道刚落库的 SMS 行 ID 时使用，
// 使转发结果可以回写到该行；实现方（notify.Manager）需类型断言判断是否可用。
type SMSIDNotifier interface {
	NotifySMSWithID(smsID uint, deviceID, sender, content, source string, timestamp time.Time)
}
