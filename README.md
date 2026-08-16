# IPAMBox

> 插上网线就能用的 IP 地址管家盒子 —— 面向中小企业的轻量级 IPAM。

## 目录结构

```
ipambox/
├── docs/UI设计稿.md        # MVP 界面设计稿（含线框图）
├── prototype/index.html    # 高保真交互原型（浏览器直接打开）
├── backend/                # Go 单二进制后端（内嵌前端，端口 18080）
│   ├── cmd/server/         #   入口：装配 store/scanner/alert/api
│   └── internal/
│       ├── models/         #   领域模型（六态 IP 状态机）
│       ├── store/          #   SQLite 持久层（WAL，免运维，含 settings KV）
│       ├── scanner/        #   多源发现引擎（ICMP+ARP，DHCP/SNMP 预留）
│       ├── alert/          #   MAC 漂移冲突检测与通知
│       ├── ai/             #   OpenAI 兼容网关 + Function Calling 循环
│       └── api/            #   REST API + 认证 + 内嵌 SPA（web/ 为前端产物）
└── frontend/               # Vue 3 + Vite + Tailwind 前端
    └── src/views/          #   向导/登录/仪表盘/IP网格/告警/设置 + AI助手浮窗
```

## 已实现功能（M2 完成）

- **首次启动向导**：3 步（设密码 → **点选本机网卡自动组网** → 开始扫描），未初始化时 API 返回 428 引导；设置页可「重新初始化」（数据统计 + 二次确认 + 清除）
- **认证**：管理员密码（SHA-256）+ 内存 token，Bearer 校验
- **自动发现闭环**：ICMP 并发扫描 + ARP 兜底 → 状态机更新（含**离线判定**：上轮在线本轮消失自动降级）→ MAC 漂移冲突告警；定时 5 分钟 + 手动触发
- **IP 网格地图**：全量网段渲染（/22 内 254~1022 格全展示，闲置格也可见）、搜索过滤、遮罩式详情抽屉、扫描后轮询刷新
- **设备台账**：跨子网全设备表格、状态过滤、"未登记设备"一键排查
- **报表**：子网使用率进度条（>85% 变红）+ Excel 兼容 CSV 导入导出（批量标注）
- **仪表盘**：总览统计 + 各子网使用率 + 未读告警
- **AI 助手**：OpenAI 兼容协议（云端/本地 Ollama 通用），Function Calling 三工具（query_ip / get_subnet_usage / list_conflicts），设置页可配可测连通
- **单二进制交付**：前端 go:embed 进后端；`scripts/install.sh`（systemd）+ `scripts/package.sh`（ARM/x86 交叉编译 tar.gz）+ `scripts/e2e_test.sh`（30 项自动化走查）

## 快速开始

```bash
# 开发模式
cd backend && go mod tidy && go run ./cmd/server   # 后端 :18080
cd frontend && npm install && npm run dev          # 前端热更新，/api 已代理

# 单二进制（生产/盒子）
cd frontend && npm run build
rm -rf ../backend/internal/api/web/* && cp -r dist/* ../backend/internal/api/web/
cd ../backend && go build -o ipambox ./cmd/server && ./ipambox

# ARM 交叉编译
GOOS=linux GOARCH=arm64 go build -o ipambox-arm64 ./cmd/server
```

## 测试

```bash
bash scripts/e2e_test.sh        # 46 项自动化 E2E 走查（独立端口+独立数据目录）
```

2026-08-10 晚：4 轮全量测试 30/30 通过 + AI Function Calling 闭环回归 + 真实局域网扫描验证（详见 git 历史/会话记录）。

2026-08-11 晚：M2.5 完成，35/35 通过（新增网络设置接口 5 项用例：网卡详情枚举、MAC 字段、虚拟接口拒绝、非法 IP 400、子网 iface 字段）。修改网卡 IP 的真实执行路径（ifconfig/dhclient）需要 root 且有断网风险，本机只验证了校验与拒绝路径，Linux 分支未实测。

## 路线图

- [x] 代码骨架 / UI 设计稿
- [x] M1：扫描闭环 + 向导 + 认证 + AI 网关 + 单二进制
- [x] M2：离线判定、设备台账、全量网格、CSV 导入导出、报表、安装/打包脚本、UI 全面美化
- [x] M2.5：IP 格子缩小显示尾数；子网绑定接入网卡；向导内修改网卡 IP；「网络设置」页（物理网卡 IPv4/IPv6 静态/DHCP，校验失败返回 400，虚拟接口拒绝操作）
- [x] M2.6：MAC 跨平台获取（macOS `arp -a`）+ 反向 DNS 主机名；网格正方形化；详情时间本地化；**子网管理页**（增删改查）；IP 地图网格/列表切换；网卡按真实硬件端口过滤 + DHCP 状态回显；**使用率按网段容量修正**（修复始终 100%）；设备台账编辑/删除/手工登记保留地址；设置页扫描计划可配（间隔 + 开关，引擎动态读取）
- [ ] M3：V1.5（DHCP 租约、SNMP 端口定位、rogue 检测、钉钉/企微、Webhook 通知）
- [ ] M4：V2.0（AI 写操作、插件机制、NetBox/phpIPAM 同步、多站点）
