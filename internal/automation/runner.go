package automation

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/travislee89/vohive/internal/db"
	"github.com/travislee89/vohive/internal/device"
	"github.com/travislee89/vohive/internal/notify"
	"github.com/travislee89/vohive/pkg/logger"
	"github.com/travislee89/vohive/pkg/smscodec"
)

// Runner 执行自动化任务：调度器（定时到期）与手动"立即运行"共用同一入口。
type Runner struct {
	pool   *device.Pool
	notify *notify.Manager
}

// NewRunner 创建自动化任务执行器。notifyMgr 可为 nil（此时跳过 automation_event 通知）。
func NewRunner(pool *device.Pool, notifyMgr *notify.Manager) *Runner {
	return &Runner{pool: pool, notify: notifyMgr}
}

// RunTask 执行一次任务：解析目标设备（单个或全部），对每个设备执行动作并写入运行日志，
// 最后推进任务的 LastRunAt/NextRunAt。triggerSource 为 "schedule" 或 "manual"。
func (r *Runner) RunTask(ctx context.Context, task db.AutomationTask, triggerSource string) {
	workers := r.resolveTargetWorkers(task.DeviceID)
	if len(workers) == 0 {
		logger.Warn("自动化任务无可用目标设备，跳过执行", "task", task.Name, "device_id", task.DeviceID)
	}

	for _, w := range workers {
		switch task.ActionType {
		case db.AutomationActionReboot:
			r.runReboot(ctx, task, w.ID, triggerSource)
		case db.AutomationActionSMS:
			r.runSMS(ctx, task, w.ID, triggerSource)
		default:
			logger.Warn("未知的自动化任务动作类型", "task", task.Name, "action_type", task.ActionType)
		}
	}

	r.advanceSchedule(task)
}

func (r *Runner) resolveTargetWorkers(deviceID string) []*device.Worker {
	if deviceID == db.AutomationDeviceAll {
		return r.pool.GetAllWorkers()
	}
	if w := r.pool.GetWorker(deviceID); w != nil {
		return []*device.Worker{w}
	}
	return nil
}

func (r *Runner) runReboot(ctx context.Context, task db.AutomationTask, deviceID, triggerSource string) {
	status := db.AutomationRunStatusSuccess
	summary := "重启指令已发送"
	errDetail := ""
	if err := r.pool.RebootWorkerAction(ctx, deviceID); err != nil {
		status = db.AutomationRunStatusFailed
		summary = "重启失败"
		errDetail = err.Error()
	}
	r.writeLog(task, deviceID, triggerSource, status, summary, errDetail)
}

func (r *Runner) runSMS(ctx context.Context, task db.AutomationTask, deviceID, triggerSource string) {
	sleepForRandomDelay(task.SMSDelayMinSec, task.SMSDelayMaxSec)

	content := RenderSMSTemplate(task.SMSContent)
	attempts := task.SMSRetryCount + 1
	var lastErr error
	var succeededOnAttempt int
	for attempt := 1; attempt <= attempts; attempt++ {
		_, err := r.pool.SendSMSAction(ctx, deviceID, task.SMSPhone, content, smscodec.SubmitOptions{})
		if err == nil {
			succeededOnAttempt = attempt
			lastErr = nil
			break
		}
		lastErr = err
	}

	status := db.AutomationRunStatusSuccess
	summary := "短信发送成功"
	errDetail := ""
	if lastErr != nil {
		status = db.AutomationRunStatusFailed
		if attempts > 1 {
			summary = fmt.Sprintf("重试 %d 次均失败", task.SMSRetryCount)
		} else {
			summary = "短信发送失败"
		}
		errDetail = lastErr.Error()
	} else if succeededOnAttempt > 1 {
		summary = fmt.Sprintf("第 %d 次尝试成功", succeededOnAttempt)
	}
	r.writeLog(task, deviceID, triggerSource, status, summary, errDetail)
}

func sleepForRandomDelay(minSec, maxSec int) {
	if minSec <= 0 && maxSec <= 0 {
		return
	}
	lo, hi := minSec, maxSec
	if hi < lo {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	delay := lo
	if hi > lo {
		delay = lo + rand.Intn(hi-lo+1)
	}
	if delay > 0 {
		time.Sleep(time.Duration(delay) * time.Second)
	}
}

func (r *Runner) writeLog(task db.AutomationTask, deviceID, triggerSource, status, summary, errDetail string) {
	if err := db.CreateAutomationRunLog(db.AutomationRunLog{
		TaskID:        task.ID,
		TaskName:      task.Name,
		ActionType:    task.ActionType,
		DeviceID:      deviceID,
		TriggerSource: triggerSource,
		Status:        status,
		ResultSummary: summary,
		ErrorDetail:   errDetail,
		Timestamp:     time.Now(),
	}); err != nil {
		logger.Warn("写入自动化运行日志失败", "task", task.Name, "device_id", deviceID, "err", err)
	}

	if r.notify != nil {
		r.notify.NotifyAutomationEvent(notify.AutomationContext{
			TaskID:        task.ID,
			TaskName:      task.Name,
			ActionType:    task.ActionType,
			DeviceID:      deviceID,
			Status:        status,
			ResultSummary: summary,
			ErrorDetail:   errDetail,
		})
	}
}

func (r *Runner) advanceSchedule(task db.AutomationTask) {
	now := time.Now()
	task.LastRunAt = &now
	next, err := ComputeNextRun(task, now)
	if err != nil {
		logger.Warn("计算自动化任务下次运行时间失败", "task", task.Name, "err", err)
		task.NextRunAt = nil
	} else {
		task.NextRunAt = &next
	}
	if _, err := db.UpsertAutomationTask(task); err != nil {
		logger.Warn("更新自动化任务运行状态失败", "task", task.Name, "err", err)
	}
}
