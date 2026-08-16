# IPAMBox

> A plug-and-play IP address manager box — a lightweight IPAM for small and mid-sized networks.

[中文文档](README.md)

IPAMBox runs as a **single binary** on low-cost hardware (Raspberry Pi, mini PC, any Linux/macOS box). It discovers devices on your LAN, visualizes every IP on a live grid map, detects conflicts and rogue devices, keeps an inventory, pushes alerts — and can answer questions in natural language through a built-in AI assistant.

## Features

- **First-run wizard**: 3 steps — set password → pick a local NIC to auto-create its subnet → start scanning. Re-runnable later from Settings (with data-wipe confirmation).
- **Discovery loop**: concurrent ICMP scan + ARP fallback → six-state IP state machine (online / offline / free / conflict / reserved / rogue) with automatic offline demotion; MAC-drift conflict detection; scheduled + on-demand scans.
- **IP grid map**: full-segment rendering (every cell visible for /22 and smaller, incl. free addresses), search & filter, grid/list views, detail drawer with labels, owner and device type.
- **Subnet management**: add / edit / delete subnets, bind subnets to physical NICs, VLAN and description fields, per-subnet usage bars.
- **Device inventory**: cross-subnet device table, status filters, one-click "unregistered devices" triage, manual registration of reserved addresses, edit / delete.
- **Network settings**: view physical NICs (real hardware ports only), configure IPv4/IPv6 static or DHCP with confirmation; routing table viewer with static route add/delete.
- **Reports**: subnet usage bars (>85% turns red) + Excel-compatible CSV import/export for bulk labeling.
- **Alerting**: conflict / offline events, mark-as-read, and push notifications via generic Webhook / DingTalk / WeCom bots.
- **Offline autonomy**: uplink probing; when the internet is down, local scanning/inventory/alerting keep working and notifications are queued and resent after recovery.
- **Read-only account**: a separate view-only password for managers/customers; all write operations are blocked.
- **Backup & restore**: one-click export/import of the entire database (subnets, inventory, alerts, settings).
- **OTA updates**: in-app "check for updates" with SHA-256-verified binary replacement and automatic restart.
- **AI assistant**: OpenAI-compatible gateway (cloud APIs or local Ollama), Function Calling tools (query_ip / get_subnet_usage / list_conflicts).
- **Bilingual UI**: Chinese / English, switchable in the sidebar.
- **Single-binary delivery**: the frontend is embedded into the Go backend via `go:embed`; cross-compiled builds for Linux x86_64 / ARM64 and macOS are published on the Releases page.

## Quick start

```bash
# Download a prebuilt binary from Releases, or build from source:
cd frontend && npm install && npm run build
rm -rf ../backend/internal/api/web/* && cp -r dist/* ../backend/internal/api/web/
cd ../backend && go mod tidy && go build -o ipambox ./cmd/server
./ipambox          # listens on http://0.0.0.0:18080 (override with IPAMBOX_PORT)

# NIC address / route changes require root:
sudo ./ipambox
```

Development mode:

```bash
cd backend && go run ./cmd/server        # backend :18080
cd frontend && npm run dev               # frontend with hot reload, /api proxied
```

## Project layout

```
ipambox/
├── docs/UI设计稿.md        # MVP UI design (wireframes)
├── prototype/index.html    # High-fidelity interactive prototype
├── backend/                # Go single-binary backend (embeds frontend, port 18080)
│   ├── cmd/server/         #   Entrypoint: wires store/scanner/alert/api
│   └── internal/
│       ├── models/         #   Domain models (six-state IP state machine)
│       ├── store/          #   SQLite persistence (WAL, zero-maintenance, settings KV)
│       ├── scanner/        #   Multi-source discovery engine (ICMP+ARP)
│       ├── alert/          #   MAC-drift conflict detection & notifications
│       ├── ai/             #   OpenAI-compatible gateway + function calling loop
│       └── api/            #   REST API + auth + embedded SPA (web/ = frontend build)
└── frontend/               # Vue 3 + Vite + Tailwind
    └── src/views/          #   Wizard / Login / Dashboard / IP grid / Alerts / Settings + AI widget
```

## Testing

```bash
bash scripts/e2e_test.sh    # automated E2E walkthrough (isolated port & data dir)
```

## Releasing (OTA channel)

```bash
git tag vX.Y.Z && git push origin vX.Y.Z
```

GitHub Actions builds binaries for darwin/amd64, darwin/arm64, linux/amd64 and linux/arm64, publishes the Release, and writes `release/latest.json` (with SHA-256 digests) back to `main`. Devices check that manifest from Settings → System Update.

## Roadmap

- [x] Code skeleton / UI design
- [x] M1: scan loop + wizard + auth + AI gateway + single binary
- [x] M2: offline detection, inventory, full grid, CSV import/export, reports, install/package scripts, UI polish
- [x] M2.5/2.6: NIC-bound subnets, network settings page, routes page, subnet management, grid/list toggle, usage fixed to segment capacity
- [x] M3 (partial): DingTalk/WeCom/Webhook notifications, read-only account, backup/restore, OTA updates, offline autonomy, bilingual UI
- [ ] M4: AI write operations, plugin mechanism, NetBox/phpIPAM sync, multi-site
