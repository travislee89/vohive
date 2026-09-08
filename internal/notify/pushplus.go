package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/pkg/logger"
)

type PushplusChannel struct {
	cfg      config.PushplusConfig
	client   *http.Client
	retryMax int
}

func NewPushplusChannel(cfg config.PushplusConfig) (*PushplusChannel, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("pushplus token is required")
	}
	return &PushplusChannel{
		cfg:      cfg,
		retryMax: normalizeRetryMax(cfg.RetryMax),
		client:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *PushplusChannel) Name() string {
	return "pushplus"
}

func (c *PushplusChannel) Send(text string) error {
	return c.SendWithContext(NotificationContext{Event: "通知", Text: text})
}

func (c *PushplusChannel) SendWithContext(ctx NotificationContext) error {
	title := fmt.Sprintf("[Vohive] %s", ctx.Event)
	if label := ctx.DeviceLabel(); label != "未知设备" {
		title = fmt.Sprintf("[Vohive] %s - %s", ctx.Event, label)
	}

	payload := map[string]interface{}{
		"token":    c.cfg.Token,
		"title":    title,
		"content":  ctx.Text,
		"template": "markdown",
	}

	if c.cfg.Topic != "" {
		payload["topic"] = c.cfg.Topic
	}

	channel := c.cfg.Channel
	if channel == "" {
		channel = "wechat"
	}
	payload["channel"] = channel

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if err := c.postWithRetry(body); err != nil {
		logger.Warn("Pushplus 发送失败", "err", err)
		return err
	}

	return nil
}

// postWithRetry 向 Pushplus 发送请求，对 5xx 和网络错误执行指数退避重试
func (c *PushplusChannel) postWithRetry(body []byte) error {
	var lastErr error

	for attempt := 0; attempt <= c.retryMax; attempt++ {
		if attempt > 0 {
			time.Sleep(retryBackoff(attempt))
		}

		resp, err := c.client.Post("http://www.pushplus.plus/send", "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			logger.Debug("Pushplus 请求失败，准备重试", "attempt", attempt+1, "err", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return nil
		}

		if !retryableHTTPStatus(resp.StatusCode) {
			return fmt.Errorf("http status code %d，不重试", resp.StatusCode)
		}

		lastErr = fmt.Errorf("http status code %d", resp.StatusCode)
		logger.Debug("Pushplus 返回 5xx，准备重试", "attempt", attempt+1, "status", resp.StatusCode)
	}

	return fmt.Errorf("pushplus 推送失败（已重试 %d 次）: %w", c.retryMax, lastErr)
}

func (c *PushplusChannel) RegisterCommand(cmd string, handler CommandHandler) {
	// Pushplus 不支持接收指令
}

func (c *PushplusChannel) Start() error {
	return nil
}

func (c *PushplusChannel) Close() error {
	return nil
}
