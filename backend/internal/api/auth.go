package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipambox/ipambox/internal/store"
)

// ---- 极简认证：初始化向导设置管理员密码，登录换取内存 token ----
// 支持两级角色：admin（管理员，全部权限）/ viewer（只读账号，仅查询）。
// 说明：面向内网单机部署，token 存内存、重启失效。

type authManager struct {
	db     *store.Store
	mu     sync.Mutex
	tokens map[string]string // token -> role

	// 登录爆破防护：按客户端 IP 记录连续失败次数与锁定截止时间
	failMu   sync.Mutex
	fails    map[string]int
	lockedTo map[string]time.Time
}

func newAuthManager(db *store.Store) *authManager {
	return &authManager{db: db, tokens: map[string]string{}, fails: map[string]int{}, lockedTo: map[string]time.Time{}}
}

// loginThrottle 登录频率限制：连续失败 5 次锁定 60 秒，之后每次失败翻倍（上限 15 分钟）。
func (a *authManager) loginThrottle(ip string) (locked bool, retryAfter int) {
	a.failMu.Lock()
	defer a.failMu.Unlock()
	if until, ok := a.lockedTo[ip]; ok {
		if d := time.Until(until); d > 0 {
			return true, int(d.Seconds()) + 1
		}
		delete(a.lockedTo, ip)
	}
	return false, 0
}

func (a *authManager) loginFailed(ip string) {
	a.failMu.Lock()
	defer a.failMu.Unlock()
	a.fails[ip]++
	if n := a.fails[ip]; n >= 5 {
		// 锁定 60s << (n-5) 次方级数，封顶 15min
		d := 60 << min(n-5, 4)
		if d > 900 {
			d = 900
		}
		a.lockedTo[ip] = time.Now().Add(time.Duration(d) * time.Second)
	}
}

func (a *authManager) loginSucceeded(ip string) {
	a.failMu.Lock()
	defer a.failMu.Unlock()
	delete(a.fails, ip)
	delete(a.lockedTo, ip)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *authManager) initialized() bool {
	v, _ := a.db.GetSetting("password_hash")
	return v != ""
}

func hashPassword(pw string) string {
	sum := sha256.Sum256([]byte("ipambox:" + pw))
	return hex.EncodeToString(sum[:])
}

func (a *authManager) issueToken(role string) string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	tok := hex.EncodeToString(buf)
	a.mu.Lock()
	a.tokens[tok] = role
	a.mu.Unlock()
	return tok
}

// roleOf 返回 token 对应角色；无效 token 返回空串。
func (a *authManager) roleOf(tok string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tokens[tok]
}

// middleware 保护 /api：未初始化时仅放行 setup；已初始化则校验 Bearer token。
// 只读账号（viewer）仅放行 GET/HEAD，写操作一律 403。
func (a *authManager) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 匿名放行仅限 状态查询 / 初始化 / 登录；其余 /setup/*（如 reset）仍需登录
		p := r.URL.Path
		if p == "/api/v1/setup/status" || p == "/api/v1/setup/init" || p == "/api/v1/setup/login" {
			next.ServeHTTP(w, r)
			return
		}
		if !a.initialized() {
			writeJSON(w, http.StatusPreconditionFailed, map[string]string{
				"error": "not_initialized", "hint": "请先完成初始化向导"})
			return
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		role := a.roleOf(tok)
		if tok == "" || role == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if role == "viewer" && r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "只读账号无此操作权限"})
			return
		}
		// 角色写入上下文，供 handler 做差异化输出（如对 viewer 隐藏敏感配置）
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), roleCtxKey, role)))
	})
}

type ctxKey int

const roleCtxKey ctxKey = iota

// roleFrom 取当前请求角色（中间件已保证非空）。
func roleFrom(r *http.Request) string {
	v, _ := r.Context().Value(roleCtxKey).(string)
	return v
}

// SetupStatus 返回是否已初始化。
func (a *authManager) SetupStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]bool{"initialized": a.initialized()})
}

// SetupInit 首次设置管理员密码，返回登录 token。
func (a *authManager) SetupInit(w http.ResponseWriter, r *http.Request) {
	if a.initialized() {
		writeErr(w, 409, errString("already initialized"))
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Password) < 6 {
		writeErr(w, 400, errString("密码至少 6 位"))
		return
	}
	if err := a.db.SetSetting("password_hash", hashPassword(body.Password)); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"token": a.issueToken("admin"), "role": "admin"})
}

// Login 校验密码并返回 token。管理员密码 → admin；只读密码（若已设置）→ viewer。
// 带爆破防护：同一 IP 连续失败 5 次后锁定（60 秒起，逐级翻倍，封顶 15 分钟）。
func (a *authManager) Login(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	if locked, wait := a.loginThrottle(ip); locked {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		writeErr(w, http.StatusTooManyRequests, errString("尝试次数过多，请稍后再试"))
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if stored, _ := a.db.GetSetting("password_hash"); stored != "" && stored == hashPassword(body.Password) {
		a.loginSucceeded(ip)
		writeJSON(w, 200, map[string]string{"token": a.issueToken("admin"), "role": "admin"})
		return
	}
	if vh, _ := a.db.GetSetting("viewer_password_hash"); vh != "" && vh == hashPassword(body.Password) {
		a.loginSucceeded(ip)
		writeJSON(w, 200, map[string]string{"token": a.issueToken("viewer"), "role": "viewer"})
		return
	}
	a.loginFailed(ip)
	writeErr(w, 401, errString("密码错误"))
}

// ViewerStatus 返回是否已设置只读账号（不泄露哈希）。
func (a *authManager) ViewerStatus(w http.ResponseWriter, _ *http.Request) {
	vh, _ := a.db.GetSetting("viewer_password_hash")
	writeJSON(w, 200, map[string]bool{"enabled": vh != ""})
}

// Me 返回当前 token 的角色。
func (a *authManager) Me(w http.ResponseWriter, r *http.Request) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	writeJSON(w, 200, map[string]string{"role": a.roleOf(tok)})
}

// SetViewerPassword 设置/停用只读账号密码（仅管理员；中间件已挡 viewer）。
func (a *authManager) SetViewerPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"` // 空 = 停用只读账号
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if body.Password == "" {
		if err := a.db.SetSetting("viewer_password_hash", ""); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]string{"status": "disabled"})
		return
	}
	if len(body.Password) < 6 {
		writeErr(w, 400, errString("密码至少 6 位"))
		return
	}
	if adminHash, _ := a.db.GetSetting("password_hash"); adminHash == hashPassword(body.Password) {
		writeErr(w, 400, errString("只读密码不能与管理员密码相同"))
		return
	}
	if err := a.db.SetSetting("viewer_password_hash", hashPassword(body.Password)); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "enabled"})
}

type strErr string

func (e strErr) Error() string { return string(e) }
func errString(s string) error { return strErr(s) }
