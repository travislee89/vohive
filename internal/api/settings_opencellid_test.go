package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/travislee89/vohive/internal/config"
)

func TestOpencellidRadioFor(t *testing.T) {
	cases := map[string]string{
		"GSM":     "GSM",
		"gsm":     "GSM",
		"WCDMA":   "UMTS",
		"UMTS":    "UMTS",
		"LTE":     "LTE",
		"lte":     "LTE",
		"NR":      "NR",
		"NR5G":    "NR",
		"":        "",
		"UNKNOWN": "",
	}
	for in, want := range cases {
		if got := opencellidRadioFor(in); got != want {
			t.Errorf("opencellidRadioFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQueryOpenCellIDSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("mcc"); got != "525" {
			t.Errorf("unexpected mcc=%q", got)
		}
		if got := r.URL.Query().Get("lac"); got != "505" {
			t.Errorf("unexpected lac=%q, want decimal conversion of 0x01F9", got)
		}
		if got := r.URL.Query().Get("cellid"); got != "13667841" {
			t.Errorf("unexpected cellid=%q, want decimal conversion of 0xD08E01", got)
		}
		w.Write([]byte(`{"lat":1.3456,"lon":103.7134,"range":755,"samples":1,"radio":"LTE"}`))
	}))
	defer srv.Close()

	origURL := openCellIDBaseURL
	openCellIDBaseURL = srv.URL
	defer func() { openCellIDBaseURL = origURL }()

	result, err := queryOpenCellID(t.Context(), "test-key", "525", 5, 0x01F9, 0xD08E01, "LTE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Lat != 1.3456 || result.Lon != 103.7134 {
		t.Fatalf("unexpected lat/lon: %+v", result)
	}
}

func TestQueryOpenCellIDNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"err":{"code":"404","info":"Cell not found"}}`))
	}))
	defer srv.Close()

	origURL := openCellIDBaseURL
	openCellIDBaseURL = srv.URL
	defer func() { openCellIDBaseURL = origURL }()

	_, err := queryOpenCellID(t.Context(), "test-key", "525", 5, 0x01F9, 0xD08E01, "LTE")
	if err == nil || !strings.Contains(err.Error(), "Cell not found") {
		t.Fatalf("expected 'Cell not found' error, got %v", err)
	}
}

func TestHandleOpenCellIDLocateNoKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{fullCfg: &config.Config{}}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/settings/opencellid/locate", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	s.handleOpenCellIDLocate(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when key is missing, got %d: %s", rec.Code, rec.Body.String())
	}
}
