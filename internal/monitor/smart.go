// SMART 健康检查。
//
// 为什么放在 monitor 而不是 alerts：它和 disk.go 的温度探测是同一块盘、
// 同一个 smartctl、同一套"半死的盘会卡住进程"的教训 —— 超时与缓存放
// 在一起才不会两处口径漂移。
//
// 口径（2026-11-01）：
//   - 只认 smartctl -H 的明确结论：PASSED = 健康，FAILED = 异常
//   - 命令失败 / 输出不含结论 = UNKNOWN，**不告警** —— 盘不存在、
//     不支持 SMART 都会走到这里，报了就是误报
//   - 结果缓存 10 分钟：smartctl 单次 10~40ms，但异常盘可能卡到秒级，
//     告警协程每 10s 轮询一次，不带缓存会放大故障盘的 IO 压力
package monitor

import (
	"os"
	"strings"
	"sync"
	"time"

	"r1-toolbox/internal/execx"
)

const smartHealthTTL = 10 * time.Minute

type smartEntry struct {
	bad    bool // 明确 FAILED
	known  bool // 拿到了明确结论（PASSED 或 FAILED）
	detail string
	at     time.Time
}

type smartCache struct {
	mu      sync.Mutex
	entries map[string]smartEntry
}

var smartHealth = &smartCache{entries: map[string]smartEntry{}}

// SMARTBad 检查给定物理盘（BaseDevice，如 sda / nvme0n1）的健康状态，
// 返回明确 FAILED 的盘列表。结果带 10 分钟缓存。
func SMARTBad(devices []string) (bad []string, detail string) {
	smartHealth.mu.Lock()
	defer smartHealth.mu.Unlock()

	var badDevs []string
	var details []string
	for _, dev := range devices {
		if dev == "" {
			continue
		}
		if e, ok := smartHealth.entries[dev]; ok && time.Since(e.at) < smartHealthTTL {
			if e.bad {
				badDevs = append(badDevs, dev)
				if e.detail != "" {
					details = append(details, dev+" "+e.detail)
				}
			}
			continue
		}
		e := probeSMART(dev)
		smartHealth.entries[dev] = e
		if e.bad {
			badDevs = append(badDevs, dev)
			if e.detail != "" {
				details = append(details, dev+" "+e.detail)
			}
		}
	}
	if len(badDevs) > 0 {
		detail = strings.Join(details, " · ")
		if detail == "" {
			detail = strings.Join(badDevs, " / ")
		}
	}
	return badDevs, detail
}

// probeSMART 单盘检查。smartctl 对 nvme 同样走 -H。
func probeSMART(base string) smartEntry {
	outB, err := execx.Run(smartctlTimeout, "smartctl", "-H", "/dev/"+base)
	if err != nil {
		// 退出码非 0 不等于盘坏：smartctl 对 USB 桥、无 SMART 的盘都会
		// 报错。没有明确结论就不下结论。
		return smartEntry{at: time.Now()}
	}
	out := string(outB)
	low := strings.ToLower(out)
	switch {
	case strings.Contains(low, "failed"):
		// 找一行结论给横幅用，如 "SMART overall-health ...: FAILED!"
		detail := ""
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(strings.ToLower(line), "failed") {
				detail = strings.TrimSpace(line)
				detail = strings.TrimPrefix(detail, "- ")
				break
			}
		}
		return smartEntry{bad: true, known: true, detail: detail, at: time.Now()}
	case strings.Contains(low, "passed"):
		return smartEntry{bad: false, known: true, at: time.Now()}
	}
	return smartEntry{at: time.Now()}
}

// 物理盘列表：从卷信息里取 BaseDevice 去重（系统盘 nvme + 数据盘 sda）。
// 放在 monitor 包里是因为它依赖 VolumeInfo 的口径。
func (d *DiskMonitor) BaseDevices() []string {
	vols := d.GetVolumes()
	seen := map[string]bool{}
	var out []string
	for _, v := range vols {
		if v.BaseDevice == "" || seen[v.BaseDevice] {
			continue
		}
		if _, err := os.Stat("/dev/" + v.BaseDevice); err != nil {
			continue
		}
		seen[v.BaseDevice] = true
		out = append(out, v.BaseDevice)
	}
	return out
}
