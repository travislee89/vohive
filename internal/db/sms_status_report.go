package db

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// SMSStatusReport 追踪一条（或一个分片）上行短信的 TP-SRR 送达报告（SMS-STATUS-REPORT）状态。
//
// 注意：这与 internal/db/sms_delivery.go 中的 SMSDelivery/SMSDeliveryPart 是完全不同的信号——
// 那是 VoWiFi/IMS 场景下的 RP-ACK/RP-ERROR（运营商是否接受了 SIP MESSAGE 提交），
// 这里是端到端的 TP-SRR（对方手机是否真正收到了短信）。
type SMSStatusReport struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	SMSID       uint       `gorm:"column:sms_id;index" json:"sms_id"`
	DeviceID    string     `gorm:"column:device_id;index:idx_ssr_device_mr" json:"device_id"`
	IMSI        string     `gorm:"column:imsi;index" json:"imsi"`
	Peer        string     `gorm:"column:peer" json:"peer"`
	PartNo      int        `gorm:"column:part_no" json:"part_no"`
	TPMR        int        `gorm:"column:tp_mr;index:idx_ssr_device_mr" json:"tp_mr"`
	RequestedAt time.Time  `gorm:"column:requested_at" json:"requested_at"`
	State       string     `gorm:"column:state;index" json:"state"`
	TPStatus    *int       `gorm:"column:tp_status" json:"tp_status,omitempty"`
	DischargeAt *time.Time `gorm:"column:discharge_at" json:"discharge_at,omitempty"`
	ReportedAt  *time.Time `gorm:"column:reported_at" json:"reported_at,omitempty"`
	RawPDUHex   string     `gorm:"column:raw_pdu_hex" json:"-"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (SMSStatusReport) TableName() string { return "sms_status_reports" }

const (
	SMSStatusReportStatePending    = "pending"
	SMSStatusReportStateDelivered  = "delivered"
	SMSStatusReportStateForwarding = "forwarding"
	SMSStatusReportStateFailed     = "failed"
	SMSStatusReportStateTimeout    = "timeout"
)

// ClassifyTPStatus 按 3GPP TS 23.040 §9.2.3.15 对 TP-Status 字节分类。
func ClassifyTPStatus(status byte) string {
	switch {
	case status <= 0x1F:
		return SMSStatusReportStateDelivered
	case status <= 0x3F:
		return SMSStatusReportStateForwarding
	default:
		return SMSStatusReportStateFailed
	}
}

// CreateSMSStatusReportPending 在发送侧成功提交一条（分片）请求了 TP-SRR 的短信后，登记一条待确认记录。
func CreateSMSStatusReportPending(smsID uint, deviceID, imsi, peer string, partNo, tpMR int, at time.Time) error {
	if DB == nil {
		return nil
	}
	if at.IsZero() {
		at = time.Now()
	}
	row := SMSStatusReport{
		SMSID:       smsID,
		DeviceID:    strings.TrimSpace(deviceID),
		IMSI:        strings.TrimSpace(imsi),
		Peer:        strings.TrimSpace(peer),
		PartNo:      partNo,
		TPMR:        tpMR,
		RequestedAt: at,
		State:       SMSStatusReportStatePending,
		CreatedAt:   at,
		UpdatedAt:   at,
	}
	return DB.Create(&row).Error
}

// MatchAndUpdateSMSStatusReport 按 (device_id, tp_mr) 找到最近一条待确认记录并写入收到的送达报告。
// TP-MR 会在 0-255 循环，因此按 requested_at 倒序只取最近一条；未匹配返回 gorm.ErrRecordNotFound，
// 调用方应视为"收到了一条无法关联的状态报告"，记日志即可，不应视为错误中断流程。
func MatchAndUpdateSMSStatusReport(deviceID string, tpMR, tpStatus int, dischargeAt, reportedAt time.Time, rawPDUHex string) (*SMSStatusReport, error) {
	if DB == nil {
		return nil, gorm.ErrRecordNotFound
	}
	deviceID = strings.TrimSpace(deviceID)
	if reportedAt.IsZero() {
		reportedAt = time.Now()
	}

	var row SMSStatusReport
	err := DB.Where("device_id = ? AND tp_mr = ? AND state IN ?", deviceID, tpMR,
		[]string{SMSStatusReportStatePending, SMSStatusReportStateForwarding}).
		Order("requested_at desc").
		First(&row).Error
	if err != nil {
		return nil, err
	}

	status := tpStatus
	state := ClassifyTPStatus(byte(status))
	updates := map[string]any{
		"state":        state,
		"tp_status":    status,
		"discharge_at": &dischargeAt,
		"reported_at":  &reportedAt,
		"raw_pdu_hex":  rawPDUHex,
		"updated_at":   reportedAt,
	}
	if err := DB.Model(&SMSStatusReport{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := DB.First(&row, row.ID).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// GetSMSStatusReportBySMSID 查询某条已发送短信（sms 表主键）的送达报告状态。
func GetSMSStatusReportBySMSID(smsID uint) (*SMSStatusReport, error) {
	if DB == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var row SMSStatusReport
	if err := DB.Where("sms_id = ?", smsID).Order("part_no asc").First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
