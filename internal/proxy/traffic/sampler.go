package traffic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/boa-z/quectel-qmi-go/pkg/qmi"
	"github.com/boa-z/vohive/internal/db"
	"github.com/boa-z/vohive/internal/device"
	"github.com/boa-z/vohive/internal/proxy/server"
	"github.com/boa-z/vohive/pkg/logger"
)

const (
	trafficCounterSourceQMIWDS = "qmi_wds"

	trafficCounterReadTimeout = 3 * time.Second

	qmiWDSTrafficStatsMask = qmi.WDSPacketStatsTxBytesOK | qmi.WDSPacketStatsRxBytesOK

	defaultBackfillHorizon  = 31
	defaultBackfillInterval = 1 * time.Hour
)

type Sampler struct {
	ctx    context.Context
	cancel context.CancelFunc

	stopOnce sync.Once
	started  sync.WaitGroup

	pool *device.Pool
	mgr  *server.Manager

	lastIface       map[string]trafficCounters
	ifaceReadErrLog map[string]time.Time

	workerInterfaces func() []workerInterface
}

type Options struct {
	Pool *device.Pool
	Mgr  *server.Manager

	workerInterfaces func() []workerInterface
}

type workerInterface struct {
	id           string
	iface        string
	source       string
	networkReady func() bool
	readCounters func(context.Context) (trafficCounters, error)
	currentICCID func() string
}

type trafficCounters struct {
	RXBytes uint64
	TXBytes uint64
}

func New(opts Options) *Sampler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Sampler{
		ctx:              ctx,
		cancel:           cancel,
		pool:             opts.Pool,
		mgr:              opts.Mgr,
		lastIface:        make(map[string]trafficCounters),
		ifaceReadErrLog:  make(map[string]time.Time),
		workerInterfaces: opts.workerInterfaces,
	}
	if s.workerInterfaces == nil {
		s.workerInterfaces = s.poolWorkerInterfaces
	}
	return s
}

func (s *Sampler) Stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		// 等待采样/回填 goroutine 退出，避免与 flushFinal 并发读写 lastIface/DB。
		s.started.Wait()
		// 补上最后一次整分钟采样之后、进程退出前的部分分钟流量增量，
		// 写入 traffic_minute 与 card_quota_usage，下次启动的上卷回填会将其纳入日/周/月汇总。
		s.flushFinal()
	})
}

func (s *Sampler) Start() {
	s.started.Add(2)
	go func() {
		defer s.started.Done()
		s.loop()
	}()
	go func() {
		defer s.started.Done()
		s.backfillLoop()
	}()
}

// flushFinal 在进程优雅退出时执行一次“最终采样”，把自上一次整分钟采样以来的
// 部分分钟增量落库（traffic_minute + 卡流量用量），并同步更新基线，
// 确保与正常采样路径不重复计数。幂等：仅在 Stop 中调用一次。
func (s *Sampler) flushFinal() {
	now := time.Now()
	periodStart := now.Truncate(time.Minute)

	var points []db.TrafficPoint

	for _, wi := range s.workerInterfaces() {
		if wi.id == "" || wi.iface == "" {
			continue
		}
		if !wi.shouldSampleTraffic() {
			s.clearWorkerInterfaceBaseline(wi)
			continue
		}
		cur, source, err := s.readWorkerCounters(wi)
		if err != nil {
			s.logCounterReadError(wi.id, wi.iface, source, err)
			continue
		}
		key := counterBaselineKey(wi.id, wi.iface, source)
		last, ok := s.lastIface[key]
		s.lastIface[key] = cur
		if !ok {
			continue
		}
		drx := int64(cur.RXBytes) - int64(last.RXBytes)
		dtx := int64(cur.TXBytes) - int64(last.TXBytes)
		if drx < 0 {
			drx = 0
		}
		if dtx < 0 {
			dtx = 0
		}
		if drx == 0 && dtx == 0 {
			continue
		}
		points = append(points,
			db.TrafficPoint{PeriodStart: periodStart, Resource: "iface", Tag: ifaceBaselineKey(wi.id, wi.iface), Direction: false, TrafficBytes: drx},
			db.TrafficPoint{PeriodStart: periodStart, Resource: "iface", Tag: ifaceBaselineKey(wi.id, wi.iface), Direction: true, TrafficBytes: dtx},
		)
		// 累加到 per-ICCID 卡流量用量（仅当该卡启用了流量限制时）
		if wi.currentICCID != nil {
			if iccid := strings.TrimSpace(wi.currentICCID()); iccid != "" {
				if pol, perr := db.GetCardPolicy(iccid); perr == nil && pol.QuotaEnabled {
					if _, aerr := db.AccumulateCardQuotaUsage(iccid, drx+dtx, pol.BillingDay, pol.BillingTimezone, now); aerr != nil {
						logger.Warn("退出时累加卡流量用量失败", "iccid", iccid, "err", aerr)
					}
				}
			}
		}
	}

	// 补上代理实例在最后一次采样后的流量增量。
	if s.mgr != nil {
		for instID, snap := range s.mgr.SnapshotAndResetTraffic() {
			if snap.Downlink > 0 {
				points = append(points, db.TrafficPoint{PeriodStart: periodStart, Resource: "proxy_instance", Tag: instID, Direction: false, TrafficBytes: snap.Downlink})
			}
			if snap.Uplink > 0 {
				points = append(points, db.TrafficPoint{PeriodStart: periodStart, Resource: "proxy_instance", Tag: instID, Direction: true, TrafficBytes: snap.Uplink})
			}
		}
	}

	if len(points) > 0 {
		if err := db.UpsertTrafficMinute(points); err != nil {
			logger.Warn("退出时写入流量分钟桶失败", "err", err)
		}
	}
}

func (s *Sampler) loop() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("traffic sampler panic recovered", "err", r)
		}
	}()

	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
		}

		s.sample(next.Add(-time.Minute))

		next = next.Add(time.Minute)
		now = time.Now()
		if now.After(next) {
			next = now.Truncate(time.Minute).Add(time.Minute)
		}
		timer.Reset(time.Until(next))
	}
}

// backfillLoop 周期性执行流量上卷回填，补偿因进程停机/重启导致的遗漏。
// 使用独立 goroutine，不阻塞采样主循环；幂等执行，出错仅记录日志。
func (s *Sampler) backfillLoop() {
	// 启动时立即执行一次回填
	_, _ = db.BackfillTraffic(time.Now(), defaultBackfillHorizon)

	ticker := time.NewTicker(defaultBackfillInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			_, _ = db.BackfillTraffic(time.Now(), defaultBackfillHorizon)
		}
	}
}

func (s *Sampler) sample(periodStart time.Time) {
	var points []db.TrafficPoint

	for _, wi := range s.workerInterfaces() {
		if wi.id == "" || wi.iface == "" {
			continue
		}
		if !wi.shouldSampleTraffic() {
			s.clearWorkerInterfaceBaseline(wi)
			continue
		}
		cur, source, err := s.readWorkerCounters(wi)
		if err != nil {
			s.logCounterReadError(wi.id, wi.iface, source, err)
			continue
		}
		key := counterBaselineKey(wi.id, wi.iface, source)
		last, ok := s.lastIface[key]
		s.lastIface[key] = cur
		if !ok {
			continue
		}
		drx := int64(cur.RXBytes) - int64(last.RXBytes)
		dtx := int64(cur.TXBytes) - int64(last.TXBytes)
		if drx < 0 {
			drx = 0
		}
		if dtx < 0 {
			dtx = 0
		}
		points = append(points,
			db.TrafficPoint{PeriodStart: periodStart, Resource: "iface", Tag: ifaceBaselineKey(wi.id, wi.iface), Direction: false, TrafficBytes: drx},
			db.TrafficPoint{PeriodStart: periodStart, Resource: "iface", Tag: ifaceBaselineKey(wi.id, wi.iface), Direction: true, TrafficBytes: dtx},
		)
		// 累加到 per-ICCID 卡流量用量（仅当该卡启用了流量限制时）
		if wi.currentICCID != nil {
			if iccid := strings.TrimSpace(wi.currentICCID()); iccid != "" {
				if pol, perr := db.GetCardPolicy(iccid); perr == nil && pol.QuotaEnabled {
					if _, aerr := db.AccumulateCardQuotaUsage(iccid, drx+dtx, pol.BillingDay, pol.BillingTimezone, periodStart.Add(time.Minute)); aerr != nil {
						logger.Warn("累加卡流量用量失败", "iccid", iccid, "err", aerr)
					}
				}
			}
		}
	}

	// 流量限制周期 rollover 协调：对启用流量限制且存储意图为开网的卡，
	// 若其用量行已不在当前计费周期（跨月），重置用量并重投影策略，
	// 使跨计费月后被拦截的卡能自动恢复开网（无需独立 cron）。
	s.reconcileCardQuota(periodStart.Add(time.Minute))

	if s.mgr != nil {
		snaps := s.mgr.SnapshotAndResetTraffic()
		for instID, snap := range snaps {
			if snap.Downlink > 0 {
				points = append(points, db.TrafficPoint{PeriodStart: periodStart, Resource: "proxy_instance", Tag: instID, Direction: false, TrafficBytes: snap.Downlink})
			}
			if snap.Uplink > 0 {
				points = append(points, db.TrafficPoint{PeriodStart: periodStart, Resource: "proxy_instance", Tag: instID, Direction: true, TrafficBytes: snap.Uplink})
			}
		}
	}

	if err := db.UpsertTrafficMinute(points); err != nil {
		logger.Warn("写入流量分钟桶失败", "err", err)
		return
	}

	now := periodStart.Add(time.Minute)
	if now.Minute() == 0 {
		prevHour := now.Truncate(time.Hour).Add(-time.Hour)
		_ = db.RollupToHour(prevHour)
	}
	if now.Hour() == 0 && now.Minute() == 0 {
		prevDay := now.Truncate(24 * time.Hour).Add(-24 * time.Hour)
		_ = db.RollupToDay(prevDay)
	}
	if now.Weekday() == time.Monday && now.Hour() == 0 && now.Minute() == 0 {
		weekStart := startOfWeek(now).Add(-7 * 24 * time.Hour)
		_ = db.RollupToWeek(weekStart)
	}
	if now.Day() == 1 && now.Hour() == 0 && now.Minute() == 0 {
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0)
		_ = db.RollupToMonth(monthStart)
	}

	if now.Minute() == 0 {
		_ = db.CleanupBefore(
			now,
			24*time.Hour,
			7*24*time.Hour,
			31*24*time.Hour,
			12*7*24*time.Hour,
			24*31*24*time.Hour,
		)
	}
}

func startOfWeek(t time.Time) time.Time {
	tt := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	for tt.Weekday() != time.Monday {
		tt = tt.Add(-24 * time.Hour)
	}
	return tt
}

func (s *Sampler) primeIfaceBaselines() {
	for _, wi := range s.workerInterfaces() {
		if wi.id == "" || wi.iface == "" {
			continue
		}
		if !wi.shouldSampleTraffic() {
			s.clearWorkerInterfaceBaseline(wi)
			continue
		}
		cur, source, err := s.readWorkerCounters(wi)
		if err != nil {
			s.logCounterReadError(wi.id, wi.iface, source, err)
			continue
		}
		s.lastIface[counterBaselineKey(wi.id, wi.iface, source)] = cur
	}
}

func (s *Sampler) poolWorkerInterfaces() []workerInterface {
	if s.pool == nil {
		return nil
	}
	workers := s.pool.GetAllWorkers()
	out := make([]workerInterface, 0, len(workers))
	for _, w := range workers {
		if w == nil {
			continue
		}
		id := strings.TrimSpace(w.ID)
		iface := strings.TrimSpace(w.Config.Interface)
		if id == "" || iface == "" {
			continue
		}
		worker := w
		networkReady := func() bool {
			return worker.Config.NetworkEnabled && worker.NetworkConnected()
		}
		currentICCID := worker.CurrentICCID
		var readCounters func(context.Context) (trafficCounters, error)
		if qmiCore := worker.QMICore; qmiCore != nil {
			readCounters = func(ctx context.Context) (trafficCounters, error) {
				return readQMIWDSTrafficCounters(ctx, qmiCore)
			}
		}
		out = append(out, workerInterface{id: id, iface: iface, source: trafficCounterSourceQMIWDS, networkReady: networkReady, readCounters: readCounters, currentICCID: currentICCID})
	}
	return out
}

func (s *Sampler) readWorkerCounters(wi workerInterface) (trafficCounters, string, error) {
	source := wi.counterSource()
	if wi.readCounters != nil {
		ctx, cancel := context.WithTimeout(s.ctx, trafficCounterReadTimeout)
		defer cancel()
		cur, err := wi.readCounters(ctx)
		if err == nil {
			return cur, source, nil
		}
		return trafficCounters{}, source, err
	}
	return trafficCounters{}, source, errors.New("qmi wds counter reader not available")
}

func (wi workerInterface) counterSource() string {
	source := strings.TrimSpace(wi.source)
	if source == "" {
		return trafficCounterSourceQMIWDS
	}
	return source
}

func (wi workerInterface) shouldSampleTraffic() bool {
	if wi.networkReady == nil {
		return true
	}
	return wi.networkReady()
}

func (s *Sampler) clearWorkerInterfaceBaseline(wi workerInterface) {
	delete(s.lastIface, counterBaselineKey(wi.id, wi.iface, wi.counterSource()))
}

func ifaceBaselineKey(deviceID string, iface string) string {
	return strings.TrimSpace(deviceID) + "@" + strings.TrimSpace(iface)
}

func counterBaselineKey(deviceID string, iface string, source string) string {
	key := ifaceBaselineKey(deviceID, iface)
	source = strings.TrimSpace(source)
	if source == "" {
		return key
	}
	return key + "#" + source
}

func (s *Sampler) logCounterReadError(deviceID string, iface string, source string, err error) {
	key := counterBaselineKey(deviceID, iface, source)
	now := time.Now()
	if last, ok := s.ifaceReadErrLog[key]; ok && now.Sub(last) < 5*time.Minute {
		return
	}
	s.ifaceReadErrLog[key] = now
	logger.Warn("流量采样读取计数器失败", "device", deviceID, "interface", iface, "source", source, "err", err)
}

// reconcileCardQuota 每分钟由采样循环调用，处理流量限制的计费周期 rollover：
// 对启用流量限制(QuotaEnabled)且存储意图为开网(NetworkEnabled)的卡，
// 若其用量行已不在当前计费周期（说明已跨月），则重置用量行到新周期并触发策略重投影，
// 使新周期里被拦截开网的卡能自动恢复。该逻辑覆盖设备一直在线跨月的场景；
// 设备重启/SIM 状态变化等场景由 resolveAndApplyPolicy 自行处理。
func (s *Sampler) reconcileCardQuota(now time.Time) {
	if s == nil {
		return
	}
	for _, wi := range s.workerInterfaces() {
		if wi.id == "" || wi.currentICCID == nil {
			continue
		}
		iccid := strings.TrimSpace(wi.currentICCID())
		if iccid == "" {
			continue
		}
		pol, perr := db.GetCardPolicy(iccid)
		if perr != nil || !pol.QuotaEnabled || !pol.NetworkEnabled {
			continue
		}
		_, currentEnd := db.BillingPeriodFor(now, pol.BillingDay, pol.BillingTimezone)
		row, rerr := db.GetCardQuotaUsage(iccid)
		needReset := false
		if rerr == nil {
			if !row.PeriodEnd.IsZero() && !row.PeriodEnd.Equal(currentEnd) {
				needReset = true
			}
		} else if errors.Is(rerr, db.ErrCardQuotaUsageNotFound) {
			// 用量行不存在：若卡意图开网却被拦截（从未累计过用量），
			// 建一个新周期空行并触发重投影以解除拦截。
			needReset = true
		}
		if !needReset {
			continue
		}
		if _, aerr := db.AccumulateCardQuotaUsage(iccid, 0, pol.BillingDay, pol.BillingTimezone, now); aerr != nil {
			logger.Warn("重置卡流量用量周期失败", "iccid", iccid, "err", aerr)
			continue
		}
		if s.pool != nil {
			s.pool.ReapplyCardPolicy(wi.id, "quota_rollover")
		}
	}
}
