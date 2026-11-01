// Package selfmon 自身可观测性（路线表 P3 应用自检）。
//
// 回答四个问题，全部不用 SSH：
//  1. 这程序自己吃了多少 CPU / 内存？—— v0.1.0 起「整机 2.1%」基线的持续自证；
//     每加一个功能（历史、告警、预览）都要拿它对比，没有这个数就没法证明"没变重"。
//  2. 渲染循环还健康吗？—— 平均/最大帧耗时、慢帧（>33ms，即 30fps 预算）计数。
//  3. 采样还活着吗？—— 最后一次成功采样的距今时长；系统页"数据"药丸删除后，
//     这个信息在面板自检卡恢复。
//  4. systemd 看门狗在不在管它？
//
// 全部是进程内自省（/proc/self + runtime + atomic 计数器），不 spawn 子进程、
// 不进渲染热路径 —— RecordFrame/MarkSample 各是一次原子加法。
package selfmon

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"r1-toolbox/internal/watchdog"
)

// 基线与阈值（洋哥 2026-09-27 定的 2.1% 基线 / 单核 5% 上限）。
const (
	CPUGood     = 2.5 // %，≤ 视为正常
	CPUWarn     = 5.0 // %，≤ 视为偏高，> 即超线
	SlowFrameMS = 33  // 30fps 单帧预算
)

// Stats 一次快照（面板 JSON 直接序列化这个结构）。
type Stats struct {
	Uptime        string  `json:"uptime"`
	StartedAt     string  `json:"started_at"`
	CPUPercent    float64 `json:"cpu_percent"`
	RSSMB         float64 `json:"rss_mb"`
	Goroutines    int     `json:"goroutines"`
	Frames        uint64  `json:"frames"`
	FrameAvgMS    float64 `json:"frame_avg_ms"`
	FrameMaxMS    float64 `json:"frame_max_ms"`
	FrameSlow     uint64  `json:"frame_slow"`
	SampleCount   uint64  `json:"sample_count"`
	SampleErrors  uint64  `json:"sample_errors"`
	LastSampleAgo float64 `json:"last_sample_ago"` // 秒；-1 = 尚未采样
	WatchdogOn    bool    `json:"watchdog_on"`
	WatchdogSec   int     `json:"watchdog_sec"`
}

type monitor struct {
	startedAt time.Time

	// CPU 差分：只在 Snapshot（web 协程）里读写，锁保护
	mu          sync.Mutex
	lastJiffies uint64
	lastAt      time.Time
	cpuPercent  float64

	// 帧统计：渲染协程写，web 协程读，全 atomic
	frames     atomic.Uint64
	frameNs    atomic.Uint64
	frameMaxNs atomic.Uint64
	slowFrames atomic.Uint64

	// 采样：sampler 协程写
	lastSampleNs atomic.Int64
	sampleCount  atomic.Uint64
	sampleErrs   atomic.Uint64
}

// M 包级单例。纯观测面，实例化无副作用；逐层传参的成本（renderer、
// sampler、main、web 四处持有同一引用）大于全局带来的测试代价。
var M = New()

func New() *monitor { return &monitor{startedAt: time.Now()} }

// RecordFrame 渲染协程每帧调用（Render 出口，含息屏早退的便宜帧）。
func (m *monitor) RecordFrame(d time.Duration) {
	ns := uint64(d)
	m.frames.Add(1)
	m.frameNs.Add(ns)
	for {
		old := m.frameMaxNs.Load()
		if ns <= old || m.frameMaxNs.CompareAndSwap(old, ns) {
			break
		}
	}
	if d > SlowFrameMS*time.Millisecond {
		m.slowFrames.Add(1)
	}
}

// MarkSample 采样协程每轮成功后调用；errs 是本轮里失败的 Update 个数
// （CPU/内存/网络各自尽力而为，一个失败不代表整轮作废）。
func (m *monitor) MarkSample(errs int) {
	m.lastSampleNs.Store(time.Now().UnixNano())
	m.sampleCount.Add(1)
	if errs > 0 {
		m.sampleErrs.Add(uint64(errs))
	}
}

// procSelfStat 取 (utime+stime) jiffies 与 RSS 页数。
// CLK_TCK 与 pagesize：Linux x86_64/arm64 用户态恒为 100 / 4096，
// 不为理论上的例外平台引入 cgo 或 syscall 包依赖。
func procSelfStat() (jiffies uint64, rssPages uint64, err error) {
	b, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, 0, err
	}
	// comm 字段可含空格与括号，从最后一个 ')' 后面开始才是稳定字段
	s := string(b)
	if i := strings.LastIndexByte(s, ')'); i >= 0 {
		s = s[i+1:]
	}
	f := strings.Fields(s)
	// 去掉 pid 和 comm 后，utime 是第 11、stime 第 12（原第 14/15），rss 第 21
	if len(f) < 22 {
		return 0, 0, fmt.Errorf("stat 字段不足")
	}
	utime, _ := strconv.ParseUint(f[11], 10, 64)
	stime, _ := strconv.ParseUint(f[12], 10, 64)
	rss, _ := strconv.ParseUint(f[21], 10, 64)
	return utime + stime, rss, nil
}

// Snapshot 汇总一次。CPU% 用两次 Snapshot 的差分（面板 5 秒一拉，
// 正好是 CPU% 的测量窗口；首次调用给 0）。
func (m *monitor) Snapshot() Stats {
	st := Stats{
		StartedAt:  m.startedAt.Format("01-02 15:04"),
		Goroutines: runtime.NumGoroutine(),
	}

	if j, rss, err := procSelfStat(); err == nil {
		st.RSSMB = float64(rss) * 4096 / 1024 / 1024
		m.mu.Lock()
		if m.lastAt.IsZero() {
			// 首次：只记基点，下一轮才有差分
			m.lastJiffies, m.lastAt = j, time.Now()
		} else if dt := time.Since(m.lastAt).Seconds(); dt > 0.5 {
			pct := float64(j-m.lastJiffies) / 100.0 / dt * 100 // CLK_TCK=100
			// EMA 0.5 平滑：面板上的数字不应该抖成心电图
			if m.cpuPercent == 0 {
				m.cpuPercent = pct
			} else {
				m.cpuPercent = m.cpuPercent*0.5 + pct*0.5
			}
			m.lastJiffies, m.lastAt = j, time.Now()
		}
		st.CPUPercent = m.cpuPercent
		m.mu.Unlock()
	}

	st.Uptime = formatDur(time.Since(m.startedAt))
	st.Frames = m.frames.Load()
	if n := m.frames.Load(); n > 0 {
		st.FrameAvgMS = float64(m.frameNs.Load()) / float64(n) / 1e6
	}
	st.FrameMaxMS = float64(m.frameMaxNs.Load()) / 1e6
	st.FrameSlow = m.slowFrames.Load()
	st.SampleCount = m.sampleCount.Load()
	st.SampleErrors = m.sampleErrs.Load()
	if ns := m.lastSampleNs.Load(); ns > 0 {
		st.LastSampleAgo = time.Since(time.Unix(0, ns)).Seconds()
	} else {
		st.LastSampleAgo = -1
	}
	st.WatchdogOn = watchdog.Enabled()
	if d := watchdog.Interval(); d > 0 {
		st.WatchdogSec = int(d.Seconds())
	}
	return st
}

func formatDur(d time.Duration) string {
	days := int(d.Hours()) / 24
	if days > 0 {
		return fmt.Sprintf("%d天%d时", days, int(d.Hours())%24)
	}
	return fmt.Sprintf("%d时%d分", int(d.Hours()), int(d.Minutes())%60)
}
