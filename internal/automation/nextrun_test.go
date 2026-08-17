package automation

import (
	"testing"
	"time"

	"github.com/travislee89/vohive/internal/db"
)

func mustParse(t *testing.T, layout, value string) time.Time {
	t.Helper()
	tm, err := time.ParseInLocation(layout, value, time.Local)
	if err != nil {
		t.Fatalf("解析时间失败: %v", err)
	}
	return tm
}

func TestComputeNextIntervalRun_Hours(t *testing.T) {
	after := mustParse(t, "2006-01-02 15:04", "2026-08-17 10:00")
	task := db.AutomationTask{
		TriggerType:   db.AutomationTriggerInterval,
		IntervalValue: 3,
		IntervalUnit:  db.AutomationIntervalHours,
		CreatedAt:     after,
	}
	next, err := ComputeNextRun(task, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := after.Add(3 * time.Hour)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestComputeNextIntervalRun_Days(t *testing.T) {
	after := mustParse(t, "2006-01-02 15:04", "2026-08-17 10:00")
	lastRun := after.Add(-1 * time.Hour)
	task := db.AutomationTask{
		TriggerType:   db.AutomationTriggerInterval,
		IntervalValue: 2,
		IntervalUnit:  db.AutomationIntervalDays,
		LastRunAt:     &lastRun,
	}
	next, err := ComputeNextRun(task, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := lastRun.AddDate(0, 0, 2)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestComputeNextIntervalRun_Overdue(t *testing.T) {
	after := mustParse(t, "2006-01-02 15:04", "2026-08-17 10:00")
	lastRun := after.AddDate(0, 0, -10) // 远早于 after，模拟守护进程停机期间错过了调度
	task := db.AutomationTask{
		TriggerType:   db.AutomationTriggerInterval,
		IntervalValue: 1,
		IntervalUnit:  db.AutomationIntervalDays,
		LastRunAt:     &lastRun,
	}
	next, err := ComputeNextRun(task, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := after.AddDate(0, 0, 1)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v (should fast-forward from `after`, not burst catch-up)", next, want)
	}
}

func TestComputeNextFixedScheduleRun_MultipleTimesNoWeekdayFilter(t *testing.T) {
	// 周一 09:00，触发时刻 04:00 和 16:30，未选星期 = 每天都触发；下一次应是当天 16:30。
	after := mustParse(t, "2006-01-02 15:04", "2026-08-17 09:00")
	task := db.AutomationTask{TriggerType: db.AutomationTriggerFixedSchedule}
	task.SetTimes([]string{"04:00", "16:30"})
	task.SetWeekdaySet(nil)

	next, err := ComputeNextRun(task, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := mustParse(t, "2006-01-02 15:04", "2026-08-17 16:30")
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestComputeNextFixedScheduleRun_PastAllTimesToday(t *testing.T) {
	// 当天所有触发时刻都已过去，应跳到下一天最早的触发时刻。
	after := mustParse(t, "2006-01-02 15:04", "2026-08-17 20:00")
	task := db.AutomationTask{TriggerType: db.AutomationTriggerFixedSchedule}
	task.SetTimes([]string{"04:00", "16:30"})

	next, err := ComputeNextRun(task, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := mustParse(t, "2006-01-02 15:04", "2026-08-18 04:00")
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestComputeNextFixedScheduleRun_WeekdayFilter(t *testing.T) {
	// 2026-08-17 is a Monday. Restrict to Wed(3)/Fri(5) only.
	after := mustParse(t, "2006-01-02 15:04", "2026-08-17 09:00")
	task := db.AutomationTask{TriggerType: db.AutomationTriggerFixedSchedule}
	task.SetTimes([]string{"04:00"})
	task.SetWeekdaySet([]int{3, 5})

	next, err := ComputeNextRun(task, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := mustParse(t, "2006-01-02 15:04", "2026-08-19 04:00") // next Wednesday
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestWeekdayCode(t *testing.T) {
	cases := map[time.Weekday]int{
		time.Monday:    1,
		time.Tuesday:   2,
		time.Wednesday: 3,
		time.Thursday:  4,
		time.Friday:    5,
		time.Saturday:  6,
		time.Sunday:    7,
	}
	for w, want := range cases {
		if got := weekdayCode(w); got != want {
			t.Errorf("weekdayCode(%v) = %d, want %d", w, got, want)
		}
	}
}
