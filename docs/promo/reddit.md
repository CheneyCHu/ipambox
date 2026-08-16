# Reddit r/selfhosted 发帖文案

## Title
IPAMBox – a lightweight self-hosted IPAM that runs on a Raspberry Pi: single binary, grid map of every IP, works offline

## Body
Hey r/selfhosted,

I built **IPAMBox**, an open-source IP address manager for small/mid-sized networks. The motivation: phpIPAM is powerful but needs a full LEMP stack, and router DHCP tables don't give you history, inventory or alerting.

What it does:

- **Single binary** — the whole UI is embedded (Go + `go:embed` + SQLite WAL). One-line installer, runs fine on a Pi.
- **IP grid map** — every address of a /24 on one screen, color-coded: online / offline / free / conflict / reserved / rogue. (screenshot in README)
- **Discovery loop** — concurrent ICMP + ARP fallback, automatic offline demotion, MAC-drift conflict alerts.
- **Offline autonomy** — when the uplink is down, scanning/inventory keep working; notifications queue and resend after recovery. Built for the "box in the corner of the server room" scenario.
- **AI assistant** — natural-language queries ("who is using 192.168.1.50?") via any OpenAI-compatible API or **local Ollama**, so nothing leaves your LAN.
- **OTA updates** from the settings page (SHA-256 verified).
- Webhook / DingTalk / WeCom notifications, read-only account for managers, CSV import/export, bilingual UI (CN/EN).

One-liner install:
```
curl -fsSL https://github.com/CheneyCHu/ipambox/releases/latest/download/install.sh | bash
```

GitHub: https://github.com/CheneyCHu/ipambox

It's v1.0.2 — tested mostly on macOS so far, so I'd really appreciate feedback from Linux/ARM folks. Happy to answer questions!

## 备注
- r/selfhosted 规则要求项目可自托管且说明清楚，正文里避免 "revolutionary" 之类词
- 可以同步发 r/homelab（需要一定账号 karma）
- 回复评论时用平常心，老外会挑技术细节（扫描准确性、权限需求）
