package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/travislee89/vohive/internal/db"
)

func parseAutomationLogTimeParam(c *gin.Context, name string) *time.Time {
	v := c.Query(name)
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil
	}
	return &t
}

func (s *Server) handleListAutomationLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	actionType := c.Query("action_type")
	status := c.Query("status")
	keyword := c.Query("q")
	start := parseAutomationLogTimeParam(c, "start")
	end := parseAutomationLogTimeParam(c, "end")

	logs, total, err := db.ListAutomationRunLogs(page, pageSize, actionType, status, keyword, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	c.JSON(http.StatusOK, gin.H{
		"logs":      logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (s *Server) handleDeleteAutomationLogs(c *gin.Context) {
	actionType := c.Query("action_type")
	status := c.Query("status")
	before := parseAutomationLogTimeParam(c, "before")

	deleted, err := db.DeleteAutomationLogs(actionType, status, before)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func (s *Server) handleGetAutomationLogRetention(c *gin.Context) {
	settings, err := db.GetAutomationSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"auto_cleanup_enabled": settings.AutoCleanupEnabled,
		"retention_days":       settings.RetentionDays,
	})
}

func (s *Server) handleUpdateAutomationLogRetention(c *gin.Context) {
	var req struct {
		AutoCleanupEnabled *bool `json:"auto_cleanup_enabled"`
		RetentionDays      *int  `json:"retention_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	settings, err := db.GetAutomationSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.AutoCleanupEnabled != nil {
		settings.AutoCleanupEnabled = *req.AutoCleanupEnabled
	}
	if req.RetentionDays != nil && *req.RetentionDays > 0 {
		settings.RetentionDays = *req.RetentionDays
	}
	if err := db.UpsertAutomationSettings(settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"auto_cleanup_enabled": settings.AutoCleanupEnabled,
		"retention_days":       settings.RetentionDays,
	})
}
