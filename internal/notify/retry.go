package notify

import (
	"net/http"
	"time"
)

// retryBackoff 返回第 attempt（从 1 开始）次重试前的等待时间：1s, 2s, 4s, 8s...
// 所有转发渠道统一使用该退避曲线，保持用户可预期的重试节奏。
func retryBackoff(attempt int) time.Duration {
	return time.Duration(1<<(attempt-1)) * time.Second
}

// normalizeRetryMax 将配置中的重试次数规整为非负值（负数视为不重试）。
func normalizeRetryMax(retryMax int) int {
	if retryMax < 0 {
		return 0
	}
	return retryMax
}

// retryableHTTPStatus 判断 HTTP 状态码是否值得重试：5xx（服务端错误）和 429（限流，
// 稍后重试即可恢复）重试；其余 4xx 视为参数/鉴权等客户端错误，重试无意义，直接放弃。
func retryableHTTPStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}
