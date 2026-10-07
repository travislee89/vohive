package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

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

// MarkSMSRead 将单条未读的接收短信标记为已读，并同步减少所在会话的未读计数（不低于 0）。
// 返回 false 表示短信不存在、不是接收短信或本就已读，调用方可据此给出不同提示。
func MarkSMSRead(id uint) (bool, error) {
	if DB == nil {
		return false, nil
	}
	var marked bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sms SMS
		if err := tx.Select("id", "imsi", "peer").
			Where("id = ? AND type = ? AND status = ?", id, 1, 0).
			First(&sms).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if err := tx.Model(&SMS{}).Where("id = ?", sms.ID).Update("status", 1).Error; err != nil {
			return err
		}
		if err := tx.Model(&SMSContact{}).
			Where("imsi = ? AND peer = ? AND unread_count > 0", sms.IMSI, sms.Peer).
			Update("unread_count", gorm.Expr("unread_count - 1")).Error; err != nil {
			return err
		}
		marked = true
		return nil
	})
	return marked, err
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
