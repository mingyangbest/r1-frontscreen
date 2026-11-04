// ============================================================
//
//	告警引擎 —— 异常可见的判定与出口
//
//	职责（2026-11-01 洋哥 P2 拍板）：
//	  1. 五条规则的状态机：连续越界 N 次（防抖）才触发，恢复时记一条
//	  2. 同一条告警静默期内不重复推送（Bark 轰炸比不报更糟）
//	  3. 触发/恢复落 JSONL（/var/lib/r1-toolbox/alerts.jsonl）——
//	     一年几百条的量级，引 SQLite 是给 P4 历史曲线用的，这里不值
//	  4. Bark 推送走独立协程，HTTP 失败只打日志不影响判定
//
//	解耦原则：本包不 import monitor/hardware —— 数据由 main 组装成
//	Inputs 喂进来。判定口径集中在 Evaluate，谁的输入谁负责。
//
//	横幅渲染约定（与洋哥确认的视觉稿一致）：
//	  - 常态（无活跃告警）不出横幅 —— 无横幅本身就是"一切正常"的信号
//	  - 故障(crit)珊瑚底白字、预警(warn)沙金底深字
//	  - 屏上只显示最严重一项 + 「共 N 项」，清单放面板
//
// ============================================================
package alerts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"r1-toolbox/internal/config"
)

// LogPath 触发/恢复记录的唯一路径。引擎写、面板读，共用一份口径
// —— 不做成注入字段：单一入口比"两处都要传对"更不容易 diverge。
//
// 顶层目录由 R1_DATA_DIR 决定（飞牛 fpk 下是 $TRIM_PKGVAR），默认沿用
// /var/lib/r1-toolbox，与命令行安装的历史路径一致。
var LogPath = config.DataFile("alerts.jsonl")

type Sev string

const (
	SevCrit Sev = "crit" // 故障：珊瑚
	SevWarn Sev = "warn" // 预警：沙金
)

// Alert 一条活跃告警
type Alert struct {
	Key   string    `json:"key"` // 规则键：smart/temp/disk/docker/net/fan
	Sev   Sev       `json:"sev"`
	Text  string    `json:"text"`  // 横幅文案（短！376px 还要留右侧计数）
	Since time.Time `json:"since"` // 触发时刻
}

// View 横幅视图：最严重一项在前。nil 或空 = 常态，屏上不画
type View struct {
	Alerts []Alert `json:"alerts"`
}

// Top 最严重的一条（crit 先于 warn，同级别按触发时间）
func (v *View) Top() (Alert, bool) {
	if v == nil || len(v.Alerts) == 0 {
		return Alert{}, false
	}
	return v.Alerts[0], true
}

// Event 一次触发/恢复的落盘记录
type Event struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // trigger / recover
	Key  string    `json:"key"`
	Sev  Sev       `json:"sev"`
	Text string    `json:"text"`
}

// Inputs 一轮判定所需的全部数据（main 组装，引擎不认识 monitor）
type Inputs struct {
	Now time.Time

	CPUTemp     float64 // 0 = 读不到，温度规则跳过
	MaxDiskTemp float64

	MaxVolumePct  float64 // 所有卷里最高的使用率
	MaxVolumeName string  // 对应卷的展示名

	// SMARTBad 明确 FAILED 的盘（BaseDevice）。UNKNOWN（不支持/命令失败）
	// 不进这个列表 —— 没有明确结论不下结论，报了就是误报
	SMARTBad    []string // 异常盘列表（BaseDevice）
	SMARTDetail string

	DockerAbnormal int // 复用 Docker 卡判据（0/143 正常）

	FanRPM   [2]int // [CPU, 机箱]，按 PWM 映射的那两路
	FanWasOn [2]bool

	// FanSensorStale 温度源失联超过 15 秒，元素是人话名字（"CPU" / "机箱"）。
	// 失联时风扇已经升到中高档，这条用于让用户知道「转速高不是坏了」。
	FanSensorStale []string

	NetOK bool // 引擎外部探测（连续失败计数在引擎内做防抖）
}

// Notifier 推送出口（Bark）。失败只记日志。
type Notifier interface {
	Push(title, body string) error
}

// Config 阈值与通道（来自 config.AlertsConfig 的值拷贝）
type Config struct {
	Enabled            bool // false = 引擎不评估，活跃告警全部恢复
	TempWarn, TempCrit int
	DiskWarn           int
	BarkOn             bool
	Debounce           int           // 连续越界次数（≥1）
	Silence            time.Duration // 同 key 静默期
}

// Engine 状态机。Run 在独立协程里周期 Evaluate。
type Engine struct {
	mu       sync.Mutex
	cfg      func() Config
	strikes  map[string]int
	lastRPM  [2]int  // 上一轮转速 —— 停转判定是"从有掉到 0"
	fanWas   [2]bool // 上轮是否有转速
	active   map[string]Alert
	lastPush map[string]time.Time
	logPath  string
	notifier Notifier
	onChange func(*View) // 活跃集变化时回调（main 用它推给渲染器）
	kick     chan struct{}

	now func() time.Time // 测试可注入
}

func NewEngine(cfg func() Config, logPath string, notifier Notifier) *Engine {
	return &Engine{
		cfg:      cfg,
		strikes:  map[string]int{},
		active:   map[string]Alert{},
		lastPush: map[string]time.Time{},
		logPath:  logPath,
		notifier: notifier,
		kick:     make(chan struct{}, 1),
		now:      time.Now,
	}
}

// OnChange 活跃告警集合变化时的回调（渲染器横幅）
func (e *Engine) OnChange(fn func(*View)) { e.onChange = fn }

// Run 周期判定。interval 建议 10s：比采样慢一个量级，比人感知快。
// 数据源闭包允许失败（SMART 探测超时等），单个输入缺失时对应规则跳过。
func (e *Engine) Run(interval time.Duration, get func() Inputs) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-e.kick: // 面板改了阈值：立即复查，不等下一拍
		}
		e.Evaluate(get())
	}
}

// Kick 请求立即补一轮判定（非阻塞；连发的 Kick 合并成一轮）
func (e *Engine) Kick() {
	select {
	case e.kick <- struct{}{}:
	default:
	}
}

// CurrentView 当前活跃告警视图（启动时喂给渲染器一次 ——
// 服务重启后横幅要立刻恢复，不能等引擎第一轮跑完）
func (e *Engine) CurrentView() *View {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.view()
}

// Evaluate 一轮判定。任何一条规则触发/恢复都会落盘、（静默期外）推送，
// 并回调 onChange。
func (e *Engine) Evaluate(in Inputs) {
	cfg := e.cfg()
	if !cfg.Enabled {
		// 总开关关闭 = 回到"无告警"常态：活跃的全部记恢复
		e.mu.Lock()
		for key, a := range e.active {
			e.record(Event{At: e.now(), Kind: "recover", Key: key, Sev: a.Sev, Text: a.Text})
		}
		had := len(e.active)
		e.active = map[string]Alert{}
		e.strikes = map[string]int{}
		e.mu.Unlock()
		if had > 0 {
			if e.onChange != nil {
				e.onChange(e.view())
			}
		}
		return
	}
	if cfg.Debounce < 1 {
		cfg.Debounce = 1
	}
	now := in.Now
	if now.IsZero() {
		now = e.now()
	}

	// ---- 候选：本轮所有"正在越界"的规则 ----
	type cand struct {
		key string
		sev Sev
		txt string
	}
	var cands []cand

	if in.CPUTemp > 0 {
		if t := max(in.CPUTemp, in.MaxDiskTemp); t >= float64(cfg.TempCrit) {
			cands = append(cands, cand{"temp", SevCrit,
				fmt.Sprintf("温度 %d°C（阈值 %d）", int(t), cfg.TempCrit)})
		} else if t >= float64(cfg.TempWarn) {
			cands = append(cands, cand{"temp", SevWarn,
				fmt.Sprintf("温度 %d°C（阈值 %d）", int(t), cfg.TempWarn)})
		}
	}

	if in.MaxVolumePct >= float64(cfg.DiskWarn) && in.MaxVolumeName != "" {
		cands = append(cands, cand{"disk", SevWarn,
			fmt.Sprintf("容量 %d%% · %s", int(in.MaxVolumePct), in.MaxVolumeName)})
	}

	if len(in.SMARTBad) > 0 {
		txt := "磁盘异常"
		if in.SMARTDetail != "" {
			txt = "磁盘异常 · " + in.SMARTDetail
		}
		cands = append(cands, cand{"smart", SevCrit, txt})
	}

	if in.DockerAbnormal > 0 {
		cands = append(cands, cand{"docker", SevCrit,
			fmt.Sprintf("容器异常 %d 个", in.DockerAbnormal)})
	}

	// 风扇停转：必须"从有转速掉到 0"。fan4/fan5 恒 0（没接风扇），
	// 按 RPM==0 判会天天误报 —— 这条洋哥 2026-11-01 特意强调过。
	for i, name := range []string{"CPU", "机箱"} {
		rpm := in.FanRPM[i]
		was := e.fanWas[i]
		if rpm > 0 {
			e.lastRPM[i], e.fanWas[i] = rpm, true
			continue
		}
		if was && e.fanWas[i] {
			cands = append(cands, cand{"fan", SevCrit,
				fmt.Sprintf("%s风扇停转", name)})
		}
		// 从一开始就是 0 的通道（未接线）不判：没有"曾经转"的基线
		e.lastRPM[i] = 0
	}

	if !in.NetOK {
		cands = append(cands, cand{"net", SevWarn, "公网连接失败"})
	}

	// 温度源失联：不是降温的借口，是「传感器这条线断了」。
	// 风扇那边已经抬到中高档，这里只负责让用户知道为什么突然变吵。
	if len(in.FanSensorStale) > 0 {
		cands = append(cands, cand{"fansensor", SevWarn,
			"温度源失联 · " + strings.Join(in.FanSensorStale, "、") + "，风扇已升档"})
	}

	// ---- 状态机：越界 strike++，达到防抖次数才算触发 ----
	e.mu.Lock()
	defer e.mu.Unlock()

	seen := map[string]bool{}
	var changed bool
	for _, c := range cands {
		seen[c.key] = true
		if _, ok := e.active[c.key]; ok {
			e.strikes[c.key] = 0 // 已触发，防抖计数清零
			continue
		}
		e.strikes[c.key]++
		if e.strikes[c.key] < cfg.Debounce {
			continue
		}
		a := Alert{Key: c.key, Sev: c.sev, Text: c.txt, Since: now}
		e.active[c.key] = a
		changed = true
		e.record(Event{At: now, Kind: "trigger", Key: c.key, Sev: c.sev, Text: c.txt})
		e.push(cfg, a)
	}

	// ---- 恢复：上轮越界、本轮不在候选里 ----
	for key, a := range e.active {
		if seen[key] {
			continue
		}
		delete(e.active, key)
		delete(e.strikes, key)
		delete(e.lastPush, key)
		changed = true
		e.record(Event{At: now, Kind: "recover", Key: key, Sev: a.Sev, Text: a.Text})
		e.push(cfg, Alert{Key: key, Sev: a.Sev, Text: "已恢复：" + a.Text})
	}

	if changed {
		if e.onChange != nil {
			e.onChange(e.view())
		}
	}
}

// view 生成横幅视图：crit 在前，同级别按触发时间早的在前。
// 调用方持有 e.mu。
func (e *Engine) view() *View {
	out := make([]Alert, 0, len(e.active))
	for _, a := range e.active {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sev != out[j].Sev {
			return out[i].Sev == SevCrit
		}
		return out[i].Since.Before(out[j].Since)
	})
	return &View{Alerts: out}
}

// Snapshot 当前活跃告警（面板用）
func (e *Engine) Snapshot() []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()
	v := e.view()
	if len(v.Alerts) == 0 {
		return nil
	}
	return v.Alerts
}

// push 静默期外推一条。触发与恢复共用静默窗 —— 同一个 key 30 分钟内
// 反复横跳只报第一次，否则夜里风扇抖一下能推一宿。
func (e *Engine) push(cfg Config, a Alert) {
	if !cfg.BarkOn || e.notifier == nil {
		return
	}
	if t, ok := e.lastPush[a.Key]; ok && e.now().Sub(t) < cfg.Silence {
		return
	}
	e.lastPush[a.Key] = e.now()
	go func() {
		verb := "异常"
		if a.Text[:6] == "已恢复" {
			verb = "恢复"
		}
		_ = e.notifier.Push("R1 "+verb, a.Text)
	}()
}

// record 追加一条事件到 JSONL。写失败只打标准错误 —— 记录是附属品，
// 不能因为它把判定协程带崩。
func (e *Engine) record(ev Event) {
	if e.logPath == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(e.logPath), 0o755)
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	f, err := os.OpenFile(e.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// ReadEvents 面板读取最近 n 条（从尾往前）。
func ReadEvents(logPath string, n int) []Event {
	b, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	var out []Event
	start := 0
	for i := 0; i <= len(b); i++ {
		if i == len(b) || b[i] == '\n' {
			if i > start {
				var ev Event
				if json.Unmarshal(b[start:i], &ev) == nil {
					out = append(out, ev)
				}
			}
			start = i + 1
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	// 新的在前
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
