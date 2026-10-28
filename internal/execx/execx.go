// Package execx 带硬超时地跑外部命令。
//
// 这块屏的后台采样会调 smartctl / lsblk / sensors / i2cset。这些都是
// 外部进程，其中 smartctl 在一块半死的 SATA 盘上会卡在 D 状态
// （不可中断睡眠）几十秒甚至更久 —— 裸 exec.Command 会把整条采样链
// 拖死，屏幕上的数字原地冻住，而进程还活着、systemd 不会重启它。
package execx

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Run 执行命令并返回 stdout，超时返回 error。
//
// 双保险：
//   - CommandContext 负责超时后 kill；
//   - 外层 select 防「kill 对 D 状态进程无效」时 Wait 永久阻塞。
//     此时放弃本次读数（goroutine 会泄漏一个，但调用方每个设备最多
//     泄漏一个，比整块屏停止刷新划算）。
func Run(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	type res struct {
		out []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		out, err := cmd.Output()
		ch <- res{out, err}
	}()

	select {
	case r := <-ch:
		return r.out, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("%s 超时（%v）", name, timeout)
	}
}
