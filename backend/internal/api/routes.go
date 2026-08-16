package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ipambox/ipambox/internal/netconfig"
)

// ---- 路由设置（本机路由表查看 / 静态路由增删） ----

// ListRoutes 返回本机路由表（IPv4 + IPv6）。
func (h *handlers) ListRoutes(w http.ResponseWriter, r *http.Request) {
	list, err := netconfig.ListRoutes()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, list)
}

// AddRoute 添加静态路由（高危：需要 root；前端二次确认）。
func (h *handlers) AddRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Destination string `json:"destination"`
		Gateway     string `json:"gateway"`
		Iface       string `json:"iface"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := netconfig.AddRoute(body.Destination, body.Gateway, body.Iface); err != nil {
		writeRouteErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// DeleteRoute 删除路由（高危：需要 root；前端二次确认）。
func (h *handlers) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Destination string `json:"destination"`
		Gateway     string `json:"gateway"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := netconfig.DeleteRoute(body.Destination, body.Gateway); err != nil {
		writeRouteErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func writeRouteErr(w http.ResponseWriter, err error) {
	var ve *netconfig.ValidationError
	if errors.As(err, &ve) {
		writeErr(w, 400, err)
	} else {
		writeErr(w, 500, err)
	}
}
