package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// NotifySettings 是通知中心的单行全局设置（转发日志自动清理策略 + 默认规则播种标记）。
type NotifySettings struct {
	ID                 uint      `gorm:"primaryKey" json:"-"` // 恒为 1
	AutoCleanupEnabled bool      `gorm:"column:auto_cleanup_enabled" json:"auto_cleanup_enabled"`
	RetentionDays      int       `gorm:"column:retention_days" json:"retention_days"`
	DefaultRuleSeeded  bool      `gorm:"column:default_rule_seeded" json:"-"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (NotifySettings) TableName() string { return "notify_settings" }

const notifySettingsSingletonID = 1

// GetNotifySettings 读取全局设置；不存在时返回默认值（不落库，由调用方按需 Upsert）。
func GetNotifySettings() (NotifySettings, error) {
	def := NotifySettings{
		ID:                 notifySettingsSingletonID,
		AutoCleanupEnabled: true,
		RetentionDays:      30,
	}
	if DB == nil {
		return def, nil
	}
	var out NotifySettings
	err := DB.First(&out, "id = ?", notifySettingsSingletonID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return def, nil
	}
	if err != nil {
		return def, err
	}
	return out, nil
}

// UpsertNotifySettings 写入全局设置。
func UpsertNotifySettings(s NotifySettings) error {
	if DB == nil {
		return nil
	}
	s.ID = notifySettingsSingletonID
	s.UpdatedAt = time.Now()
	return DB.Save(&s).Error
}
