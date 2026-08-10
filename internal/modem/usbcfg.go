package modem

import (
	"fmt"
	"strings"
)

// BuildUSBCFGEnableCmd 解析 AT+QCFG="USBCFG"? 的响应，若 UAC 开关（最后一个字段）为 0，
// 则返回把该位改成 1、其余字段原样保留的写回命令（如 `AT+QCFG="usbcfg",0x2C7C,0x0125,1,1,1,1,1,0,1`）。
// modified 为 false 表示 UAC 已开启或响应格式不匹配（不支持该指令），调用方应视为无需更改。
// 该函数为纯函数，不依赖 Manager，便于在一次性 AT 会话中复用。
func BuildUSBCFGEnableCmd(resp string) (newCmd string, modified bool) {
	// 查找 +QCFG: "usbcfg" 或 +QCFG: "USBCFG"（大小写不敏感定位，但取原始大小写的值）
	idx := strings.Index(strings.ToLower(resp), `+qcfg: "usbcfg",`)
	if idx == -1 {
		return "", false
	}

	start := idx + 7 // Skip "+QCFG: " (7 chars)
	line := resp[start:]
	if end := strings.IndexAny(line, "\r\n"); end != -1 {
		line = line[:end]
	}
	line = strings.TrimSpace(line)

	// line 例: "usbcfg",0x2C7C,0x0125,1,1,1,1,1,0,0
	parts := strings.Split(line, ",")
	if len(parts) < 8 {
		return "", false
	}

	lastIdx := len(parts) - 1
	lastVal := strings.TrimSpace(parts[lastIdx])
	if lastVal != "0" {
		return "", false
	}

	parts[lastIdx] = "1"
	return fmt.Sprintf(`AT+QCFG=%s`, strings.Join(parts, ",")), true
}
