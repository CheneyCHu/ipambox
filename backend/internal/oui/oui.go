// Package oui 提供 MAC 地址厂商（OUI）查询与设备类型推断。
// 数据来自 OUI-Master-Database（MIT，聚合 IEEE/Wireshark/Nmap），
// 精简为「前缀 → 厂商简称 + 设备类型」TSV 并通过 go:embed 内嵌，完全离线可用。
package oui

import (
	_ "embed"
	"strings"
)

//go:embed oui.tsv
var data string

// entry 厂商与（可选的）设备类型（英文原始分类）。
type entry struct {
	vendor string
	typ    string
}

var table map[string]entry

func init() {
	table = make(map[string]entry, 41000)
	for _, line := range strings.Split(data, "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 2 || len(parts[0]) != 6 {
			continue
		}
		e := entry{vendor: parts[1]}
		if len(parts) > 2 {
			e.typ = parts[2]
		}
		table[parts[0]] = e
	}
}

// normPrefix 提取 MAC 的 OUI 前缀（大写 6 位十六进制），非法输入返回空串。
func normPrefix(mac string) string {
	var b strings.Builder
	for _, r := range mac {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'a' && r <= 'f':
			b.WriteRune(r - 32)
		case r >= 'A' && r <= 'F':
			b.WriteRune(r)
		}
		if b.Len() == 6 {
			return b.String()
		}
	}
	if b.Len() == 6 {
		return b.String()
	}
	return ""
}

// Lookup 返回 MAC 对应的厂商简称，未收录返回空串。
func Lookup(mac string) string {
	p := normPrefix(mac)
	if p == "" {
		return ""
	}
	return table[p].vendor
}

// typeMap 将 OUI 库的英文设备分类映射为本工具的中文设备类型。
var typeMap = map[string]string{
	"Phone": "手机", "Tablet": "手机",
	"Computer": "电脑", "Wearable": "穿戴设备",
	"Router": "网络设备", "Switch": "网络设备", "Access Point": "网络设备", "Modem": "网络设备",
	"Camera": "摄像头",
	"Printer": "打印机",
	"Server": "服务器", "Storage": "服务器",
	"IoT": "智能设备", "Smart Home": "智能设备", "Appliance": "智能设备", "Thermostat": "智能设备",
	"TV": "影音设备", "Media Player": "影音设备", "Audio": "影音设备",
	"Gaming": "游戏设备", "VoIP": "电话设备", "Industrial": "工控设备",
	"Medical": "医疗设备", "Automotive": "车载设备",
}

// vendorKeywords 厂商名关键词 → 设备类型（库未分类时的兜底推断）。
var vendorKeywords = []struct {
	kw   string
	typ  string
}{
	{"Hikvision", "摄像头"}, {"Dahua", "摄像头"}, {"Axis", "摄像头"},
	{"Raspberry", "开发板"}, {"Arduino", "开发板"}, {"Espressif", "智能设备"},
	{"VMware", "虚拟机"}, {"VirtualBox", "虚拟机"}, {"Parallels", "虚拟机"},
	{"Apple", "电脑"}, {"Dell", "电脑"}, {"Lenovo", "电脑"}, {"ASUSTek", "电脑"},
	{"Huawei", "网络设备"}, {"H3C", "网络设备"}, {"TP-Link", "网络设备"},
	{"Cisco", "网络设备"}, {"Ubiquiti", "网络设备"}, {"MikroTik", "网络设备"},
	{"Xiaomi", "手机"}, {"Samsung", "手机"}, {"OPPO", "手机"}, {"vivo", "手机"},
	{"HPE", "服务器"}, {"Synology", "服务器"}, {"QNAP", "服务器"},
	{"Canon", "打印机"}, {"Epson", "打印机"}, {"Brother", "打印机"},
}

// InferType 推断设备类型：优先用库内分类，其次厂商名关键词；无法推断返回空串。
func InferType(mac string) string {
	p := normPrefix(mac)
	if p == "" {
		return ""
	}
	e, ok := table[p]
	if !ok {
		return ""
	}
	if t := typeMap[e.typ]; t != "" {
		return t
	}
	v := strings.ToLower(e.vendor)
	for _, k := range vendorKeywords {
		if strings.Contains(v, strings.ToLower(k.kw)) {
			return k.typ
		}
	}
	return ""
}
