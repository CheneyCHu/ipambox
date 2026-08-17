package scanner

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ICMPSource 通过系统 ping 并发探测（无 root 需求）。
type ICMPSource struct {
	Concurrency int
	Timeout     time.Duration
}

func (ICMPSource) Name() string { return "icmp" }

func (s ICMPSource) Scan(ctx context.Context, cidr string) ([]Observation, error) {
	ips, err := hostsOf(cidr)
	if err != nil {
		return nil, err
	}
	var (
		out  []Observation
		mu   sync.Mutex
		wg   sync.WaitGroup
		sem  = make(chan struct{}, s.Concurrency)
	)
	for _, ip := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			if ping(ctx, ip, s.Timeout) {
				mu.Lock()
				out = append(out, Observation{IP: ip, Source: "icmp"})
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	return out, nil
}

func ping(ctx context.Context, ip string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// 不能用 -t：macOS/Linux 上 -t 是 TTL，设为 1 会导致跨子网（经网关）的目标全部不可达。
	// 超时参数：macOS/BSD 的 -W 单位是毫秒，Linux(iputils) 的 -W 单位是秒。
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		ms := int(timeout.Milliseconds())
		if ms < 100 {
			ms = 100
		}
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(ms), ip)
	} else {
		sec := int(timeout.Seconds() + 0.5)
		if sec < 1 {
			sec = 1
		}
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(sec), ip)
	}
	return cmd.Run() == nil
}

// ARPSource 读取本机 ARP 表作为兜底（ping 不通但同网段通信过的设备）。
// 只返回属于被扫网段的条目：全量回传会拖慢反向 DNS 富化，且干扰其他子网统计。
type ARPSource struct{}

func (ARPSource) Name() string { return "arp" }

func (ARPSource) Scan(ctx context.Context, cidr string) ([]Observation, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	var out []Observation
	for ip, mac := range arpTable() {
		if mac == "" {
			continue // 广播/无效条目
		}
		if parsed := net.ParseIP(ip); parsed == nil || !ipnet.Contains(parsed) {
			continue
		}
		out = append(out, Observation{IP: ip, MAC: mac, Source: "arp"})
	}
	return out, nil
}

// arpTable 跨平台读取本机 ARP 表（ip -> mac）。
// Linux 读 /proc/net/arp；macOS 解析 `arp -a` 输出：
//   ? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]
func arpTable() map[string]string {
	t := map[string]string{}
	if f, err := os.Open("/proc/net/arp"); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) >= 4 && net.ParseIP(fields[0]) != nil && strings.Contains(fields[3], ":") {
				if m := normalizeMAC(fields[3]); m != "" {
					t[fields[0]] = m
				}
			}
		}
		return t
	}
	// macOS / BSD：必须加 -n（不做 DNS 反向解析），否则 DNS 异常时 arp -a 会无限期卡住；
	// 同时加超时兜底，卡死也不拖垮扫描流程。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "arp", "-n", "-a").Output()
	if err != nil {
		return t
	}
	for _, line := range strings.Split(string(out), "\n") {
		l := strings.Index(line, "(")
		r := strings.Index(line, ")")
		if l < 0 || r < 0 || r <= l+1 {
			continue
		}
		ip := line[l+1 : r]
		if net.ParseIP(ip) == nil {
			continue
		}
		rest := line[r+1:]
		ai := strings.Index(rest, " at ")
		if ai < 0 {
			continue
		}
		mac := strings.Fields(rest[ai+4:])
		if len(mac) > 0 && strings.Contains(mac[0], ":") {
			if m := normalizeMAC(mac[0]); m != "" {
				t[ip] = m
			}
		}
	}
	return t
}

// normalizeMAC 统一 MAC 格式：小写、每段补零（macOS arp 输出不补零）。
// 广播/全零地址返回空串表示无效。
func normalizeMAC(s string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), ":")
	if len(parts) != 6 {
		return ""
	}
	for i, p := range parts {
		if len(p) > 2 {
			return ""
		}
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	m := strings.Join(parts, ":")
	if m == "ff:ff:ff:ff:ff:ff" || m == "00:00:00:00:00:00" {
		return ""
	}
	return m
}

// hostsOf 展开 CIDR 的所有主机地址（不含网络/广播地址）。
func hostsOf(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	var out []string
	for cur := ip.Mask(ipnet.Mask); ipnet.Contains(cur); incIP(cur) {
		out = append(out, cur.String())
	}
	if len(out) > 2 { // 去掉网络地址与广播地址
		out = out[1 : len(out)-1]
	}
	return out, nil
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			return
		}
	}
}
