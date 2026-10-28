// Package watchdog 实现 systemd 的 sd_notify 心跳。
//
// 为什么需要它：这块屏常亮，最难受的故障形态不是进程崩了（systemd 会
// 重启），而是进程还活着、主循环却卡死了 —— 屏幕定格在一小时前的画面，
// 看起来"正常"，其实早就不刷新了。systemd 的 WatchdogSec 配合
// WATCHDOG=1 心跳能识别这种状态：超时没收到心跳就杀掉重启。
//
// 不引第三方依赖：sd_notify 就是在 $NOTIFY_SOCKET 上发一条 unixgram
// 报文，几十行足够。
package watchdog

import (
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"r1-toolbox/internal/logx"
)

var (
	initOnce sync.Once
	conn     *net.UnixConn
	interval time.Duration
)

// init 懒初始化：读 NOTIFY_SOCKET 并连上。
// 非 systemd 环境（开发机、手动跑）下 NOTIFY_SOCKET 为空，全部操作退化为 no-op。
func init() {
	initOnce.Do(func() {
		sock := os.Getenv("NOTIFY_SOCKET")
		if sock == "" {
			return
		}
		name := sock
		// 抽象命名空间以 '@' 表示，内核里其实是第一个字节为 \0
		if name[0] == '@' {
			name = "\x00" + name[1:]
		}
		addr, err := net.ResolveUnixAddr("unixgram", name)
		if err != nil {
			logx.Printf("[watchdog] NOTIFY_SOCKET 解析失败，看门狗不可用: %v", err)
			return
		}
		c, err := net.DialUnix("unixgram", nil, addr)
		if err != nil {
			logx.Printf("[watchdog] 连接 NOTIFY_SOCKET 失败，看门狗不可用: %v", err)
			return
		}
		conn = c

		// systemd 通过 WATCHDOG_USEC 告知超时（微秒），按 1/3 发心跳
		if v := os.Getenv("WATCHDOG_USEC"); v != "" {
			if usec, err := strconv.ParseInt(v, 10, 64); err == nil && usec > 0 {
				interval = time.Duration(usec) * time.Microsecond / 3
			}
		}
	})
}

func send(msg string) error {
	if conn == nil {
		return nil
	}
	_, err := conn.Write([]byte(msg))
	return err
}

// Enabled 是否有可用的 systemd 通知通道
func Enabled() bool { return conn != nil }

// Interval 建议的心跳间隔（未配置时返回 10 秒）
func Interval() time.Duration {
	if interval <= 0 {
		return 10 * time.Second
	}
	return interval
}

// Ready 告诉 systemd 启动完成（Type=notify 时必需；Type=simple 下无副作用）
func Ready() {
	if err := send("READY=1\nSTATUS=屏幕运行中"); err != nil {
		logx.Printf("[watchdog] READY 发送失败: %v", err)
	}
}

// Watchdog 心跳。主循环每 Interval 调一次 —— 心跳停了就意味着主循环停了。
func Watchdog() {
	if err := send("WATCHDOG=1"); err != nil {
		logx.Printf("[watchdog] 心跳发送失败: %v", err)
	}
}

// Status 上报当前状态文本（systemctl status 可见）
func Status(text string) {
	send("STATUS=" + text)
}
