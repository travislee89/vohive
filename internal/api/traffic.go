package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/travislee89/vohive/internal/db"

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

// handleTrafficRollup 手动触发流量上卷回填
// POST /api/traffic/rollup?horizon=31
func (s *Server) handleTrafficRollup(c *gin.Context) {
	horizon := 31
	if v := c.Query("horizon"); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h >= 1 && h <= 93 {
			horizon = h
		}
	}

	result, err := db.BackfillTraffic(time.Now(), horizon)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"rollup": gin.H{
			"horizon_days": result.HorizonDays,
			"days":         result.Days,
			"weeks":        result.Weeks,
			"months":       result.Months,
		},
	})
}
