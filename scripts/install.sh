#!/usr/bin/env bash
# IPAMBox 一键安装脚本
# 用法：
#   在线安装：  curl -fsSL <发布地址>/install.sh | bash
#   本地安装：  bash install.sh --local /path/to/ipambox
# 选项：
#   --dir <安装目录>   默认 ~/ipambox（root 且 Linux 时默认 /opt/ipambox）
#   --port <端口>      默认 18080
#   --no-service       只放文件，不注册开机自启服务
#   --local <二进制>   不从网络下载，直接使用本地二进制
set -euo pipefail

APP=ipambox
PORT=18080
DIR=""
NO_SERVICE=0
LOCAL_BIN=""
DOWNLOAD_BASE="${IPAMBOX_DOWNLOAD_BASE:-https://raw.githubusercontent.com/ipambox/ipambox/main/release}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) DIR="$2"; shift 2;;
    --port) PORT="$2"; shift 2;;
    --no-service) NO_SERVICE=1; shift;;
    --local) LOCAL_BIN="$2"; shift 2;;
    *) echo "未知参数: $1" >&2; exit 1;;
  esac
done

OS="$(uname -s | tr 'A-Z' 'a-z')"   # linux / darwin
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64;;
  arm64|aarch64) ARCH=arm64;;
  *) echo "不支持的架构: $ARCH" >&2; exit 1;;
esac

# 默认安装目录：Linux + root → /opt/ipambox；否则 ~/ipambox
if [[ -z "$DIR" ]]; then
  if [[ "$OS" == "linux" && "$(id -u)" == "0" ]]; then DIR=/opt/ipambox; else DIR="$HOME/ipambox"; fi
fi

echo "==> 安装 IPAMBox ($OS/$ARCH) 到 $DIR"
mkdir -p "$DIR"

# 1. 获取二进制
if [[ -n "$LOCAL_BIN" ]]; then
  cp "$LOCAL_BIN" "$DIR/ipambox"
else
  URL="$DOWNLOAD_BASE/ipambox_${OS}_${ARCH}"
  echo "==> 下载 $URL"
  curl -fsSL "$URL" -o "$DIR/ipambox"
fi
chmod +x "$DIR/ipambox"

VER="$("$DIR/ipambox" -version 2>/dev/null || echo unknown)"
echo "==> 版本: $VER"

# 2. 注册开机自启服务（尽力而为，失败则给出手动启动方式）
SERVICE_OK=0
if [[ "$NO_SERVICE" == "0" ]]; then
  if [[ "$OS" == "linux" ]] && command -v systemctl >/dev/null 2>&1 && [[ "$(id -u)" == "0" ]]; then
    cat > /etc/systemd/system/ipambox.service <<EOF
[Unit]
Description=IPAMBox IP Address Manager
After=network-online.target
Wants=network-online.target

[Service]
WorkingDirectory=$DIR
Environment=IPAMBOX_PORT=$PORT
ExecStart=$DIR/ipambox
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable --now ipambox
    SERVICE_OK=1
    echo "==> 已注册 systemd 服务并启动"
  elif [[ "$OS" == "darwin" ]]; then
    PLIST="$HOME/Library/LaunchAgents/com.ipambox.server.plist"
    cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.ipambox.server</string>
  <key>ProgramArguments</key><array><string>$DIR/ipambox</string></array>
  <key>WorkingDirectory</key><string>$DIR</string>
  <key>EnvironmentVariables</key><dict><key>IPAMBOX_PORT</key><string>$PORT</string></dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
</dict></plist>
EOF
    launchctl unload "$PLIST" 2>/dev/null || true
    launchctl load "$PLIST"
    SERVICE_OK=1
    echo "==> 已注册 launchd 服务并启动"
  fi
fi

cat <<EOF

✅ 安装完成！

  访问地址:  http://$(hostname -I 2>/dev/null | awk '{print $1}' || echo localhost):$PORT
  数据目录:  $DIR/data
EOF

if [[ "$SERVICE_OK" == "0" ]]; then
  cat <<EOF
  启动方式:  cd $DIR && IPAMBOX_PORT=$PORT ./ipambox
  （可用 nohup 或 screen 保持后台运行；Linux root 用户可省去 --no-service 自动注册 systemd）
EOF
else
  cat <<EOF
  服务管理:  $([ "$OS" == "linux" ] && echo "systemctl status|restart|stop ipambox" || echo "launchctl unload/load ~/Library/LaunchAgents/com.ipambox.server.plist")
EOF
fi
echo
