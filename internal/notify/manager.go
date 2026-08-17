package notify

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/internal/db"
	"github.com/travislee89/vohive/internal/device"
	"github.com/travislee89/vohive/pkg/logger"
)

// Manager 统一通知管理器
// 持有多个 Channel 实例，向所有已启用渠道广播通知和命令
type Manager struct {
	pool     *device.Pool
	channels []Channel // 所有已启用的通知渠道
}

type NotificationContext struct {
	Event      string
	Text       string
	DeviceID   string
	DeviceName string
	Timestamp  time.Time
	SMS        *SMSContext        // 仅 sms_received 事件非空，供转发规则匹配/模板渲染使用
	Automation *AutomationContext // 仅 automation_event 事件非空，供转发规则匹配/模板渲染使用
}

// SMSContext 携带短信通知的结构化字段，供转发规则做条件匹配与模板变量渲染。
type SMSContext struct {
	ID         uint   // 对应的 db.SMS 行 ID；0 表示调用方无法关联到具体的落库记录
	Sender     string // 发送方号码
	Content    string // 短信内容
	Source     string // 蜂窝 | VoWiFi
	LocalPhone string // 本机号码（best-effort 解析，可能为空）
	Operator   string // 运营商（best-effort 解析，可能为空）
}

// AutomationContext 携带自动化任务执行结果的结构化字段，供转发规则做条件匹配与模板变量渲染。
type AutomationContext struct {
	TaskID        string
	TaskName      string
	ActionType    string // reboot | sms
	DeviceID      string
	Status        string // success | failed
	ResultSummary string
	ErrorDetail   string
}

func (c NotificationContext) DeviceLabel() string {
	id := strings.TrimSpace(c.DeviceID)
	name := strings.TrimSpace(c.DeviceName)
	if name != "" && id != "" {
		return fmt.Sprintf("%s (%s)", name, id)
	}
	if name != "" {
		return name
	}
	if id != "" {
		return id
	}
	return "未知设备"
}

type contextualChannel interface {
	SendWithContext(ctx NotificationContext) error
}

// NewManager 根据配置创建通知管理器，初始化所有已启用的通知渠道
func NewManager(cfg *config.Config, pool *device.Pool) (*Manager, error) {
	m := &Manager{
		pool: pool,
	}

	// 初始化所有通知渠道
	if err := m.initChannels(cfg); err != nil {
		return nil, err
	}

	// 首次启动时为 SMS 播种一条默认转发规则（全部匹配，转发到当前所有已启用渠道），
	// 确保升级后不会因规则表为空而静默丢失原本无条件转发的行为。幂等：仅播种一次。
	if err := db.SeedDefaultSMSRuleIfEmpty(enabledChannelKeys(cfg)); err != nil {
		logger.Warn("播种默认短信转发规则失败", "err", err)
	}

	return m, nil
}

// enabledChannelKeys 返回配置中当前已启用渠道的 key 列表（与各 Channel.Name() 返回值一致）。
func enabledChannelKeys(cfg *config.Config) []string {
	var keys []string
	if cfg.Telegram.Enabled {
		keys = append(keys, "telegram")
	}
	if cfg.Feishu.Enabled {
		keys = append(keys, "feishu")
	}
	if cfg.QQ.Enabled {
		keys = append(keys, "qq")
	}
	if cfg.Webhook.Enabled {
		keys = append(keys, "webhook")
	}
	if cfg.Bark.Enabled {
		keys = append(keys, "bark")
	}
	if cfg.Email.Enabled {
		keys = append(keys, "email")
	}
	if cfg.Pushplus.Enabled {
		keys = append(keys, "pushplus")
	}
	return keys
}

// initChannels 根据配置创建并启动所有通知渠道
func (m *Manager) initChannels(cfg *config.Config) error {
	m.channels = nil

	// Telegram 渠道
	if cfg.Telegram.Enabled {
		tg, err := NewTelegramChannel(cfg.Telegram)
		if err != nil {
			logger.Error("初始化 Telegram 渠道失败", "err", err)
			return err
		}
		if tg != nil {
			m.channels = append(m.channels, tg)
		}
	}

	// 飞书渠道
	if cfg.Feishu.Enabled {
		fs, err := NewFeishuChannel(cfg.Feishu)
		if err != nil {
			logger.Error("初始化飞书渠道失败", "err", err)
			return err
		}
		if fs != nil {
			m.channels = append(m.channels, fs)
		}
	}

	// QQ 渠道
	if cfg.QQ.Enabled {
		qq, err := NewQQChannel(cfg.QQ)
		if err != nil {
			logger.Error("初始化 QQ 渠道失败", "err", err)
			return err
		}
		if qq != nil {
			m.channels = append(m.channels, qq)
		}
	}

	// Webhook 渠道
	if cfg.Webhook.Enabled {
		wh, err := NewWebhookChannel(cfg.Webhook)
		if err != nil {
			logger.Error("初始化 Webhook 渠道失败", "err", err)
			return err
		}
		if wh != nil {
			m.channels = append(m.channels, wh)
		}
	}

	// Bark 渠道
	if cfg.Bark.Enabled {
		bk, err := NewBarkChannel(cfg.Bark)
		if err != nil {
			logger.Error("初始化 Bark 渠道失败", "err", err)
			return err
		}
		if bk != nil {
			m.channels = append(m.channels, bk)
		}
	}

	// Email 渠道
	if cfg.Email.Enabled {
		em, err := NewEmailChannel(cfg.Email)
		if err != nil {
			logger.Error("初始化 Email 渠道失败", "err", err)
			return err
		}
		if em != nil {
			m.channels = append(m.channels, em)
		}
	}

	// Pushplus 渠道
	if cfg.Pushplus.Enabled {
		pp, err := NewPushplusChannel(cfg.Pushplus)
		if err != nil {
			logger.Error("初始化 Pushplus 渠道失败", "err", err)
			return err
		}
		if pp != nil {
			m.channels = append(m.channels, pp)
		}
	}

	// 向所有渠道注册命令
	m.registerCommands()

	// 启动所有渠道的命令监听
	for _, ch := range m.channels {
		ch := ch
		go func() {
			if err := ch.Start(); err != nil {
				logger.Error("通知渠道命令监听失败", "channel", ch.Name(), "err", err)
			}
		}()
	}

	return nil
}

// registerCommands 向所有已启用渠道注册同一组命令处理器
func (m *Manager) registerCommands() {
	type registeredCommand struct {
		cmd     string
		desc    string
		handler CommandHandler
	}

	// 有序定义命令，保证命令菜单展示顺序稳定
	commands := []registeredCommand{
		{"list", "查看设备列表与运行状态", m.handleCmdList},
		{"sms", "查看最近短信记录", m.handleCmdSMSInbox},
		{"esim", "查看设备 eSIM 配置", m.handleCmdEsim},
		{"switch", "切换 eSIM Profile", m.handleCmdSwitch},
		{"vocall", "发起 VoWiFi 模拟呼叫", m.handleCmdCall},
		{"send", "发送短信", m.handleCmdSendSMS},
		{"status", "查看设备详细状态", m.handleCmdStatus},
		{"rotate", "切换公网 IP", m.handleCmdRotate},
	}

	for _, ch := range m.channels {
		for _, c := range commands {
			ch.RegisterCommand(c.cmd, c.handler)
		}
	}

	// 向支持命令菜单的渠道（如 Telegram）推送命令说明
	menu := make([]ChannelCommand, 0, len(commands))
	for _, c := range commands {
		menu = append(menu, ChannelCommand{Command: c.cmd, Description: c.desc})
	}
	for _, ch := range m.channels {
		if menuCh, ok := ch.(commandMenuChannel); ok {
			if err := menuCh.SetCommandMenu(menu); err != nil {
				logger.Warn("设置命令菜单失败", "channel", ch.Name(), "err", err)
			}
		}
	}
}

// Close 关闭所有通知渠道
func (m *Manager) Close() {
	for _, ch := range m.channels {
		_ = ch.Close()
	}
}

// UpdateConfig 重新加载通知配置（热更新）
func (m *Manager) UpdateConfig(cfg *config.Config) error {
	// 关闭现有渠道
	m.Close()
	m.channels = nil

	// 重新初始化所有渠道
	return m.initChannels(cfg)
}

// NotifySMS 实现 device.Notifier 接口 — 收到短信通知
func (m *Manager) NotifySMS(deviceID, sender, content string, timestamp time.Time) {
	m.notifySMS(0, deviceID, sender, content, "蜂窝", timestamp)
}

func (m *Manager) NotifySMSWithSource(deviceID, sender, content, source string, timestamp time.Time) {
	m.notifySMS(0, deviceID, sender, content, source, timestamp)
}

// NotifySMSWithID 实现 device.SMSIDNotifier 接口 — 收到短信通知，且调用方已知对应的
// db.SMS 行 ID，转发结果（成功/失败/部分成功）会回写到该行，供短信中心展示转发状态。
func (m *Manager) NotifySMSWithID(smsID uint, deviceID, sender, content, source string, timestamp time.Time) {
	m.notifySMS(smsID, deviceID, sender, content, source, timestamp)
}

func (m *Manager) notifySMS(smsID uint, deviceID, sender, content, source string, timestamp time.Time) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "蜂窝"
	}
	msg := fmt.Sprintf("收到新短信 / %s\n设备  %s\n号码  %s\n时间  %s\n内容  %s",
		source, deviceID, sender, timestamp.Format("2006-01-02 15:04:05"), content)

	logger.Info("开始发送短信通知",
		"event", "sms_received",
		"sms_device", deviceID,
		"source", source,
		"channel_count", len(m.channels))

	localPhone, operator := m.resolveSMSMeta(deviceID)

	m.broadcastWithContext(NotificationContext{
		Event:      "sms_received",
		Text:       msg,
		DeviceID:   deviceID,
		DeviceName: m.resolveDeviceName(deviceID),
		Timestamp:  timestamp,
		SMS: &SMSContext{
			ID:         smsID,
			Sender:     sender,
			Content:    content,
			Source:     source,
			LocalPhone: localPhone,
			Operator:   operator,
		},
	})
}

// resolveSMSMeta best-effort 解析设备当前 ICCID/IMSI 对应的本机号码与运营商，
// 供转发规则模板变量使用；任何一步失败都静默返回空字符串，不影响通知发送。
func (m *Manager) resolveSMSMeta(deviceID string) (localPhone, operator string) {
	if m.pool == nil || strings.TrimSpace(deviceID) == "" {
		return "", ""
	}
	worker := m.pool.GetWorker(deviceID)
	if worker == nil {
		return "", ""
	}
	imsi := worker.GetIMSI()
	iccid := worker.CurrentICCID()
	if phone, err := db.GetPhoneNumberByIMSIOrICCID(imsi, iccid); err == nil {
		localPhone = phone
	}
	if op, err := db.GetSIMOperatorByIMSI(imsi); err == nil {
		operator = op
	}
	return localPhone, operator
}

// NotifyAutomationEvent 发送自动化中心任务执行结果通知——由 internal/automation.Runner 在每次
// 任务执行（含"全部设备"展开后的每个设备）后调用，走与 SMS 相同的转发规则引擎（message_type=automation_event）。
func (m *Manager) NotifyAutomationEvent(ctx AutomationContext) {
	statusLabel := "成功"
	if ctx.Status != db.NotifyLogStatusSuccess {
		statusLabel = "失败"
	}
	actionLabel := ctx.ActionType
	switch ctx.ActionType {
	case "reboot":
		actionLabel = "重启基带"
	case "sms":
		actionLabel = "发送短信"
	}
	msg := fmt.Sprintf("自动化任务 / %s\n任务    %s\n动作    %s\n设备    %s\n结果    %s",
		statusLabel, ctx.TaskName, actionLabel, ctx.DeviceID, ctx.ResultSummary)

	m.broadcastWithContext(NotificationContext{
		Event:      "automation_event",
		Text:       msg,
		DeviceID:   ctx.DeviceID,
		DeviceName: m.resolveDeviceName(ctx.DeviceID),
		Timestamp:  time.Now(),
		Automation: &ctx,
	})
}

// NotifyRaw 发送原始文本通知到所有渠道
func (m *Manager) NotifyRaw(msg string) {
	m.broadcastWithContext(NotificationContext{
		Event:     "raw",
		Text:      msg,
		Timestamp: time.Now(),
	})
}

// NotifyIPRotated 实现 device.Notifier 接口 — IP 切换通知
func (m *Manager) NotifyIPRotated(deviceID, oldIP, newIP string, duration time.Duration) {
	displayName := deviceID
	if m.pool != nil {
		if worker := m.pool.GetWorker(deviceID); worker != nil && worker.Config.Name != "" {
			displayName = fmt.Sprintf("%s (%s)", worker.Config.Name, deviceID)
		}
	}
	msg := fmt.Sprintf("公网切换 / 完成\n设备    %s\n旧 IP   %s\n新 IP   %s\n耗时    %s", displayName, oldIP, newIP, duration.String())
	m.broadcastWithContext(NotificationContext{
		Event:      "ip_rotated",
		Text:       msg,
		DeviceID:   deviceID,
		DeviceName: m.resolveDeviceName(deviceID),
		Timestamp:  time.Now(),
	})
}

// NotifyIncomingCall 实现 voice.CallNotifier 接口 — 来电通知
func (m *Manager) NotifyIncomingCall(deviceID, caller, callee string) {
	if len(m.channels) == 0 {
		return
	}

	msg := fmt.Sprintf("来电通知\n设备    %s\n主叫    %s\n被叫    %s",
		deviceID, caller, callee)

	logger.Info("开始发送来电通知", "device", deviceID, "caller", caller, "channel_count", len(m.channels))

	m.broadcastWithContext(NotificationContext{
		Event:      "incoming_call",
		Text:       msg,
		DeviceID:   deviceID,
		DeviceName: m.resolveDeviceName(deviceID),
		Timestamp:  time.Now(),
	})
}

func (m *Manager) resolveDeviceName(deviceID string) string {
	if strings.TrimSpace(deviceID) == "" || m.pool == nil {
		return ""
	}
	worker := m.pool.GetWorker(deviceID)
	if worker == nil {
		return ""
	}
	return strings.TrimSpace(worker.Config.Name)
}

func (m *Manager) broadcastWithContext(ctx NotificationContext) {
	ctx.Text = strings.TrimSpace(ctx.Text)
	if ctx.Text == "" {
		return
	}
	if ctx.Timestamp.IsZero() {
		ctx.Timestamp = time.Now()
	}
	if strings.TrimSpace(ctx.Event) == "" {
		ctx.Event = "notification"
	}

	// SMS / 自动化事件走转发规则引擎（按规则匹配后有选择地转发到目标渠道并记录日志）；
	// 其余事件类型（ip_rotated/incoming_call/raw 等）v1 未建规则，保持原有「广播到所有已启用渠道」行为。
	switch {
	case ctx.SMS != nil:
		m.broadcastWithRules(ctx, "sms")
		return
	case ctx.Automation != nil:
		m.broadcastWithRules(ctx, "automation_event")
		return
	}

	for _, ch := range m.channels {
		ch := ch // capture variable
		go func() {
			if withCtx, ok := ch.(contextualChannel); ok {
				if err := withCtx.SendWithContext(ctx); err != nil {
					logger.Warn("通知渠道发送失败", "channel", ch.Name(), "event", ctx.Event, "err", err)
				}
				return
			}
			if err := ch.Send(ctx.Text); err != nil {
				logger.Warn("通知渠道发送失败", "channel", ch.Name(), "event", ctx.Event, "err", err)
			}
		}()
	}
}

// channelsByName 按 Channel.Name() 建立当前已启用渠道的索引，供规则按渠道 key 定向发送。
func (m *Manager) channelsByName() map[string]Channel {
	out := make(map[string]Channel, len(m.channels))
	for _, ch := range m.channels {
		out[ch.Name()] = ch
	}
	return out
}

// broadcastWithRules 用转发规则引擎处理指定消息类型（sms / automation_event）的通知：
// 匹配到规则则渲染模板并按规则指定的渠道发送，每次渠道发送尝试都写一条转发日志；
// 未匹配到任何规则则不转发，写一条 status=unmatched 的日志。
func (m *Manager) broadcastWithRules(ctx NotificationContext, messageType string) {
	var smsIDPtr *uint
	if ctx.SMS != nil && ctx.SMS.ID != 0 {
		id := ctx.SMS.ID
		smsIDPtr = &id
	}

	rule, err := evaluateRules(messageType, ctx)
	if err != nil {
		logger.Warn("转发规则匹配失败", "event", ctx.Event, "err", err)
	}
	if rule == nil {
		if err := db.CreateNotifyLog(db.NotifyLog{
			MessageType:    messageType,
			Status:         db.NotifyLogStatusUnmatched,
			ContentSummary: db.SummarizeNotifyLogContent(ctx.Text),
			DeviceID:       ctx.DeviceID,
			SMSID:          smsIDPtr,
			Timestamp:      ctx.Timestamp,
		}); err != nil {
			logger.Warn("写入转发日志失败", "err", err)
		}
		// 未命中规则 = 未转发，sms 行的 ForwardStatus 默认值 0 语义已经正确，无需回写。
		return
	}

	values := buildTemplateValues(messageType, ctx)
	title := RenderPlaceholders(rule.TitleTemplate, values)
	body := ctx.Text
	if strings.TrimSpace(rule.BodyTemplate) != "" {
		body = RenderPlaceholders(rule.BodyTemplate, values)
	}
	renderedText := body
	if strings.TrimSpace(title) != "" {
		renderedText = title + "\n" + body
	}

	// sendCtx 保留原始事件的设备/时间等元数据，仅替换文本为规则渲染结果——
	// 这样 Webhook/Bark/Email/Pushplus 等实现了 SendWithContext 的渠道仍能拿到完整上下文
	// （包括它们自己独立配置的模板，如 Webhook 的 text_template，会在此基础上再包一层信封，而不是丢失元数据）。
	sendCtx := ctx
	sendCtx.Text = renderedText

	byName := m.channelsByName()
	ruleID := rule.ID
	ruleName := rule.Name

	// wg/resultMu 用于在所有目标渠道各自异步发送完成后，聚合出一个整体转发状态回写到 sms 行；
	// 不影响每个渠道各自的发送逻辑和各自的 NotifyLog 写入，聚合本身也在独立 goroutine 里等待，
	// 不会阻塞 broadcastWithRules 的 fire-and-forget 语义。
	var wg sync.WaitGroup
	var resultMu sync.Mutex
	var attemptedChannels []string
	var successCount, failCount int

	for _, target := range rule.Channels() {
		ch, ok := byName[target]
		if !ok {
			continue // 规则目标渠道当前未启用/不存在，静默跳过
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			var sendErr error
			switch {
			case rule.BodyMode == SendBodyModeCustomJSON && target == "webhook":
				if wh, ok := ch.(*WebhookChannel); ok {
					sendErr = wh.SendRaw(body)
				} else if withCtx, ok := ch.(contextualChannel); ok {
					sendErr = withCtx.SendWithContext(sendCtx)
				} else {
					sendErr = ch.Send(renderedText)
				}
			default:
				if withCtx, ok := ch.(contextualChannel); ok {
					sendErr = withCtx.SendWithContext(sendCtx)
				} else {
					sendErr = ch.Send(renderedText)
				}
			}

			status := db.NotifyLogStatusSuccess
			errDetail := ""
			if sendErr != nil {
				status = db.NotifyLogStatusFailed
				errDetail = sendErr.Error()
				logger.Warn("转发规则渠道发送失败", "channel", target, "rule", ruleName, "err", sendErr)
			}
			if err := db.CreateNotifyLog(db.NotifyLog{
				MessageType:     messageType,
				Status:          status,
				ContentSummary:  db.SummarizeNotifyLogContent(renderedText),
				MatchedRuleID:   &ruleID,
				MatchedRuleName: ruleName,
				Channel:         target,
				ErrorDetail:     errDetail,
				DeviceID:        ctx.DeviceID,
				SMSID:           smsIDPtr,
				Timestamp:       ctx.Timestamp,
			}); err != nil {
				logger.Warn("写入转发日志失败", "err", err)
			}

			resultMu.Lock()
			attemptedChannels = append(attemptedChannels, target)
			if sendErr != nil {
				failCount++
			} else {
				successCount++
			}
			resultMu.Unlock()
		}()
	}

	if smsIDPtr != nil {
		smsID := *smsIDPtr
		go func() {
			wg.Wait()
			resultMu.Lock()
			channels := append([]string(nil), attemptedChannels...)
			sc, fc := successCount, failCount
			resultMu.Unlock()
			if len(channels) == 0 {
				// 规则命中但目标渠道均不可用，视为未实际转发，保留 sms 行的默认"未转发"状态。
				return
			}
			status := db.SMSForwardStatusSuccess
			switch {
			case fc > 0 && sc > 0:
				status = db.SMSForwardStatusPartial
			case fc > 0 && sc == 0:
				status = db.SMSForwardStatusFailed
			}
			if err := db.UpdateSMSForwardStatus(smsID, status, channels, ruleName); err != nil {
				logger.Warn("更新短信转发状态失败", "sms_id", smsID, "err", err)
			}
		}()
	}
}

// GetChannelNames 返回所有已启用渠道的名称列表
func (m *Manager) GetChannelNames() []string {
	names := make([]string, 0, len(m.channels))
	for _, ch := range m.channels {
		names = append(names, ch.Name())
	}
	return names
}
