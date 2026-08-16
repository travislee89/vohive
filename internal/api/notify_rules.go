package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/boa-z/vohive/internal/db"
	"github.com/gin-gonic/gin"
)

// validNotifyChannelKeys 是当前支持的固定渠道 key 集合，与各 notify.Channel.Name() 返回值一致。
var validNotifyChannelKeys = map[string]bool{
	"telegram": true,
	"feishu":   true,
	"qq":       true,
	"webhook":  true,
	"bark":     true,
	"email":    true,
	"pushplus": true,
}

// notifyRuleResponse 是 NotifyRule 面向前端的展开形式（target_channels 由内部 JSON 列解码为数组）。
type notifyRuleResponse struct {
	ID             string   `json:"id"`
	MessageType    string   `json:"message_type"`
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	Priority       int      `json:"priority"`
	MatchField     string   `json:"match_field"`
	MatchMethod    string   `json:"match_method"`
	MatchContent   string   `json:"match_content"`
	TargetChannels []string `json:"target_channels"`
	TitleTemplate  string   `json:"title_template"`
	BodyMode       string   `json:"body_mode"`
	BodyTemplate   string   `json:"body_template"`
	IsDefault      bool     `json:"is_default"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

func toNotifyRuleResponse(r db.NotifyRule) notifyRuleResponse {
	return notifyRuleResponse{
		ID:             r.ID,
		MessageType:    r.MessageType,
		Name:           r.Name,
		Enabled:        r.Enabled,
		Priority:       r.Priority,
		MatchField:     r.MatchField,
		MatchMethod:    r.MatchMethod,
		MatchContent:   r.MatchContent,
		TargetChannels: r.Channels(),
		TitleTemplate:  r.TitleTemplate,
		BodyMode:       r.BodyMode,
		BodyTemplate:   r.BodyTemplate,
		IsDefault:      r.IsDefault,
		CreatedAt:      r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      r.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *Server) handleListNotifyRules(c *gin.Context) {
	messageType := c.Query("type")
	rules, err := db.ListNotifyRules(messageType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]notifyRuleResponse, 0, len(rules))
	for _, r := range rules {
		out = append(out, toNotifyRuleResponse(r))
	}
	enabled, total, err := db.CountNotifyRulesByType(messageType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": out, "enabled": enabled, "total": total})
}

type notifyRuleRequest struct {
	MessageType    string   `json:"message_type"`
	Name           *string  `json:"name"`
	Enabled        *bool    `json:"enabled"`
	Priority       *int     `json:"priority"`
	MatchField     *string  `json:"match_field"`
	MatchMethod    *string  `json:"match_method"`
	MatchContent   *string  `json:"match_content"`
	TargetChannels []string `json:"target_channels"`
	TitleTemplate  *string  `json:"title_template"`
	BodyMode       *string  `json:"body_mode"`
	BodyTemplate   *string  `json:"body_template"`
}

func validateNotifyRuleChannels(channels []string) error {
	for _, ch := range channels {
		if !validNotifyChannelKeys[strings.TrimSpace(ch)] {
			return fmt.Errorf("未知的通知渠道: %s", ch)
		}
	}
	return nil
}

func (s *Server) handleCreateNotifyRule(c *gin.Context) {
	var req notifyRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.MessageType = strings.TrimSpace(req.MessageType)
	if req.MessageType == "" {
		req.MessageType = "sms"
	}
	if req.MessageType != "sms" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "v1 仅支持 message_type=sms"})
		return
	}
	name := strings.TrimSpace(derefString(req.Name))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 不能为空"})
		return
	}
	if err := validateNotifyRuleChannels(req.TargetChannels); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule := db.NotifyRule{
		MessageType:  req.MessageType,
		Name:         name,
		Enabled:      req.Enabled == nil || *req.Enabled,
		MatchField:   defaultString(derefString(req.MatchField), "any"),
		MatchMethod:  defaultString(derefString(req.MatchMethod), "all"),
		MatchContent: derefString(req.MatchContent),
		BodyMode:     defaultString(derefString(req.BodyMode), "plain"),
	}
	if req.Priority != nil {
		rule.Priority = *req.Priority
	}
	if req.TitleTemplate != nil {
		rule.TitleTemplate = *req.TitleTemplate
	}
	if req.BodyTemplate != nil {
		rule.BodyTemplate = *req.BodyTemplate
	}
	rule.SetChannels(req.TargetChannels)

	saved, err := db.UpsertNotifyRule(rule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toNotifyRuleResponse(saved))
}

func (s *Server) handleUpdateNotifyRule(c *gin.Context) {
	id := c.Param("id")
	existing, err := db.GetNotifyRule(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
		return
	}

	var req notifyRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.TargetChannels != nil {
		if err := validateNotifyRuleChannels(req.TargetChannels); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	rule := *existing
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		rule.Name = strings.TrimSpace(*req.Name)
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.Priority != nil {
		rule.Priority = *req.Priority
	}
	if req.MatchField != nil {
		rule.MatchField = *req.MatchField
	}
	if req.MatchMethod != nil {
		rule.MatchMethod = *req.MatchMethod
	}
	if req.MatchContent != nil {
		rule.MatchContent = *req.MatchContent
	}
	if req.TargetChannels != nil {
		rule.SetChannels(req.TargetChannels)
	}
	if req.TitleTemplate != nil {
		rule.TitleTemplate = *req.TitleTemplate
	}
	if req.BodyMode != nil {
		rule.BodyMode = *req.BodyMode
	}
	if req.BodyTemplate != nil {
		rule.BodyTemplate = *req.BodyTemplate
	}

	saved, err := db.UpsertNotifyRule(rule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toNotifyRuleResponse(saved))
}

func (s *Server) handleDeleteNotifyRule(c *gin.Context) {
	id := c.Param("id")
	if err := db.DeleteNotifyRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func defaultString(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
