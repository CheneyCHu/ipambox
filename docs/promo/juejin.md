# 掘金长文（分类：运维 / 开源项目）

## 标题（三选一）
1. 受够了 Excel 管 IP，我写了个开源的 IP 地址管理盒子，树莓派就能跑
2. 中小企业 IP 管理之痛：从零打造一个轻量级开源 IPAM（附完整源码）
3. 一个二进制搞定全网 IP 可视化管理：IPAMBox 设计与实现

## 正文结构

### 一、痛点
中小企业、实验室、工作室的网络管理现状：
- IP 靠 Excel / 脑子记，冲突了才知道
- phpIPAM 功能强但要 Nginx + PHP + MySQL 全家桶，部署一小时起步
- 路由器/防火墙自带的地址列表只能看"现在连着谁"，没有台账、没有历史、没有告警

### 二、IPAMBox 是什么
一句话：插上网线就能用的 IP 地址管家盒子。
（此处插入 IP 地图截图：docs/screenshots/ip-map.jpg 的仓库 raw 链接或上传掘金图床）

### 三、功能亮点（配截图）
1. **IP 网格地图**：整个网段一页俯瞰，六种状态色块（在线/离线/闲置/冲突/保留/未授权），格子显示 IP 尾数，悬停出详情气泡（截图 ip-map.jpg）
2. **3 步初始化向导**：选网卡自动组网，不需要懂 CIDR
3. **自动发现 + 冲突检测**：ICMP + ARP 交叉验证，MAC 漂移立即告警（截图 alerts.jpg）
4. **设备台账**：负责人/类型字典化管理，CSV 批量导入导出（截图 devices.jpg）
5. **断网续存**：外网中断进入"离线自治"，通知排队恢复补发
6. **AI 助手**：Function Calling 查 IP、问使用率，支持本地 Ollama 数据不出内网
7. **OTA 升级**：页面上点一下就完成升级，SHA-256 校验
8. 中英双语、只读账号、备份恢复……

### 四、技术实现（掘金读者爱看）
- Go 单二进制 + go:embed 嵌入前端，SQLite WAL 免运维
- 六态 IP 状态机设计：free → online → offline，conflict/reserved/rogue 旁路
- 扫描引擎：ICMP 并发 + ARP 兜底（禁 ping 的机器也能发现）
- 前端 Vue3 + Tailwind，网格页 1022 格渲染优化
- 81 项 E2E 自动化测试脚本，CI 四平台交叉编译出包

### 五、30 秒上手
```bash
curl -fsSL https://github.com/CheneyCHu/ipambox/releases/latest/download/install.sh | bash
# 浏览器打开 http://设备IP:18080
```

### 六、与 phpIPAM 的对比表
| | IPAMBox | phpIPAM |
|---|---|---|
| 部署 | 单二进制/一键脚本 | LNMP 全家桶 |
| 硬件 | 树莓派即可 | 建议 2C2G+ |
| 数据库 | 内置 SQLite | 外部 MySQL |
| 可视化 | IP 网格地图 | 表格为主 |
| AI | 内置，可本地化 | 无 |
| 适用 | 中小企业/实验室 | 中大型网络 |

### 七、最后
项目地址：https://github.com/CheneyCHu/ipambox
目前 v1.0.2，路线图里有 SNMP 端口定位、多站点同步。觉得有用欢迎 Star，有想法直接提 Issue。

## 备注
- 掘金支持 Markdown 直接粘贴；封面图建议用 ip-map.jpg 裁 16:9
- 标签：开源、运维、Go、网络、效率工具
