package api

import (
	"context"
	"net/http"
	"time"

	"github.com/boa-z/vohive/internal/cscall"
	"github.com/gin-gonic/gin"
)

// cscallStateText 将 cscall.CallState 映射为可读的状态文本
func cscallStateText(s cscall.CallState) string {
	switch s {
	case cscall.CallStateRinging:
		return "ringing"
	case cscall.CallStateDialing:
		return "dialing"
	case cscall.CallStateConnected:
		return "connected"
	default:
		return "idle"
	}
}

// cscallCallInfoDTO 对外暴露的呼叫信息 DTO
type cscallCallInfoDTO struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Direction string `json:"direction"` // in:来电, out:去电
	State     string `json:"state"`     // ringing/dialing/connected/idle
}

// cscallCallEventDTO 对外暴露的呼叫事件 DTO
type cscallCallEventDTO struct {
	Type   string `json:"type"` // incoming/hangup/connected
	CallID string `json:"call_id"`
	Number string `json:"number,omitempty"`
	Ts     int64  `json:"ts"`
}

// cscallCallsResponse 呼叫列表响应
type cscallCallsResponse struct {
	DeviceID string              `json:"device_id"`
	Calls    []cscallCallInfoDTO `json:"calls"`
}

func cscallCallInfosToDTOs(calls []cscall.CallInfo) []cscallCallInfoDTO {
	if len(calls) == 0 {
		return []cscallCallInfoDTO{}
	}
	out := make([]cscallCallInfoDTO, 0, len(calls))
	for _, c := range calls {
		out = append(out, cscallCallInfoDTO{
			ID:        c.ID,
			Number:    c.Number,
			Direction: c.Direction,
			State:     cscallStateText(c.State),
		})
	}
	return out
}

// resolveCSCallManager 根据 device_id 获取对应的 CSCall 管理器
func (s *Server) resolveCSCallManager(deviceID string) *cscall.Manager {
	w := s.pool.GetWorker(deviceID)
	if w == nil {
		return nil
	}
	return w.CSCallMgr
}

// handleDeviceMgmtCSCallList 处理 GET /devices/:device_id/calls —— 查询当前活跃呼叫列表
func (s *Server) handleDeviceMgmtCSCallList(c *gin.Context) {
	deviceID := deviceIDParam(c)
	mgr := s.resolveCSCallManager(deviceID)
	if mgr == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "设备未找到或呼叫控制未初始化"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	c.JSON(http.StatusOK, cscallCallsResponse{
		DeviceID: deviceID,
		Calls:    cscallCallInfosToDTOs(mgr.Calls(ctx)),
	})
}

// handleDeviceMgmtCSCallEvents 处理 GET /devices/:device_id/calls/events —— SSE 实时推送来电事件
func (s *Server) handleDeviceMgmtCSCallEvents(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	deviceID := deviceIDParam(c)
	mgr := s.resolveCSCallManager(deviceID)
	if mgr == nil {
		c.SSEvent("cscall_event", cscallCallEventDTO{
			Type:   "error",
			CallID: "",
			Number: "设备未找到或呼叫控制未初始化",
			Ts:     time.Now().Unix(),
		})
		c.Writer.Flush()
		return
	}

	// 先推送一次当前快照，便于前端初始化状态
	c.SSEvent("cscall_snapshot", cscallCallsResponse{
		DeviceID: deviceID,
		Calls:    cscallCallInfosToDTOs(mgr.Calls(c.Request.Context())),
	})
	c.Writer.Flush()

	eventCh, unsubscribe := mgr.Subscribe()
	defer unsubscribe()

	notify := c.Writer.CloseNotify()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-notify:
			return
		case <-c.Request.Context().Done():
			return
		case <-s.shutdownCh:
			return
		case <-heartbeat.C:
			// 发送心跳保持连接
			c.SSEvent("cscall_ping", time.Now().Unix())
			c.Writer.Flush()
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			dto := cscallCallEventDTO{
				Type:   string(event.Type),
				CallID: event.CallID,
				Number: event.Number,
				Ts:     time.Now().Unix(),
			}
			c.SSEvent("cscall_event", dto)
			c.Writer.Flush()
		}
	}
}
