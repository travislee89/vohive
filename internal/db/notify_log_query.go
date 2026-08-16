package db

import (
	"strings"
	"time"
)

const (
	notifyLogDefaultPageSize = 20
	notifyLogMaxPageSize     = 100
)

// ListNotifyLogs 分页查询转发日志（页码分页，配合管理端表格 + el-pagination）。
func ListNotifyLogs(page, pageSize int, messageType, status, keyword string, start, end *time.Time) ([]NotifyLog, int64, error) {
	if DB == nil {
		return nil, 0, nil
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = notifyLogDefaultPageSize
	}
	if pageSize > notifyLogMaxPageSize {
		pageSize = notifyLogMaxPageSize
	}

	q := DB.Model(&NotifyLog{})
	if messageType = strings.TrimSpace(messageType); messageType != "" {
		q = q.Where("message_type = ?", messageType)
	}
	if status = strings.TrimSpace(status); status != "" {
		q = q.Where("status = ?", status)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("content_summary LIKE ? OR matched_rule_name LIKE ?", like, like)
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

	var out []NotifyLog
	if err := q.Order("timestamp desc, id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// DeleteNotifyLogsBefore 删除指定时间之前的所有转发日志（自动清理用）。
func DeleteNotifyLogsBefore(cutoff time.Time) error {
	if DB == nil {
		return nil
	}
	return DB.Where("timestamp < ?", cutoff).Delete(&NotifyLog{}).Error
}

// RunNotifyLogRetentionOnce 读取自动清理设置并按需删除过期转发日志；供定时任务调用。
func RunNotifyLogRetentionOnce() error {
	settings, err := GetNotifySettings()
	if err != nil {
		return err
	}
	if !settings.AutoCleanupEnabled || settings.RetentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -settings.RetentionDays)
	return DeleteNotifyLogsBefore(cutoff)
}

// DeleteNotifyLogs 按过滤条件手动删除转发日志（高级清理），返回删除行数。
func DeleteNotifyLogs(messageType, status string, before *time.Time) (int64, error) {
	if DB == nil {
		return 0, nil
	}
	q := DB.Model(&NotifyLog{})
	if messageType = strings.TrimSpace(messageType); messageType != "" {
		q = q.Where("message_type = ?", messageType)
	}
	if status = strings.TrimSpace(status); status != "" {
		q = q.Where("status = ?", status)
	}
	if before != nil && !before.IsZero() {
		q = q.Where("timestamp < ?", *before)
	}
	res := q.Delete(&NotifyLog{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}
