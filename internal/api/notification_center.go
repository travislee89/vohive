package api

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/internal/db"
)

// notificationSummaryDTO 通知中心徽标汇总
type notificationSummaryDTO struct {
	SMSUnread   int64 `json:"sms_unread"`
	CallsUnread int64 `json:"calls_unread"`
}

// notificationFeedItemDTO 通知中心信息流条目（短信或来电二选一携带对应字段）
type notificationFeedItemDTO struct {
	Kind        string `json:"kind"` // sms | call
	DeviceID    string `json:"device_id,omitempty"`
	DeviceName  string `json:"device_name,omitempty"`
	Timestamp   string `json:"timestamp"`
	timestampNs int64  // 仅用于排序，不序列化

	// kind=sms
	IMSI        string `json:"imsi,omitempty"`
	Peer        string `json:"peer,omitempty"`
	Preview     string `json:"preview,omitempty"`
	UnreadCount int    `json:"unread_count,omitempty"`

	// kind=call
	CallID  uint   `json:"call_id,omitempty"`
	Number  string `json:"number,omitempty"`
	Outcome string `json:"outcome,omitempty"`
}

// deviceNameIndex 建立 ICCID -> 设备信息 与 设备ID -> 设备名 的索引，
// 用于把 SMSContact(仅有 ICCID) 与 CallLog(已有 DeviceID) 都补全为对外可读的设备名。
type deviceNameIndex struct {
	byICCID map[string]struct{ id, name string }
	byID    map[string]string
}

func (s *Server) buildDeviceNameIndex() deviceNameIndex {
	idx := deviceNameIndex{
		byICCID: make(map[string]struct{ id, name string }),
		byID:    make(map[string]string),
	}

	cfgByID := map[string]config.DeviceConfig{}
	for _, d := range config.ListDevices() {
		cfgByID[d.ID] = d
	}

	for _, w := range s.pool.GetAllWorkers() {
		name := ""
		if v, ok := cfgByID[w.ID]; ok {
			name = v.Name
		} else {
			name = w.Config.Name
		}
		if name == "" {
			name = w.ID
		}
		idx.byID[w.ID] = name
		if iccid := w.CurrentICCID(); iccid != "" {
			idx.byICCID[iccid] = struct{ id, name string }{id: w.ID, name: name}
		}
	}
	return idx
}

// handleNotificationCenterSummary 处理 GET /api/notification-center/summary —— 短信/来电未读徽标计数
func (s *Server) handleNotificationCenterSummary(c *gin.Context) {
	smsUnread, err := db.SumSMSUnreadCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询未读短信数失败"})
		return
	}
	callsUnread, err := db.CountUnreadCalls()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询未读来电数失败"})
		return
	}
	c.JSON(http.StatusOK, notificationSummaryDTO{SMSUnread: smsUnread, CallsUnread: callsUnread})
}

// handleNotificationCenterFeed 处理 GET /api/notification-center/feed —— 合并未读短信会话与未读来电，按时间倒序
func (s *Server) handleNotificationCenterFeed(c *gin.Context) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	contacts, err := db.GetUnreadSMSContacts(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询未读短信失败"})
		return
	}
	calls, err := db.ListRecentUnreadCalls(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询未读来电失败"})
		return
	}

	idx := s.buildDeviceNameIndex()

	items := make([]notificationFeedItemDTO, 0, len(contacts)+len(calls))
	for _, ct := range contacts {
		info := idx.byICCID[ct.ICCID]
		items = append(items, notificationFeedItemDTO{
			Kind:        "sms",
			DeviceID:    info.id,
			DeviceName:  info.name,
			Timestamp:   ct.LastTimestamp.UTC().Format("2006-01-02T15:04:05Z07:00"),
			timestampNs: ct.LastTimestamp.UnixNano(),
			IMSI:        ct.IMSI,
			Peer:        ct.Peer,
			Preview:     ct.LastContent,
			UnreadCount: ct.UnreadCount,
		})
	}
	for _, cl := range calls {
		items = append(items, notificationFeedItemDTO{
			Kind:        "call",
			DeviceID:    cl.DeviceID,
			DeviceName:  idx.byID[cl.DeviceID],
			Timestamp:   cl.StartedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			timestampNs: cl.StartedAt.UnixNano(),
			CallID:      cl.ID,
			Number:      cl.Number,
			Outcome:     cl.Outcome,
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].timestampNs > items[j].timestampNs })
	if len(items) > limit {
		items = items[:limit]
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// notificationMarkSMSReadRequest 标记短信会话已读的请求体
type notificationMarkSMSReadRequest struct {
	IMSI string `json:"imsi"`
	Peer string `json:"peer" binding:"required"`
}

// handleNotificationCenterMarkSMSRead 处理 POST /api/notification-center/sms/read —— 清零某会话未读计数
func (s *Server) handleNotificationCenterMarkSMSRead(c *gin.Context) {
	var req notificationMarkSMSReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	if err := db.ResetSMSContactUnread(req.IMSI, req.Peer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "标记已读失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// notificationMarkCallsReadRequest 标记通话记录已读的请求体：按单条 ID，或按设备批量清零
type notificationMarkCallsReadRequest struct {
	ID       uint   `json:"id"`
	DeviceID string `json:"device_id"`
}

// handleNotificationCenterMarkCallsRead 处理 POST /api/notification-center/calls/read —— 标记通话记录已读
func (s *Server) handleNotificationCenterMarkCallsRead(c *gin.Context) {
	var req notificationMarkCallsReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	if req.ID == 0 && req.DeviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "需要提供 id 或 device_id 之一"})
		return
	}

	var err error
	if req.ID != 0 {
		_, err = db.MarkCallLogsRead([]uint{req.ID})
	} else {
		_, err = db.MarkAllCallLogsReadForDevice(req.DeviceID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "标记已读失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleNotificationCenterClearAll 处理 POST /api/notification-center/clear-all —— 清零全部短信与来电未读
func (s *Server) handleNotificationCenterClearAll(c *gin.Context) {
	if err := db.ResetAllSMSUnread(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清理未读短信失败"})
		return
	}
	if _, err := db.MarkAllCallLogsRead(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清理未读来电失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
