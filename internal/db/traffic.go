package db

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

type TrafficBucket struct {
	Bucket      string    `json:"bucket"`
	PeriodStart time.Time `json:"period_start"`
	RxBytes     int64     `json:"rx_bytes"`
	TxBytes     int64     `json:"tx_bytes"`
	TotalBytes  int64     `json:"total_bytes"`
}

func GetTrafficAnalysis(rangeName string, deviceID string, now time.Time) ([]TrafficBucket, error) {
	buckets, _, err := GetTrafficAnalysisWithChart(rangeName, deviceID, now)
	if err != nil {
		return nil, err
	}
	return buckets, nil
}

type TrafficChartData struct {
	Timestamps   []string           `json:"timestamps"`
	PeriodStarts []time.Time        `json:"period_starts"`
	Devices      []string           `json:"devices"`
	Series       map[string][]int64 `json:"series"` // device_id -> [bytes, bytes, ...]
}

// TrafficTotal 单个时间范围内的下载/上传/合计（字节）。
type TrafficTotal struct {
	RxBytes    int64 `json:"rx_bytes"`
	TxBytes    int64 `json:"tx_bytes"`
	TotalBytes int64 `json:"total_bytes"`
}

// TrafficRangeTotals 同时聚合 day/week/month 三个范围的统计，供概览数值框一次取全。
type TrafficRangeTotals struct {
	Day   TrafficTotal `json:"day"`
	Week  TrafficTotal `json:"week"`
	Month TrafficTotal `json:"month"`
}

// GetTrafficRangeTotals 计算 day/week/month 三个范围的下载/上传/合计。
// 复用了与图表一致的 range spec 与聚合逻辑，保证口径与各 tab 展示的数值一致。
func GetTrafficRangeTotals(deviceID string, now time.Time) (TrafficRangeTotals, error) {
	var out TrafficRangeTotals
	for _, name := range []string{"day", "week", "month"} {
		buckets, _, err := GetTrafficAnalysisWithChart(name, deviceID, now)
		if err != nil {
			return out, err
		}
		var rx, tx int64
		for _, b := range buckets {
			rx += b.RxBytes
			tx += b.TxBytes
		}
		t := TrafficTotal{RxBytes: rx, TxBytes: tx, TotalBytes: rx + tx}
		switch name {
		case "day":
			out.Day = t
		case "week":
			out.Week = t
		case "month":
			out.Month = t
		}
	}
	return out, nil
}

func GetTrafficChartData(rangeName string, deviceID string, now time.Time) (*TrafficChartData, error) {
	_, chart, err := GetTrafficAnalysisWithChart(rangeName, deviceID, now)
	if err != nil {
		return nil, err
	}
	return chart, nil
}

func GetTrafficAnalysisWithChart(rangeName string, deviceID string, now time.Time) ([]TrafficBucket, *TrafficChartData, error) {
	if DB == nil {
		return nil, nil, fmt.Errorf("db not initialized")
	}

	spec, err := newTrafficRangeSpec(rangeName, now)
	if err != nil {
		return nil, nil, err
	}

	timestamps := make([]string, 0)
	periodStarts := make([]time.Time, 0)
	tsMap := make(map[int64]int)
	bucketAgg := map[int64]*TrafficBucket{}
	bucketOrder := make([]int64, 0)
	cursor := spec.since
	for !cursor.After(now) {
		periodStart := spec.periodStart(cursor)
		periodKey := trafficPeriodKey(periodStart)
		bucketAgg[periodKey] = &TrafficBucket{
			Bucket:      spec.bucketKey(periodStart),
			PeriodStart: periodStart,
		}
		bucketOrder = append(bucketOrder, periodKey)
		tsMap[periodKey] = len(periodStarts)
		periodStarts = append(periodStarts, periodStart)
		timestamps = append(timestamps, spec.chartKey(periodStart))
		cursor = cursor.Add(spec.step)
	}

	rows, err := queryTrafficRollupRows(rangeName, deviceID, spec.since, now)
	if err != nil {
		return nil, nil, err
	}
	currentRows, err := queryTrafficCurrentRows(rangeName, deviceID, spec.currentStart, now)
	if err != nil {
		return nil, nil, err
	}

	deviceSet := map[string]struct{}{}
	tempSeries := map[string]map[int]int64{}
	applyRow := func(r trafficRollupRow) {
		ps := spec.periodStart(r.PeriodStart.In(now.Location()))
		periodKey := trafficPeriodKey(ps)
		if b, ok := bucketAgg[periodKey]; ok {
			if r.Direction {
				b.TxBytes += r.TrafficBytes
			} else {
				b.RxBytes += r.TrafficBytes
			}
			b.TotalBytes = b.RxBytes + b.TxBytes
		}

		tIdx, ok := tsMap[periodKey]
		if !ok {
			return
		}
		dev := trafficDeviceFromTag(r.Tag)
		deviceSet[dev] = struct{}{}
		if _, exists := tempSeries[dev]; !exists {
			tempSeries[dev] = make(map[int]int64)
		}
		tempSeries[dev][tIdx] += r.TrafficBytes
	}

	for _, r := range rows {
		applyRow(r)
	}
	for _, r := range currentRows {
		applyRow(r)
	}

	buckets := make([]TrafficBucket, 0, len(bucketOrder))
	for _, k := range bucketOrder {
		buckets = append(buckets, *bucketAgg[k])
	}

	devices := make([]string, 0, len(deviceSet))
	for d := range deviceSet {
		devices = append(devices, d)
	}
	sort.Strings(devices)

	series := make(map[string][]int64)
	for _, dev := range devices {
		data := make([]int64, len(timestamps))
		if points, ok := tempSeries[dev]; ok {
			for tIdx, val := range points {
				data[tIdx] = val
			}
		}
		series[dev] = data
	}

	return buckets, &TrafficChartData{
		Timestamps:   timestamps,
		PeriodStarts: periodStarts,
		Devices:      devices,
		Series:       series,
	}, nil
}

type trafficRangeSpec struct {
	since        time.Time
	step         time.Duration
	currentStart time.Time
	periodStart  func(time.Time) time.Time
	bucketKey    func(time.Time) string
	chartKey     func(time.Time) string
}

func newTrafficRangeSpec(rangeName string, now time.Time) (trafficRangeSpec, error) {
	switch rangeName {
	case "day":
		return trafficRangeSpec{
			since:        now.Add(-24 * time.Hour).Truncate(time.Hour),
			step:         time.Hour,
			currentStart: now.Truncate(time.Hour),
			periodStart: func(t time.Time) time.Time {
				return t.Truncate(time.Hour)
			},
			bucketKey: func(t time.Time) string {
				return t.Truncate(time.Hour).Format("2006-01-02 15:00")
			},
			chartKey: func(t time.Time) string {
				return t.Truncate(time.Hour).Format("15:00")
			},
		}, nil
	case "week":
		since := startOfTrafficDay(now.Add(-7 * 24 * time.Hour))
		return trafficRangeSpec{
			since:        since,
			step:         24 * time.Hour,
			currentStart: startOfTrafficDay(now),
			periodStart:  startOfTrafficDay,
			bucketKey: func(t time.Time) string {
				return startOfTrafficDay(t).Format("2006-01-02")
			},
			chartKey: func(t time.Time) string {
				return startOfTrafficDay(t).Format("01-02")
			},
		}, nil
	case "month":
		since := startOfTrafficDay(now.Add(-30 * 24 * time.Hour))
		return trafficRangeSpec{
			since:        since,
			step:         24 * time.Hour,
			currentStart: startOfTrafficDay(now),
			periodStart:  startOfTrafficDay,
			bucketKey: func(t time.Time) string {
				return startOfTrafficDay(t).Format("2006-01-02")
			},
			chartKey: func(t time.Time) string {
				return startOfTrafficDay(t).Format("01-02")
			},
		}, nil
	default:
		return trafficRangeSpec{}, fmt.Errorf("invalid range")
	}
}

func startOfTrafficDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func trafficPeriodKey(t time.Time) int64 {
	return t.UnixNano()
}

type trafficRollupRow struct {
	PeriodStart  time.Time
	Tag          string
	Direction    bool
	TrafficBytes int64
}

func queryTrafficRollupRows(rangeName string, deviceID string, since time.Time, now time.Time) ([]trafficRollupRow, error) {
	var rows []trafficRollupRow
	var q *gorm.DB
	if rangeName == "day" {
		q = DB.Model(&TrafficHour{})
	} else {
		q = DB.Model(&TrafficDay{})
	}
	q = q.Select("period_start, tag, direction, traffic_bytes").
		Where("resource = ? AND period_start >= ? AND period_start <= ?", "iface", since, now)
	q = applyTrafficDeviceFilter(q, deviceID)
	if err := q.Order("period_start asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func queryTrafficCurrentRows(rangeName string, deviceID string, currentStart time.Time, now time.Time) ([]trafficRollupRow, error) {
	type currentRow struct {
		Tag          string
		Direction    bool
		TrafficBytes int64
	}

	// 对指定明细表按 [start, now] 聚合查询，所有行统一落在 currentStart 对应桶。
	collect := func(model interface{}, start time.Time) ([]trafficRollupRow, error) {
		var rows []currentRow
		q := DB.Model(model).
			Select("tag, direction, sum(traffic_bytes) as traffic_bytes").
			Where("resource = ? AND period_start >= ? AND period_start <= ?", "iface", start, now).
			Group("tag, direction")
		q = applyTrafficDeviceFilter(q, deviceID)
		if err := q.Find(&rows).Error; err != nil {
			return nil, err
		}
		out := make([]trafficRollupRow, 0, len(rows))
		for _, r := range rows {
			out = append(out, trafficRollupRow{
				PeriodStart:  currentStart,
				Tag:          r.Tag,
				Direction:    r.Direction,
				TrafficBytes: r.TrafficBytes,
			})
		}
		return out, nil
	}

	if rangeName == "day" {
		// 当天：历史已完成整点用 traffic_hour，当前未完成小时用 traffic_minute。
		return collect(&TrafficMinute{}, currentStart)
	}

	// week/month 的当天部分：已完成整点（traffic_hour）：
	//   traffic_hour 要到下个整点才由 RollupToHour 生成，因此当前未完成小时还需
	//   读 traffic_minute，与 day 的实时口径对齐，避免「日流量 > 周/月流量」。
	var out []trafficRollupRow
	hourRows, err := collect(&TrafficHour{}, currentStart)
	if err != nil {
		return nil, err
	}
	out = append(out, hourRows...)

	minuteRows, err := collect(&TrafficMinute{}, now.Truncate(time.Hour))
	if err != nil {
		return nil, err
	}
	out = append(out, minuteRows...)
	return out, nil
}

func trafficDeviceFromTag(tag string) string {
	if idx := indexByte(tag, '@'); idx >= 0 {
		return tag[:idx]
	}
	return tag
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func applyTrafficDeviceFilter(tx *gorm.DB, deviceID string) *gorm.DB {
	pattern := trafficTagPrefixPattern(deviceID)
	if pattern == "" {
		return tx
	}
	return tx.Where("tag LIKE ? ESCAPE '\\'", pattern)
}

func trafficTagPrefixPattern(deviceID string) string {
	trimmed := strings.TrimSpace(deviceID)
	if trimmed == "" {
		return ""
	}
	return escapeLikePattern(trimmed) + "@%"
}

func escapeLikePattern(v string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"%", "\\%",
		"_", "\\_",
	)
	return replacer.Replace(v)
}
