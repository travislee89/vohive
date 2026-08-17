package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// AutomationSettings 是自动化中心运行日志的单行全局设置（自动清理策略），与通知中心的 NotifySettings 相互独立。
type AutomationSettings struct {
	ID                 uint      `gorm:"primaryKey" json:"-"` // 恒为 1
	AutoCleanupEnabled bool      `gorm:"column:auto_cleanup_enabled" json:"auto_cleanup_enabled"`
	RetentionDays      int       `gorm:"column:retention_days" json:"retention_days"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (AutomationSettings) TableName() string { return "automation_settings" }

const automationSettingsSingletonID = 1

// GetAutomationSettings 读取全局设置；不存在时返回默认值（不落库，由调用方按需 Upsert）。
func GetAutomationSettings() (AutomationSettings, error) {
	def := AutomationSettings{
		ID:                 automationSettingsSingletonID,
		AutoCleanupEnabled: true,
		RetentionDays:      30,
	}
	if DB == nil {
		return def, nil
	}
	var out AutomationSettings
	err := DB.First(&out, "id = ?", automationSettingsSingletonID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return def, nil
	}
	if err != nil {
		return def, err
	}
	return out, nil
}

// UpsertAutomationSettings 写入全局设置。
func UpsertAutomationSettings(s AutomationSettings) error {
	if DB == nil {
		return nil
	}
	s.ID = automationSettingsSingletonID
	s.UpdatedAt = time.Now()
	return DB.Save(&s).Error
}
