package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/pkg/logger"
)

// BarkChannel 实现 Channel 接口的 Bark 通知渠道
type BarkChannel struct {
	urls     []string
	group    string
	icon     string
	level    string
	client   *http.Client
	retryMax int // 最大重试次数
}

type barkPayload struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Group     string `json:"group,omitempty"`
	Icon      string `json:"icon,omitempty"`
	Level     string `json:"level,omitempty"`
	DeviceKey string `json:"device_key,omitempty"` // 一般在 URL 里带有，这里留空以兼容各种 URL 写法
}

func NewBarkChannel(cfg config.BarkConfig) (*BarkChannel, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if len(cfg.URLs) == 0 {
		logger.Warn("Bark 渠道已启用但未配置 urls，消息不会推送")
	}

	ch := &BarkChannel{
		urls:     cfg.URLs,
		group:    strings.TrimSpace(cfg.Group),
		icon:     strings.TrimSpace(cfg.Icon),
		level:    strings.TrimSpace(cfg.Level),
		retryMax: normalizeRetryMax(cfg.RetryMax),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	logger.Info("Bark 渠道已创建", "urls_count", len(cfg.URLs), "retry_max", ch.retryMax)
	return ch, nil
}

func (b *BarkChannel) Name() string { return "bark" }

func (b *BarkChannel) Send(text string) error {
	return b.SendWithContext(NotificationContext{
		Event:     "notification",
		Text:      text,
		Timestamp: time.Now(),
	})
}

func (b *BarkChannel) SendWithContext(ctx NotificationContext) error {
	_, err := b.SendWithContextDetailed(ctx)
	return err
}

type SendBarkResult struct {
	FailedURLs []string
}

func (b *BarkChannel) SendWithContextDetailed(ctx NotificationContext) (SendBarkResult, error) {
	result := SendBarkResult{}
	if b == nil || b.client == nil || len(b.urls) == 0 {
		return result, nil
	}

	text := strings.TrimSpace(ctx.Text)
	if text == "" {
		return result, nil
	}

	title := ctx.DeviceLabel()
	if strings.TrimSpace(title) == "" || title == "未知设备" {
		title = "Vohive Notification"
	}

	// 如果是特殊的事件类型，可以在这里定制 title
	if ctx.Event == "sms_received" {
		title = "💬 " + title
	} else if ctx.Event == "incoming_call" {
		title = "📞 " + title
	} else if ctx.Event == "ip_rotated" {
		title = "🔄 " + title
	}

	payload := barkPayload{
		Title: title,
		Body:  text,
		Group: b.group,
		Icon:  b.icon,
		Level: b.level,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return result, fmt.Errorf("序列化 bark payload 失败: %w", err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var lastErr error
	failedURLs := make([]string, 0)

	for _, u := range b.urls {
		wg.Add(1)
		go func(targetURL string) {
			defer wg.Done()
			if err := b.postWithRetry(targetURL, body); err != nil {
				mu.Lock()
				lastErr = err
				failedURLs = append(failedURLs, targetURL)
				mu.Unlock()
				logger.Warn("Bark 推送失败", "url", targetURL, "err", err)
			}
		}(u)
	}

	wg.Wait()
	result.FailedURLs = failedURLs
	return result, lastErr
}

// postWithRetry 向目标 URL 发送 POST 请求，对 5xx 和网络错误执行指数退避重试
func (b *BarkChannel) postWithRetry(targetURL string, body []byte) error {
	var lastErr error

	for attempt := 0; attempt <= b.retryMax; attempt++ {
		if attempt > 0 {
			time.Sleep(retryBackoff(attempt))
		}

		statusCode, err := b.doPost(targetURL, body)
		if err != nil {
			lastErr = err
			logger.Debug("Bark POST 失败，准备重试", "url", targetURL, "attempt", attempt+1, "err", err)
			continue
		}

		if statusCode < 400 {
			return nil
		}

		if !retryableHTTPStatus(statusCode) {
			return fmt.Errorf("HTTP 状态码错误: %d，不重试", statusCode)
		}

		lastErr = fmt.Errorf("HTTP 状态码错误: %d", statusCode)
		logger.Debug("Bark 返回 5xx，准备重试", "url", targetURL, "attempt", attempt+1, "status", statusCode)
	}

	return fmt.Errorf("bark 推送失败（已重试 %d 次）: %w", b.retryMax, lastErr)
}

// doPost 执行单次 HTTP POST 请求，返回状态码
func (b *BarkChannel) doPost(targetURL string, body []byte) (int, error) {
	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := b.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("请求发送失败: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

// RegisterCommand 空实现 — Bark 渠道不支持接收命令
func (b *BarkChannel) RegisterCommand(_ string, _ CommandHandler) {}

// Start 空实现 — Bark 渠道无需命令监听
func (b *BarkChannel) Start() error { return nil }

// Close 关闭 HTTP 客户端的空闲连接
func (b *BarkChannel) Close() error {
	if b != nil && b.client != nil {
		b.client.CloseIdleConnections()
	}
	return nil
}
