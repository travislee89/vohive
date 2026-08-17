package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/travislee89/vohive/internal/db"
	"github.com/gin-gonic/gin"
)

func parseNotifyLogTimeParam(c *gin.Context, name string) *time.Time {
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

func (s *Server) handleListNotifyLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	messageType := c.Query("type")
	status := c.Query("status")
	keyword := c.Query("q")
	start := parseNotifyLogTimeParam(c, "start")
	end := parseNotifyLogTimeParam(c, "end")

	logs, total, err := db.ListNotifyLogs(page, pageSize, messageType, status, keyword, start, end)
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

func (s *Server) handleDeleteNotifyLogs(c *gin.Context) {
	messageType := c.Query("type")
	status := c.Query("status")
	before := parseNotifyLogTimeParam(c, "before")

	deleted, err := db.DeleteNotifyLogs(messageType, status, before)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func (s *Server) handleGetNotifyLogRetention(c *gin.Context) {
	settings, err := db.GetNotifySettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"auto_cleanup_enabled": settings.AutoCleanupEnabled,
		"retention_days":       settings.RetentionDays,
	})
}

func (s *Server) handleUpdateNotifyLogRetention(c *gin.Context) {
	var req struct {
		AutoCleanupEnabled *bool `json:"auto_cleanup_enabled"`
		RetentionDays      *int  `json:"retention_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	settings, err := db.GetNotifySettings()
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
	if err := db.UpsertNotifySettings(settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"auto_cleanup_enabled": settings.AutoCleanupEnabled,
		"retention_days":       settings.RetentionDays,
	})
}
