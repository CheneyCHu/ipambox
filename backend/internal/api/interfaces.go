package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ipambox/ipambox/internal/netconfig"
)

// NICInfo 一块物理网卡的 IPv4 信息及其推算出的受管网段。
type NICInfo struct {
	Name     string `json:"name"`      // 网卡名，如 en0
	IP       string `json:"ip"`        // 本机地址，如 192.168.1.153
	Mask     string `json:"mask"`      // 点分掩码，如 255.255.255.0
	Prefix   int    `json:"prefix"`    // 前缀长度，如 24
	CIDR     string `json:"cidr"`      // 推算网段，如 192.168.1.0/24
	IPv4Mode string `json:"ipv4_mode"` // 获取方式：dhcp / static / ""（未知）
}

// listNICs 枚举处于 UP 状态的物理网卡（排除回环/虚拟/隧道），返回 IPv4 信息。
func listNICs() []NICInfo {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := []NICInfo{}
	for _, ifi := range ifaces {
		// 仅保留已启用且非回环的接口；排除常见虚拟接口命名
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualIface(ifi.Name) {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue // 仅 IPv4
			}
			ones, bits := ipnet.Mask.Size()
			if bits != 32 || ones == 0 {
				continue
			}
			network := ip4.Mask(ipnet.Mask)
			out = append(out, NICInfo{
				Name:     ifi.Name,
				IP:       ip4.String(),
				Mask:     net.IP(ipnet.Mask).String(),
				Prefix:   ones,
				CIDR:     fmt.Sprintf("%s/%d", network.String(), ones),
				IPv4Mode: netconfig.IPv4ModeFor(ifi.Name),
			})
			break // 每块网卡只取第一个 IPv4
		}
	}
	return out
}

// isVirtualIface 排除桥接/隧道/VPN 等虚拟接口（跨 macOS/Linux）。
func isVirtualIface(name string) bool {
	virtualPrefixes := []string{
		"lo", "utun", "gif", "stf", "awdl", "llw", // macOS
		"docker", "veth", "br-", "vmnet", "vboxnet", "tun", "tap", "wg", "zt", // Linux/VPN
		"bridge", "anpi", "ap", "p2p", "ipsec",
	}
	for _, p := range virtualPrefixes {
		if len(name) >= len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}

// ---- 网络设置（读写本机网卡配置） ----

// ListNetworkInterfaces 返回物理网卡完整信息（IPv4/IPv6/MAC/网关）。
func (h *handlers) ListNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	list, err := netconfig.List()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, list)
}

// ConfigureInterface 修改网卡 IPv4/IPv6 配置（静态或 DHCP）。
// 高危操作：需要 root；前端必须二次确认。
func (h *handlers) ConfigureInterface(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var cfg netconfig.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, 400, err)
		return
	}
	if cfg.Family == "" {
		cfg.Family = "ipv4"
	}
	if err := netconfig.Apply(name, cfg); err != nil {
		var ve *netconfig.ValidationError
		if errors.As(err, &ve) {
			writeErr(w, 400, err)
		} else {
			writeErr(w, 500, err)
		}
		return
	}
	writeJSON(w, 200, map[string]string{"status": "applied"})
}
