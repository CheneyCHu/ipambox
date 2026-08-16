// Package scanner 实现多源自动发现引擎。
//
// 设计要点：
//   - 多源交叉验证：ICMP 不可达不代表离线（防火墙禁 ping），
//     需结合 ARP 表、DHCP 租约、SNMP 网桥表综合判断；
//   - 每种来源实现 Source 接口，引擎聚合结果；
//   - 扫描限流，避免被安全设备误判为扫描器。
package scanner

import (
	"context"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipambox/ipambox/internal/alert"
	"github.com/ipambox/ipambox/internal/models"
	"github.com/ipambox/ipambox/internal/store"
)

// Observation 一次"看到某 IP"的观测。
type Observation struct {
	IP     string
	MAC    string
	Source string // icmp / arp / snmp / mdns
}

// Source 是一个发现来源。
type Source interface {
	Name() string
	Scan(ctx context.Context, cidr string) ([]Observation, error)
}

// Engine 聚合各来源并驱动状态机。
type Engine struct {
	db      *store.Store
	sources []Source
	alerter *alert.Alerter
}

// New 创建引擎并注册默认来源。
func New(db *store.Store, al *alert.Alerter) *Engine {
	return &Engine{
		db:      db,
		alerter: al,
		sources: []Source{
			ICMPSource{Concurrency: 128, Timeout: 800 * time.Millisecond},
			ARPSource{}, // 本机 ARP 表兜底
			// TODO V1.5: SNMPSource{}, MDNSSource{},
		},
	}
}

// RunScheduler 周期扫描所有子网。每轮结束后重新读取设置：
// scan_interval_min（分钟）、auto_scan（0 时暂停定时扫描）。
func (e *Engine) RunScheduler(defaultInterval time.Duration) {
	e.scanAll() // 启动即扫一轮
	for {
		interval := defaultInterval
		if v, err := e.db.GetSetting("scan_interval_min"); err == nil {
			if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 1440 {
				interval = time.Duration(n) * time.Minute
			}
		}
		time.Sleep(interval)
		if v, err := e.db.GetSetting("auto_scan"); err == nil && v == "0" {
			continue // 用户关闭了定时扫描
		}
		e.scanAll()
	}
}

// ScanNow 供 API 触发即时扫描（异步执行，立即返回）。
func (e *Engine) ScanNow(subnetID int64) error {
	sub, err := e.db.GetSubnet(subnetID)
	if err != nil {
		return err
	}
	go func() {
		if err := e.scanSubnet(context.Background(), *sub); err != nil {
			log.Printf("scanner: scan-now %s: %v", sub.CIDR, err)
		}
	}()
	return nil
}

// resolveHostnames 并发反向解析主机名（单个 2s 超时，最多 32 并发）。
func resolveHostnames(seen map[string]Observation) map[string]string {
	out := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	for ip := range seen {
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			r := &net.Resolver{}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			names, err := r.LookupAddr(ctx, ip)
			if err == nil && len(names) > 0 {
				mu.Lock()
				out[ip] = strings.TrimSuffix(names[0], ".")
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	return out
}

func (e *Engine) scanAll() {
	subs, err := e.db.ListSubnets()
	if err != nil {
		log.Printf("scanner: list subnets: %v", err)
		return
	}
	for _, sub := range subs {
		if err := e.scanSubnet(context.Background(), sub); err != nil {
			log.Printf("scanner: %s: %v", sub.CIDR, err)
		}
	}
}

func (e *Engine) scanSubnet(ctx context.Context, sub models.Subnet) error {
	cidr := sub.CIDR
	seen := map[string]Observation{} // ip -> 合并后的观测
	for _, src := range e.sources {
		obs, err := src.Scan(ctx, cidr)
		if err != nil {
			log.Printf("scanner: source %s: %v", src.Name(), err)
			continue // 单源失败不中断
		}
		for _, o := range obs {
			if prev, ok := seen[o.IP]; !ok || prev.MAC == "" {
				seen[o.IP] = o
			}
		}
	}
	// 富化：ICMP 观测不带 MAC，用本机 ARP 表补齐（ping 会触发 ARP 解析）
	arp := arpTable()
	for ip, o := range seen {
		if o.MAC == "" {
			if mac, ok := arp[ip]; ok {
				o.MAC = mac
				seen[ip] = o
			}
		}
	}
	// 反向 DNS 解析主机名（并发、单个限时，失败静默）
	hosts := resolveHostnames(seen)
	for _, o := range seen {
		if err := e.db.MarkSeen(o.IP, o.MAC, hosts[o.IP], o.Source); err != nil {
			log.Printf("scanner: mark seen %s: %v", o.IP, err)
		}
		// 冲突检测：同一 IP 出现新 MAC → 告警
		if e.alerter != nil {
			e.alerter.CheckConflict(o.IP, o.MAC)
		}
	}
	// 离线判定：上一轮在线而本轮未出现的地址降级为 offline
	seenIPs := make(map[string]bool, len(seen))
	for ip := range seen {
		seenIPs[ip] = true
	}
	if gone, err := e.db.MarkOfflineMissing(sub.ID, seenIPs); err != nil {
		log.Printf("scanner: offline check %s: %v", cidr, err)
	} else if len(gone) > 0 {
		log.Printf("scanner: %s marked %d hosts offline", cidr, len(gone))
		if e.alerter != nil {
			e.alerter.NotifyOffline(cidr, gone)
		}
	}
	log.Printf("scanner: %s done, %d hosts seen", cidr, len(seen))
	return nil
}
