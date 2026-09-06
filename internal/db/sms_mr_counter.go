package db

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SMSMRCounter 持久化每个设备的 TP-MR（TP-Message-Reference）计数器，
// 避免进程重启后计数器归零导致新发短信与旧的 sms_status_reports 记录发生 MR 冲突。
type SMSMRCounter struct {
	DeviceID string `gorm:"column:device_id;primaryKey" json:"device_id"`
	LastMR   int    `gorm:"column:last_mr" json:"last_mr"`
}

func (SMSMRCounter) TableName() string { return "sms_mr_counters" }

// NextTPMR 返回指定设备下一个可用的 TP-MR（0-255 循环）。
// TP-MR 按 3GPP TS 23.040 要求以 TPDU 为单位递增，而非以整条（可能分片的）消息为单位。
//
// 依赖 Init() 中 sqlDB.SetMaxOpenConns(1)：由于全局只有一条数据库连接，
// 事务内的读-改-写不会与其他 goroutine 的同类事务交叠，天然避免了竞态。
func NextTPMR(deviceID string) (int, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || DB == nil {
		return 0, nil
	}

	var next int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var cur SMSMRCounter
		err := tx.Where("device_id = ?", deviceID).First(&cur).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		next = (cur.LastMR + 1) % 256
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "device_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_mr"}),
		}).Create(&SMSMRCounter{DeviceID: deviceID, LastMR: next}).Error
	})
	if err != nil {
		return 0, err
	}
	return next, nil
}
