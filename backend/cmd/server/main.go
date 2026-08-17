// Command server 是 IPAMBox 的单一入口：内嵌前端静态资源，启动 HTTP 服务与定时扫描。
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ipambox/ipambox/internal/alert"
	"github.com/ipambox/ipambox/internal/api"
	"github.com/ipambox/ipambox/internal/build"
	"github.com/ipambox/ipambox/internal/oui"
	"github.com/ipambox/ipambox/internal/scanner"
	"github.com/ipambox/ipambox/internal/store"
	"github.com/ipambox/ipambox/internal/uplink"
)

func main() {
	// -version 打印版本号（安装脚本与运维排障用）
	for _, a := range os.Args[1:] {
		if a == "-version" || a == "--version" || a == "version" {
			fmt.Println(build.Version)
			return
		}
	}

	// 1. 打开 SQLite（WAL 模式），首次运行自动建表
	db, err := store.Open("data/ipambox.db")
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// 2. 告警器与扫描引擎
	alerter := alert.New(db)
	engine := scanner.New(db, alerter)

	// 2.1 启动回填：为存量数据补齐 OUI 厂商与推断设备类型
	if missing, err := db.ListMissingVendor(); err == nil && len(missing) > 0 {
		n := 0
		for id, mac := range missing {
			if v := oui.Lookup(mac); v != "" {
				if db.SetVendorType(id, v, oui.InferType(mac)) == nil {
					n++
				}
			}
		}
		log.Printf("oui: 回填 %d/%d 条地址的厂商信息", n, len(missing))
	}

	// 3. 外网连通监测（断网续存/边缘自治）：离线时通知入队，恢复后自动补发
	mon := uplink.New(db, alerter)
	go mon.Run()

	// 4. 定时扫描（默认 5 分钟；设置页可配，MVP 固定）
	go engine.RunScheduler(5 * time.Minute)

	// 5. HTTP 服务：/api/* 为 REST，其余路径回退到内嵌 SPA
	port := os.Getenv("IPAMBOX_PORT")
	if port == "" {
		port = "18080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           api.NewRouter(db, engine, mon),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("IPAMBox listening on http://0.0.0.0:%s", port)
	log.Fatal(srv.ListenAndServe())
}
