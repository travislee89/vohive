package db

import "time"

func GetSMSContacts(limit int, beforeTs *time.Time, beforePeer string) ([]SMSContact, error) {
	if DB == nil {
		return []SMSContact{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := DB.Model(&SMSContact{})
	if beforeTs != nil && !beforeTs.IsZero() {
		if beforePeer != "" {
			q = q.Where("last_timestamp < ? OR (last_timestamp = ? AND peer < ?)", *beforeTs, *beforeTs, beforePeer)
		} else {
			q = q.Where("last_timestamp < ?", *beforeTs)
		}
	}

	var out []SMSContact
	err := q.Order("last_timestamp desc, peer desc").Limit(limit).Find(&out).Error
	return out, err
}

func GetSMSContactsByIMSI(imsi string, limit int, beforeTs *time.Time, beforePeer string) ([]SMSContact, error) {
	if DB == nil {
		return []SMSContact{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := DB.Model(&SMSContact{}).Where("imsi = ?", imsi)
	if beforeTs != nil && !beforeTs.IsZero() {
		if beforePeer != "" {
			q = q.Where("last_timestamp < ? OR (last_timestamp = ? AND peer < ?)", *beforeTs, *beforeTs, beforePeer)
		} else {
			q = q.Where("last_timestamp < ?", *beforeTs)
		}
	}

	var out []SMSContact
	err := q.Order("last_timestamp desc, peer desc").Limit(limit).Find(&out).Error
	return out, err
}

func GetSMSContactsByICCID(iccid string, limit int, beforeTs *time.Time, beforePeer string) ([]SMSContact, error) {
	if DB == nil {
		return []SMSContact{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := DB.Model(&SMSContact{}).Where("iccid = ?", iccid)
	if beforeTs != nil && !beforeTs.IsZero() {
		if beforePeer != "" {
			q = q.Where("last_timestamp < ? OR (last_timestamp = ? AND peer < ?)", *beforeTs, *beforeTs, beforePeer)
		} else {
			q = q.Where("last_timestamp < ?", *beforeTs)
		}
	}

	var out []SMSContact
	err := q.Order("last_timestamp desc, peer desc").Limit(limit).Find(&out).Error
	return out, err
}

// SumSMSUnreadCount 返回全部会话未读短信总数，供通知中心汇总徽标使用。
func SumSMSUnreadCount() (int64, error) {
	if DB == nil {
		return 0, nil
	}
	var total int64
	err := DB.Model(&SMSContact{}).Select("COALESCE(SUM(unread_count), 0)").Scan(&total).Error
	return total, err
}

// GetUnreadSMSContacts 返回存在未读消息的会话，按最后消息时间倒序，供通知中心信息流使用。
func GetUnreadSMSContacts(limit int) ([]SMSContact, error) {
	if DB == nil {
		return []SMSContact{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	var out []SMSContact
	err := DB.Where("unread_count > 0").Order("last_timestamp desc").Limit(limit).Find(&out).Error
	return out, err
}

// ResetSMSContactUnread 将指定会话的未读计数清零。
func ResetSMSContactUnread(imsi, peer string) error {
	if DB == nil {
		return nil
	}
	return DB.Model(&SMSContact{}).Where("imsi = ? AND peer = ?", imsi, peer).Update("unread_count", 0).Error
}

// ResetAllSMSUnread 清零全部会话的未读计数，供通知中心「清理全部」使用。
func ResetAllSMSUnread() error {
	if DB == nil {
		return nil
	}
	return DB.Model(&SMSContact{}).Where("unread_count > 0").Update("unread_count", 0).Error
}

func GetSMSByIMSIAndPeer(imsi string, peer string, limit int, beforeTs *time.Time, beforeID uint) ([]SMS, error) {
	if DB == nil {
		return []SMS{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := DB.Model(&SMS{}).Where("imsi = ? AND peer = ?", imsi, peer)
	if beforeTs != nil && !beforeTs.IsZero() && beforeID > 0 {
		q = q.Where("timestamp < ? OR (timestamp = ? AND id < ?)", *beforeTs, *beforeTs, beforeID)
	} else if beforeTs != nil && !beforeTs.IsZero() {
		q = q.Where("timestamp < ?", *beforeTs)
	} else if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}

	var out []SMS
	err := q.Order("timestamp desc, id desc").Limit(limit).Find(&out).Error
	return out, err
}

func GetSMSByICCIDAndPeer(iccid string, peer string, limit int, beforeTs *time.Time, beforeID uint) ([]SMS, error) {
	if DB == nil {
		return []SMS{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := DB.Model(&SMS{}).Where("iccid = ? AND peer = ?", iccid, peer)
	if beforeTs != nil && !beforeTs.IsZero() && beforeID > 0 {
		q = q.Where("timestamp < ? OR (timestamp = ? AND id < ?)", *beforeTs, *beforeTs, beforeID)
	} else if beforeTs != nil && !beforeTs.IsZero() {
		q = q.Where("timestamp < ?", *beforeTs)
	} else if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}

	var out []SMS
	err := q.Order("timestamp desc, id desc").Limit(limit).Find(&out).Error
	return out, err
}
