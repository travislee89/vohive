package db

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CardPolicy 是跟随卡(ICCID)走的可配置策略。SMS 恒开、SMSC 动态取，均不在此。
//
// 流量限制字段（跟卡走，per-ICCID）：
//   - QuotaBytes:             套餐流量上限（字节）。0 表示未设套餐流量。
//   - BillingDay:             计费日（1-31）。0 表示未设，按自然月起始计。
//   - BillingTimezone:        计费时区（IANA 名或 UTC 偏移如 "+08:00"）。空串表示跟随系统时区。
//   - AutoStopEnabled:        达到使用量阈值后是否自动关闭/拦截网络的开关。
//   - AutoStopThresholdBytes: 触发自动关闭的「使用量阈值」（字节），与 QuotaBytes 独立。
//                             通常略大于 QuotaBytes，留出漏记流量的余量。0 表示未设阈值。
type CardPolicy struct {
	ICCID                string    `gorm:"column:iccid;primaryKey" json:"iccid"`
	NetworkEnabled       bool      `gorm:"column:network_enabled" json:"network_enabled"`
	VoWiFiEnabled        bool      `gorm:"column:vowifi_enabled" json:"vowifi_enabled"`
	AirplaneEnabled      bool      `gorm:"column:airplane_enabled" json:"airplane_enabled"`
	IPVersion            string    `gorm:"column:ip_version" json:"ip_version"`
	APN                  string    `gorm:"column:apn" json:"apn"`
	Source               string    `gorm:"column:source" json:"source"` // auto | user
	RoamingDataEnabled   bool      `gorm:"column:roaming_data_enabled" json:"roaming_data_enabled"` // 漫游时是否允许开启蜂窝数据网络，默认关闭
	QuotaEnabled         bool      `gorm:"column:quota_enabled" json:"quota_enabled"`
	QuotaBytes           int64     `gorm:"column:quota_bytes" json:"quota_bytes"`
	BillingDay           int       `gorm:"column:billing_day" json:"billing_day"`
	BillingTimezone      string    `gorm:"column:billing_timezone" json:"billing_timezone"`
	AutoStopEnabled      bool      `gorm:"column:auto_stop_enabled" json:"auto_stop_enabled"`
	AutoStopThresholdBytes int64   `gorm:"column:auto_stop_threshold_bytes" json:"auto_stop_threshold_bytes"`
	CreatedAt            time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (CardPolicy) TableName() string { return "card_policies" }

// DefaultCardPolicy 是新卡自动建档用的硬编码安全默认（不落配置文件）。
func DefaultCardPolicy(iccid string) CardPolicy {
	return CardPolicy{
		ICCID:           strings.TrimSpace(iccid),
		NetworkEnabled:     false,
		VoWiFiEnabled:      false,
		AirplaneEnabled:    false,
		IPVersion:          "v4",
		APN:                "",
		Source:             "auto",
		RoamingDataEnabled: false, // 默认关闭：漫游时不开蜂窝数据
	}
}

// NormalizeCardPolicy 仅做字段归一（trim ICCID、空 ip 归一为 v4）。
// 注意：airplane_enabled 表示“用户的纯飞行意图”，独立于 vowifi——不再强制
// vowifi=on ⇒ airplane=on。VoWiFi 接管射频是运行时投影的派生行为（见
// applyPolicyToWorker / resolveAndApplyPolicy 的 VoWiFi 优先分支），不污染存储意图；
// 这样关闭 VoWiFi 后能按存储的飞行意图正确回退（之前是飞行回飞行，之前在线回在线）。
func NormalizeCardPolicy(p *CardPolicy) {
	if p == nil {
		return
	}
	p.ICCID = strings.TrimSpace(p.ICCID)
	switch strings.TrimSpace(p.IPVersion) {
	case "v4", "v6", "v4v6":
		p.IPVersion = strings.TrimSpace(p.IPVersion)
	default:
		p.IPVersion = "v4"
	}
	// 计费日裁剪到 1-31（0 表示未设）
	if p.BillingDay < 0 {
		p.BillingDay = 0
	}
	if p.BillingDay > 31 {
		p.BillingDay = 31
	}
	if p.QuotaBytes < 0 {
		p.QuotaBytes = 0
	}
	if p.AutoStopThresholdBytes < 0 {
		p.AutoStopThresholdBytes = 0
	}
	p.BillingTimezone = strings.TrimSpace(p.BillingTimezone)
}

// ErrCardPolicyNotFound 表示 DB 中没有该 ICCID 的策略行。
var ErrCardPolicyNotFound = errors.New("card policy not found")

// GetCardPolicy 读一行；缺失返回 ErrCardPolicyNotFound。
func GetCardPolicy(iccid string) (CardPolicy, error) {
	iccid = strings.TrimSpace(iccid)
	if iccid == "" || DB == nil {
		return CardPolicy{}, ErrCardPolicyNotFound
	}
	var out CardPolicy
	err := DB.Where("iccid = ?", iccid).First(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CardPolicy{}, ErrCardPolicyNotFound
	}
	return out, err
}

// UpsertCardPolicy 写入/覆盖一行（归一化不变式后落库）。
func UpsertCardPolicy(p CardPolicy) error {
	if DB == nil {
		return errors.New("db 未初始化")
	}
	NormalizeCardPolicy(&p)
	if p.ICCID == "" {
		return errors.New("ICCID 为空")
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "iccid"}},
		DoUpdates: clause.Assignments(map[string]any{
			"network_enabled":            p.NetworkEnabled,
			"vowifi_enabled":             p.VoWiFiEnabled,
			"airplane_enabled":           p.AirplaneEnabled,
			"ip_version":                 p.IPVersion,
			"apn":                        p.APN,
			"source":                     p.Source,
			"roaming_data_enabled":       p.RoamingDataEnabled,
			"quota_enabled":              p.QuotaEnabled,
			"quota_bytes":              p.QuotaBytes,
			"billing_day":              p.BillingDay,
			"billing_timezone":         p.BillingTimezone,
			"auto_stop_enabled":        p.AutoStopEnabled,
			"auto_stop_threshold_bytes": p.AutoStopThresholdBytes,
			"updated_at":               p.UpdatedAt,
		}),
	}).Create(&p).Error
}

// ResolveCardPolicy 解析 ICCID→策略；缺失则按默认模板自动建档并返回。
func ResolveCardPolicy(iccid string) (CardPolicy, error) {
	got, err := GetCardPolicy(iccid)
	if err == nil {
		return got, nil
	}
	if !errors.Is(err, ErrCardPolicyNotFound) {
		return CardPolicy{}, err
	}
	def := DefaultCardPolicy(iccid)
	if def.ICCID == "" {
		return CardPolicy{}, ErrCardPolicyNotFound
	}
	if err := UpsertCardPolicy(def); err != nil {
		return CardPolicy{}, err
	}
	return GetCardPolicy(iccid)
}
