package api

import (
	"net/http"
	"strconv"

	"github.com/travislee89/vohive/internal/db"
	"github.com/gin-gonic/gin"
)

// handleGetSMSForwardLog 返回单条短信的转发日志明细（按渠道拆分的每次转发尝试），
// 供短信中心的转发状态角标点击展开详情使用。
func (s *Server) handleGetSMSForwardLog(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "非法的短信 ID"})
		return
	}

	logs, err := db.ListNotifyLogsBySMSID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "查询转发日志失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}
