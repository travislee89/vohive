package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/travislee89/vohive/internal/automation"
	"github.com/travislee89/vohive/internal/db"
)

// automationTaskResponse 是 AutomationTask 面向前端的展开形式（fixed_times/weekdays 由内部 JSON 列解码为数组）。
type automationTaskResponse struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	ActionType     string   `json:"action_type"`
	DeviceID       string   `json:"device_id"`
	SMSPhone       string   `json:"sms_phone,omitempty"`
	SMSContent     string   `json:"sms_content,omitempty"`
	SMSDelayMinSec int      `json:"sms_delay_min_sec,omitempty"`
	SMSDelayMaxSec int      `json:"sms_delay_max_sec,omitempty"`
	SMSRetryCount  int      `json:"sms_retry_count,omitempty"`
	TriggerType    string   `json:"trigger_type"`
	FixedTimes     []string `json:"fixed_times,omitempty"`
	Weekdays       []int    `json:"weekdays,omitempty"`
	IntervalValue  int      `json:"interval_value,omitempty"`
	IntervalUnit   string   `json:"interval_unit,omitempty"`
	LastRunAt      *string  `json:"last_run_at,omitempty"`
	NextRunAt      *string  `json:"next_run_at,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

func toAutomationTaskResponse(t db.AutomationTask) automationTaskResponse {
	resp := automationTaskResponse{
		ID:             t.ID,
		Name:           t.Name,
		Enabled:        t.Enabled,
		ActionType:     t.ActionType,
		DeviceID:       t.DeviceID,
		SMSPhone:       t.SMSPhone,
		SMSContent:     t.SMSContent,
		SMSDelayMinSec: t.SMSDelayMinSec,
		SMSDelayMaxSec: t.SMSDelayMaxSec,
		SMSRetryCount:  t.SMSRetryCount,
		TriggerType:    t.TriggerType,
		FixedTimes:     t.Times(),
		Weekdays:       t.WeekdaySet(),
		IntervalValue:  t.IntervalValue,
		IntervalUnit:   t.IntervalUnit,
		CreatedAt:      t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      t.UpdatedAt.Format(time.RFC3339),
	}
	if t.LastRunAt != nil {
		s := t.LastRunAt.Format(time.RFC3339)
		resp.LastRunAt = &s
	}
	if t.NextRunAt != nil {
		s := t.NextRunAt.Format(time.RFC3339)
		resp.NextRunAt = &s
	}
	return resp
}

func (s *Server) handleListAutomationTasks(c *gin.Context) {
	tasks, err := db.ListAutomationTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]automationTaskResponse, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, toAutomationTaskResponse(t))
	}
	c.JSON(http.StatusOK, gin.H{"tasks": out, "total": len(out)})
}

type automationTaskRequest struct {
	Name           *string  `json:"name"`
	Enabled        *bool    `json:"enabled"`
	ActionType     *string  `json:"action_type"`
	DeviceID       *string  `json:"device_id"`
	SMSPhone       *string  `json:"sms_phone"`
	SMSContent     *string  `json:"sms_content"`
	SMSDelayMinSec *int     `json:"sms_delay_min_sec"`
	SMSDelayMaxSec *int     `json:"sms_delay_max_sec"`
	SMSRetryCount  *int     `json:"sms_retry_count"`
	TriggerType    *string  `json:"trigger_type"`
	FixedTimes     []string `json:"fixed_times"`
	Weekdays       []int    `json:"weekdays"`
	IntervalValue  *int     `json:"interval_value"`
	IntervalUnit   *string  `json:"interval_unit"`
}

var automationHHMMPattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// validateAutomationTask 校验一个即将保存的任务是否满足动作类型/触发机制各自要求的字段完整性。
func (s *Server) validateAutomationTask(t *db.AutomationTask) error {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return errors.New("任务名称不能为空")
	}
	if t.ActionType != db.AutomationActionReboot && t.ActionType != db.AutomationActionSMS {
		return fmt.Errorf("未知的执行动作: %s", t.ActionType)
	}
	t.DeviceID = strings.TrimSpace(t.DeviceID)
	if t.DeviceID == "" {
		return errors.New("设备不能为空")
	}
	if t.DeviceID != db.AutomationDeviceAll && s.pool.GetWorker(t.DeviceID) == nil {
		return fmt.Errorf("设备不存在: %s", t.DeviceID)
	}

	if t.ActionType == db.AutomationActionSMS {
		t.SMSPhone = strings.TrimSpace(t.SMSPhone)
		if t.SMSPhone == "" {
			return errors.New("接收号码不能为空")
		}
		if strings.TrimSpace(t.SMSContent) == "" {
			return errors.New("短信内容不能为空")
		}
		if t.SMSDelayMinSec < 0 || t.SMSDelayMaxSec < 0 {
			return errors.New("随机延迟范围不能为负数")
		}
		if t.SMSDelayMinSec > t.SMSDelayMaxSec {
			return errors.New("随机延迟范围下限不能大于上限")
		}
		if t.SMSRetryCount < 0 {
			return errors.New("失败重试次数不能为负数")
		}
	}

	switch t.TriggerType {
	case db.AutomationTriggerFixedSchedule:
		times := t.Times()
		if len(times) == 0 {
			return errors.New("触发时刻不能为空")
		}
		for _, ts := range times {
			if !automationHHMMPattern.MatchString(strings.TrimSpace(ts)) {
				return fmt.Errorf("无效的触发时刻: %s", ts)
			}
		}
		for _, d := range t.WeekdaySet() {
			if d < 1 || d > 7 {
				return fmt.Errorf("无效的星期取值: %d", d)
			}
		}
	case db.AutomationTriggerInterval:
		if t.IntervalValue <= 0 {
			return errors.New("间隔时长必须大于 0")
		}
		if t.IntervalUnit != db.AutomationIntervalHours && t.IntervalUnit != db.AutomationIntervalDays {
			return fmt.Errorf("未知的时间单位: %s", t.IntervalUnit)
		}
	default:
		return fmt.Errorf("未知的触发机制: %s", t.TriggerType)
	}
	return nil
}

func (s *Server) handleCreateAutomationTask(c *gin.Context) {
	var req automationTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := db.AutomationTask{Enabled: true}
	applyAutomationTaskRequest(&task, req)

	if err := s.validateAutomationTask(&task); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.recomputeAutomationNextRun(&task); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	saved, err := db.UpsertAutomationTask(task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toAutomationTaskResponse(saved))
}

func (s *Server) handleUpdateAutomationTask(c *gin.Context) {
	id := c.Param("id")
	existing, err := db.GetAutomationTask(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	var req automationTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := *existing
	applyAutomationTaskRequest(&task, req)

	if err := s.validateAutomationTask(&task); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.recomputeAutomationNextRun(&task); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	saved, err := db.UpsertAutomationTask(task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toAutomationTaskResponse(saved))
}

// applyAutomationTaskRequest 将请求中显式提供的字段覆盖到 task 上（未提供的字段保留原值）。
func applyAutomationTaskRequest(task *db.AutomationTask, req automationTaskRequest) {
	if req.Name != nil {
		task.Name = *req.Name
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if req.ActionType != nil {
		task.ActionType = *req.ActionType
	}
	if req.DeviceID != nil {
		task.DeviceID = *req.DeviceID
	}
	if req.SMSPhone != nil {
		task.SMSPhone = *req.SMSPhone
	}
	if req.SMSContent != nil {
		task.SMSContent = *req.SMSContent
	}
	if req.SMSDelayMinSec != nil {
		task.SMSDelayMinSec = *req.SMSDelayMinSec
	}
	if req.SMSDelayMaxSec != nil {
		task.SMSDelayMaxSec = *req.SMSDelayMaxSec
	}
	if req.SMSRetryCount != nil {
		task.SMSRetryCount = *req.SMSRetryCount
	}
	if req.TriggerType != nil {
		task.TriggerType = *req.TriggerType
	}
	if req.FixedTimes != nil {
		task.SetTimes(req.FixedTimes)
	}
	if req.Weekdays != nil {
		task.SetWeekdaySet(req.Weekdays)
	}
	if req.IntervalValue != nil {
		task.IntervalValue = *req.IntervalValue
	}
	if req.IntervalUnit != nil {
		task.IntervalUnit = *req.IntervalUnit
	}
}

// recomputeAutomationNextRun 在任务创建/更新后重新计算 NextRunAt，确保列表页"下次运行"无需额外请求即可展示。
func (s *Server) recomputeAutomationNextRun(task *db.AutomationTask) error {
	next, err := automation.ComputeNextRun(*task, time.Now())
	if err != nil {
		return err
	}
	task.NextRunAt = &next
	return nil
}

func (s *Server) handleToggleAutomationTask(c *gin.Context) {
	id := c.Param("id")
	existing, err := db.GetAutomationTask(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled 不能为空"})
		return
	}

	task := *existing
	task.Enabled = *req.Enabled
	if task.Enabled {
		// 重新从当前时刻计算下次运行时间，避免任务停用期间累积的过期排期在启用瞬间被误判为"已到期"。
		if err := s.recomputeAutomationNextRun(&task); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	saved, err := db.UpsertAutomationTask(task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toAutomationTaskResponse(saved))
}

func (s *Server) handleDeleteAutomationTask(c *gin.Context) {
	id := c.Param("id")
	if err := db.DeleteAutomationTask(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleRunAutomationTaskNow(c *gin.Context) {
	id := c.Param("id")
	task, err := db.GetAutomationTask(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if s.automation == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "自动化执行器未就绪"})
		return
	}

	go s.automation.RunTask(context.Background(), *task, db.AutomationTriggerSourceManual)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "任务已开始执行"})
}
