package notify

// CommandContext 传递命令上下文，使得异步操作可以精准回复本会话
type CommandContext interface {
	Reply(text string)
}

// CommandHandler 命令处理器，接收上下文及参数切片，返回回复文本
type CommandHandler func(cmdCtx CommandContext, args []string) string

// ChannelCommand 描述一个可用于命令菜单的命令（名称 + 说明文案）
type ChannelCommand struct {
	Command     string // 命令名（不含 "/"）
	Description string // 展示给用户的说明
}

// commandMenuChannel 可选接口：支持向平台展示命令菜单的渠道（如 Telegram 的 setMyCommands）
type commandMenuChannel interface {
	SetCommandMenu(commands []ChannelCommand) error
}

// MenuItem 描述交互菜单中的一个可选项
type MenuItem struct {
	Label string // 按钮显示文字，如 "ec20_1 (名称)" 或 "ProfileA（尾号 1234）"
	Value string // 回调携带的数据（设备 ID，或 "设备ID:序号" 等多级参数）
}

// MenuContext 可选能力：支持展示交互菜单（如 Telegram 内联键盘）的渠道。
// 未实现该接口的渠道（飞书/QQ 等）会回退到普通文本用法提示。
type MenuContext interface {
	// ShowDevicePicker 展示一个可点选的选择菜单。route 为回调路由前缀（通常为命令名），
	// 按钮的回调数据形如 "route:value"。
	ShowDevicePicker(prompt, route string, items []MenuItem) error
}

// Channel 统一通知渠道接口
// 所有通知渠道（Telegram、飞书、未来的 Discord/Slack 等）均实现此接口
type Channel interface {
	// Name 返回渠道名称（如 "telegram"、"feishu"）
	Name() string

	// Send 发送文本消息
	Send(text string) error

	// RegisterCommand 注册命令处理器
	// cmd: 命令名（如 "send"），handler: 处理函数
	RegisterCommand(cmd string, handler CommandHandler)

	// Start 启动命令监听（阻塞式，应在 goroutine 中调用）
	Start() error

	// Close 释放资源，停止监听
	Close() error
}
