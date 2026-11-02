#!/usr/bin/env bash
#
# R1 FrontScreen 安装 / 升级脚本
#
# 默认行为：装依赖（运行时库，不含编译器）→ 取 Release 预编译二进制 →
#          备份并不覆盖已有配置 → 注册 systemd → 启动 → 自检。
#          整个过程不提问，可以挂着走开。
#
#   sudo ./install.sh                  # 装最新版
#   sudo ./install.sh --port 9000      # 指定面板端口（仅首次生成配置时生效）
#   sudo ./install.sh --version v0.9.6 # 装指定版本（默认 latest）
#   sudo ./install.sh --binary ./r1-toolbox-linux-amd64   # 离线：用本地包
#   sudo ./install.sh --from-source    # 从源码编译（开发者 / arm64）
#   sudo ./install.sh --force          # 跳过硬件检查（无屏环境 / Docker）
#
# 为什么不再默认编译：R1 是 N100 小机器，apt 装 Go + SDL 开发库再编译要几分钟，
# 网络一抖就失败；而 Release 里本来就有编好的 amd64 包。编译只留给改代码的人。

set -euo pipefail

REPO="mingyangbest/r1-frontscreen"
BIN="/usr/local/bin/r1-toolbox"
UNIT="/etc/systemd/system/r1-toolbox.service"
CONFIG_DIR="${R1TB_CONFIG_DIR:-/etc/r1-toolbox}"
CONFIG="$CONFIG_DIR/config.yaml"
BACKUP_DIR="/var/backups/r1-toolbox"
ASSET="r1-toolbox-linux-amd64"

# 运行依赖：SDL2 运行时（不是 -dev）、CJK 字体、传感器与 SMART 工具
RUNTIME_PKGS=(libsdl2-2.0-0 libsdl2-ttf-2.0-0 fonts-wqy-zenhei lm-sensors smartmontools i2c-tools)

VERSION="latest"
PORT="8080"
FROM_SOURCE=0
BINARY=""
FORCE=0
FIX_GRUB=0
SKIP_DEPS=0

# ---------------------------------------------------------------- 输出

c_ok()   { printf '\033[32m✓\033[0m %s\n' "$*"; }
c_warn() { printf '\033[33m!\033[0m %s\n' "$*"; }
c_err()  { printf '\033[31m✗\033[0m %s\n' "$*" >&2; }
die()    { c_err "$*"; exit 1; }
step()   { printf '\n\033[1m%s\033[0m\n' "$*"; }

usage() {
  cat <<'USAGE'
R1 FrontScreen 安装 / 升级脚本（海康 R1 / R1X，fnOS 或 Debian）

  sudo ./install.sh                  装最新版（不提问）
  sudo ./install.sh --port 9000      指定面板端口（仅首次生成配置时生效）
  sudo ./install.sh --version v0.9.6 装指定版本
  sudo ./install.sh --binary ./r1-toolbox-linux-amd64
                                     离线：用本地包，不联网下载
  sudo ./install.sh --from-source    从源码编译（开发者 / ARM 主板）
  sudo ./install.sh --force          跳过硬件检查（无屏环境）
  sudo ./install.sh --fix-grub       移除 GRUB 里的关屏参数（会改引导配置）
  sudo ./install.sh --skip-deps      不装依赖（自己管）

默认走 Release 预编译包：只装 SDL2 运行时，不装 Go、不编译。
已有配置不会被覆盖，会先备份到 /var/backups/r1-toolbox/。
USAGE
  exit 0
}

# ---------------------------------------------------------------- 参数

while [ $# -gt 0 ]; do
  case "$1" in
    -y|--yes)         shift ;;                 # 兼容旧习惯：脚本本来就非交互
    --port)           PORT="${2:?--port 需要值}"; shift 2 ;;
    --port=*)         PORT="${1#*=}"; shift ;;
    --version)        VERSION="${2:?--version 需要值}"; shift 2 ;;
    --version=*)      VERSION="${1#*=}"; shift ;;
    --binary)         BINARY="${2:?--binary 需要路径}"; shift 2 ;;
    --binary=*)       BINARY="${1#*=}"; shift ;;
    --from-source)    FROM_SOURCE=1; shift ;;
    --skip-deps)      SKIP_DEPS=1; shift ;;
    --fix-grub)       FIX_GRUB=1; shift ;;
    --force)          FORCE=1; shift ;;
    -h|--help)        usage ;;
    *)                die "不认识的参数：$1（--help 看用法）" ;;
  esac
done

[ "$(id -u)" -eq 0 ] || die "需要 root：sudo ./install.sh"
[ -d /run/systemd/system ] || die "没有 systemd，这个脚本只适用于 fnOS / Debian 系"

# ---------------------------------------------------------------- 前置检查

step "1/6 检查这台机器"

arch="$(uname -m)"
if [ "$FROM_SOURCE" -eq 0 ] && [ -z "$BINARY" ] && [ "$arch" != "x86_64" ]; then
  die "预编译包只有 x86_64（海康 R1 / R1X 是 Intel N100）。当前架构 $arch，
    ARM 机器请用 --from-source 从源码编译。"
fi
c_ok "架构 $arch"

product="$(cat /sys/class/dmi/id/product_name 2>/dev/null || echo '未知')"
if printf '%s' "$product" | grep -qiE 'R1'; then
  c_ok "机型 $product"
else
  c_warn "机型是「$product」，不像海康 R1 / R1X —— 屏幕相关功能大概率不可用"
  [ "$FORCE" -eq 1 ] || die "确认仍要安装请加 --force"
fi

if [ -e /dev/fb0 ] && compgen -G '/dev/dri/card*' >/dev/null; then
  c_ok "显示设备 /dev/fb0、/dev/dri/card* 就位"
else
  c_warn "找不到 /dev/fb0 或 /dev/dri/card*，屏幕可能点不亮"
  [ "$FORCE" -eq 1 ] || die "确认仍要安装请加 --force"
fi

# GRUB 里如果留着关屏参数，装完也是黑屏 —— 只提示，不擅自改引导配置
grub_file=/etc/default/grub
if [ -f "$grub_file" ] && grep -qE 'video=DSI-1:d|i915\.modeset=0|nomodeset' "$grub_file"; then
  c_warn "GRUB 里有禁用内置屏的参数（video=DSI-1:d / i915.modeset=0 / nomodeset）"
  if [ "$FIX_GRUB" -eq 1 ]; then
    cp -a "$grub_file" "$grub_file.bak.$(date +%Y%m%d-%H%M%S)"
    sed -i -E 's/video=DSI-1:d//g; s/i915\.modeset=0//g; s/nomodeset//g; s/  +/ /g' "$grub_file"
    update-grub >/dev/null 2>&1 || die "update-grub 失败，请手动检查 $grub_file"
    c_ok "已移除关屏参数（重启后生效）"
  else
    echo "  修复：sudo ./install.sh --fix-grub && sudo reboot"
    echo "  不改也能装，但屏幕不会亮。"
  fi
fi

# ---------------------------------------------------------------- 依赖

step "2/6 安装运行依赖"

if [ "$SKIP_DEPS" -eq 1 ]; then
  c_warn "按 --skip-deps 跳过依赖安装"
elif [ "$FROM_SOURCE" -eq 1 ]; then
  # 编译路线要的是开发库，和运行时不是一回事
  if ! command -v go >/dev/null 2>&1; then
    c_warn "没装 Go，先装 golang-go"
    apt-get update -qq && apt-get install -y --no-install-recommends golang-go
  fi
  apt-get install -y --no-install-recommends libsdl2-dev libsdl2-ttf-dev \
    fonts-wqy-zenhei lm-sensors smartmontools i2c-tools
  c_ok "编译工具链就位"
else
  missing=()
  for p in "${RUNTIME_PKGS[@]}"; do
    dpkg -s "$p" >/dev/null 2>&1 || missing+=("$p")
  done
  if [ ${#missing[@]} -eq 0 ]; then
    c_ok "依赖已齐（SDL2 运行时 / 字体 / sensors / smartctl / i2c-tools）"
  else
    echo "  需要安装：${missing[*]}"
    apt-get update -qq
    apt-get install -y --no-install-recommends "${missing[@]}" || die "依赖安装失败，检查 apt 源"
    c_ok "依赖装好"
  fi
fi

# ---------------------------------------------------------------- 取二进制

step "3/6 准备 r1-toolbox"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

install_from="$tmp/$ASSET"

if [ -n "$BINARY" ]; then
  [ -f "$BINARY" ] || die "找不到 $BINARY"
  cp "$BINARY" "$install_from"
  c_ok "用本地包：$BINARY"
elif [ "$FROM_SOURCE" -eq 1 ]; then
  src_root="$(cd "$(dirname "$0")" && pwd)"
  [ -f "$src_root/go.mod" ] || die "--from-source 要在仓库目录里跑"
  echo "  编译中（首次约 1–3 分钟）…"
  ( cd "$src_root" && make build >/dev/null ) || die "编译失败：cd $src_root && make build"
  cp "$src_root/build/r1-toolbox" "$install_from"
  c_ok "从源码构建完成"
else
  if [ "$VERSION" = "latest" ]; then
    url="https://github.com/$REPO/releases/latest/download/$ASSET"
  else
    url="https://github.com/$REPO/releases/download/$VERSION/$ASSET"
  fi
  echo "  下载 $url"
  if command -v curl >/dev/null 2>&1; then
    # 交互终端才画进度条；ssh / 管道里画出来是一屏乱码
    if [ -t 2 ]; then
      curl -fL --retry 3 --connect-timeout 20 --progress-bar -o "$install_from" "$url" \
        || die "下载失败。国内网络可先手动下载，再用 --binary 指定本地文件。"
    else
      curl -fsSL --retry 3 --connect-timeout 20 -o "$install_from" "$url" \
        || die "下载失败。国内网络可先手动下载，再用 --binary 指定本地文件。"
    fi
  else
    wget -q --show-progress -O "$install_from" "$url" \
      || die "下载失败。可手动下载后用 --binary 指定本地文件。"
  fi
  c_ok "下载完成（$(du -h "$install_from" | cut -f1)）"
fi

# 架构校验：拿错包（比如把 arm64 装到 R1 上）现象是「无法执行」，提前拦住
file "$install_from" | grep -q 'x86-64' || {
  [ "$FROM_SOURCE" -eq 1 ] && c_warn "非 x86_64 产物，跳过架构校验" || die "这个包不是 x86-64，装不上去"
}
chmod 0755 "$install_from"

# 校验和：Release 里带了 .sha256 就核，没带就跳过（历史版本没有）
if [ -z "$BINARY" ] && [ "$FROM_SOURCE" -eq 0 ]; then
  if curl -fsL --connect-timeout 10 -o "$tmp/$ASSET.sha256" "$url.sha256" 2>/dev/null; then
    want="$(awk 'NR==1{print $1}' "$tmp/$ASSET.sha256")"
    got="$(sha256sum "$install_from" | awk '{print $1}')"
    [ "$want" = "$got" ] || die "SHA-256 校验失败（下载损坏或被篡改）"
    c_ok "SHA-256 校验通过"
  else
    c_warn "这个版本没有一并发布 .sha256，跳过校验"
  fi
fi

if [ -f "$BIN" ]; then
  bk="$BIN.bak.$(date +%Y%m%d-%H%M%S)"
  cp -a "$BIN" "$bk"
  c_ok "旧二进制已备份：$bk"
fi
install -m 0755 "$install_from" "$BIN"
c_ok "已安装到 $BIN"

# ---------------------------------------------------------------- 配置

step "4/6 配置文件"

mkdir -p "$CONFIG_DIR"

# 旧版 install.sh 把配置写在 /etc/r1-control/config.yaml —— 和程序读的路径
# 根本不是一个地方，等于装完就在用默认值。升级上来的机器先把它搬过来，
# 否则当初设的端口、风扇曲线会在这次升级里「凭空消失」。
LEGACY_CONFIG="/etc/r1-control/config.yaml"
if [ ! -f "$CONFIG" ] && [ -f "$LEGACY_CONFIG" ] && [ "$CONFIG_DIR" = "/etc/r1-toolbox" ]; then
  cp -a "$LEGACY_CONFIG" "$CONFIG"
  c_ok "已把旧路径 $LEGACY_CONFIG 的配置迁移到 $CONFIG"
fi

if [ -f "$CONFIG" ]; then
  mkdir -p "$BACKUP_DIR"
  cp -a "$CONFIG" "$BACKUP_DIR/config.yaml.$(date +%Y%m%d-%H%M%S)"
  # 只留最近 5 份，反复升级不至于把 /var 塞满。
  # 末尾的 || true：备份目录可能是空的，ls 无匹配返回 2，配 pipefail 会误杀脚本。
  ls -1t "$BACKUP_DIR"/config.yaml.* 2>/dev/null | tail -n +6 | xargs -r rm -f || true
  c_ok "保留现有配置，未覆盖（备份在 $BACKUP_DIR）"
  echo "  端口、风扇曲线、令牌都在原文件里，脚本不碰。"
  if [ "$PORT" != "8080" ]; then
    c_warn "已有配置，--port $PORT 未生效；要改端口请编辑 $CONFIG 后重启服务"
  fi
else
  umask 077
  cat > "$CONFIG" <<EOF
# R1 FrontScreen 配置（首次安装生成；改完重启服务生效）
# 面板令牌由程序首次启动时自动生成并回写到本文件。

screen:
  width: 376
  height: 960
  brightness: 36000
  # 夜间（18:00–06:00）自动调暗后的背光值，0 = 不调暗
  night_brightness: 12000
  timeout: 300

rgb:
  mode: solid
  color: green
  i2c_bus: "0"

fan:
  cpu:
    mode: auto
    pwm_index: 3
    temp_sensor: "it8628:temp1"
    min_temp: 35
    max_temp: 75
    min_speed: 50
    max_speed: 255
  hdd:
    mode: auto
    pwm_index: 2
    temp_sensor: "it8628:temp2"
    min_temp: 30
    max_temp: 45
    min_speed: 50
    max_speed: 255

web:
  enabled: true
  port: $PORT

hardware:
  framebuffer: /dev/fb0
EOF
  umask 022
  c_ok "已生成配置：$CONFIG（端口 $PORT）"
fi

# ---------------------------------------------------------------- systemd

step "5/6 注册系统服务"

cat > "$UNIT" <<'EOF'
[Unit]
Description=R1 FrontScreen (r1-toolbox) display service
After=network.target

# 禁用启动限流：7x24 无人值守的常亮屏，一次 DRM 争用不该让屏幕永远黑着。
# 配合 Restart=always + RestartSec=5，开机早期 DRM 未就绪也能几轮内自己起来。
StartLimitIntervalSec=0

[Service]
# Type=notify + WatchdogSec：应用通过 sd_notify 上报就绪与心跳。
# 主循环卡死（屏幕定格但进程活着）30s 内被 systemd 识别并重启 ——
# 进程崩溃 Restart 本来就能救，卡死只有看门狗能救。
Type=notify
WatchdogSec=30
NotifyAccess=main
User=root
Environment=SDL_VIDEODRIVER=kmsdrm
Environment=SDL_FBDEV=/dev/fb0
Environment=XDG_RUNTIME_DIR=/run/r1-toolbox
ExecStartPre=/bin/mkdir -p /run/r1-toolbox
ExecStartPre=/bin/chmod 700 /run/r1-toolbox
ExecStart=/usr/local/bin/r1-toolbox
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable r1-toolbox >/dev/null 2>&1 || die "启用开机自启失败"
c_ok "服务已注册（Type=notify + 30s 看门狗）"

# ---------------------------------------------------------------- 启动自检

step "6/6 启动并自检"

systemctl restart r1-toolbox
sleep 4

if systemctl is-active --quiet r1-toolbox; then
  c_ok "服务已启动"
else
  c_err "服务没起来，最近日志："
  journalctl -u r1-toolbox -n 25 --no-pager || true
  exit 1
fi

# 端口探测：有面板令牌时 401 也算「在监听」
lan_ip="$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src") print $(i+1)}' | head -1)"
lan_ip="${lan_ip:-<设备IP>}"

if curl -s -o /dev/null --connect-timeout 3 "http://127.0.0.1:$PORT/" 2>/dev/null; then
  c_ok "面板端口 $PORT 在监听"
else
  c_warn "端口 $PORT 暂时没响应，稍等几秒再试；持续不通用 journalctl 看日志"
fi

cat <<EOF

$(printf '\033[1m✓ 安装完成\033[0m')

  屏幕  应该已经点亮，左滑 / 右滑切换三页（环境 / 详情 / 系统）
  面板  http://$lan_ip:$PORT
  令牌  在屏幕上滑到「系统」→ 点「控制」卡里的「面板令牌」一行，显示 10 秒
        也可以直接读：sudo grep token $CONFIG

  屏幕没亮？
    1) sudo journalctl -u r1-toolbox -n 50
    2) 确认 GRUB 没有关屏参数：sudo ./install.sh --fix-grub && sudo reboot
    3) 确认接的是机器自带的那块小屏，不是 HDMI

EOF
