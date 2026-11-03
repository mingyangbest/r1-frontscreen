#!/bin/bash
# 替代 systemd 的 Restart=always + WatchdogSec。
# worker 退出就 5 秒后重启；心跳文件超过 60 秒没更新，判定卡死并强杀。

set -u

APPDEST="${TRIM_APPDEST:?}"
VAR="${TRIM_PKGVAR:?}"

BIN="$APPDEST/bin/r1-toolbox"
LOG="$VAR/service.log"
HB="$VAR/heartbeat"
WORKER_PID="$VAR/worker.pid"
STOP_FLAG="$VAR/stop.flag"

HB_TIMEOUT=60
child=0

log() { echo "$(date '+%F %T') [supervisor] $*" >> "$LOG"; }

cleanup() {
  if [ "$child" -gt 0 ]; then
    kill -TERM "$child" 2>/dev/null
    sleep 1
    kill -KILL "$child" 2>/dev/null
  fi
  rm -f "$WORKER_PID"
  exit 0
}
trap cleanup TERM INT

log "守护启动 pid=$$"

while [ ! -f "$STOP_FLAG" ]; do
  "$BIN" >> "$LOG" 2>&1 &
  child=$!
  echo "$child" > "$WORKER_PID"
  log "worker 启动 pid=$child"

  # 等 worker 退出，期间盯心跳
  while kill -0 "$child" 2>/dev/null; do
    sleep 5
    [ -f "$STOP_FLAG" ] && break

    if [ -f "$HB" ]; then
      last=$(stat -c %Y "$HB" 2>/dev/null || echo 0)
      now=$(date +%s)
      if [ $((now - last)) -gt "$HB_TIMEOUT" ]; then
        log "心跳超时 $((now - last))s，判定主循环卡死，强杀 worker"
        kill -KILL "$child" 2>/dev/null
        break
      fi
    else
      # 文件被清理脚本误删时重建基准，不误杀
      date +%s > "$HB"
    fi
  done

  wait "$child" 2>/dev/null
  rc=$?
  log "worker 退出 rc=$rc"

  [ -f "$STOP_FLAG" ] && break
  sleep 5
done

log "收到停止信号，守护退出"
cleanup
