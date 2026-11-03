#!/bin/bash
# 用法：./collect-libs.sh app/bin/r1-toolbox app/lib
set -eu

BIN="${1:?二进制路径}"
DEST="${2:?目标目录}"

mkdir -p "$DEST"

ldd "$BIN" | awk '/=> \//{print $3}' | while read -r so; do
  base=$(basename "$so")

  # glibc 家族与内核/系统版本绑定，绝不能随包分发
  case "$base" in
    libc.so*|libm.so*|libdl.so*|libpthread.so*|librt.so*|libresolv.so*|ld-linux*) continue ;;
  esac

  cp -L "$so" "$DEST/$base"
  echo "  + $base"
done

echo "共 $(ls -1 "$DEST" | wc -l) 个库，合计 $(du -sh "$DEST" | cut -f1)"
