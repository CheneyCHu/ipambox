// Package models 定义 IPAMBox 的核心领域模型。
package models

import "time"

// IPStatus 是 IP 地址的状态机取值。
type IPStatus string

const (
	StatusFree       IPStatus = "free"       // 从未见过
	StatusOnline     IPStatus = "online"     // 最近扫描可达
	StatusOffline    IPStatus = "offline"    // 曾经在线，当前不可达
	StatusConflict   IPStatus = "conflict"   // 多个 MAC 声称同一 IP
	StatusReserved   IPStatus = "reserved"   // 人工保留（网关、规划预留等）
	StatusRogue      IPStatus = "rogue"      // 在线但从未登记（未授权设备）
)

// Subnet 一个受管网段。
type Subnet struct {
	ID          int64     `json:"id"`
	CIDR        string    `json:"cidr"`        // 例如 192.168.1.0/24
	Name        string    `json:"name"`        // 例如 "办公网"
	Description string    `json:"description"`
	VLAN        int       `json:"vlan"`
	Iface       string    `json:"iface"`       // 接入网卡，如 en0（空 = 未绑定）
	CreatedAt   time.Time `json:"created_at"`
}

// IPAddress 台账中的一条地址记录（含人工标注与最近观测）。
type IPAddress struct {
	ID        int64     `json:"id"`
	SubnetID  int64     `json:"subnet_id"`
	IP        string    `json:"ip"`
	Status    IPStatus  `json:"status"`
	MAC       string    `json:"mac,omitempty"`
	Vendor    string    `json:"vendor,omitempty"` // MAC OUI 厂商
	Hostname  string    `json:"hostname,omitempty"`
	Label     string    `json:"label,omitempty"`  // 人工标注，如 "张三的笔记本"
	Owner     string    `json:"owner,omitempty"`
	DevType   string    `json:"dev_type,omitempty"` // 终端/打印机/服务器/AP…
	FirstSeen time.Time `json:"first_seen,omitempty"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
}

// ScanEvent 一次观测事件，构成时间轴历史。
type ScanEvent struct {
	ID        int64     `json:"id"`
	IP        string    `json:"ip"`
	MAC       string    `json:"mac"`
	Source    string    `json:"source"` // icmp / arp / dhcp / snmp / mdns
	SeenAt    time.Time `json:"seen_at"`
}

// Alert 一条待处理告警。
type Alert struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"` // conflict / new_device / offline / capacity
	Level     string    `json:"level"` // info / warn / critical
	Message   string    `json:"message"`
	Params    string    `json:"params,omitempty"` // JSON 参数，供前端按语言渲染
	IP        string    `json:"ip,omitempty"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// SubnetStats 仪表盘聚合数据。
type SubnetStats struct {
	SubnetID int64   `json:"subnet_id"`
	Total    int     `json:"total"`
	Online   int     `json:"online"`
	Offline  int     `json:"offline"`
	Free     int     `json:"free"`
	Conflict int     `json:"conflict"`
	UsagePct float64 `json:"usage_pct"`
}
