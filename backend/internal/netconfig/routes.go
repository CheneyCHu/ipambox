package netconfig

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// Route 一条路由表项。
type Route struct {
	Destination string `json:"destination"` // CIDR（如 192.168.1.0/24）或 "default"
	Gateway     string `json:"gateway"`     // 下一跳；直连路由为空或为链路地址
	Iface       string `json:"iface"`       // 出接口，如 en0
	Family      string `json:"family"`      // ipv4 / ipv6
}

// ListRoutes 读取本机路由表（IPv4 + IPv6，跨 macOS/Linux）。
func ListRoutes() ([]Route, error) {
	if runtime.GOOS == "darwin" {
		return darwinRoutes()
	}
	return linuxRoutes()
}

// AddRoute 添加静态路由。dest 为 CIDR 或 "default"；需要 root。
func AddRoute(dest, gateway, iface string) error {
	if dest != "default" {
		if _, _, err := net.ParseCIDR(dest); err != nil {
			return validationf("非法目的网段: %q（应为 CIDR，如 10.0.0.0/24，或 default）", dest)
		}
	}
	if gateway == "" || net.ParseIP(gateway) == nil {
		return validationf("非法网关地址: %q", gateway)
	}
	if runtime.GOOS == "darwin" {
		if dest == "default" {
			return run([]string{"route", "-n", "add", "default", gateway})
		}
		return run([]string{"route", "-n", "add", "-net", dest, gateway})
	}
	args := []string{"ip", "route", "add", dest, "via", gateway}
	if iface != "" {
		args = append(args, "dev", iface)
	}
	return run(args)
}

// DeleteRoute 删除路由。dest 为 CIDR 或 "default"；需要 root。
func DeleteRoute(dest, gateway string) error {
	if dest == "" {
		return validationf("目的网段不能为空")
	}
	if dest != "default" {
		if _, _, err := net.ParseCIDR(dest); err != nil {
			return validationf("非法目的网段: %q", dest)
		}
	}
	if runtime.GOOS == "darwin" {
		if dest == "default" {
			return run([]string{"route", "-n", "delete", "default"})
		}
		return run([]string{"route", "-n", "delete", "-net", dest})
	}
	args := []string{"ip", "route", "del", dest}
	if gateway != "" {
		args = append(args, "via", gateway)
	}
	return run(args)
}

// ---- macOS ----

func darwinRoutes() ([]Route, error) {
	var out []Route
	for _, fam := range []struct {
		flag  string
		label string
	}{{"inet", "ipv4"}, {"inet6", "ipv6"}} {
		b, err := exec.Command("netstat", "-rn", "-f", fam.flag).Output()
		if err != nil {
			return nil, fmt.Errorf("读取路由表失败: %v", err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) < 4 || f[0] == "Destination" || f[0] == "Routing" {
				continue
			}
			dest := f[0]
			if fam.flag == "inet" {
				// netstat 输出形如 192.168.1/24 或 default 或单地址 127
				dest = normalizeDarwinDest(dest)
			} else if strings.HasSuffix(dest, "%"+f[3]) {
				dest = strings.TrimSuffix(dest, "%"+f[3])
			}
			gw := f[1]
			if strings.HasPrefix(gw, "link#") {
				gw = ""
			}
			out = append(out, Route{Destination: dest, Gateway: gw, Iface: f[3], Family: fam.label})
		}
	}
	return out, nil
}

// normalizeDarwinDest 把 netstat 的 IPv4 目的地规范成 CIDR；无法识别时原样返回。
// BSD netstat 会省略尾部零段（如 192.168.1 表示 192.168.1.0/24，169.254 表示 169.254.0.0/16）。
func normalizeDarwinDest(s string) string {
	if s == "default" {
		return s
	}
	if strings.Contains(s, "/") {
		return s
	}
	parts := strings.Split(s, ".")
	if len(parts) >= 1 && len(parts) <= 3 {
		valid := true
		for _, p := range parts {
			if p == "" {
				valid = false
				break
			}
			for _, c := range p {
				if c < '0' || c > '9' {
					valid = false
					break
				}
			}
		}
		if valid {
			for len(parts) < 4 {
				parts = append(parts, "0")
			}
			ip := strings.Join(parts, ".")
			if net.ParseIP(ip) != nil {
				return fmt.Sprintf("%s/%d", ip, 8*len(strings.Split(s, ".")))
			}
		}
	}
	if ip := net.ParseIP(s); ip != nil {
		return s + "/32"
	}
	return s
}

// ---- Linux ----

func linuxRoutes() ([]Route, error) {
	var out []Route
	for _, fam := range []struct {
		args  []string
		label string
	}{{[]string{"ip", "route", "show"}, "ipv4"}, {[]string{"ip", "-6", "route", "show"}, "ipv6"}} {
		b, err := exec.Command(fam.args[0], fam.args[1:]...).Output()
		if err != nil {
			return nil, fmt.Errorf("读取路由表失败: %v（需要 iproute2）", err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			rt := Route{Destination: f[0], Family: fam.label}
			for i := 1; i+1 < len(f); i++ {
				switch f[i] {
				case "via":
					rt.Gateway = f[i+1]
				case "dev":
					rt.Iface = f[i+1]
				}
			}
			out = append(out, rt)
		}
	}
	return out, nil
}
