package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ipambox/ipambox/internal/build"
)

// ---- OTA 在线升级 ----
// 清单（manifest）JSON 格式（单平台简写或按平台分发均可）：
//   {"version":"1.1.0","url":"https://…","sha256":"…","notes":"…"}
//   {"version":"1.1.0","notes":"…","platforms":{"linux_arm64":{"url":"…","sha256":"…"},…}}
// 升级流程：按本机 GOOS/GOARCH 选包 → 下载 → SHA256 校验 → 原子替换自身 → 重启。
// 全程不中断数据库（SQLite 文件不受影响），重启后 token 失效需重新登录。

type updateManifest struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Notes     string `json:"notes"`
	Platforms map[string]struct {
		URL    string `json:"url"`
		SHA256 string `json:"sha256"`
	} `json:"platforms"`
}

// forThisDevice 按本机平台取出下载地址与校验和（无平台表时回落顶层字段）。
func (m *updateManifest) forThisDevice() (url, sha string, err error) {
	key := runtime.GOOS + "_" + runtime.GOARCH
	if m.Platforms != nil {
		if p, ok := m.Platforms[key]; ok && p.URL != "" {
			return p.URL, p.SHA256, nil
		}
		return "", "", fmt.Errorf("更新清单没有适配本机平台（%s）的安装包", key)
	}
	if m.URL == "" {
		return "", "", fmt.Errorf("更新清单缺少下载地址")
	}
	return m.URL, m.SHA256, nil
}

// VersionInfo 当前版本。
func (h *handlers) VersionInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"version": build.Version})
}

// compareVersion 返回 a>b:1, a<b:-1, a==b:0（按点分数字比较，非数字段忽略）。
func compareVersion(a, b string) int {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

func (h *handlers) manifestURL() string {
	v, _ := h.db.GetSetting("update_manifest_url")
	if strings.TrimSpace(v) == "" {
		v = "https://raw.githubusercontent.com/CheneyCHu/ipambox/main/release/latest.json"
	}
	return v
}

func fetchManifest(url string) (*updateManifest, error) {
	client := &http.Client{Timeout: 30 * time.Second} // 部分网络访问 GitHub 较慢，8s 易误判
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取更新清单失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("获取更新清单失败: HTTP %d", resp.StatusCode)
	}
	var m updateManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&m); err != nil {
		return nil, fmt.Errorf("更新清单格式错误: %v", err)
	}
	if m.Version == "" {
		return nil, fmt.Errorf("更新清单缺少 version 字段")
	}
	if _, _, err := m.forThisDevice(); err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateCheck 检查是否有新版本（不下载）。
func (h *handlers) UpdateCheck(w http.ResponseWriter, _ *http.Request) {
	m, err := fetchManifest(h.manifestURL())
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"current":    build.Version,
		"latest":     m.Version,
		"has_update": compareVersion(m.Version, build.Version) > 0,
		"notes":      m.Notes,
	})
}

// UpdateApply 下载清单中的新二进制并原地替换重启（仅管理员）。
func (h *handlers) UpdateApply(w http.ResponseWriter, _ *http.Request) {
	m, err := fetchManifest(h.manifestURL())
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	if compareVersion(m.Version, build.Version) <= 0 {
		writeErr(w, 400, errString("当前已是最新版本"))
		return
	}
	exe, err := os.Executable()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	exe, _ = filepath.EvalSymlinks(exe)

	// 0. 选本机平台的下载地址
	dlURL, wantSHA, err := m.forThisDevice()
	if err != nil {
		writeErr(w, 502, err)
		return
	}

	// 1. 下载到同目录临时文件（同分区，保证 rename 原子）
	tmp := exe + ".new"
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(dlURL)
	if err != nil {
		writeErr(w, 502, fmt.Errorf("下载失败: %v", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		writeErr(w, 502, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode))
		return
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	hasher := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, hasher), resp.Body)
	f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		writeErr(w, 502, fmt.Errorf("下载中断: %v", copyErr))
		return
	}

	// 2. 校验完整性
	if wantSHA != "" {
		if got := hex.EncodeToString(hasher.Sum(nil)); !strings.EqualFold(got, wantSHA) {
			os.Remove(tmp)
			writeErr(w, 502, fmt.Errorf("SHA256 校验失败（期望 %s，实际 %s）", wantSHA, got))
			return
		}
	}

	// 3. 备份旧版本并原子替换
	backup := exe + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(exe, backup); err != nil {
		os.Remove(tmp)
		writeErr(w, 500, fmt.Errorf("备份旧版本失败: %v", err))
		return
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(backup, exe) // 回滚
		writeErr(w, 500, fmt.Errorf("替换失败（已回滚）: %v", err))
		return
	}

	writeJSON(w, 200, map[string]string{"status": "restarting", "version": m.Version})

	// 4. 延迟原地重启（先让响应发出去）
	go func() {
		time.Sleep(800 * time.Millisecond)
		_ = syscall.Exec(exe, os.Args, os.Environ())
		os.Exit(0) // Exec 失败（如权限受限）时退出，交给 systemd/launchd 拉起来
	}()
}
