// Package api 提供 REST API 与 SPA 静态资源服务。
package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ipambox/ipambox/internal/scanner"
	"github.com/ipambox/ipambox/internal/store"
	"github.com/ipambox/ipambox/internal/uplink"
)

// NewRouter 装配全部路由。
func NewRouter(db *store.Store, engine *scanner.Engine, mon *uplink.Monitor) http.Handler {
	h := &handlers{db: db, engine: engine, uplink: mon}
	auth := newAuthManager(db)

	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer, middleware.Timeout(30*time.Second))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.middleware)

		r.Route("/setup", func(r chi.Router) {
			r.Get("/status", auth.SetupStatus)
			r.Post("/init", auth.SetupInit)
			r.Post("/login", auth.Login)
			r.Post("/reset", h.SetupReset) // 清除数据并重新初始化（需已登录）
		})

		r.Route("/auth", func(r chi.Router) {
			r.Get("/me", auth.Me)                             // 当前角色
			r.Get("/viewer", auth.ViewerStatus)               // 只读账号是否启用
			r.Post("/viewer", auth.SetViewerPassword)         // 设置/停用只读账号（viewer 已被中间件拦截）
		})

		r.Get("/stats/overview", h.Overview)
		r.Get("/interfaces", h.ListInterfaces)

		r.Route("/uplink", func(r chi.Router) {
			r.Get("/", h.UplinkStatus)        // 当前连通状态 + 待补发数 + 最近事件
			r.Get("/events", h.UplinkEvents)  // 状态变化历史
			r.Post("/check", h.UplinkCheck)   // 手动探测（恢复时顺带补发）
		})

		r.Route("/network/interfaces", func(r chi.Router) {
			r.Get("/", h.ListNetworkInterfaces)
			r.Post("/{name}/config", h.ConfigureInterface)
		})

		r.Route("/network/routes", func(r chi.Router) {
			r.Get("/", h.ListRoutes)
			r.Post("/", h.AddRoute)
			r.Delete("/", h.DeleteRoute)
		})

		r.Route("/subnets", func(r chi.Router) {
			r.Get("/", h.ListSubnets)
			r.Post("/", h.CreateSubnet)
			r.Put("/{id}", h.UpdateSubnet)
			r.Delete("/{id}", h.DeleteSubnet)
			r.Get("/{id}/addresses", h.ListAddresses)
			r.Post("/{id}/addresses", h.CreateAddress)
			r.Get("/{id}/stats", h.SubnetStats)
			r.Post("/{id}/scan", h.ScanNow)
			r.Get("/{id}/export", h.ExportSubnetCSV)
			r.Post("/{id}/import", h.ImportCSV)
		})

		r.Get("/devices", h.ListDevices)

		r.Route("/addresses", func(r chi.Router) {
			r.Patch("/{id}", h.UpdateAnnotation) // 人工标注
			r.Delete("/{id}", h.DeleteAddress)   // 删除台账记录
		})

		r.Route("/settings", func(r chi.Router) {
			r.Get("/", h.GetSettings)
			r.Put("/", h.SaveSettings)
			r.Post("/dict/rename", h.RenameDictItem)
		})
		r.Post("/notify/test", h.NotifyTest)

		r.Get("/version", h.VersionInfo)
		r.Route("/update", func(r chi.Router) {
			r.Get("/check", h.UpdateCheck)   // 检查新版本
			r.Post("/apply", h.UpdateApply)  // 下载并原地升级重启（仅管理员）
		})

		r.Route("/backup", func(r chi.Router) {
			r.Get("/export", h.BackupExport)
			r.Post("/import", h.BackupImport)
		})

		r.Route("/alerts", func(r chi.Router) {
			r.Get("/", h.ListAlerts)
			r.Post("/{id}/read", h.MarkAlertRead)
		})

		r.Route("/ai", func(r chi.Router) {
			r.Post("/chat", h.AIChat)
			r.Post("/config", h.SaveAIConfig)
			r.Post("/test", h.AITest)
		})
	})

	// SPA 回退：非 /api 路径交给内嵌前端
	r.Handle("/*", spaHandler())
	return r
}
