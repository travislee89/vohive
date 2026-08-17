package db

import (
	"strings"
	"time"
)

const (
	AutomationRunStatusSuccess = "success"
	AutomationRunStatusFailed  = "failed"

	AutomationTriggerSourceSchedule = "schedule"
	AutomationTriggerSourceManual   = "manual"
)

// AutomationRunLog 记录一次自动化任务的执行结果；"全部设备" 任务每个设备各写一条记录。
type AutomationRunLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TaskID        string    `gorm:"column:task_id;index" json:"task_id"`
	TaskName      string    `gorm:"column:task_name" json:"task_name"` // 快照，任务被删除后仍可读
	ActionType    string    `gorm:"column:action_type;index" json:"action_type"`
	DeviceID      string    `gorm:"column:device_id" json:"device_id,omitempty"`
	TriggerSource string    `gorm:"column:trigger_source" json:"trigger_source"` // schedule | manual
	Status        string    `gorm:"column:status;index" json:"status"`           // success | failed
	ResultSummary string    `gorm:"column:result_summary" json:"result_summary"`
	ErrorDetail   string    `gorm:"column:error_detail" json:"error_detail,omitempty"`
	Timestamp     time.Time `gorm:"column:timestamp;index:idx_automation_log_ts,sort:desc" json:"timestamp"`
}

func (AutomationRunLog) TableName() string { return "automation_run_logs" }

// CreateAutomationRunLog 写入一条运行日志（fire-and-forget，调用方应仅记录错误，不阻塞任务执行）。
func CreateAutomationRunLog(log AutomationRunLog) error {
	if DB == nil {
		return nil
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}
	return DB.Create(&log).Error
}

const (
	automationLogDefaultPageSize = 20
	automationLogMaxPageSize     = 100
)

// ListAutomationRunLogs 分页查询运行日志。
func ListAutomationRunLogs(page, pageSize int, actionType, status, keyword string, start, end *time.Time) ([]AutomationRunLog, int64, error) {
	if DB == nil {
		return nil, 0, nil
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = automationLogDefaultPageSize
	}
	if pageSize > automationLogMaxPageSize {
		pageSize = automationLogMaxPageSize
	}

	q := DB.Model(&AutomationRunLog{})
	if actionType = strings.TrimSpace(actionType); actionType != "" {
		q = q.Where("action_type = ?", actionType)
	}
	if status = strings.TrimSpace(status); status != "" {
		q = q.Where("status = ?", status)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("task_name LIKE ? OR result_summary LIKE ? OR error_detail LIKE ?", like, like, like)
	}
	if start != nil && !start.IsZero() {
		q = q.Where("timestamp >= ?", *start)
	}
	if end != nil && !end.IsZero() {
		q = q.Where("timestamp <= ?", *end)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var out []AutomationRunLog
	if err := q.Order("timestamp desc, id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// DeleteAutomationLogsBefore 删除指定时间之前的所有运行日志（自动清理用）。
func DeleteAutomationLogsBefore(cutoff time.Time) error {
	if DB == nil {
		return nil
	}
	return DB.Where("timestamp < ?", cutoff).Delete(&AutomationRunLog{}).Error
}

// RunAutomationLogRetentionOnce 读取自动清理设置并按需删除过期运行日志；供定时任务调用。
func RunAutomationLogRetentionOnce() error {
	settings, err := GetAutomationSettings()
	if err != nil {
		return err
	}
	if !settings.AutoCleanupEnabled || settings.RetentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -settings.RetentionDays)
	return DeleteAutomationLogsBefore(cutoff)
}

// DeleteAutomationLogs 按过滤条件手动删除运行日志（高级清理），返回删除行数。
func DeleteAutomationLogs(actionType, status string, before *time.Time) (int64, error) {
	if DB == nil {
		return 0, nil
	}
	q := DB.Model(&AutomationRunLog{})
	if actionType = strings.TrimSpace(actionType); actionType != "" {
		q = q.Where("action_type = ?", actionType)
	}
	if status = strings.TrimSpace(status); status != "" {
		q = q.Where("status = ?", status)
	}
	if before != nil && !before.IsZero() {
		q = q.Where("timestamp < ?", *before)
	}
	res := q.Delete(&AutomationRunLog{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}
