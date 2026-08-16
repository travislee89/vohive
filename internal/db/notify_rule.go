package db

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NotifyRule 是通知中心的转发规则：某种消息类型（v1 仅 "sms"）在满足匹配条件时，
// 转发到指定的一组通知渠道，并可自定义标题/正文模板。
type NotifyRule struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	MessageType    string    `gorm:"column:message_type;index" json:"message_type"` // v1 仅 "sms"
	Name           string    `gorm:"column:name" json:"name"`
	Enabled        bool      `gorm:"column:enabled" json:"enabled"`
	Priority       int       `gorm:"column:priority" json:"priority"`           // 数值越小越先匹配；命中即停止
	MatchField     string    `gorm:"column:match_field" json:"match_field"`     // any | content | sender
	MatchMethod    string    `gorm:"column:match_method" json:"match_method"`   // all | contains | not_contains | equals | regex
	MatchContent   string    `gorm:"column:match_content" json:"match_content"` // match_method=all 时可为空
	TargetChannels string    `gorm:"column:target_channels" json:"-"`           // JSON 编码的渠道 key 数组
	TitleTemplate  string    `gorm:"column:title_template" json:"title_template"`
	BodyMode       string    `gorm:"column:body_mode" json:"body_mode"` // plain | custom_json
	BodyTemplate   string    `gorm:"column:body_template" json:"body_template"`
	IsDefault      bool      `gorm:"column:is_default" json:"is_default"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (NotifyRule) TableName() string { return "notify_rules" }

// Channels 解码 TargetChannels 为渠道 key 列表。
func (r *NotifyRule) Channels() []string {
	if r == nil || strings.TrimSpace(r.TargetChannels) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(r.TargetChannels), &out); err != nil {
		return nil
	}
	return out
}

// SetChannels 编码渠道 key 列表到 TargetChannels。
func (r *NotifyRule) SetChannels(channels []string) {
	if r == nil {
		return
	}
	if len(channels) == 0 {
		r.TargetChannels = "[]"
		return
	}
	b, err := json.Marshal(channels)
	if err != nil {
		r.TargetChannels = "[]"
		return
	}
	r.TargetChannels = string(b)
}

// ListNotifyRules 按 message_type 列出规则（为空则列出全部），按优先级/创建时间排序。
func ListNotifyRules(messageType string) ([]NotifyRule, error) {
	if DB == nil {
		return nil, nil
	}
	q := DB.Order("priority asc, created_at asc")
	if messageType = strings.TrimSpace(messageType); messageType != "" {
		q = q.Where("message_type = ?", messageType)
	}
	var out []NotifyRule
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ListEnabledNotifyRules 列出某消息类型下已启用的规则，按优先级排序（供匹配引擎使用）。
func ListEnabledNotifyRules(messageType string) ([]NotifyRule, error) {
	if DB == nil {
		return nil, nil
	}
	var out []NotifyRule
	err := DB.Where("message_type = ? AND enabled = ?", messageType, true).
		Order("priority asc, created_at asc").
		Find(&out).Error
	return out, err
}

// GetNotifyRule 按 ID 读取单条规则；不存在返回 (nil, nil)。
func GetNotifyRule(id string) (*NotifyRule, error) {
	id = strings.TrimSpace(id)
	if id == "" || DB == nil {
		return nil, nil
	}
	var out NotifyRule
	err := DB.First(&out, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

// UpsertNotifyRule 创建或更新一条规则；ID 为空时自动生成。
func UpsertNotifyRule(r NotifyRule) (NotifyRule, error) {
	if DB == nil {
		return NotifyRule{}, errors.New("db 未初始化")
	}
	r.MessageType = strings.TrimSpace(r.MessageType)
	r.Name = strings.TrimSpace(r.Name)
	if r.MessageType == "" {
		return NotifyRule{}, errors.New("message_type 不能为空")
	}
	now := time.Now()
	if strings.TrimSpace(r.ID) == "" {
		r.ID = uuid.NewString()
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	if err := DB.Save(&r).Error; err != nil {
		return NotifyRule{}, err
	}
	return r, nil
}

// DeleteNotifyRule 删除一条规则。
func DeleteNotifyRule(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("empty id")
	}
	if DB == nil {
		return nil
	}
	return DB.Delete(&NotifyRule{}, "id = ?", id).Error
}

// CountNotifyRulesByType 返回某消息类型下 (已启用数, 总数)，供前端侧边栏统计展示。
func CountNotifyRulesByType(messageType string) (enabled int64, total int64, err error) {
	if DB == nil {
		return 0, 0, nil
	}
	messageType = strings.TrimSpace(messageType)
	if err = DB.Model(&NotifyRule{}).Where("message_type = ?", messageType).Count(&total).Error; err != nil {
		return 0, 0, err
	}
	if err = DB.Model(&NotifyRule{}).Where("message_type = ? AND enabled = ?", messageType, true).Count(&enabled).Error; err != nil {
		return 0, 0, err
	}
	return enabled, total, nil
}

// SeedDefaultSMSRuleIfEmpty 首次启动时为 SMS 播种一条“默认短信规则”（全部匹配，转发到当前所有已启用渠道），
// 确保升级后不会因为规则表为空而静默丢失原本无条件转发的行为。
// 使用 notify_settings.default_rule_seeded 标记只播种一次：用户之后主动删除该规则不会被重新种回。
func SeedDefaultSMSRuleIfEmpty(enabledChannelKeys []string) error {
	if DB == nil {
		return nil
	}
	settings, err := GetNotifySettings()
	if err != nil {
		return err
	}
	if settings.DefaultRuleSeeded {
		return nil
	}
	rule := NotifyRule{
		MessageType:   "sms",
		Name:          "默认短信规则",
		Enabled:       true,
		Priority:      0,
		MatchField:    "any",
		MatchMethod:   "all",
		TitleTemplate: "",
		BodyMode:      "plain",
		IsDefault:     true,
	}
	rule.SetChannels(enabledChannelKeys)
	if _, err := UpsertNotifyRule(rule); err != nil {
		return err
	}
	settings.DefaultRuleSeeded = true
	return UpsertNotifySettings(settings)
}
