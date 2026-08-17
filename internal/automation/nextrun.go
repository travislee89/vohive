package automation

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/travislee89/vohive/internal/db"
)

// ComputeNextRun 计算任务下一次应触发的时间（使用服务器本地时间，与 HH:MM 输入语义一致）。
func ComputeNextRun(t db.AutomationTask, after time.Time) (time.Time, error) {
	switch t.TriggerType {
	case db.AutomationTriggerInterval:
		return computeNextIntervalRun(t, after)
	case db.AutomationTriggerFixedSchedule:
		return computeNextFixedScheduleRun(t, after)
	default:
		return time.Time{}, fmt.Errorf("未知的触发机制: %s", t.TriggerType)
	}
}

func intervalDuration(t db.AutomationTask) (time.Duration, error) {
	if t.IntervalValue <= 0 {
		return 0, fmt.Errorf("间隔时长必须大于 0")
	}
	switch t.IntervalUnit {
	case db.AutomationIntervalHours:
		return time.Duration(t.IntervalValue) * time.Hour, nil
	case db.AutomationIntervalDays:
		return time.Duration(t.IntervalValue) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("未知的时间单位: %s", t.IntervalUnit)
	}
}

func computeNextIntervalRun(t db.AutomationTask, after time.Time) (time.Time, error) {
	interval, err := intervalDuration(t)
	if err != nil {
		return time.Time{}, err
	}
	base := after
	if t.LastRunAt != nil {
		base = *t.LastRunAt
	} else if !t.CreatedAt.IsZero() {
		base = t.CreatedAt
	}
	next := base.Add(interval)
	if !next.After(after) {
		// 任务逾期未触发（如程序停机期间错过了调度），从当前时刻起重新计时，避免补跑造成的连续触发。
		next = after.Add(interval)
	}
	return next, nil
}

// weekdayCode 将 time.Weekday（0=周日）转换为界面使用的 1-7 编码（1=一...7=日）。
func weekdayCode(w time.Weekday) int {
	return ((int(w)+6)%7 + 1)
}

func parseHHMM(s string) (hour, minute int, err error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("无效的时刻格式: %s", s)
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("无效的小时: %s", s)
	}
	minute, err = strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("无效的分钟: %s", s)
	}
	return hour, minute, nil
}

const fixedScheduleLookaheadDays = 8

func computeNextFixedScheduleRun(t db.AutomationTask, after time.Time) (time.Time, error) {
	times := t.Times()
	if len(times) == 0 {
		return time.Time{}, fmt.Errorf("触发时刻不能为空")
	}
	type hm struct{ hour, minute int }
	parsed := make([]hm, 0, len(times))
	for _, ts := range times {
		hour, minute, err := parseHHMM(ts)
		if err != nil {
			return time.Time{}, err
		}
		parsed = append(parsed, hm{hour, minute})
	}

	weekdays := t.WeekdaySet()
	allowed := make(map[int]bool, len(weekdays))
	for _, d := range weekdays {
		allowed[d] = true
	}
	everyDay := len(allowed) == 0

	var candidates []time.Time
	loc := after.Location()
	for dayOffset := 0; dayOffset < fixedScheduleLookaheadDays; dayOffset++ {
		day := after.AddDate(0, 0, dayOffset)
		if !everyDay && !allowed[weekdayCode(day.Weekday())] {
			continue
		}
		for _, p := range parsed {
			candidate := time.Date(day.Year(), day.Month(), day.Day(), p.hour, p.minute, 0, 0, loc)
			if candidate.After(after) {
				candidates = append(candidates, candidate)
			}
		}
	}

	if len(candidates) == 0 {
		return time.Time{}, fmt.Errorf("未来 %d 天内无满足条件的触发时刻", fixedScheduleLookaheadDays)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Before(candidates[j]) })
	return candidates[0], nil
}
