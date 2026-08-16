#!/usr/bin/env bash
# IPAMBox 打包脚本：构建前端 → 嵌入 → 交叉编译 ARM/x86 Linux 二进制 → tar.gz
# 用法：bash scripts/package.sh [版本号]
set -euo pipefail

VERSION="${1:-0.2.0}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$ROOT/dist"
mkdir -p "$OUT_DIR"

echo "==> 构建前端"
cd "$ROOT/frontend"
npm run build

echo "==> 嵌入前端产物"
rm -rf "$ROOT/backend/internal/api/web/*" || true
mkdir -p "$ROOT/backend/internal/api/web"
cp -r dist/* "$ROOT/backend/internal/api/web/"

cd "$ROOT/backend"
for TARGET in "linux/arm64" "linux/amd64" "linux/arm"; do
  GOOS="${TARGET%/*}"; GOARCH="${TARGET#*/}"
  PKG="ipambox_${VERSION}_${GOOS}_${GOARCH}"
  echo "==> 交叉编译 $TARGET"
  GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=1 go build -ldflags "-s -w" -o "$OUT_DIR/$PKG/ipambox" ./cmd/server \
    || { echo "（跳过 $TARGET：CGO 交叉编译需要对应工具链）"; rm -rf "$OUT_DIR/$PKG"; continue; }
  cp "$ROOT/scripts/install.sh" "$OUT_DIR/$PKG/"
  cat > "$OUT_DIR/$PKG/README.txt" <<EOF
IPAMBox $VERSION ($GOOS/$GOARCH)
解压后运行: sudo bash install.sh
或手动:     mkdir -p data && ./ipambox   # 访问 http://<IP>:18080
EOF
  tar -C "$OUT_DIR" -czf "$OUT_DIR/${PKG}.tar.gz" "$PKG"
  rm -rf "$OUT_DIR/$PKG"
  echo "    产物: $OUT_DIR/${PKG}.tar.gz"
done
echo "==> 全部完成"
