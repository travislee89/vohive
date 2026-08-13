package notify

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestBuildTelegramTextMessageKeepsRawSMSContent(t *testing.T) {
	t.Parallel()

	text := "📩 收到新短信\n内容: <#> 验证码 #123456 <b>TAG</b>"
	msg := buildTelegramTextMessage(12345, text)

	wantText := "📩 收到新短信\n内容: &lt;#&gt; 验证码 #123456 &lt;b&gt;TAG&lt;/b&gt;"
	if msg.Text != wantText {
		t.Fatalf("Text = %q, want %q", msg.Text, wantText)
	}
	if msg.ParseMode != "HTML" {
		t.Fatalf("ParseMode = %q, want HTML", msg.ParseMode)
	}
	if msg.ChatID != 12345 {
		t.Fatalf("ChatID = %d, want 12345", msg.ChatID)
	}
}

func TestUnknownCommandReplyUsesPlainTemplate(t *testing.T) {
	t.Parallel()

	got := unknownCommandReply("badcmd")
	want := "未知命令 / badcmd\n提示    请检查命令名或使用 /list、/status、/send 等已注册命令"
	if got != want {
		t.Fatalf("unknownCommandReply() = %q, want %q", got, want)
	}
}

func TestCommandMenuEqual(t *testing.T) {
	t.Parallel()

	base := []tgbotapi.BotCommand{
		{Command: "list", Description: "查看设备列表与运行状态"},
		{Command: "send", Description: "发送短信"},
		{Command: "status", Description: "查看设备详细状态"},
	}

	// 相同命令、顺序不同应视为相等（与顺序无关）
	same := []tgbotapi.BotCommand{
		{Command: "status", Description: "查看设备详细状态"},
		{Command: "list", Description: "查看设备列表与运行状态"},
		{Command: "send", Description: "发送短信"},
	}
	if !commandMenuEqual(base, same) {
		t.Fatalf("commandMenuEqual(base, same) = false, want true")
	}

	// 长度不同
	if commandMenuEqual(base, base[:2]) {
		t.Fatalf("commandMenuEqual(base, shorter) = true, want false")
	}

	// 说明文案不同
	diffDesc := []tgbotapi.BotCommand{
		{Command: "list", Description: "查看设备列表与运行状态"},
		{Command: "send", Description: "发送短信"},
		{Command: "status", Description: "不同的说明文案"},
	}
	if commandMenuEqual(base, diffDesc) {
		t.Fatalf("commandMenuEqual(base, diffDesc) = true, want false")
	}

	// 命令名不同
	diffCmd := []tgbotapi.BotCommand{
		{Command: "list", Description: "查看设备列表与运行状态"},
		{Command: "send", Description: "发送短信"},
		{Command: "other", Description: "查看设备详细状态"},
	}
	if commandMenuEqual(base, diffCmd) {
		t.Fatalf("commandMenuEqual(base, diffCmd) = true, want false")
	}

	// 两个空列表
	if !commandMenuEqual(nil, []tgbotapi.BotCommand{}) {
		t.Fatalf("commandMenuEqual(nil, []) = false, want true")
	}
}

func TestParseCallbackData(t *testing.T) {
	t.Parallel()

	cases := []struct {
		data  string
		route string
		args  []string
	}{
		{"status:ec20_1", "status", []string{"ec20_1"}},
		{"switch:ec20_1:2", "switch", []string{"ec20_1", "2"}},
		{"esim", "esim", nil},
		{"rotate:dev1:dev2", "rotate", []string{"dev1", "dev2"}},
		{"send:ec20_1:+8613800000000:hi", "send", []string{"ec20_1", "+8613800000000", "hi"}},
		{"", "", nil},
	}

	for _, c := range cases {
		route, args := parseCallbackData(c.data)
		if route != c.route {
			t.Fatalf("parseCallbackData(%q) route = %q, want %q", c.data, route, c.route)
		}
		if len(args) != len(c.args) {
			t.Fatalf("parseCallbackData(%q) args = %v, want %v", c.data, args, c.args)
		}
		for i := range c.args {
			if args[i] != c.args[i] {
				t.Fatalf("parseCallbackData(%q) args = %v, want %v", c.data, args, c.args)
			}
		}
	}
}
