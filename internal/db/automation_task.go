package db

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AutomationActionReboot = "reboot"
	AutomationActionSMS    = "sms"

	AutomationTriggerFixedSchedule = "fixed_schedule"
	AutomationTriggerInterval      = "interval"

	AutomationIntervalHours = "hours"
	AutomationIntervalDays  = "days"

	// AutomationDeviceAll 是 DeviceID 的特殊取值，表示任务在触发时对设备池中所有设备执行。
	AutomationDeviceAll = "all"
)

// AutomationTask 是自动化中心的一条任务：在满足触发条件时对指定设备（或全部设备）执行一次动作（重启基带/发送短信）。
type AutomationTask struct {
	ID         string `gorm:"primaryKey" json:"id"`
	Name       string `gorm:"column:name" json:"name"`
	Enabled    bool   `gorm:"column:enabled" json:"enabled"`
	ActionType string `gorm:"column:action_type;index" json:"action_type"` // reboot | sms
	DeviceID   string `gorm:"column:device_id" json:"device_id"`           // 具体设备 id，或 "all"

	// 以下字段仅 ActionType == sms 时有意义
	SMSPhone       string `gorm:"column:sms_phone" json:"sms_phone,omitempty"`
	SMSContent     string `gorm:"column:sms_content" json:"sms_content,omitempty"` // 原始模板，支持 {{时间}}/{{随机字符串}}
	SMSDelayMinSec int    `gorm:"column:sms_delay_min_sec" json:"sms_delay_min_sec,omitempty"`
	SMSDelayMaxSec int    `gorm:"column:sms_delay_max_sec" json:"sms_delay_max_sec,omitempty"`
	SMSRetryCount  int    `gorm:"column:sms_retry_count" json:"sms_retry_count,omitempty"`

	TriggerType   string `gorm:"column:trigger_type" json:"trigger_type"` // fixed_schedule | interval
	FixedTimes    string `gorm:"column:fixed_times" json:"-"`             // JSON []string，"HH:MM"
	Weekdays      string `gorm:"column:weekdays" json:"-"`                // JSON []int，1=一...7=日；为空表示每天
	IntervalValue int    `gorm:"column:interval_value" json:"interval_value,omitempty"`
	IntervalUnit  string `gorm:"column:interval_unit" json:"interval_unit,omitempty"` // hours | days

	LastRunAt *time.Time `gorm:"column:last_run_at" json:"last_run_at,omitempty"`
	NextRunAt *time.Time `gorm:"column:next_run_at;index" json:"next_run_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AutomationTask) TableName() string { return "automation_tasks" }

// Times 解码 FixedTimes 为 "HH:MM" 列表。
func (t *AutomationTask) Times() []string {
	if t == nil || strings.TrimSpace(t.FixedTimes) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(t.FixedTimes), &out); err != nil {
		return nil
	}
	return out
}

// SetTimes 编码 "HH:MM" 列表到 FixedTimes。
func (t *AutomationTask) SetTimes(times []string) {
	if t == nil {
		return
	}
	if len(times) == 0 {
		t.FixedTimes = "[]"
		return
	}
	b, err := json.Marshal(times)
	if err != nil {
		t.FixedTimes = "[]"
		return
	}
	t.FixedTimes = string(b)
}

// WeekdaySet 解码 Weekdays 为 1-7（1=一...7=日）的集合；为空表示每天都触发。
func (t *AutomationTask) WeekdaySet() []int {
	if t == nil || strings.TrimSpace(t.Weekdays) == "" {
		return nil
	}
	var out []int
	if err := json.Unmarshal([]byte(t.Weekdays), &out); err != nil {
		return nil
	}
	return out
}

// SetWeekdaySet 编码 1-7 的星期集合到 Weekdays；传空切片表示每天都触发。
func (t *AutomationTask) SetWeekdaySet(days []int) {
	if t == nil {
		return
	}
	if len(days) == 0 {
		t.Weekdays = "[]"
		return
	}
	b, err := json.Marshal(days)
	if err != nil {
		t.Weekdays = "[]"
		return
	}
	t.Weekdays = string(b)
}

// ListAutomationTasks 列出全部自动化任务，按创建时间排序。
func ListAutomationTasks() ([]AutomationTask, error) {
	if DB == nil {
		return nil, nil
	}
	var out []AutomationTask
	if err := DB.Order("created_at asc").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// GetAutomationTask 按 ID 读取单个任务；不存在返回 (nil, nil)。
func GetAutomationTask(id string) (*AutomationTask, error) {
	id = strings.TrimSpace(id)
	if id == "" || DB == nil {
		return nil, nil
	}
	var out AutomationTask
	err := DB.First(&out, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

// UpsertAutomationTask 创建或更新一条任务；ID 为空时自动生成。
func UpsertAutomationTask(t AutomationTask) (AutomationTask, error) {
	if DB == nil {
		return AutomationTask{}, errors.New("db 未初始化")
	}
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return AutomationTask{}, errors.New("name 不能为空")
	}
	now := time.Now()
	if strings.TrimSpace(t.ID) == "" {
		t.ID = uuid.NewString()
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if err := DB.Save(&t).Error; err != nil {
		return AutomationTask{}, err
	}
	return t, nil
}

// DeleteAutomationTask 删除一条任务（运行日志中的历史记录保留，不级联删除）。
func DeleteAutomationTask(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("empty id")
	}
	if DB == nil {
		return nil
	}
	return DB.Delete(&AutomationTask{}, "id = ?", id).Error
}

// ListDueAutomationTasks 列出已启用且下次运行时间已到的任务，供调度器轮询调用。
func ListDueAutomationTasks(now time.Time) ([]AutomationTask, error) {
	if DB == nil {
		return nil, nil
	}
	var out []AutomationTask
	err := DB.Where("enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", true, now).
		Find(&out).Error
	return out, err
}
