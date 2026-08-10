package cardpolicy

// Policy 是跟卡走的运行时策略快照（与 db.CardPolicy 字段对应，但不绑 gorm）。
type Policy struct {
	ICCID                  string
	NetworkEnabled         bool
	VoWiFiEnabled          bool
	AirplaneEnabled        bool
	IPVersion              string
	APN                    string
	QuotaEnabled           bool
	QuotaBytes             int64
	BillingDay             int
	BillingTimezone        string
	AutoStopEnabled        bool
	AutoStopThresholdBytes int64
}

// Resolver 把 ICCID 解析为策略；缺失实现方负责按默认模板自动建档。
type Resolver interface {
	Resolve(iccid string) (Policy, error)
}
