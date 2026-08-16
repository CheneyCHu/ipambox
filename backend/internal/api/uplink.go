package api

import (
	"net/http"
	"strconv"
)

// ---- 外网连通状态（断网续存 / 边缘自治） ----

// UplinkStatus 当前连通状态 + 待补发数 + 最近事件。
func (h *handlers) UplinkStatus(w http.ResponseWriter, _ *http.Request) {
	st := h.uplink.Status()
	pending, _ := h.db.PendingCount()
	events, _ := h.db.ListUplinkEvents(10)
	writeJSON(w, 200, map[string]any{
		"status":  st,
		"pending": pending,
		"events":  events,
	})
}

// UplinkEvents 连通状态变化历史（默认最近 50 条）。
func (h *handlers) UplinkEvents(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	events, err := h.db.ListUplinkEvents(limit)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, events)
}

// UplinkCheck 手动触发一次探测（管理员；viewer 已被中间件拦截）。
// 若刚恢复在线，会顺带触发补发队列。
func (h *handlers) UplinkCheck(w http.ResponseWriter, _ *http.Request) {
	st := h.uplink.CheckNow()
	pending, _ := h.db.PendingCount()
	writeJSON(w, 200, map[string]any{"status": st, "pending": pending})
}
