package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/internal/db"
	"github.com/travislee89/vohive/internal/device"
	"github.com/gin-gonic/gin"
)

func TestHandleDeviceMgmtCSCallHistoryScopedToDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	base := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	if _, err := db.CreateRingingCallLog("wwan-1", "iccid-1", "+861111", db.CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}
	if _, err := db.CreateRingingCallLog("wwan-2", "iccid-2", "+862222", db.CallDirectionIn, base); err != nil {
		t.Fatalf("CreateRingingCallLog() error=%v", err)
	}

	p := device.NewPool(&config.Config{})
	injectWorker(p, &device.Worker{ID: "wwan-1"})

	s := &Server{pool: p}
	r := gin.Default()
	r.GET("/api/devices/:device_id/calls/history", s.handleDeviceMgmtCSCallHistory)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/devices/wwan-1/calls/history", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp cscallCallHistoryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Logs) != 1 || resp.Logs[0].Number != "+861111" {
		t.Fatalf("unexpected logs=%+v", resp.Logs)
	}
}

func TestHandleDeviceMgmtCSCallHistoryUnknownDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	s := &Server{pool: device.NewPool(&config.Config{})}
	r := gin.Default()
	r.GET("/api/devices/:device_id/calls/history", s.handleDeviceMgmtCSCallHistory)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/devices/missing/calls/history", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("code=%d want=404 body=%s", w.Code, w.Body.String())
	}
}
