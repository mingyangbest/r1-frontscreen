#!/bin/bash
# 一键出包：编译 → 收依赖 → 收字体 → fnpack build
set -euo pipefail

cd "$(dirname "$0")"
HERE="$(pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
VERSION="${VERSION:-$(grep '^version' manifest | awk -F= '{print $2}' | tr -d ' ')}"

if ! pkg-config --exists sdl2 2>/dev/null; then
  echo "缺少 SDL2 开发文件。两种办法："
  echo "  1) 标准安装：sudo apt-get install libsdl2-dev libsdl2-ttf-dev"
  echo "  2) 系统 libgbm 版本冲突装不上时，私有解包后用："
  echo "     export PKG_CONFIG_PATH=/path/to/sdlroot/usr/lib/x86_64-linux-gnu/pkgconfig"
  exit 1
fi

echo "==> 编译 v$VERSION"
# 必须回到仓库根：go 要在 module 上下文里才能用 -mod=vendor
cd "$ROOT"
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
go build -mod=vendor -trimpath \
  -ldflags "-s -w -X main.version=v$VERSION -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo dev)" \
  -o "$HERE/app/bin/r1-toolbox" ./cmd
cd "$HERE"

echo "==> 收集依赖"
rm -rf app/lib
./collect-libs.sh app/bin/r1-toolbox app/lib

echo "==> 收集中文字体"
mkdir -p app/share/fonts
if [ ! -f app/share/fonts/wqy-microhei.ttc ]; then
  cp /usr/share/fonts/truetype/wqy/wqy-microhei.ttc app/share/fonts/
fi

echo "==> 校验必备文件"
for f in manifest ICON.PNG ICON_256.PNG config/privilege config/resource \
         app/bin/r1-toolbox app/bin/supervisor.sh app/ui/config; do
  [ -e "$f" ] || { echo "缺文件：$f"; exit 1; }
done
python3 -c "import json;[json.load(open(p)) for p in ['config/privilege','config/resource','app/ui/config']]" \
  || { echo "JSON 格式不合法"; exit 1; }

echo "==> 打包"
fnpack build

ls -lh ./*.fpk
echo "完成。本机验证："
echo "  sudo appcenter-cli install-fpk $(pwd)/$(basename "$(ls -1 ./*.fpk | head -1)")"
