package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/internal/db"
	"github.com/travislee89/vohive/internal/device"
	"github.com/gin-gonic/gin"
)

// injectWorker 通过 unsafe 反射将 worker 注入到 pool 的内部 workers map，
// 用于无需完整启动流程的测试场景。
func injectWorker(p *device.Pool, w *device.Worker) {
	pv := reflect.ValueOf(p).Elem().FieldByName("workers")
	m := reflect.NewAt(pv.Type(), unsafe.Pointer(pv.UnsafeAddr())).Elem()
	m.SetMapIndex(reflect.ValueOf(w.ID), reflect.ValueOf(w))
}

func openTestDB(t *testing.T) {
	t.Helper()
	if err := db.Init(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("Init() error=%v", err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if sqlDB, err := db.DB.DB(); err == nil && sqlDB != nil {
				_ = sqlDB.Close()
			}
		}
	})
}

func TestGetCardPolicyEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)
	_ = db.UpsertCardPolicy(db.CardPolicy{ICCID: "8986004", NetworkEnabled: true, IPVersion: "v4", Source: "user"})

	s := &Server{}
	r := gin.Default()
	r.GET("/api/cards/:iccid/policy", s.handleGetCardPolicy)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/8986004/policy", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var got db.CardPolicy
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.NetworkEnabled {
		t.Fatalf("payload 错: %+v", got)
	}
}

func TestPutCardPolicyEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)
	s := &Server{
		pool: device.NewPool(&config.Config{}),
	}
	r := gin.Default()
	r.PUT("/api/cards/:iccid/policy", s.handlePutCardPolicy)

	body := `{"network_enabled":true,"vowifi_enabled":true,"ip_version":"v4v6","apn":"ims"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/cards/8986005/policy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	got, _ := db.GetCardPolicy("8986005")
	if !got.NetworkEnabled || !got.VoWiFiEnabled || got.IPVersion != "v4v6" || got.APN != "ims" {
		t.Fatalf("未成功更新: %+v", got)
	}
}

// TestPatchCardPolicyForDevice 验证 patchCardPolicyForDevice helper 正确解析 ICCID 并落库。
func TestPatchCardPolicyForDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	p := device.NewPool(&config.Config{})
	w := &device.Worker{ID: "wwan-patch"}
	setNestedPrivateField(t, w, []string{"state", "Identity", "ICCID"}, "8986patch001")
	injectWorker(p, w)

	s := &Server{pool: p}
	iccid, applied, err := s.patchCardPolicyForDevice("wwan-patch", func(pol *db.CardPolicy) {
		pol.NetworkEnabled = true
		pol.IPVersion = "v4v6"
		pol.APN = "ims"
	})

	if err != nil {
		t.Fatalf("error=%v", err)
	}
	if !applied {
		t.Fatalf("expected applied=true")
	}
	if iccid != "8986patch001" {
		t.Fatalf("iccid=%q", iccid)
	}
	got, err := db.GetCardPolicy("8986patch001")
	if err != nil {
		t.Fatal(err)
	}
	if !got.NetworkEnabled || got.IPVersion != "v4v6" || got.APN != "ims" {
		t.Fatalf("card policy mismatch: %+v", got)
	}
	if got.Source != "user" {
		t.Fatalf("source=%q want user", got.Source)
	}
}

// TestPatchCardPolicyForDeviceNoICCID 验证设备无 ICCID 时 applied=false 且不报错。
func TestPatchCardPolicyForDeviceNoICCID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	p := device.NewPool(&config.Config{})
	w := &device.Worker{ID: "wwan-nocard"}
	// 不设置 ICCID，模拟无卡状态
	injectWorker(p, w)

	s := &Server{pool: p}
	iccid, applied, err := s.patchCardPolicyForDevice("wwan-nocard", func(pol *db.CardPolicy) {
		pol.NetworkEnabled = true
	})

	if err != nil {
		t.Fatalf("error=%v", err)
	}
	if applied {
		t.Fatalf("expected applied=false when no ICCID")
	}
	if iccid != "" {
		t.Fatalf("iccid=%q want empty", iccid)
	}
}

// TestPatchCardPolicyVoWiFiKeepsAirplaneIntent 验证开 VoWiFi 不再强制 airplane=true：
// airplane 反映用户的纯飞行意图，独立于 vowifi。
func TestPatchCardPolicyVoWiFiKeepsAirplaneIntent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	p := device.NewPool(&config.Config{})
	w := &device.Worker{ID: "wwan-vowifi"}
	setNestedPrivateField(t, w, []string{"state", "Identity", "ICCID"}, "8986vowifi01")
	injectWorker(p, w)

	s := &Server{pool: p}
	// 从在线开 VoWiFi（飞行意图为 false）：airplane 应保持 false，不被强制为 true。
	_, _, err := s.patchCardPolicyForDevice("wwan-vowifi", vowifiEnablePolicyMutation)
	if err != nil {
		t.Fatalf("error=%v", err)
	}
	got, _ := db.GetCardPolicy("8986vowifi01")
	if !got.VoWiFiEnabled || got.AirplaneEnabled {
		t.Fatalf("开 VoWiFi 不应强制 airplane=true: vowifi=%v airplane=%v", got.VoWiFiEnabled, got.AirplaneEnabled)
	}
}

// TestVoWiFiToggleCyclePreservesAirplaneIntent 复现并锁定 bug 修复：
// 先开飞行 → 开 VoWiFi → 关 VoWiFi，应回退到飞行（airplane 意图被保留）。
func TestVoWiFiToggleCyclePreservesAirplaneIntent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)

	p := device.NewPool(&config.Config{})
	w := &device.Worker{ID: "wwan-cycle"}
	setNestedPrivateField(t, w, []string{"state", "Identity", "ICCID"}, "8986cycle001")
	injectWorker(p, w)
	s := &Server{pool: p}

	// 1) 用户先开飞行
	if _, _, err := s.patchCardPolicyForDevice("wwan-cycle", func(pol *db.CardPolicy) {
		pol.AirplaneEnabled = true
		pol.VoWiFiEnabled = false
		pol.NetworkEnabled = false
	}); err != nil {
		t.Fatalf("开飞行 error=%v", err)
	}

	// 2) 开 VoWiFi（落库副作用：只置 vowifi）
	if _, _, err := s.patchCardPolicyForDevice("wwan-cycle", vowifiEnablePolicyMutation); err != nil {
		t.Fatalf("开 vowifi error=%v", err)
	}
	mid, _ := db.GetCardPolicy("8986cycle001")
	if !mid.VoWiFiEnabled || !mid.AirplaneEnabled {
		t.Fatalf("开 VoWiFi 期间飞行意图应保留: %+v", mid)
	}

	// 3) 关 VoWiFi（落库副作用：只清 vowifi），应回退到飞行
	if _, _, err := s.patchCardPolicyForDevice("wwan-cycle", vowifiDisablePolicyMutation); err != nil {
		t.Fatalf("关 vowifi error=%v", err)
	}
	got, _ := db.GetCardPolicy("8986cycle001")
	if got.VoWiFiEnabled || !got.AirplaneEnabled {
		t.Fatalf("关 VoWiFi 后应回退到飞行模式: vowifi=%v airplane=%v", got.VoWiFiEnabled, got.AirplaneEnabled)
	}
}

// TestPutCardPolicyQuotaFields 验证 PUT 卡策略带流量限制字段能原样落库。
func TestPutCardPolicyQuotaFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)
	s := &Server{pool: device.NewPool(&config.Config{})}
	r := gin.Default()
	r.PUT("/api/cards/:iccid/policy", s.handlePutCardPolicy)

	body := `{"quota_enabled":true,"quota_bytes":1073741824,"billing_day":15,"billing_timezone":"Asia/Shanghai","auto_stop_enabled":true,"auto_stop_threshold_bytes":1181116006}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/cards/8986q10/policy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	got, _ := db.GetCardPolicy("8986q10")
	if !got.QuotaEnabled || got.QuotaBytes != 1073741824 || got.BillingDay != 15 ||
		got.BillingTimezone != "Asia/Shanghai" || !got.AutoStopEnabled || got.AutoStopThresholdBytes != 1181116006 {
		t.Fatalf("流量限制字段未原样落库: %+v", got)
	}
}

// TestGetCardPolicyReturnsQuotaUsage 验证 GET 卡策略附带 quota_usage，且超限时 exceeded=true。
func TestGetCardPolicyReturnsQuotaUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)
	// 配置：quota=1GB，threshold=1GB，计费日15，autoStop 开
	_ = db.UpsertCardPolicy(db.CardPolicy{
		ICCID: "8986q11", QuotaEnabled: true, QuotaBytes: 1073741824,
		BillingDay: 15, BillingTimezone: "UTC", AutoStopEnabled: true,
		AutoStopThresholdBytes: 1073741824, Source: "user",
	})
	// 累计用量到 1.5GB，超过阈值
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	if _, err := db.AccumulateCardQuotaUsage("8986q11", 1610612736, 15, "UTC", now); err != nil {
		t.Fatal(err)
	}

	s := &Server{}
	r := gin.Default()
	r.GET("/api/cards/:iccid/policy", s.handleGetCardPolicy)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/cards/8986q11/policy", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	var usage struct {
		UsedBytes  int64  `json:"used_bytes"`
		Exceeded   bool   `json:"exceeded"`
		Threshold  int64  `json:"threshold_bytes"`
		PeriodEnd  string `json:"period_end"`
	}
	if err := json.Unmarshal(resp["quota_usage"], &usage); err != nil {
		t.Fatalf("解析 quota_usage 失败: %v", err)
	}
	if usage.UsedBytes != 1610612736 {
		t.Fatalf("used_bytes=%d", usage.UsedBytes)
	}
	if !usage.Exceeded {
		t.Fatal("已用1.5GB超过1GB阈值，应 exceeded=true")
	}
	if usage.Threshold != 1073741824 {
		t.Fatalf("threshold=%d", usage.Threshold)
	}
}

// TestPutCardPolicyRoamingDataEnabled 验证 PUT 卡策略带 roaming_data_enabled 字段能原样落库并回读。
func TestPutCardPolicyRoamingDataEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openTestDB(t)
	s := &Server{pool: device.NewPool(&config.Config{})}
	r := gin.Default()
	r.PUT("/api/cards/:iccid/policy", s.handlePutCardPolicy)
	r.GET("/api/cards/:iccid/policy", s.handleGetCardPolicy)

	// 1. 写入 roaming_data_enabled=true
	body := `{"roaming_data_enabled":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/cards/8986r01/policy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT code=%d body=%s", w.Code, w.Body.String())
	}
	got, _ := db.GetCardPolicy("8986r01")
	if !got.RoamingDataEnabled {
		t.Fatalf("roaming_data_enabled 应为 true: %+v", got)
	}

	// 2. GET 响应也应包含 roaming_data_enabled
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/cards/8986r01/policy", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET code=%d body=%s", w2.Code, w2.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	var roamingEnabled bool
	if raw, ok := resp["roaming_data_enabled"]; ok {
		if err := json.Unmarshal(raw, &roamingEnabled); err != nil {
			t.Fatalf("解析 roaming_data_enabled 失败: %v", err)
		}
	}
	if !roamingEnabled {
		t.Fatal("GET 响应 roaming_data_enabled 应为 true")
	}

	// 3. 默认值（未建档的新卡）应为 false
	def := db.DefaultCardPolicy("newcard")
	if def.RoamingDataEnabled {
		t.Fatal("新卡默认 roaming_data_enabled 应为 false")
	}
}
