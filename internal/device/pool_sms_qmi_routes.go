package device

import (
	"context"

	"github.com/travislee89/quectel-qmi-go/pkg/qmi"
	"github.com/travislee89/vohive/pkg/logger"
)

// applyQMIStatusReportRouting 读-改-写 WMS 路由表，只切换 TransferStatusReportToClient，
// 保留现有的 Routes 列表不变——绝不能用空/默认路由表覆盖，否则会破坏正常来信路由。
func applyQMIStatusReportRouting(ctx context.Context, core qmiSMSCore, enabled bool) error {
	if core == nil {
		return nil
	}
	cfg, err := core.WMSGetRoutes(ctx)
	if err != nil {
		return err
	}
	if cfg != nil && cfg.TransferStatusReportToClient == enabled {
		return nil
	}
	var routes []qmi.WMSRoute
	if cfg != nil {
		routes = cfg.Routes
	}
	return core.WMSSetRoutes(ctx, routes, enabled)
}

func (p *Pool) applyQMIStatusReportRoutingForWorker(w *Worker, enabled bool) {
	if w == nil {
		return
	}
	core := w.smsQMICore()
	if core == nil {
		return
	}
	if err := applyQMIStatusReportRouting(p.ctx, core, enabled); err != nil {
		logger.Warn("设置 QMI 短信送达报告路由失败", "device", w.ID, "enabled", enabled, "err", err)
	}
}
