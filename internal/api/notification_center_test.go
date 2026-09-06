package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/internal/db"
	"github.com/travislee89/vohive/internal/device"
)

func TestHandleNotificationCenterSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	if _, err := db.SaveSMS("imsi-1", "+10086", "+86138", "hi", 1, 0, base); err != nil {
		t.Fatalf("SaveSMS() error=%v", err)
	}
	if _, err := db.CreateRingingCallLog("dev-1", "iccid-1", "+861111", db.CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}

	s := &Server{pool: device.NewPool(&config.Config{})}
	r := gin.Default()
	r.GET("/api/notification-center/summary", s.handleNotificationCenterSummary)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/notification-center/summary", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var got notificationSummaryDTO
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SMSUnread != 1 {
		t.Fatalf("SMSUnread=%d want=1", got.SMSUnread)
	}
	if got.CallsUnread != 1 {
		t.Fatalf("CallsUnread=%d want=1", got.CallsUnread)
	}
}

func TestHandleNotificationCenterFeedMergesAndSorts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	if _, err := db.SaveSMS("imsi-1", "+10086", "+86138", "older sms", 1, 0, base); err != nil {
		t.Fatalf("SaveSMS() error=%v", err)
	}
	if _, err := db.CreateRingingCallLog("dev-1", "iccid-1", "+861111", db.CallDirectionIn, base.Add(time.Minute)); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}

	s := &Server{pool: device.NewPool(&config.Config{})}
	r := gin.Default()
	r.GET("/api/notification-center/feed", s.handleNotificationCenterFeed)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/notification-center/feed", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []notificationFeedItemDTO `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("len(items)=%d want=2 body=%s", len(resp.Items), w.Body.String())
	}
	// 通话记录时间更晚，应排在前面
	if resp.Items[0].Kind != "call" || resp.Items[1].Kind != "sms" {
		t.Fatalf("unexpected order: %+v", resp.Items)
	}
}

func TestHandleNotificationCenterMarkSMSRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	if _, err := db.SaveSMS("imsi-1", "+10086", "+86138", "hi", 1, 0, base); err != nil {
		t.Fatalf("SaveSMS() error=%v", err)
	}
	if _, err := db.SaveSMS("imsi-2", "+20086", "+86139", "hi2", 1, 0, base); err != nil {
		t.Fatalf("SaveSMS() error=%v", err)
	}

	s := &Server{}
	r := gin.Default()
	r.POST("/api/notification-center/sms/read", s.handleNotificationCenterMarkSMSRead)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/notification-center/sms/read", strings.NewReader(`{"imsi":"imsi-1","peer":"+10086"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}

	total, err := db.SumSMSUnreadCount()
	if err != nil {
		t.Fatalf("SumSMSUnreadCount() error=%v", err)
	}
	if total != 1 {
		t.Fatalf("SumSMSUnreadCount()=%d want=1 (only imsi-2/+20086 thread should remain unread)", total)
	}
}

func TestHandleNotificationCenterMarkCallsRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	a, err := db.CreateRingingCallLog("dev-1", "iccid-1", "+861111", db.CallDirectionIn, base)
	if err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if _, err := db.CreateRingingCallLog("dev-2", "iccid-2", "+862222", db.CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}

	s := &Server{}
	r := gin.Default()
	r.POST("/api/notification-center/calls/read", s.handleNotificationCenterMarkCallsRead)

	w := httptest.NewRecorder()
	body := fmt.Sprintf(`{"id":%d}`, a.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/notification-center/calls/read", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}

	count, err := db.CountUnreadCalls()
	if err != nil {
		t.Fatalf("CountUnreadCalls() error=%v", err)
	}
	if count != 1 {
		t.Fatalf("CountUnreadCalls()=%d want=1 (dev-2's call should remain unread)", count)
	}
}

func TestHandleNotificationCenterClearAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	if _, err := db.SaveSMS("imsi-1", "+10086", "+86138", "hi", 1, 0, base); err != nil {
		t.Fatalf("SaveSMS() error=%v", err)
	}
	if _, err := db.SaveSMS("imsi-2", "+20086", "+86139", "hi2", 1, 0, base); err != nil {
		t.Fatalf("SaveSMS() error=%v", err)
	}
	if _, err := db.CreateRingingCallLog("dev-1", "iccid-1", "+861111", db.CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if _, err := db.CreateRingingCallLog("dev-2", "iccid-2", "+862222", db.CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}

	s := &Server{}
	r := gin.Default()
	r.POST("/api/notification-center/clear-all", s.handleNotificationCenterClearAll)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/notification-center/clear-all", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}

	smsTotal, err := db.SumSMSUnreadCount()
	if err != nil {
		t.Fatalf("SumSMSUnreadCount() error=%v", err)
	}
	if smsTotal != 0 {
		t.Fatalf("SumSMSUnreadCount()=%d want=0", smsTotal)
	}
	callCount, err := db.CountUnreadCalls()
	if err != nil {
		t.Fatalf("CountUnreadCalls() error=%v", err)
	}
	if callCount != 0 {
		t.Fatalf("CountUnreadCalls()=%d want=0", callCount)
	}
}
