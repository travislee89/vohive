package db

import (
	"path/filepath"
	"testing"
	"time"
)

func openQuotaTestDB(t *testing.T) {
	t.Helper()
	if err := Init(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("Init() error=%v", err)
	}
}

func TestBillingPeriodForNaturalMonth(t *testing.T) {
	// 计费日 0/未设 → 按自然月起始
	now := time.Date(2026, 5, 26, 10, 30, 0, 0, time.UTC)
	start, end := BillingPeriodFor(now, 0, "UTC")
	wantStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("got [%v,%v) want [%v,%v)", start, end, wantStart, wantEnd)
	}
}

func TestBillingPeriodForDay15(t *testing.T) {
	// 计费日 15，5月10日 → 周期起始在上个月15日，到本月15日
	now := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	start, end := BillingPeriodFor(now, 15, "UTC")
	wantStart := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("got [%v,%v) want [%v,%v)", start, end, wantStart, wantEnd)
	}
	// 5月16日 → 本月15日起始，到下月15日
	now2 := time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC)
	s2, e2 := BillingPeriodFor(now2, 15, "UTC")
	wantStart2 := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	wantEnd2 := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if !s2.Equal(wantStart2) || !e2.Equal(wantEnd2) {
		t.Fatalf("got [%v,%v) want [%v,%v)", s2, e2, wantStart2, wantEnd2)
	}
}

func TestBillingPeriodForDay31ClampsMonthEnd(t *testing.T) {
	// 计费日 31，2月（只有28天）→ 取月底2月28日
	now := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	start, end := BillingPeriodFor(now, 31, "UTC")
	wantStart := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("got [%v,%v) want [%v,%v)", start, end, wantStart, wantEnd)
	}
}

func TestBillingPeriodForTimezone(t *testing.T) {
	// 系统 UTC，计费时区 +08:00；UTC 5月1日 16:00 = +08:00 5月2日 00:00
	// 计费日 2 → 周期应为本月2日起
	now := time.Date(2026, 5, 1, 16, 0, 0, 0, time.UTC)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start, end := BillingPeriodFor(now, 2, "Asia/Shanghai")
	wantStart := time.Date(2026, 5, 2, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 6, 2, 0, 0, 0, 0, loc)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("got [%v,%v) want [%v,%v)", start, end, wantStart, wantEnd)
	}
}

func TestAccumulateCardQuotaUsageAndRollover(t *testing.T) {
	openQuotaTestDB(t)
	iccid := "8986q001"
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	// 首次累加：建档，rolledOver=true
	rolled, err := AccumulateCardQuotaUsage(iccid, 1000, 15, "UTC", now)
	if err != nil {
		t.Fatalf("Accumulate error=%v", err)
	}
	if !rolled {
		t.Fatal("首次累加应 rolledOver=true")
	}
	got, _ := GetCardQuotaUsage(iccid)
	if got.UsedBytes != 1000 {
		t.Fatalf("UsedBytes=%d want 1000", got.UsedBytes)
	}
	// 同期再累加 500：不 rollover，累加到 1500
	rolled, _ = AccumulateCardQuotaUsage(iccid, 500, 15, "UTC", now)
	if rolled {
		t.Fatal("同期再累加应 rolledOver=false")
	}
	got, _ = GetCardQuotaUsage(iccid)
	if got.UsedBytes != 1500 {
		t.Fatalf("UsedBytes=%d want 1500", got.UsedBytes)
	}
	// 跨入新计费周期：5月20日的周期是 [5/15, 6/15)，6月16日起新周期 → rollover 重置
	now2 := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	rolled, _ = AccumulateCardQuotaUsage(iccid, 200, 15, "UTC", now2)
	if !rolled {
		t.Fatal("跨周期应 rolledOver=true")
	}
	got, _ = GetCardQuotaUsage(iccid)
	if got.UsedBytes != 200 {
		t.Fatalf("新周期 UsedBytes=%d want 200", got.UsedBytes)
	}
	wantStart := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if !got.PeriodStart.Equal(wantStart) {
		t.Fatalf("PeriodStart=%v want %v", got.PeriodStart, wantStart)
	}
}

func TestIsCardQuotaExceededAutoStopOff(t *testing.T) {
	openQuotaTestDB(t)
	iccid := "8986q002"
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	_, _ = AccumulateCardQuotaUsage(iccid, 999_999_999, 15, "UTC", now)
	// autoStop=false：即使超 quota 也 Exceeded=false
	res := IsCardQuotaExceeded(iccid, 1000, 1000, false, 15, "UTC", now)
	if res.Exceeded {
		t.Fatal("autoStop=false 不应拦截")
	}
	if res.UsedBytes != 999_999_999 {
		t.Fatalf("UsedBytes=%d", res.UsedBytes)
	}
}

func TestIsCardQuotaExceededThresholdSeparateFromQuota(t *testing.T) {
	openQuotaTestDB(t)
	iccid := "8986q003"
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	_, _ = AccumulateCardQuotaUsage(iccid, 500, 15, "UTC", now)
	// quota=1000，threshold=400（独立阈值）→ 已用500>=400 → 超限
	res := IsCardQuotaExceeded(iccid, 1000, 400, true, 15, "UTC", now)
	if !res.Exceeded {
		t.Fatal("已用500>=阈值400 应超限")
	}
	if res.Threshold != 400 {
		t.Fatalf("Threshold=%d want 400", res.Threshold)
	}
	if res.QuotaBytes != 1000 {
		t.Fatalf("QuotaBytes=%d want 1000", res.QuotaBytes)
	}
	// 已用 500 < quota 1000 但 >= threshold 400：证明阈值与套餐独立
}

func TestIsCardQuotaExceededFallsBackToQuota(t *testing.T) {
	openQuotaTestDB(t)
	iccid := "8986q004"
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	_, _ = AccumulateCardQuotaUsage(iccid, 1500, 15, "UTC", now)
	// threshold=0 → 回退用 quota=1000；已用1500>=1000 → 超限
	res := IsCardQuotaExceeded(iccid, 1000, 0, true, 15, "UTC", now)
	if !res.Exceeded {
		t.Fatal("回退 quota 后已用1500>=1000 应超限")
	}
	if res.Threshold != 1000 {
		t.Fatalf("Threshold=%d want 1000", res.Threshold)
	}
}

func TestIsCardQuotaExceededNoLimitConfigured(t *testing.T) {
	openQuotaTestDB(t)
	iccid := "8986q005"
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	_, _ = AccumulateCardQuotaUsage(iccid, 9999, 15, "UTC", now)
	// quota=0, threshold=0 → 未配置上限，不拦截
	res := IsCardQuotaExceeded(iccid, 0, 0, true, 15, "UTC", now)
	if res.Exceeded {
		t.Fatal("未配置上限不应拦截")
	}
}
