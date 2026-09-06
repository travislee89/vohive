package device

import (
	"strings"
	"time"

	"github.com/travislee89/vohive/internal/cscall"
	"github.com/travislee89/vohive/internal/db"
)

// csCallHistoryRecorder 将 CS 域呼叫事件落库为通话记录，供通知中心统计未读来电。
//
// cscall.Event.CallID 在 AT 控制器下对所有事件恒为常量 "at"，不能作为呼叫的唯一关联键，
// 因此这里以“当前设备正在进行中的一条记录”作为关联依据，而非按 CallID 匹配。
type csCallHistoryRecorder struct {
	worker *Worker
}

func startCSCallHistoryRecorder(w *Worker, mgr *cscall.Manager) {
	if w == nil || mgr == nil {
		return
	}
	r := csCallHistoryRecorder{worker: w}
	ch, unsubscribe := mgr.Subscribe()
	go r.run(ch, unsubscribe)
}

func (r csCallHistoryRecorder) run(ch <-chan cscall.Event, unsubscribe func()) {
	defer unsubscribe()
	var currentLogID uint

	for event := range ch {
		switch event.Type {
		case cscall.EventIncoming:
			log, err := db.CreateRingingCallLog(r.worker.ID, r.iccid(), r.number(event.Number), db.CallDirectionIn, time.Now())
			if err == nil && log != nil {
				currentLogID = log.ID
			}
		case cscall.EventDialing:
			log, err := db.CreateRingingCallLog(r.worker.ID, r.iccid(), r.number(event.Number), db.CallDirectionOut, time.Now())
			if err == nil && log != nil {
				currentLogID = log.ID
			}
		case cscall.EventConnected:
			if currentLogID != 0 {
				_ = db.MarkCallLogConnected(currentLogID, time.Now())
			}
		case cscall.EventHangup:
			if currentLogID != 0 {
				_ = db.MarkCallLogEnded(currentLogID, time.Now())
				currentLogID = 0
			}
		}
	}
}

func (r csCallHistoryRecorder) iccid() string {
	if r.worker == nil {
		return ""
	}
	return strings.TrimSpace(r.worker.GetCachedDeviceStatus().ICCID)
}

func (r csCallHistoryRecorder) number(n string) string {
	n = strings.TrimSpace(n)
	if n == "" {
		return "Unknown"
	}
	return n
}
