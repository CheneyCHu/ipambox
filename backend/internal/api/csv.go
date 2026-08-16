package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"
)

// ---- 设备台账 ----

func (h *handlers) ListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.db.ListAllDevices()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, devices)
}

// ---- CSV 导入导出 ----

var csvHeader = []string{"ip", "mac", "status", "hostname", "label", "owner", "dev_type", "last_seen"}

// ExportSubnetCSV 导出子网台账（Excel 可直接打开的 CSV，带 BOM）。
func (h *handlers) ExportSubnetCSV(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	sub, err := h.db.GetSubnet(id)
	if err != nil {
		writeErr(w, 404, errString("子网不存在"))
		return
	}
	addrs, err := h.db.ListAddresses(id)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	filename := "ipambox_" + strings.ReplaceAll(sub.CIDR, "/", "_") + ".csv"
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM，让 Excel 正确识别 UTF-8
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHeader)
	for _, a := range addrs {
		last := ""
		if !a.LastSeen.IsZero() {
			last = a.LastSeen.Format("2006-01-02 15:04:05")
		}
		_ = cw.Write([]string{a.IP, a.MAC, string(a.Status), a.Hostname, a.Label, a.Owner, a.DevType, last})
	}
	cw.Flush()
}

// ImportCSV 导入标注（仅更新 label/owner/dev_type/mac 备注字段，不改扫描状态）。
// 请求体为 CSV 文本（与导出格式一致），按 ip 匹配更新。
func (h *handlers) ImportCSV(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r)
	cr := csv.NewReader(r.Body)
	rows, err := cr.ReadAll()
	if err != nil {
		writeErr(w, 400, errString("CSV 解析失败: "+err.Error()))
		return
	}
	updated, skipped := 0, 0
	for i, row := range rows {
		if i == 0 && len(row) > 0 && strings.EqualFold(strings.TrimPrefix(row[0], "\uFEFF"), "ip") {
			continue // 表头
		}
		if len(row) < 7 {
			skipped++
			continue
		}
		ip, label, owner, devType := strings.TrimSpace(row[0]), row[4], row[5], row[6]
		if ip == "" {
			skipped++
			continue
		}
		ok, err := h.db.UpdateAnnotationByIP(id, ip, label, owner, devType)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		if ok {
			updated++
		} else {
			skipped++ // 该 IP 尚无记录（未被扫描发现过），跳过
		}
	}
	writeJSON(w, 200, map[string]int{"updated": updated, "skipped": skipped})
}
