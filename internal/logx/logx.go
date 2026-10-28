// Package logx 给日志分两级：状态级（默认开）与明细级（默认关）。
//
// 为什么要这个包：v0.5.0 之前主循环每 5 秒打两条性能日志、磁盘每 60 秒
// 打三到四条审计日志，实测一天 91,492 行 / 163 MB —— 真正的事故记录
// （重启、关屏、配置写入失败）全被淹在里面，journalctl 翻不到。
//
// 分级约定：
//   - logx.Printf / logx.Println：状态变更与错误，永远输出。
//     数量级应该是「一天几十行」，新增调用前先想想它配不配。
//   - logx.Debugf：性能、采样明细、审计对账，只在 SetDebug(true) 时输出。
//     排障时用 R1_DEBUG=1 或配置 debug: true 临时打开即可。
package logx

import (
	"log"
	"sync/atomic"
)

var debug atomic.Bool

// SetDebug 打开/关闭明细日志
func SetDebug(on bool) { debug.Store(on) }

// DebugEnabled 当前是否输出明细日志（调用方可用它跳过昂贵的字符串拼接）
func DebugEnabled() bool { return debug.Load() }

// Printf 状态级日志（永远输出）
func Printf(format string, args ...any) { log.Printf(format, args...) }

// Println 状态级日志（永远输出）
func Println(args ...any) { log.Println(args...) }

// Debugf 明细级日志（默认不输出）
func Debugf(format string, args ...any) {
	if debug.Load() {
		log.Printf(format, args...)
	}
}
