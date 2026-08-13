package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBackfillTrafficBasic(t *testing.T) {
	now := initTrafficTestDB(t)

	day0 := startOfTrafficDay(now.AddDate(0, 0, -4))
	day1 := startOfTrafficDay(now.AddDate(0, 0, -3))
	day2 := startOfTrafficDay(now.AddDate(0, 0, -2))
	day3 := startOfTrafficDay(now.AddDate(0, 0, -1))

	mustInsertTrafficDay(t, now, day0, "dev-a@wwan0", false, 1000)
	mustInsertTrafficDay(t, now, day0, "dev-a@wwan0", true, 500)
	mustInsertTrafficDay(t, now, day1, "dev-a@wwan0", false, 2000)
	mustInsertTrafficDay(t, now, day1, "dev-a@wwan0", true, 1000)
	mustInsertTrafficDay(t, now, day2, "dev-a@wwan0", false, 1500)
	mustInsertTrafficDay(t, now, day2, "dev-a@wwan0", true, 750)
	mustInsertTrafficDay(t, now, day3, "dev-a@wwan0", false, 3000)
	mustInsertTrafficDay(t, now, day3, "dev-a@wwan0", true, 1500)

	result, err := BackfillTraffic(now, 5)
	if err != nil {
		t.Fatalf("BackfillTraffic() error = %v", err)
	}

	if result.HorizonDays != 5 {
		t.Errorf("HorizonDays = %d, want 5", result.HorizonDays)
	}
	if result.Days != 5 {
		t.Errorf("Days = %d, want 5", result.Days)
	}
	if result.Weeks < 1 {
		t.Errorf("Weeks = %d, want >= 1", result.Weeks)
	}
	if result.Months < 1 {
		t.Errorf("Months = %d, want >= 1", result.Months)
	}

	var weeks []TrafficWeek
	if err := DB.Where("resource = ? AND tag = ?", "iface", "dev-a@wwan0").Find(&weeks).Error; err != nil {
		t.Fatalf("query traffic_week failed: %v", err)
	}
	if len(weeks) == 0 {
		t.Fatalf("no traffic_week rows generated")
	}

	var months []TrafficMonth
	if err := DB.Where("resource = ? AND tag = ?", "iface", "dev-a@wwan0").Find(&months).Error; err != nil {
		t.Fatalf("query traffic_month failed: %v", err)
	}
	if len(months) == 0 {
		t.Fatalf("no traffic_month rows generated")
	}
}

func TestBackfillTrafficIdempotent(t *testing.T) {
	now := initTrafficTestDB(t)

	day0 := startOfTrafficDay(now.AddDate(0, 0, -2))
	mustInsertTrafficDay(t, now, day0, "dev-b@wwan0", false, 100)
	mustInsertTrafficDay(t, now, day0, "dev-b@wwan0", true, 50)

	result1, err := BackfillTraffic(now, 3)
	if err != nil {
		t.Fatalf("BackfillTraffic(1) error = %v", err)
	}

	var weekCount1 int64
	DB.Model(&TrafficWeek{}).Count(&weekCount1)

	result2, err := BackfillTraffic(now, 3)
	if err != nil {
		t.Fatalf("BackfillTraffic(2) error = %v", err)
	}

	if result1.Days != result2.Days || result1.Weeks != result2.Weeks || result1.Months != result2.Months {
		t.Errorf("idempotent mismatch: first=%+v second=%+v", result1, result2)
	}

	var weekCount2 int64
	DB.Model(&TrafficWeek{}).Count(&weekCount2)
	if weekCount1 != weekCount2 {
		t.Errorf("week count changed after idempotent run: before=%d after=%d", weekCount1, weekCount2)
	}
}

func TestBackfillTrafficBoundaryCrossMonday(t *testing.T) {
	saturday := time.Date(2026, time.March, 28, 15, 0, 0, 0, time.UTC)
	now := initTrafficTestDBWithTime(t, saturday)

	friday := startOfTrafficDay(saturday.AddDate(0, 0, -1))
	sunday := startOfTrafficDay(saturday)

	mustInsertTrafficDay(t, now, friday, "dev-c@wwan0", false, 500)
	mustInsertTrafficDay(t, now, friday, "dev-c@wwan0", true, 250)
	mustInsertTrafficDay(t, now, sunday, "dev-c@wwan0", false, 300)
	mustInsertTrafficDay(t, now, sunday, "dev-c@wwan0", true, 150)

	_, err := BackfillTraffic(saturday, 5)
	if err != nil {
		t.Fatalf("BackfillTraffic() error = %v", err)
	}

	var weeks []TrafficWeek
	if err := DB.Where("resource = ? AND tag = ?", "iface", "dev-c@wwan0").Find(&weeks).Error; err != nil {
		t.Fatalf("query traffic_week failed: %v", err)
	}
	if len(weeks) == 0 {
		t.Fatalf("no traffic_week rows generated across Monday boundary")
	}

	for _, w := range weeks {
		if w.PeriodStart.Weekday() != time.Monday {
			t.Errorf("week period_start should be Monday, got %s", w.PeriodStart.Weekday())
		}
	}
}

func TestBackfillTrafficBoundaryCrossMonth(t *testing.T) {
	lastDay := time.Date(2026, time.March, 30, 15, 0, 0, 0, time.UTC)
	now := initTrafficTestDBWithTime(t, lastDay)

	day28 := startOfTrafficDay(lastDay.AddDate(0, 0, -2))
	day29 := startOfTrafficDay(lastDay.AddDate(0, 0, -1))
	day30 := startOfTrafficDay(lastDay)

	mustInsertTrafficDay(t, now, day28, "dev-d@wwan0", false, 200)
	mustInsertTrafficDay(t, now, day28, "dev-d@wwan0", true, 100)
	mustInsertTrafficDay(t, now, day29, "dev-d@wwan0", false, 400)
	mustInsertTrafficDay(t, now, day29, "dev-d@wwan0", true, 200)
	mustInsertTrafficDay(t, now, day30, "dev-d@wwan0", false, 600)
	mustInsertTrafficDay(t, now, day30, "dev-d@wwan0", true, 300)

	_, err := BackfillTraffic(lastDay, 5)
	if err != nil {
		t.Fatalf("BackfillTraffic() error = %v", err)
	}

	var months []TrafficMonth
	if err := DB.Where("resource = ? AND tag = ?", "iface", "dev-d@wwan0").Find(&months).Error; err != nil {
		t.Fatalf("query traffic_month failed: %v", err)
	}
	if len(months) == 0 {
		t.Fatalf("no traffic_month rows generated across month boundary")
	}

	for _, m := range months {
		if m.PeriodStart.Day() != 1 {
			t.Errorf("month period_start should be 1st, got day=%d", m.PeriodStart.Day())
		}
	}
}

func TestBackfillTrafficDefaultHorizon(t *testing.T) {
	now := initTrafficTestDB(t)

	result, err := BackfillTraffic(now, 0)
	if err != nil {
		t.Fatalf("BackfillTraffic(0) error = %v", err)
	}
	if result.HorizonDays != 31 {
		t.Errorf("HorizonDays = %d, want 31 (default)", result.HorizonDays)
	}

	result, err = BackfillTraffic(now, -1)
	if err != nil {
		t.Fatalf("BackfillTraffic(-1) error = %v", err)
	}
	if result.HorizonDays != 31 {
		t.Errorf("HorizonDays = %d, want 31 (default)", result.HorizonDays)
	}
}

func TestBackfillTrafficMaxHorizon(t *testing.T) {
	now := initTrafficTestDB(t)

	result, err := BackfillTraffic(now, 100)
	if err != nil {
		t.Fatalf("BackfillTraffic(100) error = %v", err)
	}
	if result.HorizonDays != 93 {
		t.Errorf("HorizonDays = %d, want 93 (max)", result.HorizonDays)
	}
}

func initTrafficTestDBWithTime(t *testing.T, fixedTime time.Time) time.Time {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "traffic.db")
	if err := Init(dbPath); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	t.Cleanup(func() {
		DB = nil
	})
	return fixedTime
}
