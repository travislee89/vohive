package automation

import (
	"crypto/rand"
	"math/big"
	"strings"
	"time"
)

// randomTokenAlphabet 用于生成 {{随机字符串}} 占位符的取值集合。
const randomTokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const randomTokenLength = 8

// randomToken 生成一个供 {{随机字符串}} 占位符使用的随机字母数字串。
func randomToken() string {
	b := make([]byte, randomTokenLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(randomTokenAlphabet))))
		if err != nil {
			// crypto/rand 出错概率极低；退化为基于当前时间的确定性取值，保证短信仍能正常发出。
			b[i] = randomTokenAlphabet[time.Now().UnixNano()%int64(len(randomTokenAlphabet))]
			continue
		}
		b[i] = randomTokenAlphabet[n.Int64()]
	}
	return string(b)
}

// RenderSMSTemplate 替换短信内容模板中的 {{时间}}/{{随机字符串}} 占位符。
// 与 internal/notify.RenderPlaceholders 不同：该处占位符 key 为中文，且当前仅这两个固定变量，
// 因此使用简单的字符串替换而非正则，避免为中文 key 重写共用的占位符正则。
func RenderSMSTemplate(tmpl string) string {
	replacer := strings.NewReplacer(
		"{{时间}}", time.Now().Format("2006-01-02 15:04:05"),
		"{{随机字符串}}", randomToken(),
	)
	return replacer.Replace(tmpl)
}
