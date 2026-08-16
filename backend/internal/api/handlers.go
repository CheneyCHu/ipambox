package api

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ipambox/ipambox/internal/ai"
	"github.com/ipambox/ipambox/internal/alert"
	"github.com/ipambox/ipambox/internal/models"
	"github.com/ipambox/ipambox/internal/scanner"
	"github.com/ipambox/ipambox/internal/store"
	"github.com/ipambox/ipambox/internal/uplink"
)

//go:embed all:web
var webFS embed.FS

type handlers struct {
	db     *store.Store
	engine *scanner.Engine
	uplink *uplink.Monitor
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// ---- handlers ----

func (h *handlers) Overview(w http.ResponseWriter, r *http.Request) {
	subs, err := h.db.ListSubnets()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	stats, err := h.db.GlobalStats()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	usage := 0.0
	if stats["total"] > 0 {
		usage = float64(stats["total"]-stats["free"]) / float64(stats["total"]) * 100
	}
	alerts, _ := h.db.ListAlerts(true)
	writeJSON(w, 200, map[string]any{
		"subnets": len(subs), "stats": stats, "usage_pct": usage, "unread_alerts": len(alerts),
	})
}

// SetupReset 清除全部业务数据并解除初始化标记，之后前端跳转回初始化向导。
// 属于破坏性操作，前端须先向用户展示数据统计并二次确认。
func (h *handlers) SetupReset(w http.ResponseWriter, r *http.Request) {
	if err := h.db.ResetAll(); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "reset"})
}

// ListInterfaces 返回本机物理网卡及其推算网段，供向导/添加子网时选择。
func (h *handlers) ListInterfaces(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, listNICs())
}

func (h *handlers) ListSubnets(w http.ResponseWriter, r *http.Request) {
	subs, err := h.db.ListSubnets()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, subs)
}

func (h *handlers) CreateSubnet(w http.ResponseWriter, r *http.Request) {
	var sub models.Subnet
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		writeErr(w, 400, err)
		return
	}
	if sub.CIDR == "" {
		writeErr(w, 400, errString("cidr 不能为空"))
		return
	}
	if err := h.db.CreateSubnet(&sub); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 201, sub)
}

func (h *handlers) DeleteSubnet(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	if err := h.db.DeleteSubnet(id); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// UpdateSubnet 修改子网名称/描述/VLAN/接入网卡（CIDR 不可改）。
func (h *handlers) UpdateSubnet(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	var sub models.Subnet
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		writeErr(w, 400, err)
		return
	}
	sub.ID = id
	if err := h.db.UpdateSubnet(&sub); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// CreateAddress 人工登记地址（保留/规划）。IP 必须属于该子网。
func (h *handlers) CreateAddress(w http.ResponseWriter, r *http.Request) {
	subnetID, _ := idParam(r)
	sub, err := h.db.GetSubnet(subnetID)
	if err != nil {
		writeErr(w, 404, errString("子网不存在"))
		return
	}
	var a models.IPAddress
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeErr(w, 400, err)
		return
	}
	ip := net.ParseIP(a.IP)
	_, ipnet, _ := net.ParseCIDR(sub.CIDR)
	if ip == nil || ipnet == nil || !ipnet.Contains(ip) {
		writeErr(w, 400, errString("IP 不合法或不属于该子网"))
		return
	}
	a.SubnetID = subnetID
	if a.Status == "" || a.Status == models.StatusFree || a.Status == models.StatusOnline {
		a.Status = models.StatusReserved // 人工登记默认保留；online 由扫描引擎维护
	}
	if err := h.db.CreateAddress(&a); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, 409, errString("该 IP 已存在台账记录"))
			return
		}
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 201, a)
}

// DeleteAddress 删除一条台账记录（回收/误登记清理）。
func (h *handlers) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	if err := h.db.DeleteAddress(id); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *handlers) ListAddresses(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	addrs, err := h.db.ListAddresses(id)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, addrs)
}

func (h *handlers) SubnetStats(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	st, err := h.db.Stats(id)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, st)
}

func (h *handlers) ScanNow(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	if err := h.engine.ScanNow(id); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "scanning"})
}

func (h *handlers) UpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	var body struct {
		Label   string `json:"label"`
		Owner   string `json:"owner"`
		DevType string `json:"dev_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := h.db.UpdateAnnotation(id, body.Label, body.Owner, body.DevType); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *handlers) ListAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.db.ListAlerts(r.URL.Query().Get("unread") == "1")
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, alerts)
}

func (h *handlers) MarkAlertRead(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	if err := h.db.MarkAlertRead(id); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---- 系统设置 ----

// settingsWhitelist 可通过 /settings 读写的键（AI 配置走 /ai/config 单独管理）。
var settingsWhitelist = map[string]string{
	"scan_interval_min": "5", // 定时扫描间隔（分钟）
	"auto_scan":         "1", // 是否启用定时扫描
	// 台账字典（JSON 数组字符串），供设备编辑时下拉/自动补全
	"dev_types": `["服务器","网络设备","电脑","打印机","摄像头","手机/平板","IoT 设备","虚拟机","其他"]`,
	"owners":    `[]`,
	// 告警通知渠道
	"notify_enabled": "0",                // 是否启用通知推送
	"notify_channel": "webhook",          // webhook / dingtalk / wecom
	"notify_webhook": "",                 // Webhook 地址
	"notify_secret":  "",                 // 钉钉加签密钥（可选）
	"notify_events":  "conflict,offline", // 关注的事件类型（逗号分隔）
	// 外网连通探测（断网续存）
	"uplink_probe":     "223.5.5.5:53,114.114.114.114:53", // 探测目标（host:port，逗号分隔多个）
	"uplink_check_sec": "30",                              // 探测间隔（秒，5~3600）
	// OTA 升级
	"update_manifest_url": "", // 更新清单 URL（空=官方源）
}

func (h *handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	for k, def := range settingsWhitelist {
		v, err := h.db.GetSetting(k)
		if err != nil || v == "" {
			v = def
		}
		// 字典键：存了空数组时回落默认值（避免误保存后字典清空）
		if k == "dev_types" || k == "owners" {
			var arr []string
			if json.Unmarshal([]byte(v), &arr) != nil || (len(arr) == 0 && def != `[]`) {
				v = def
			}
		}
		out[k] = v
	}
	writeJSON(w, 200, out)
}

// RenameDictItem 重命名字典项（设备类型/负责人），并级联更新台账中的引用。
func (h *handlers) RenameDictItem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string `json:"kind"` // dev_types / owners
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if body.Kind != "dev_types" && body.Kind != "owners" {
		writeErr(w, 400, errString("kind 须为 dev_types 或 owners"))
		return
	}
	body.From, body.To = strings.TrimSpace(body.From), strings.TrimSpace(body.To)
	if body.From == "" || body.To == "" {
		writeErr(w, 400, errString("新旧名称均不能为空"))
		return
	}
	if body.From == body.To {
		writeJSON(w, 200, map[string]any{"updated": 0})
		return
	}

	// 读当前字典（空数组/损坏时回落默认值，与 GetSettings 行为一致）
	raw, err := h.db.GetSetting(body.Kind)
	if err != nil || raw == "" {
		raw = settingsWhitelist[body.Kind]
	}
	var arr []string
	if json.Unmarshal([]byte(raw), &arr) != nil || (len(arr) == 0 && settingsWhitelist[body.Kind] != `[]`) {
		arr = nil
		if def := settingsWhitelist[body.Kind]; def != "" {
			_ = json.Unmarshal([]byte(def), &arr)
		}
	}
	found := false
	for i, v := range arr {
		if v == body.To {
			writeErr(w, 400, errString("新名称已存在"))
			return
		}
		if v == body.From {
			arr[i] = body.To
			found = true
		}
	}
	if !found {
		writeErr(w, 404, errString("字典中不存在该项"))
		return
	}
	b, _ := json.Marshal(arr)
	if err := h.db.SetSetting(body.Kind, string(b)); err != nil {
		writeErr(w, 500, err)
		return
	}
	// 级联更新台账引用
	n, err := h.db.RenameAnnotationRefs(body.Kind, body.From, body.To)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"updated": n})
}

func (h *handlers) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	for k, v := range body {
		if _, ok := settingsWhitelist[k]; !ok {
			continue // 忽略未知键
		}
		if k == "scan_interval_min" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 1440 {
				writeErr(w, 400, errString("扫描间隔须为 1~1440 分钟"))
				return
			}
		}
		if k == "dev_types" || k == "owners" {
			var arr []string
			if err := json.Unmarshal([]byte(v), &arr); err != nil {
				writeErr(w, 400, errString("字典格式须为 JSON 字符串数组"))
				return
			}
		}
		if k == "notify_channel" && v != "webhook" && v != "dingtalk" && v != "wecom" {
			writeErr(w, 400, errString("通知渠道须为 webhook / dingtalk / wecom"))
			return
		}
		if k == "uplink_check_sec" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 5 || n > 3600 {
				writeErr(w, 400, errString("探测间隔须为 5~3600 秒"))
				return
			}
		}
		if k == "uplink_probe" && v != "" {
			for _, t := range strings.Split(v, ",") {
				if _, _, err := net.SplitHostPort(strings.TrimSpace(t)); err != nil {
					writeErr(w, 400, errString("探测目标格式须为 host:port（逗号分隔多个）"))
					return
				}
			}
		}
		if k == "notify_webhook" && v != "" && !strings.HasPrefix(v, "http") {
			writeErr(w, 400, errString("Webhook 地址须以 http(s):// 开头"))
			return
		}
		if err := h.db.SetSetting(k, v); err != nil {
			writeErr(w, 500, err)
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// NotifyTest 用当前已保存的配置发送一条测试通知。
func (h *handlers) NotifyTest(w http.ResponseWriter, r *http.Request) {
	cfg := alert.LoadNotifyConfig(h.db)
	if !cfg.Valid() {
		writeErr(w, 400, errString("通知未启用或配置不完整（请先保存渠道配置）"))
		return
	}
	title, text := alert.TestMessage()
	if err := alert.Send(cfg, title, text); err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---- AI ----

// AIChat 从 settings 读取 AI 配置，执行带工具调用的对话。
func (h *handlers) AIChat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		writeErr(w, 400, errString("message 不能为空"))
		return
	}
	baseURL, _ := h.db.GetSetting("ai_base_url")
	model, _ := h.db.GetSetting("ai_model")
	apiKey, _ := h.db.GetSetting("ai_api_key")
	if baseURL == "" || model == "" {
		writeJSON(w, 503, map[string]string{"error": "AI 未配置，请先在设置中填写 Base URL 与模型名"})
		return
	}
	gw := ai.New(ai.Config{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Model: model})
	reply, err := gw.Chat(r.Context(), body.Message, h.aiToolHandler)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]string{"reply": reply})
}

// aiToolHandler 把模型的工具调用路由到真实数据查询。
func (h *handlers) aiToolHandler(name string, args json.RawMessage) (string, error) {
	switch name {
	case "query_ip":
		var a struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", err
		}
		id, ok, err := h.db.SubnetIDForIP(a.IP)
		if err != nil || !ok {
			return `{"found":false,"reason":"该 IP 不在任何受管子网中"}`, nil
		}
		addrs, err := h.db.ListAddresses(id)
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			if addr.IP == a.IP {
				out, _ := json.Marshal(addr)
				return string(out), nil
			}
		}
		return `{"found":false,"reason":"从未观测到该 IP，处于闲置状态"}`, nil

	case "get_subnet_usage":
		var a struct {
			CIDR string `json:"cidr"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", err
		}
		subs, err := h.db.ListSubnets()
		if err != nil {
			return "", err
		}
		for _, sub := range subs {
			if sub.CIDR == a.CIDR {
				st, err := h.db.Stats(sub.ID)
				if err != nil {
					return "", err
				}
				out, _ := json.Marshal(st)
				return string(out), nil
			}
		}
		return `{"found":false,"reason":"子网不存在"}`, nil

	case "list_conflicts":
		alerts, err := h.db.ListAlerts(true)
		if err != nil {
			return "", err
		}
		out, _ := json.Marshal(alerts)
		return string(out), nil
	}
	return "", errString("unknown tool: " + name)
}

// SaveAIConfig 保存 AI 设置。
func (h *handlers) SaveAIConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
		APIKey  string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	for k, v := range map[string]string{"ai_base_url": body.BaseURL, "ai_model": body.Model, "ai_api_key": body.APIKey} {
		if err := h.db.SetSetting(k, v); err != nil {
			writeErr(w, 500, err)
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// AITest 用一句问候测试 AI 配置连通性。
func (h *handlers) AITest(w http.ResponseWriter, r *http.Request) {
	baseURL, _ := h.db.GetSetting("ai_base_url")
	model, _ := h.db.GetSetting("ai_model")
	apiKey, _ := h.db.GetSetting("ai_api_key")
	if baseURL == "" || model == "" {
		writeJSON(w, 503, map[string]string{"error": "AI 未配置"})
		return
	}
	gw := ai.New(ai.Config{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Model: model})
	ctx, cancel := context.WithTimeout(r.Context(), 30e9)
	defer cancel()
	reply, err := gw.Chat(ctx, "你好，请用一句话介绍你自己。", func(string, json.RawMessage) (string, error) {
		return "{}", nil
	})
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]string{"reply": reply})
}

// ---- SPA ----

// spaHandler 服务嵌入的前端产物；非 /api 路径回退到 index.html。
func spaHandler() http.Handler {
	dist, err := fs.Sub(webFS, "web")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(dist, path); err != nil {
			r.URL.Path = "/" // SPA 回退
		}
		fileServer.ServeHTTP(w, r)
	})
}
