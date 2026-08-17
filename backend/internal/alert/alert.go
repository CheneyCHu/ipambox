// Package alert 负责告警的产生与推送（Webhook / 钉钉 / 企业微信）。
package alert

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/ipambox/ipambox/internal/models"
	"github.com/ipambox/ipambox/internal/store"
)

// Alerter 聚合告警规则与通知渠道。
type Alerter struct {
	db       *store.Store
	knownMAC map[string]string // ip -> 上次观测到的 MAC（内存态，重启后重建）
	rogueHit map[string]bool   // "ip|mac" -> 已告警过（内存态，重启后重建）
}

func New(db *store.Store) *Alerter {
	return &Alerter{db: db, knownMAC: map[string]string{}, rogueHit: map[string]bool{}}
}

// NotifyRogue 首次发现未登记设备时产生未授权告警（同一 ip+mac 只报一次）。
func (a *Alerter) NotifyRogue(ip, mac, vendor string) {
	key := ip + "|" + mac
	if a.rogueHit[key] {
		return
	}
	a.rogueHit[key] = true
	vendorText := vendor
	if vendorText == "" {
		vendorText = "未知厂商"
	}
	msg := "发现未授权设备：" + ip
	if mac != "" {
		msg += "（MAC " + mac + "，" + vendorText + "）"
	}
	params, _ := json.Marshal(map[string]any{"ip": ip, "mac": mac, "vendor": vendor})
	a.raise(&models.Alert{
		Type:    "rogue",
		Level:   "warn",
		Message: msg,
		Params:  string(params),
		IP:      ip,
	})
}

// CheckConflict 同一 IP 的 MAC 发生变化时产生冲突告警。
func (a *Alerter) CheckConflict(ip, mac string) {
	if mac == "" {
		return
	}
	prev, ok := a.knownMAC[ip]
	a.knownMAC[ip] = mac
	if ok && prev != mac {
		a.raise(&models.Alert{
			Type:    "conflict",
			Level:   "critical",
			Message: "IP " + ip + " 的 MAC 由 " + prev + " 变为 " + mac + "，疑似地址冲突",
			Params:  `{"ip":"` + ip + `","prev":"` + prev + `","mac":"` + mac + `"}`,
			IP:      ip,
		})
	}
}

// NotifyOffline 一轮扫描后报告离线的 IP（合并为一条告警）。
func (a *Alerter) NotifyOffline(subnetCIDR string, ips []string) {
	if len(ips) == 0 {
		return
	}
	show := ips
	suffix := ""
	if len(ips) > 5 {
		show = ips[:5]
		suffix = " 等共 " + strconv.Itoa(len(ips)) + " 个"
	}
	params, _ := json.Marshal(map[string]any{"cidr": subnetCIDR, "ips": ips, "count": len(ips)})
	a.raise(&models.Alert{
		Type:    "offline",
		Level:   "warn",
		Message: subnetCIDR + " 有设备离线：" + strings.Join(show, "、") + suffix,
		Params:  string(params),
		IP:      ips[0],
	})
}

// RaiseRaw 供 uplink 等模块直接产生一条告警（走统一的落库+推送流程）。
func (a *Alerter) RaiseRaw(alertType, level, message string, params ...string) {
	p := ""
	if len(params) > 0 {
		p = params[0]
	}
	a.raise(&models.Alert{Type: alertType, Level: level, Message: message, Params: p})
}

// raise 落库并按配置推送通知；推送失败进入待补发队列（断网续存）。
func (a *Alerter) raise(al *models.Alert) {
	if err := a.db.CreateAlert(al); err != nil {
		log.Printf("alert: 写入失败: %v", err)
		return
	}
	cfg := LoadNotifyConfig(a.db)
	if !cfg.Valid() || !cfg.Wants(al.Type) {
		return
	}
	go func() {
		title, text := levelTitle(al.Level), al.Message
		if err := Send(cfg, title, text); err != nil {
			log.Printf("alert: 通知推送失败，已入待补发队列: %v", err)
			if qerr := a.db.EnqueueNotification(title, text, al.Type, err.Error()); qerr != nil {
				log.Printf("alert: 补发队列写入失败: %v", qerr)
			}
		}
	}()
}

// LoadNotifyConfig 从 settings 表读取通知配置（测试接口与推送共用）。
func LoadNotifyConfig(db *store.Store) NotifyConfig {
	get := func(k string) string { v, _ := db.GetSetting(k); return v }
	cfg := NotifyConfig{
		Enabled: get("notify_enabled") == "1",
		Channel: get("notify_channel"),
		Webhook: get("notify_webhook"),
		Secret:  get("notify_secret"),
		Events:  map[string]bool{},
	}
	if cfg.Channel == "" {
		cfg.Channel = "webhook"
	}
	for _, e := range strings.Split(get("notify_events"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			cfg.Events[e] = true
		}
	}
	return cfg
}

func levelTitle(level string) string {
	switch level {
	case "critical":
		return "严重告警"
	case "warn":
		return "警告"
	default:
		return "提示"
	}
}

// 测试消息内容
func TestMessage() (string, string) {
	return "通知测试", "这是一条测试消息，发送于 "+time.Now().Format("2006-01-02 15:04:05")+"。\n如果你看到它，说明通知渠道配置正确。"
}
