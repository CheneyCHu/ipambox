package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/ipambox/ipambox/internal/store"
)

// ---- 极简认证：初始化向导设置管理员密码，登录换取内存 token ----
// 支持两级角色：admin（管理员，全部权限）/ viewer（只读账号，仅查询）。
// 说明：面向内网单机部署，token 存内存、重启失效。

type authManager struct {
	db     *store.Store
	mu     sync.Mutex
	tokens map[string]string // token -> role
}

func newAuthManager(db *store.Store) *authManager {
	return &authManager{db: db, tokens: map[string]string{}}
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
		next.ServeHTTP(w, r)
	})
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
func (a *authManager) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if stored, _ := a.db.GetSetting("password_hash"); stored != "" && stored == hashPassword(body.Password) {
		writeJSON(w, 200, map[string]string{"token": a.issueToken("admin"), "role": "admin"})
		return
	}
	if vh, _ := a.db.GetSetting("viewer_password_hash"); vh != "" && vh == hashPassword(body.Password) {
		writeJSON(w, 200, map[string]string{"token": a.issueToken("viewer"), "role": "viewer"})
		return
	}
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
