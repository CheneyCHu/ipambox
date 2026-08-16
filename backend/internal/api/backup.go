package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ---- 备份与恢复 ----

// BackupExport 导出数据库一致性快照（.db 附件下载）。
func (h *handlers) BackupExport(w http.ResponseWriter, r *http.Request) {
	path, cleanup, err := h.db.ExportSnapshot()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	defer cleanup()
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	defer f.Close()
	name := "ipambox-backup-" + time.Now().Format("20060102-150405") + ".db"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	_, _ = io.Copy(w, f)
}

// BackupImport 导入备份：上传 .db 文件，校验后热替换当前数据库（不可恢复，前端须确认）。
func (h *handlers) BackupImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<20) // 上限 256MB
	tmp, err := os.CreateTemp("", "ipambox-restore-*.db")
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, r.Body); err != nil {
		writeErr(w, 400, fmt.Errorf("读取上传内容失败: %v", err))
		return
	}
	if err := h.db.ReplaceWith(tmp.Name()); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
