package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/travislee89/vohive/internal/config"
	"github.com/travislee89/vohive/pkg/logger"

	"github.com/gin-gonic/gin"
)

var openCellIDHTTPClient = &http.Client{Timeout: 10 * time.Second}

// openCellIDBaseURL is a package-level var so tests can point it at a local httptest server.
var openCellIDBaseURL = "https://opencellid.org/cell/get"

type openCellIDSettingsResponse struct {
	Key string `json:"key"`
}

type updateOpenCellIDSettingsRequest struct {
	Key string `json:"key"`
}

func (s *Server) handleGetOpenCellIDSettings(c *gin.Context) {
	c.JSON(http.StatusOK, openCellIDSettingsResponse{Key: s.fullCfg.OpenCellID.Key})
}

func (s *Server) handleUpdateOpenCellIDSettings(c *gin.Context) {
	var req updateOpenCellIDSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "参数错误"})
		return
	}

	key := strings.TrimSpace(req.Key)
	if err := config.UpdateOpenCellIDInFile(s.configPath, key); err != nil {
		logger.Error("写入 OpenCellID 配置失败", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "写入配置文件失败: " + err.Error()})
		return
	}

	s.fullCfg.OpenCellID.Key = key
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// opencellidLocateRequest 是根据小区定位参数请求 OpenCellID 基站定位的入参。
// LAC/CID 沿用设备详情中展示的十六进制字符串格式（如 "01F9"/"D08E01"）。
type opencellidLocateRequest struct {
	MCC         string `json:"mcc"`
	MNC         string `json:"mnc"`
	LAC         string `json:"lac"`
	CID         string `json:"cid"`
	NetworkMode string `json:"network_mode"` // GSM|WCDMA|LTE|NR
}

type opencellidLocateResponse struct {
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Range   int     `json:"range"`
	Samples int     `json:"samples"`
	Radio   string  `json:"radio"`
}

// opencellidRawResponse 对应 OpenCellID /cell/get 接口的返回结构，
// 成功与出错（如未找到基站）时字段形状不同，因此 Err 字段单独解析。
type opencellidRawResponse struct {
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Range   int     `json:"range"`
	Samples int     `json:"samples"`
	Radio   string  `json:"radio"`
	Err     *struct {
		Code string `json:"code"`
		Info string `json:"info"`
	} `json:"err"`
}

// opencellidRadioFor 将设备上报的网络制式转换为 OpenCellID 接受的 radio 参数。
func opencellidRadioFor(networkMode string) string {
	switch strings.ToUpper(strings.TrimSpace(networkMode)) {
	case "GSM":
		return "GSM"
	case "WCDMA", "UMTS", "TD-SCDMA":
		return "UMTS"
	case "LTE":
		return "LTE"
	case "NR", "NR5G", "5G":
		return "NR"
	default:
		return ""
	}
}

func (s *Server) handleOpenCellIDLocate(c *gin.Context) {
	key := strings.TrimSpace(s.fullCfg.OpenCellID.Key)
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "尚未配置 OpenCellID Key，请先在系统设置中填写"})
		return
	}

	var req opencellidLocateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "参数错误"})
		return
	}

	mcc := strings.TrimSpace(req.MCC)
	mnc := strings.TrimSpace(req.MNC)
	lacHex := strings.TrimSpace(req.LAC)
	cidHex := strings.TrimSpace(req.CID)
	if mcc == "" || mnc == "" || lacHex == "" || cidHex == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "缺少小区定位参数（MCC/MNC/LAC/CID）"})
		return
	}

	mncNum, err := strconv.Atoi(mnc)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "MNC 格式错误: " + mnc})
		return
	}

	lac, err := strconv.ParseInt(lacHex, 16, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "LAC/TAC 格式错误: " + lacHex})
		return
	}

	cid, err := strconv.ParseInt(cidHex, 16, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "CID 格式错误: " + cidHex})
		return
	}

	radio := opencellidRadioFor(req.NetworkMode)
	if radio == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无法识别当前网络制式: " + req.NetworkMode})
		return
	}

	result, err := queryOpenCellID(c.Request.Context(), key, mcc, mncNum, lac, cid, radio)
	if err != nil {
		logger.Error("请求 OpenCellID 失败", "err", err)
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func queryOpenCellID(ctx context.Context, key, mcc string, mnc int, lac, cid int64, radio string) (*opencellidLocateResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	url := fmt.Sprintf(
		"%s?key=%s&mcc=%s&mnc=%d&lac=%d&cellid=%d&radio=%s&format=json",
		openCellIDBaseURL, key, mcc, mnc, lac, cid, radio,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造 OpenCellID 请求失败: %w", err)
	}

	resp, err := openCellIDHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 OpenCellID 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 OpenCellID 响应失败: %w", err)
	}

	var raw opencellidRawResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("解析 OpenCellID 响应失败: %w", err)
	}

	if raw.Err != nil {
		info := raw.Err.Info
		if info == "" {
			info = "未知错误"
		}
		return nil, fmt.Errorf("OpenCellID 返回错误: %s", info)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenCellID 请求失败: HTTP %d", resp.StatusCode)
	}

	if raw.Lat == 0 && raw.Lon == 0 {
		return nil, fmt.Errorf("OpenCellID 未找到该基站的定位数据")
	}

	return &opencellidLocateResponse{
		Lat:     raw.Lat,
		Lon:     raw.Lon,
		Range:   raw.Range,
		Samples: raw.Samples,
		Radio:   raw.Radio,
	}, nil
}
