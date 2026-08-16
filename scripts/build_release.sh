#!/usr/bin/env bash
# IPAMBox 发布构建：构建前端 → 嵌入后端 → 交叉编译 linux/darwin × amd64/arm64。
# 产物在 dist/，文件名与 install.sh 的下载路径约定一致。
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-1.0.0}"
echo "==> 构建前端"
(cd frontend && npx vite build)
rm -rf backend/internal/api/web/*
cp -r frontend/dist/* backend/internal/api/web/

mkdir -p dist
# SQLite 驱动依赖 CGO；本机有 zig 时用它做跨平台 C 工具链，否则仅本机平台能成功
ZIG="$(command -v zig || true)"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  GOOS="${target%/*}"; GOARCH="${target#*/}"
  OUT="dist/ipambox_${GOOS}_${GOARCH}"
  echo "==> $GOOS/$GOARCH → $OUT"
  CC_ARGS=()
  if [[ -n "$ZIG" && "$GOOS" != "$(go env GOOS)" ]]; then
    ZT="${GOARCH}-$GOOS"   # amd64-linux / arm64-macos
    [[ "$GOOS" == "darwin" ]] && ZT="${GOARCH}-macos"
    CC_ARGS=(CC="$ZIG cc -target $ZT" CXX="$ZIG c++ -target $ZT")
  fi
  (cd backend && env CGO_ENABLED=1 GOOS="$GOOS" GOARCH="$GOARCH" ${CC_ARGS[@]+"${CC_ARGS[@]}"} \
    go build -trimpath -ldflags "-s -w -X github.com/ipambox/ipambox/internal/build.Version=$VERSION" \
    -o "../$OUT" ./cmd/server) || echo "   ⚠ $target 构建失败（安装 zig 可跨平台构建：brew install zig）"
done
echo "==> 完成。产物："
ls -la dist/ | grep ipambox_ || true
