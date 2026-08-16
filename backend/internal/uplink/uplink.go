// Package uplink 负责外网连通性监测与断网续存补发：
// 周期性探测外网（TCP 拨号），状态变化落库并产生告警；
// 离线期间推送失败的通知进入队列，恢复在线时自动补发。
package uplink

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipambox/ipambox/internal/alert"
	"github.com/ipambox/ipambox/internal/store"
)

const maxAttempts = 10 // 单条通知最多补发 10 次，超过后放弃

// Status 当前连通状态快照（供 API 返回）。
type Status struct {
	Online     bool   `json:"online"`
	Since      string `json:"since"`        // 当前状态起始时间
	LastOnline string `json:"last_online"`  // 最近一次在线时间
	Probe      string `json:"probe"`        // 当前使用的探测目标
	Detail     string `json:"detail"`       // 最近一次探测的说明/错误
	Interval   int    `json:"interval_sec"` // 探测间隔
}

// Monitor 外网连通监测器。
type Monitor struct {
	db      *store.Store
	alerter *alert.Alerter

	mu         sync.Mutex
	online     bool
	since      time.Time
	lastOnline time.Time
	detail     string
	checked    bool // 是否已完成首次探测
}

func New(db *store.Store, alerter *alert.Alerter) *Monitor {
	return &Monitor{db: db, alerter: alerter}
}

// probeTargets 读取设置的探测目标（host:port，逗号可配多个），缺省公共 DNS。
func (m *Monitor) probeTargets() []string {
	v, _ := m.db.GetSetting("uplink_probe")
	if strings.TrimSpace(v) == "" {
		v = "223.5.5.5:53,114.114.114.114:53"
	}
	out := []string{}
	for _, t := range strings.Split(v, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// checkInterval 探测间隔，设置键 uplink_check_sec（5~3600 秒），默认 30 秒。
func (m *Monitor) checkInterval() time.Duration {
	v, _ := m.db.GetSetting("uplink_check_sec")
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 5 && n <= 3600 {
		return time.Duration(n) * time.Second
	}
	return 30 * time.Second
}

// probe 依次尝试各探测目标，任一可达即视为在线。
func (m *Monitor) probe() (bool, string) {
	targets := m.probeTargets()
	var lastErr error
	for _, t := range targets {
		conn, err := net.DialTimeout("tcp", t, 3*time.Second)
		if err == nil {
			conn.Close()
			return true, "探测 " + t + " 可达"
		}
		lastErr = err
	}
	return false, fmt.Sprintf("探测 %s 均不可达: %v", strings.Join(targets, "、"), lastErr)
}

// CheckNow 立即执行一次探测并处理状态变化；返回最新状态。
func (m *Monitor) CheckNow() Status {
	online, detail := m.probe()

	m.mu.Lock()
	first := !m.checked
	changed := m.checked && online != m.online
	m.checked = true
	if first || changed {
		m.since = time.Now()
	}
	if online {
		m.lastOnline = time.Now()
	}
	m.detail = detail
	m.online = online
	m.mu.Unlock()

	if changed || (first && !online) {
		_ = m.db.RecordUplinkEvent(online, detail)
	}
	switch {
	case changed && !online:
		log.Printf("uplink: 外网中断（%s），进入离线自治模式", detail)
		m.alerter.RaiseRaw("uplink", "warn",
			"外网连接中断，进入离线自治模式：本地扫描与台账记录不受影响，通知将在网络恢复后补发。")
	case changed && online:
		log.Printf("uplink: 外网已恢复（%s）", detail)
		m.onRecovered()
	}
	return m.Status()
}

// onRecovered 网络恢复：补发队列中的通知，并产生恢复告警。
func (m *Monitor) onRecovered() {
	pending, err := m.db.ListPendingNotifications()
	if err != nil {
		log.Printf("uplink: 读取待补发队列失败: %v", err)
		return
	}
	sent, failed := 0, 0
	cfg := alert.LoadNotifyConfig(m.db)
	if cfg.Valid() {
		for _, p := range pending {
			if err := alert.Send(cfg, p.Title, p.Text); err != nil {
				failed++
				_ = m.db.BumpPendingAttempt(p.ID, err.Error())
				if p.Attempts+1 >= maxAttempts {
					log.Printf("uplink: 通知补发超过 %d 次，放弃（%s）", maxAttempts, p.Title)
					_ = m.db.DeletePending(p.ID)
				}
				continue
			}
			sent++
			_ = m.db.DeletePending(p.ID)
		}
	} else {
		failed = len(pending)
	}
	msg := fmt.Sprintf("外网连接已恢复。补发通知 %d 条", sent)
	if failed > 0 {
		msg += fmt.Sprintf("，%d 条仍待补发", failed)
	}
	m.alerter.RaiseRaw("uplink", "info", msg+"。")
}

// Status 返回当前状态快照。
func (m *Monitor) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{
		Online:   m.online,
		Probe:    strings.Join(m.probeTargets(), ", "),
		Detail:   m.detail,
		Interval: int(m.checkInterval().Seconds()),
	}
	if m.checked {
		st.Since = m.since.Format("2006-01-02 15:04:05")
	}
	if !m.lastOnline.IsZero() {
		st.LastOnline = m.lastOnline.Format("2006-01-02 15:04:05")
	}
	return st
}

// Run 启动周期探测（阻塞，应 go 调用）。
func (m *Monitor) Run() {
	m.CheckNow() // 启动即探测一次，尽快确定初始状态
	for {
		time.Sleep(m.checkInterval())
		m.CheckNow()
	}
}
