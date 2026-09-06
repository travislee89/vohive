package db

import "time"

// CallLog.Outcome 取值
const (
	CallOutcomeRinging  = "ringing"
	CallOutcomeAnswered = "answered"
	CallOutcomeMissed   = "missed"
)

// CallLog.Direction 取值
const (
	CallDirectionIn  = "in"
	CallDirectionOut = "out"
)

// CallLog 通话记录表（关联设备）
type CallLog struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	DeviceID   string     `gorm:"column:device_id;index:idx_calllog_device_ts,priority:1" json:"device_id"`
	ICCID      string     `gorm:"column:iccid;index" json:"iccid"`
	Number     string     `json:"number"`
	Direction  string     `gorm:"column:direction" json:"direction"` // in: 来电, out: 去电
	Outcome    string     `gorm:"column:outcome" json:"outcome"`     // ringing/answered/missed
	StartedAt  time.Time  `gorm:"column:started_at;index:idx_calllog_device_ts,priority:2,sort:desc;index:idx_calllog_ts,sort:desc" json:"started_at"`
	AnsweredAt *time.Time `gorm:"column:answered_at" json:"answered_at,omitempty"`
	EndedAt    *time.Time `gorm:"column:ended_at" json:"ended_at,omitempty"`
	Unread     bool       `gorm:"column:unread;index" json:"unread"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (CallLog) TableName() string { return "call_logs" }
