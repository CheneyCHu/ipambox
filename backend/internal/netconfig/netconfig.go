// Package netconfig 读写本机物理网卡的网络配置（IPv4/IPv6、DHCP/静态）。
//
// 注意：修改网卡配置是高危操作（可能导致设备失联），且需要 root 权限。
// 所有 Apply 函数在执行前做参数校验，失败时返回底层错误输出供前端展示。
package netconfig

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Addr 一条地址及其前缀。
type Addr struct {
	IP     string `json:"ip"`
	Prefix int    `json:"prefix"`
}

// Interface 一块物理网卡的完整网络信息。
type Interface struct {
	Name     string `json:"name"`
	MAC      string `json:"mac"`
	Up       bool   `json:"up"`
	MTU      int    `json:"mtu"`
	IPv4     []Addr `json:"ipv4"`
	IPv6     []Addr `json:"ipv6"`
	Gateway  string `json:"gateway"`   // 默认网关（本机级，取一次）
	PortName string `json:"port_name"` // 硬件端口名（如 Wi-Fi、Ethernet；Linux 为空）
	IPv4Mode string `json:"ipv4_mode"` // dhcp / static / ""（未知）
}

// Config 一次配置请求。
type Config struct {
	Family  string `json:"family"`  // ipv4 / ipv6
	Mode    string `json:"mode"`    // dhcp / static
	IP      string `json:"ip"`      // static 时必填
	Prefix  int    `json:"prefix"`  // static 时必填
	Gateway string `json:"gateway"` // 可选
}

// virtualIfacePrefixes 排除回环/虚拟/隧道接口（跨 macOS/Linux）。
var virtualIfacePrefixes = []string{
	"lo", "utun", "gif", "stf", "awdl", "llw",
	"docker", "veth", "br-", "vmnet", "vboxnet", "tun", "tap", "wg", "zt",
	"bridge", "anpi", "p2p", "ipsec",
}

// ValidationError 参数校验类错误，HTTP 层据此返回 400 而非 500。
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

func validationf(format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}

// IsPhysical 判断接口名是否像物理网卡。
func IsPhysical(name string) bool {
	for _, p := range virtualIfacePrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}

// List 枚举真实物理网卡的地址信息。
// macOS 以 `networksetup -listallhardwareports` 的硬件端口清单为准（过滤桥接成员、
// 虚拟 AP 等伪接口）；Linux 以 /sys/class/net/<name>/device 是否存在为准。
func List() ([]Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	gw := defaultGateway()
	ports := hardwarePorts() // device -> 端口名；空 map 表示无法获取（不过滤）
	out := []Interface{}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagLoopback != 0 || !IsPhysical(ifi.Name) {
			continue
		}
		portName, isHardware := ports[ifi.Name]
		if len(ports) > 0 && !isHardware {
			continue // 不在硬件端口清单里（如 ap1、桥接成员口）
		}
		info := Interface{
			Name: ifi.Name, MAC: ifi.HardwareAddr.String(),
			Up: ifi.Flags&net.FlagUp != 0, MTU: ifi.MTU, Gateway: gw,
			PortName: portName,
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ones, bits := ipnet.Mask.Size()
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				info.IPv4 = append(info.IPv4, Addr{IP: ip4.String(), Prefix: ones})
			} else if ipnet.IP.IsGlobalUnicast() && !ipnet.IP.IsLinkLocalUnicast() {
				info.IPv6 = append(info.IPv6, Addr{IP: ipnet.IP.String(), Prefix: ones})
			} else if ipnet.IP.IsLinkLocalUnicast() && bits == 128 {
				// 链路本地地址也列出（只读展示）
				info.IPv6 = append(info.IPv6, Addr{IP: ipnet.IP.String() + "%" + ifi.Name, Prefix: ones})
			}
		}
		// 无 IPv4 的 Thunderbolt / iPhone USB / Bluetooth 类端口不展示（未使用的扩展口）
		if len(info.IPv4) == 0 && isIrrelevantPort(portName) {
			continue
		}
		info.IPv4Mode = detectIPv4Mode(ifi.Name, portName)
		out = append(out, info)
	}
	return out, nil
}

// isIrrelevantPort 判断未使用的扩展/ tethering 端口（雷雳、iPhone USB、蓝牙等）。
// macOS 自动生成的 "Ethernet Adapter (enN)" 幻影端口（雷雳桥接衍生）也算无关项。
func isIrrelevantPort(portName string) bool {
	p := strings.ToLower(portName)
	for _, kw := range []string{"thunderbolt", "iphone", "bluetooth", "vpn", "firewire"} {
		if strings.Contains(p, kw) {
			return true
		}
	}
	if strings.HasPrefix(p, "ethernet adapter (") {
		return true
	}
	return false
}

// hardwarePorts 返回 device -> 硬件端口名映射。
// macOS 解析 networksetup；Linux 检查 /sys/class/net/<dev>/device（物理设备才有）。
func hardwarePorts() map[string]string {
	if runtime.GOOS == "darwin" {
		return darwinHardwarePorts()
	}
	// Linux：sysfs 中有 device 链接的才是物理网卡
	out := map[string]string{}
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if _, err := os.Stat("/sys/class/net/" + e.Name() + "/device"); err == nil {
			out[e.Name()] = e.Name()
		}
	}
	return out
}

func darwinHardwarePorts() map[string]string {
	out := map[string]string{}
	b, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		return out
	}
	var lastPort string
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "Hardware Port: "); ok {
			lastPort = strings.TrimSpace(rest)
		} else if rest, ok := strings.CutPrefix(line, "Device: "); ok {
			dev := strings.TrimSpace(rest)
			if lastPort != "" && dev != "" {
				out[dev] = lastPort
			}
		}
	}
	return out
}

// detectIPv4Mode 检测当前 IPv4 获取方式（dhcp/static/未知）。
func detectIPv4Mode(dev, portName string) string {
	if runtime.GOOS == "darwin" {
		if portName == "" {
			return ""
		}
		b, err := exec.Command("networksetup", "-getinfo", portName).Output()
		if err != nil {
			return ""
		}
		first := strings.ToLower(strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0]))
		switch {
		case strings.HasPrefix(first, "dhcp"):
			return "dhcp"
		case strings.HasPrefix(first, "manual"):
			return "static"
		}
		return ""
	}
	// Linux：优先 NetworkManager
	if b, err := exec.Command("nmcli", "-g", "ipv4.method", "dev", "show", dev).Output(); err == nil {
		switch strings.TrimSpace(string(b)) {
		case "auto":
			return "dhcp"
		case "manual":
			return "static"
		}
	}
	return ""
}

// IPv4ModeFor 对外暴露的 IPv4 获取方式检测：内部解析硬件端口名后调用 detectIPv4Mode。
// 返回 "dhcp" / "static" / ""（未知，如非 macOS 硬件端口清单外的设备）。
func IPv4ModeFor(dev string) string {
	return detectIPv4Mode(dev, hardwarePorts()[dev])
}

func defaultGateway() string {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("route", "-n", "get", "default").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "gateway:") {
					return strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
				}
			}
		}
		return ""
	}
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err == nil {
		f := strings.Fields(string(out))
		for i, w := range f {
			if w == "via" && i+1 < len(f) {
				return f[i+1]
			}
		}
	}
	return ""
}

// Apply 应用网卡配置。需要 root；失败返回包含命令输出的错误。
func Apply(iface string, cfg Config) error {
	if !IsPhysical(iface) {
		return validationf("拒绝操作虚拟接口 %q", iface)
	}
	if _, err := net.InterfaceByName(iface); err != nil {
		return validationf("接口不存在: %s", iface)
	}
	switch cfg.Mode {
	case "dhcp":
		return applyDHCP(iface, cfg.Family)
	case "static":
		if net.ParseIP(cfg.IP) == nil {
			return validationf("非法 IP: %q", cfg.IP)
		}
		if cfg.Prefix <= 0 || cfg.Prefix > 128 {
			return validationf("非法前缀长度: %d", cfg.Prefix)
		}
		return applyStatic(iface, cfg)
	default:
		return validationf("未知模式: %q（应为 dhcp 或 static）", cfg.Mode)
	}
}

func run(cmds ...[]string) error {
	for _, c := range cmds {
		out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("执行 %s 失败: %v\n%s（需要 root 权限运行 IPAMBox）",
				strings.Join(c, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func applyStatic(iface string, cfg Config) error {
	cidr := fmt.Sprintf("%s/%d", cfg.IP, cfg.Prefix)
	if runtime.GOOS == "darwin" {
		if cfg.Family == "ipv6" {
			return run([]string{"ifconfig", iface, "inet6", cidr})
		}
		mask := prefixToMask(cfg.Prefix)
		cmds := [][]string{{"ifconfig", iface, "inet", cfg.IP, "netmask", mask}}
		if cfg.Gateway != "" {
			cmds = append(cmds, []string{"route", "change", "default", cfg.Gateway})
		}
		return run(cmds...)
	}
	// Linux (iproute2)
	if cfg.Family == "ipv6" {
		return run([]string{"ip", "-6", "addr", "add", cidr, "dev", iface})
	}
	cmds := [][]string{
		{"ip", "addr", "flush", "dev", iface, "scope", "global"},
		{"ip", "addr", "add", cidr, "dev", iface},
	}
	if cfg.Gateway != "" {
		cmds = append(cmds, []string{"ip", "route", "replace", "default", "via", cfg.Gateway, "dev", iface})
	}
	return run(cmds...)
}

func applyDHCP(iface, family string) error {
	if runtime.GOOS == "darwin" {
		if family == "ipv6" {
			return validationf("macOS 下 IPv6 请使用自动配置（默认即为 SLAAC/DHCPv6）")
		}
		return run([]string{"ipconfig", "set", iface, "DHCP"})
	}
	if family == "ipv6" {
		return run([]string{"dhclient", "-6", iface})
	}
	return run([]string{"dhclient", iface})
}

// prefixToMask 将前缀长度转为点分掩码（仅 IPv4）。
func prefixToMask(prefix int) string {
	if prefix < 0 || prefix > 32 {
		prefix = 24
	}
	mask := net.CIDRMask(prefix, 32)
	return net.IP(mask).String()
}
