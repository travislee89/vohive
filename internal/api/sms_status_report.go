package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/travislee89/vohive/internal/db"
	"gorm.io/gorm"
)

// handleSMSStatusReport 查询一条通过 AT/QMI/MBIM 发送的短信的送达报告（TP-SRR）状态。
// 与 handleSMSDelivery（VoWiFi 专用，按 message_id 字符串查询）不是同一套机制。
func (s *Server) handleSMSStatusReport(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("sms_id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "非法的短信 ID"})
		return
	}

	report, err := db.GetSMSStatusReportBySMSID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "未找到该短信的送达报告记录（可能未请求送达报告，或尚未收到网络回执）"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "查询送达报告失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "report": report})
}
