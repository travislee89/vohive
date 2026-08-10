package db

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CardQuotaUsage 是 per-ICCID 的当期流量用量累加行。
// 每张卡一行；PeriodStart/PeriodEnd 由计费日 + 计费时区决定；跨过 PeriodEnd 自动重置。
type CardQuotaUsage struct {
	ICCID       string    `gorm:"column:iccid;primaryKey" json:"iccid"`
	PeriodStart time.Time `gorm:"column:period_start" json:"period_start"`
	PeriodEnd   time.Time `gorm:"column:period_end" json:"period_end"`
	UsedBytes   int64     `gorm:"column:used_bytes" json:"used_bytes"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (CardQuotaUsage) TableName() string { return "card_quota_usage" }

// ErrCardQuotaUsageNotFound 表示 DB 中没有该 ICCID 的用量行（首次使用前）。
var ErrCardQuotaUsageNotFound = errors.New("card quota usage not found")

// billingLocation 解析计费时区。空串或解析失败回退 time.Local（系统时区）。
// 支持两种形式：IANA 名称（如 "Asia/Shanghai"）或 UTC 偏移（如 "+08:00"、"UTC"）。
func billingLocation(tz string) *time.Location {
	tz = strings.TrimSpace(tz)
	if tz == "" || strings.EqualFold(tz, "local") {
		return time.Local
	}
	if strings.EqualFold(tz, "UTC") || tz == "Z" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	if offset, err := parseUTCOffset(tz); err == nil {
		return time.FixedZone(tz, offset)
	}
	return time.Local
}

// parseUTCOffset 解析形如 "+08:00"、"-05:30" 的 UTC 偏移，返回秒偏移。
func parseUTCOffset(s string) (int, error) {
	if len(s) < 3 || (s[0] != '+' && s[0] != '-') {
		return 0, fmt.Errorf("invalid utc offset: %s", s)
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
	}
	rest := s[1:]
	var hh, mm int
	if i := strings.Index(rest, ":"); i >= 0 {
		if _, err := fmt.Sscanf(rest[:i], "%d", &hh); err != nil {
			return 0, err
		}
		if _, err := fmt.Sscanf(rest[i+1:], "%d", &mm); err != nil {
			return 0, err
		}
	} else {
		if _, err := fmt.Sscanf(rest, "%d", &hh); err != nil {
			return 0, err
		}
	}
	return sign * (hh*3600 + mm*60), nil
}

// daysInMonth 返回 y/m 月的天数。
func daysInMonth(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// BillingPeriodFor 在计费时区内，按计费日计算当前用量周期 [start, end)。
// billingDay 为 1-31；当月无该日（如 2 月 30 日）取当月最后一天。
// billingDay <= 0 时按自然月起始（每月 1 日）。
// 返回的 time.Time 携带计费时区的 Location 信息。
func BillingPeriodFor(now time.Time, billingDay int, tz string) (start, end time.Time) {
	loc := billingLocation(tz)
	t := now.In(loc)
	year, month, _ := t.Date()
	day := billingDay
	if day <= 0 {
		day = 1
	}
	// 若当前日早于计费日，说明还没到本月的计费日，账单起始在上个月。
	if t.Day() < day {
		if month == time.January {
			year--
			month = time.December
		} else {
			month--
		}
	}
	dim := daysInMonth(year, month)
	if day > dim {
		day = dim
	}
	start = time.Date(year, month, day, 0, 0, 0, 0, loc)
	// end = 下个计费月的同一天；次月不足该日时取次月最后一天。
	nextYear, nextMonth := year, month+1
	if month == time.December {
		nextYear = year + 1
		nextMonth = time.January
	}
	endDay := day
	if dim := daysInMonth(nextYear, nextMonth); endDay > dim {
		endDay = dim
	}
	end = time.Date(nextYear, nextMonth, endDay, 0, 0, 0, 0, loc)
	return start, end
}

// GetCardQuotaUsage 读一行；缺失返回 ErrCardQuotaUsageNotFound。
func GetCardQuotaUsage(iccid string) (CardQuotaUsage, error) {
	iccid = strings.TrimSpace(iccid)
	if iccid == "" || DB == nil {
		return CardQuotaUsage{}, ErrCardQuotaUsageNotFound
	}
	var out CardQuotaUsage
	err := DB.Where("iccid = ?", iccid).First(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CardQuotaUsage{}, ErrCardQuotaUsageNotFound
	}
	return out, err
}

// AccumulateCardQuotaUsage 把 delta 字节累加到该卡的当期用量。
// 若行不存在或 now >= PeriodEnd（已跨入新计费周期），则重置为新一轮周期并累加 delta，
// 此时返回 rolledOver=true。
// 用原子 UPSERT 保证并发安全：当 excluded.period_start 与现有一致则累加，否则覆盖。
func AccumulateCardQuotaUsage(iccid string, delta int64, billingDay int, tz string, now time.Time) (rolledOver bool, err error) {
	iccid = strings.TrimSpace(iccid)
	if iccid == "" {
		return false, errors.New("ICCID 为空")
	}
	if DB == nil {
		return false, errors.New("db 未初始化")
	}
	if delta < 0 {
		delta = 0
	}
	start, end := BillingPeriodFor(now, billingDay, tz)
	cur, getErr := GetCardQuotaUsage(iccid)
	if getErr == nil && !cur.PeriodEnd.IsZero() && !now.Before(cur.PeriodEnd) {
		rolledOver = true
	}
	if errors.Is(getErr, ErrCardQuotaUsageNotFound) {
		rolledOver = true
	}
	row := CardQuotaUsage{
		ICCID:       iccid,
		PeriodStart: start,
		PeriodEnd:   end,
		UsedBytes:   delta,
		UpdatedAt:   now,
	}
	if err = DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "iccid"}},
		DoUpdates: clause.Assignments(map[string]any{
			"used_bytes":   gorm.Expr("CASE WHEN excluded.period_start = card_quota_usage.period_start THEN card_quota_usage.used_bytes + excluded.used_bytes ELSE excluded.used_bytes END"),
			"period_start": gorm.Expr("excluded.period_start"),
			"period_end":   gorm.Expr("excluded.period_end"),
			"updated_at":   now,
		}),
	}).Create(&row).Error; err != nil {
		return false, err
	}
	return rolledOver, nil
}

// QuotaCheckResult 是 IsCardQuotaExceeded 的返回结构。
type QuotaCheckResult struct {
	Exceeded    bool      `json:"exceeded"`
	UsedBytes   int64     `json:"used_bytes"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	Threshold   int64     `json:"threshold_bytes"`
	QuotaBytes  int64     `json:"quota_bytes"`
	AutoStop    bool      `json:"auto_stop_enabled"`
}

// IsCardQuotaExceeded 综合判定某卡当前是否应被拦截开网。
//   - autoStop=false → Exceeded 恒为 false（仅做进度展示，仍返回用量与周期）。
//   - autoStop=true  → 比较 UsedBytes >= threshold；threshold>0 用 threshold，否则回退 quotaBytes。
//   - quotaBytes/threshold 均为 0 时，Exceeded=false（未配置上限）。
func IsCardQuotaExceeded(iccid string, quotaBytes, thresholdBytes int64, autoStop bool, billingDay int, tz string, now time.Time) QuotaCheckResult {
	res := QuotaCheckResult{
		QuotaBytes: quotaBytes,
		Threshold:  thresholdBytes,
		AutoStop:   autoStop,
	}
	start, end := BillingPeriodFor(now, billingDay, tz)
	res.PeriodStart = start
	res.PeriodEnd = end
	if u, err := GetCardQuotaUsage(iccid); err == nil {
		res.UsedBytes = u.UsedBytes
	}
	if !autoStop {
		return res
	}
	effective := thresholdBytes
	if effective <= 0 {
		effective = quotaBytes
	}
	if effective <= 0 {
		return res
	}
	res.Threshold = effective
	res.Exceeded = res.UsedBytes >= effective
	return res
}

