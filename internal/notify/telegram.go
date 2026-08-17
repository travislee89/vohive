package notify

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramChannel 实现 Channel 接口的 Telegram 通知渠道
type TelegramChannel struct {
	api      *tgbotapi.BotAPI
	chatID   int64
	handlers map[string]CommandHandler
}

// NewTelegramChannel 根据配置创建 Telegram 渠道
func NewTelegramChannel(cfg config.TelegramConfig) (*TelegramChannel, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	endpoint := tgbotapi.APIEndpoint
	if cfg.BaseURL != "" {
		endpoint = cfg.BaseURL
		if !strings.Contains(endpoint, "bot%s/%s") {
			endpoint = strings.TrimSuffix(endpoint, "/") + "/bot%s/%s"
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			logger.Error("解析 Telegram 代理地址失败", "proxy", cfg.Proxy, "err", err)
		} else {
			transport.Proxy = http.ProxyURL(proxyURL)
			logger.Info("Telegram Bot 使用代理", "proxy", cfg.Proxy)
		}
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
	}

	bot, err := tgbotapi.NewBotAPIWithClient(cfg.BotToken, endpoint, httpClient)
	if err != nil {
		msg := err.Error()
		if cfg.BotToken != "" {
			msg = strings.ReplaceAll(msg, cfg.BotToken, "<redacted>")
		}
		return nil, fmt.Errorf("创建 telegram bot 失败: %s", msg)
	}

	logger.Info("已授权账户 (TG)", "username", bot.Self.UserName)

	return &TelegramChannel{
		api:      bot,
		chatID:   cfg.ChatID,
		handlers: make(map[string]CommandHandler),
	}, nil
}

func (t *TelegramChannel) Name() string { return "telegram" }

func buildTelegramTextMessage(chatID int64, text string) tgbotapi.MessageConfig {
	// 过滤非法的 UTF-8 字符，防止 Telegram API 报错
	cleanText := strings.Map(func(r rune) rune {
		if r == utf8.RuneError {
			return -1 // 丢弃非法字符
		}
		return r
	}, text)

	escaped := html.EscapeString(cleanText)
	msg := tgbotapi.NewMessage(chatID, escaped)
	// 使用 HTML 模式，但先转义短信原文，避免 "<#>" 等内容被当作标签解析。
	msg.ParseMode = "HTML"
	return msg
}

func (t *TelegramChannel) Send(text string) error {
	if t == nil || t.api == nil {
		return nil
	}

	msg := buildTelegramTextMessage(t.chatID, text)
	_, err := t.api.Send(msg)
	if err != nil {
		logger.Error("发送 telegram 消息失败", "err", err)
		return err
	}
	return nil
}

func (t *TelegramChannel) RegisterCommand(cmd string, handler CommandHandler) {
	if t == nil {
		return
	}
	t.handlers[cmd] = handler
	logger.Info("注册 Telegram 命令", "command", "/"+cmd)
}

// SetCommandMenu 通过 Telegram setMyCommands API 将命令列表推送到客户端菜单。
// 使用 bot_command scope 限定为授权会话，使菜单只对授权用户可见。
// 会先调用 getMyCommands 与期望列表比对，若完全一致则跳过更新，避免无谓的 API 调用。
func (t *TelegramChannel) SetCommandMenu(commands []ChannelCommand) error {
	if t == nil || t.api == nil {
		return nil
	}

	botCommands := make([]tgbotapi.BotCommand, 0, len(commands))
	for _, c := range commands {
		botCommands = append(botCommands, tgbotapi.BotCommand{
			Command:     c.Command,
			Description: c.Description,
		})
	}

	var scope *tgbotapi.BotCommandScope
	if t.chatID != 0 {
		scope = &tgbotapi.BotCommandScope{Type: "chat", ChatID: t.chatID}
	}

	// 先读取当前命令菜单，若与期望一致则无需更新
	current, err := t.api.GetMyCommandsWithConfig(tgbotapi.GetMyCommandsConfig{Scope: scope})
	if err != nil {
		logger.Warn("获取 Telegram 当前命令菜单失败，将直接更新", "err", err)
	} else if commandMenuEqual(current, botCommands) {
		logger.Info("Telegram 命令菜单无变化，跳过更新", "count", len(botCommands))
		return nil
	}

	cfg := tgbotapi.SetMyCommandsConfig{Commands: botCommands, Scope: scope}
	if _, err := t.api.Request(cfg); err != nil {
		logger.Error("设置 Telegram 命令菜单失败", "err", err)
		return err
	}

	logger.Info("已设置 Telegram 命令菜单", "count", len(botCommands))
	return nil
}

// commandMenuEqual 判断两个命令列表是否等价（与顺序无关，按命令名匹配说明文案）
func commandMenuEqual(a, b []tgbotapi.BotCommand) bool {
	if len(a) != len(b) {
		return false
	}
	desc := make(map[string]string, len(a))
	for _, c := range a {
		desc[c.Command] = c.Description
	}
	for _, c := range b {
		if d, ok := desc[c.Command]; !ok || d != c.Description {
			return false
		}
	}
	return true
}

// tgCommandContext 实现了 CommandContext 接口
type tgCommandContext struct {
	channel *TelegramChannel
}

func (c *tgCommandContext) Reply(text string) {
	if c == nil || c.channel == nil {
		return
	}
	// 避免 Telegram API 短时阻塞拖住命令处理主循环
	go func() {
		_ = c.channel.Send(text)
	}()
}

// ShowDevicePicker 实现 MenuContext：通过 Telegram 内联键盘让用户点选设备或 eSIM 配置。
// 按钮回调数据形如 "route:value"（value 可为单级设备 ID，或多级 "设备ID:序号"）。
func (c *tgCommandContext) ShowDevicePicker(prompt, route string, items []MenuItem) error {
	if c == nil || c.channel == nil || c.channel.api == nil {
		return nil
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(items))
	for _, it := range items {
		data := route
		if it.Value != "" {
			data = route + ":" + it.Value
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(it.Label, data),
		))
	}

	msg := tgbotapi.NewMessage(c.channel.chatID, prompt)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)

	// 异步发送，避免阻塞命令处理主循环
	go func() {
		if _, err := c.channel.api.Send(msg); err != nil {
			logger.Error("发送 Telegram 交互菜单失败", "err", err)
		}
	}()
	return nil
}

// Start 启动 Telegram long-polling 命令监听（阻塞式）
func (t *TelegramChannel) Start() error {
	if t == nil || t.api == nil {
		return nil
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := t.api.GetUpdatesChan(u)
	logger.Info("Telegram Bot 命令监听已启动")

	for update := range updates {
		if update.CallbackQuery != nil {
			t.handleCallback(update.CallbackQuery)
			continue
		}
		if update.Message == nil || !update.Message.IsCommand() {
			continue
		}

		// 仅处理来自授权用户的命令
		if update.Message.Chat.ID != t.chatID {
			continue
		}

		command := update.Message.Command()
		args := strings.Fields(update.Message.CommandArguments())

		logger.Info("收到 Telegram 命令", "command", command, "args", args)

		handler, ok := t.handlers[command]
		if !ok {
			ctx := &tgCommandContext{channel: t}
			ctx.Reply(unknownCommandReply(command))
			continue
		}

		ctx := &tgCommandContext{channel: t}
		response := handler(ctx, args)
		if response != "" {
			ctx.Reply(response)
		}
	}
	return nil
}

// interactiveRoutes 允许通过 callback（内联键盘点选）触发的命令白名单。
// 刻意排除 /send、/vocall：两者含自由文本参数，不能由可伪造的 callback data 触发。
var interactiveRoutes = map[string]bool{
	"status": true,
	"esim":   true,
	"rotate": true,
	"switch": true,
}

// handleCallback 处理内联键盘回调（callback_query）
func (t *TelegramChannel) handleCallback(cb *tgbotapi.CallbackQuery) {
	if cb == nil {
		return
	}
	// 仅处理来自授权会话的回调，防止越权
	if cb.Message == nil || cb.Message.Chat.ID != t.chatID {
		return
	}
	// 应答回调，消除客户端加载态
	if _, err := t.api.Request(tgbotapi.NewCallback(cb.ID, "")); err != nil {
		logger.Warn("应答 Telegram 回调失败", "err", err)
	}

	route, args := parseCallbackData(cb.Data)
	// 仅允许白名单内的交互命令，防止伪造 callback data 触发 /send、/vocall 等
	if !interactiveRoutes[route] {
		logger.Warn("忽略非交互回调", "route", route, "data", cb.Data)
		return
	}
	handler, ok := t.handlers[route]
	if !ok {
		logger.Warn("回调命令未注册", "route", route)
		return
	}

	logger.Info("收到 Telegram 回调", "route", route, "args", args)
	ctx := &tgCommandContext{channel: t}
	resp := handler(ctx, args)
	if resp != "" {
		ctx.Reply(resp)
	}
}

// parseCallbackData 解析回调数据 "route:arg1:arg2..." 为路由与参数切片
func parseCallbackData(data string) (string, []string) {
	parts := strings.SplitN(data, ":", 2)
	route := parts[0]
	var args []string
	if len(parts) == 2 && parts[1] != "" {
		args = strings.Split(parts[1], ":")
	}
	return route, args
}

func (t *TelegramChannel) Close() error {
	if t == nil || t.api == nil {
		return nil
	}
	t.api.StopReceivingUpdates()
	return nil
}
