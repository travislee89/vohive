package db

import (
	"time"

	"github.com/travislee89/vohive/pkg/logger"
)

// BackfillResult 记录回填执行的结果统计
type BackfillResult struct {
	HorizonDays int `json:"horizon_days"`
	Days        int `json:"days"`
	Weeks       int `json:"weeks"`
	Months      int `json:"months"`
}

// BackfillTraffic 对最近 horizonDays 内逐天/逐周/逐月补跑上卷（幂等）。
// 参数 now 用于定位当前周期；窗口按系统时区自然边界。
// horizonDays <= 0 时使用默认值 31。
func BackfillTraffic(now time.Time, horizonDays int) (BackfillResult, error) {
	if horizonDays <= 0 {
		horizonDays = 31
	}
	if horizonDays > 93 {
		horizonDays = 93
	}

	result := BackfillResult{HorizonDays: horizonDays}

	// 计算 since（含首日）
	since := startOfTrafficDay(now.AddDate(0, 0, -(horizonDays - 1)))
	today := startOfTrafficDay(now)

	// 按天：从 since 到 today 逐天 RollupToDay
	for dayStart := since; dayStart.Before(today) || dayStart.Equal(today); dayStart = dayStart.AddDate(0, 0, 1) {
		if err := RollupToDay(dayStart); err != nil {
			logger.Warn("回填 traffic_day 失败", "day", dayStart.Format("2006-01-02"), "err", err)
		} else {
			result.Days++
		}
	}

	// 按周：以 since 所在 ISO 周起点为起点，逐周 RollupToWeek
	weekStart := startOfWeek(since)
	weekEnd := startOfWeek(today)
	if weekEnd.Before(today) {
		weekEnd = weekEnd.AddDate(0, 0, 7)
	}
	for w := weekStart; w.Before(weekEnd) || w.Equal(weekEnd); w = w.AddDate(0, 0, 7) {
		if err := RollupToWeek(w); err != nil {
			logger.Warn("回填 traffic_week 失败", "week_start", w.Format("2006-01-02"), "err", err)
		} else {
			result.Weeks++
		}
	}

	// 按月：以 since 所在自然月为起点，逐月 RollupToMonth
	monthStart := time.Date(since.Year(), since.Month(), 1, 0, 0, 0, 0, since.Location())
	monthEnd := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
	for m := monthStart; m.Before(monthEnd) || m.Equal(monthEnd); m = m.AddDate(0, 1, 0) {
		if err := RollupToMonth(m); err != nil {
			logger.Warn("回填 traffic_month 失败", "month_start", m.Format("2006-01-02"), "err", err)
		} else {
			result.Months++
		}
	}

	return result, nil
}

// startOfWeek 计算 ISO 周起点（周一 00:00）
func startOfWeek(t time.Time) time.Time {
	tt := startOfTrafficDay(t)
	for tt.Weekday() != time.Monday {
		tt = tt.AddDate(0, 0, -1)
	}
	return tt
}
