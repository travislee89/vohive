package db

import "time"

// CreateRingingCallLog 在振铃/拨号开始时创建一条通话记录，来电默认标记为未读。
func CreateRingingCallLog(deviceID, iccid, number, direction string, at time.Time) (*CallLog, error) {
	if DB == nil {
		return nil, nil
	}
	log := &CallLog{
		DeviceID:  deviceID,
		ICCID:     iccid,
		Number:    number,
		Direction: direction,
		Outcome:   CallOutcomeRinging,
		StartedAt: at,
		Unread:    direction == CallDirectionIn,
	}
	if err := DB.Create(log).Error; err != nil {
		return nil, err
	}
	return log, nil
}

// MarkCallLogConnected 标记通话已接通。
func MarkCallLogConnected(id uint, at time.Time) error {
	if DB == nil || id == 0 {
		return nil
	}
	return DB.Model(&CallLog{}).Where("id = ?", id).Updates(map[string]any{
		"outcome":     CallOutcomeAnswered,
		"answered_at": at,
	}).Error
}

// MarkCallLogEnded 标记通话结束；若从未接通过的来电，则记为未接。
func MarkCallLogEnded(id uint, at time.Time) error {
	if DB == nil || id == 0 {
		return nil
	}
	var log CallLog
	if err := DB.First(&log, id).Error; err != nil {
		return err
	}
	outcome := CallOutcomeAnswered
	if log.AnsweredAt == nil && log.Direction == CallDirectionIn {
		outcome = CallOutcomeMissed
	}
	return DB.Model(&CallLog{}).Where("id = ?", id).Updates(map[string]any{
		"outcome":  outcome,
		"ended_at": at,
	}).Error
}

// CountUnreadCalls 返回全部设备未读来电数。
func CountUnreadCalls() (int64, error) {
	if DB == nil {
		return 0, nil
	}
	var count int64
	err := DB.Model(&CallLog{}).Where("unread = ?", true).Count(&count).Error
	return count, err
}

// ListRecentUnreadCalls 返回最近的未读来电，供通知中心展示。
func ListRecentUnreadCalls(limit int) ([]CallLog, error) {
	if DB == nil {
		return []CallLog{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	var out []CallLog
	err := DB.Where("unread = ?", true).Order("started_at desc").Limit(limit).Find(&out).Error
	return out, err
}

// MarkCallLogsRead 将指定 ID 的通话记录标记为已读。
func MarkCallLogsRead(ids []uint) (int64, error) {
	if DB == nil || len(ids) == 0 {
		return 0, nil
	}
	res := DB.Model(&CallLog{}).Where("id IN ? AND unread = ?", ids, true).Update("unread", false)
	return res.RowsAffected, res.Error
}

// MarkAllCallLogsReadForDevice 将指定设备全部未读通话记录标记为已读。
func MarkAllCallLogsReadForDevice(deviceID string) (int64, error) {
	if DB == nil || deviceID == "" {
		return 0, nil
	}
	res := DB.Model(&CallLog{}).Where("device_id = ? AND unread = ?", deviceID, true).Update("unread", false)
	return res.RowsAffected, res.Error
}

// MarkAllCallLogsRead 将全部设备的未读通话记录标记为已读，供通知中心「清理全部」使用。
func MarkAllCallLogsRead() (int64, error) {
	if DB == nil {
		return 0, nil
	}
	res := DB.Model(&CallLog{}).Where("unread = ?", true).Update("unread", false)
	return res.RowsAffected, res.Error
}

// ListCallLogsByDevice 返回指定设备最近的通话记录（含已读/未读、来电/去电）。
func ListCallLogsByDevice(deviceID string, limit int) ([]CallLog, error) {
	if DB == nil {
		return []CallLog{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var out []CallLog
	err := DB.Where("device_id = ?", deviceID).Order("started_at desc").Limit(limit).Find(&out).Error
	return out, err
}
