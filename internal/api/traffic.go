package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/boa-z/vohive/internal/db"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleTrafficAnalysis(c *gin.Context) {
	rng := c.Query("range")
	if rng == "" {
		rng = "day"
	}
	deviceID := strings.TrimSpace(c.Query("device_id"))
	now := time.Now()

	buckets, chartData, err := db.GetTrafficAnalysisWithChart(rng, deviceID, now)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}

	// 同时聚合 day/week/month 三个范围的下载/上传/合计，供概览数值框一次取全，
	// 无需前端再按 tab 分别请求。
	summary, _ := db.GetTrafficRangeTotals(deviceID, now)

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"range":   rng,
		"buckets": buckets,
		"chart":   chartData,
		"summary": summary,
	})
}
