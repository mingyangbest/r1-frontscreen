package ui

import (
	"fmt"
	"os"
	"time"

	"r1-toolbox/internal/hardware"
	"r1-toolbox/internal/monitor"
	"r1-toolbox/internal/netinfo"
	"r1-toolbox/internal/screen"
)

// ============================================================
//  绘制上下文
// ============================================================

type DrawCtx struct {
	Theme     *Theme
	Fonts     *FontSet
	Mon       *monitor.SystemMonitor
	Fan       *hardware.FanController
	Screen    *screen.Control  // 背光状态机（系统页滑条/关屏按钮）
	Net       *netinfo.Service // 公网 IP / 天气（可能为 nil）
	Now       time.Time
	T         float64 // 启动至今秒数（仅用于动画相位）
	StartTime time.Time
	Host      string
	// WebToken 面板令牌，显示在系统页底部 —— 「物理接触设备即授权」，
	// 不需要去翻 journal 找。
	WebToken string

	// OnScreenOff 由主程序注入：请求立即关屏（并可被触摸唤醒）
	OnScreenOff func()

	anim  map[string]*animState
	dirty bool
	// easing 与 dirty 分开：dirty 表示"这一帧画面还会变"（含常驻动画），
	// easing 只表示"有数值正在缓动插值" —— 后者会让一大片区域每帧都变，
	// 是局部重绘的否决条件，前者不是。
	easing bool
	// dataGen 每完成一次数据采样自增。数字类内容散落在整页，
	// 代数一变就整页重画，比逐块去猜哪里变了更简单也更可靠。
	dataGen uint64

	// ---- 触摸按压反馈 ----
	//
	// 点到卡片时给一次 260ms 的亮度微脉冲（很轻，峰值只有 5% 白），
	// 这是"屏幕响应了我"最便宜的证明。PressOverlay 在绘制卡片时调用，
	// 命中则返回当前脉冲强度并顺带声明本帧需要重绘。
	PressX, PressY int
	PressAt        time.Time
}

// BumpData 数据采样完成后调用，宣告"屏幕上的数字全部需要刷新"
func (d *DrawCtx) BumpData() { d.dataGen++ }

// DataGen 当前数据代数
func (d *DrawCtx) DataGen() uint64 { return d.dataGen }

// Easing 本帧是否有数值在缓动
func (d *DrawCtx) Easing() bool { return d.easing }

func NewDrawCtx(fs *FontSet) *DrawCtx {
	host, _ := os.Hostname()
	return &DrawCtx{
		Fonts:     fs,
		Theme:     themeInk,
		anim:      map[string]*animState{},
		Now:       time.Now(),
		StartTime: time.Now(),
		Host:      host,
	}
}

// animState 一个缓动键的过渡状态。
//
// 🔴 旧实现是"每帧逼近剩余差值的 rate 倍"，也就是**按帧数收敛**：
// 数据每秒变一次，要 log(1/0.5)(D/0.2) 帧才收敛，D≈5.6 时约 5 帧。
// 这带来两个后果，都不可接受：
//
//  1. 缓动期间必须整页重画，于是"每秒 1 次数据更新"被放大成"每秒 5 次全屏重绘"，
//     而**降帧率根本不会减少它**（帧数没变，只是拉长了墙钟时间），
//     降帧率反而让数字的过渡拖到 0.33 秒 —— 更假、更慢。
//  2. 屏幕上显示的是"若干个采样周期前的插值"，对一个数据必须真实的仪表盘本身就是错的。
//
// 改成按**绝对时间**收敛后，过渡时长是固定的（easeDur 秒），
// 代价 = easeDur × 帧率，于是降帧率能等比例省下这部分开销，
// 而且任何帧率下过渡都是同样的 0.16 秒 —— 语义稳定，可预测。
type animState struct {
	cur    float64
	from   float64
	to     float64
	t0     float64 // 过渡起点（秒，取自 DrawCtx.T）
	active bool
}

// easeDur 数值过渡时长（秒）。取 0.16s：短到不会读出"滞后"，
// 又长到在最低 12fps 下也能落到 2 帧以上、看得出是渐变而不是跳变。
const easeDur = 0.16

// Ease 把数值平滑逼近目标值。
//
// 返回的永远是"当前应当显示的值"；过渡结束后逐位等于 target，
// 不留任何残差 —— 这点很重要，否则数字会永远差最后一点点，
// 而"数据必须真实"是这块屏的第一原则。
func (d *DrawCtx) Ease(key string, target float64) float64 {
	st := d.anim[key]
	if st == nil {
		// 首次出现：直接就位，不做无意义的"从 0 涨上来"
		st = &animState{cur: target, from: target, to: target, t0: d.T}
		d.anim[key] = st
		return target
	}
	if target != st.to {
		// 目标变了：从**当前显示值**重新起步（而不是从上一个目标），
		// 这样连续两次更新之间的过渡不会被拉长成两段。
		st.from = st.cur
		st.to = target
		st.t0 = d.T
	}
	u := (d.T - st.t0) / easeDur
	if u >= 1 || u < 0 {
		st.cur = st.to
		st.active = false
		return st.cur
	}
	st.cur = st.from + (st.to-st.from)*easeOutCubic(u)
	st.active = true
	d.dirty = true
	d.easing = true
	return st.cur
}

// BeginFrame 重置本帧动画标记
func (d *DrawCtx) BeginFrame() {
	d.dirty = false
	d.easing = false
}

// NeedsAnim 本帧是否有动画在进行（用于自适应降帧）
func (d *DrawCtx) NeedsAnim() bool { return d.dirty }

// KeepAnim 声明本页需要持续重绘（有常驻动画，例如首页天气）。
// 只在首页调用，其他页仍走 8fps 静止档以省 CPU。
func (d *DrawCtx) KeepAnim() { d.dirty = true }

// pressDur 按压脉冲时长（秒）
const pressDur = 0.26

// RegisterPress 记录一次点击（dispatchTap 调用，点击落点）
func (d *DrawCtx) RegisterPress(x, y int) {
	d.PressX, d.PressY, d.PressAt = x, y, time.Now()
}

// PressOverlay 点击落在 (x,y,w,h) 内时返回当前脉冲强度 0..1，
// 否则返回 0。脉冲在前 pressDur 内线性衰减；结束后再多触发一帧
// 把残余提亮清掉 —— 不补这一帧，跳帧机制会让卡片永久停在最后一丝亮。
func (d *DrawCtx) PressOverlay(x, y, w, h int) float64 {
	if d.PressAt.IsZero() {
		return 0
	}
	el := time.Since(d.PressAt).Seconds()
	if el < 0 {
		return 0
	}
	if el > pressDur+0.1 {
		d.PressAt = time.Time{}
		return 0
	}
	if x > d.PressX || d.PressX >= x+w || y > d.PressY || d.PressY >= y+h {
		return 0
	}
	d.dirty = true
	d.easing = true // 让局部重绘接管：被按的卡都在 DataRects 里
	if el > pressDur {
		return 0 // 收尾帧：只负责把画面清干净
	}
	return (1 - el/pressDur) * 0.055
}

// WeatherNow 取天气快照（无服务时返回零值）
func (d *DrawCtx) WeatherNow() netinfo.Weather {
	if d.Net == nil {
		return netinfo.Weather{}
	}
	return d.Net.Weather()
}

// ------------------------------------------------------------
//  页面接口
// ------------------------------------------------------------

type Page interface {
	Draw(c *Canvas, d *DrawCtx)
}

// Named 可选实现：返回页面名（顶栏中间显示）
type Named interface {
	PageName() string
}

// Tappable 可选实现：接收点击
type Tappable interface {
	Tap(x, y int, d *DrawCtx)
}

// Staggered 可选实现：声明入场编排的动画单元（页面坐标矩形，按浮入顺序）。
//
// 换页滑动结束后，渲染器把整页快照按这些矩形切成子图，
// 逐个「淡入 + 上浮 10px」错峰浮入 —— Material 3 的 container 入场节奏。
// 渲染期间页面 Draw 不会跑（走快照 blit），所以这些矩形必须覆盖
// 页面上所有内容；矩形之外的区域在编排期间保持背景。
type Staggered interface {
	CardRects() []Rect
}

// 曲线量程档位：量化到固定档，既有起伏又不会频繁抖动
var historySteps = []float64{20, 30, 40, 50, 60, 70, 80, 90, 100}

// autoTop 依据历史峰值挑一个留 20% 余量的量程档
func autoTop(data []float64, floor float64) float64 {
	mx := floor
	for _, v := range data {
		if v > mx {
			mx = v
		}
	}
	for _, s := range historySteps {
		if mx <= s*0.8 {
			return s
		}
	}
	return 100
}

// formatUptime 把系统开机至今的秒数写成中文时长。
//
// 注意：这里用的是 /proc/uptime（系统运行时长），
// 不是本进程的启动时间 —— 否则一重启服务就显示「0时0分」。
func formatUptime(sec float64) string {
	s := int(sec)
	if s < 0 {
		s = 0
	}
	d := s / 86400
	h := (s % 86400) / 3600
	m := (s % 3600) / 60
	switch {
	case d > 0:
		return fmt.Sprintf("%d天%d时", d, h)
	case h > 0:
		return fmt.Sprintf("%d时%d分", h, m)
	default:
		return fmt.Sprintf("%d分", m)
	}
}

// fmtBytes 人类可读容量
func fmtBytes(b uint64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.2f TB", float64(b)/TB)
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/MB)
	default:
		return fmt.Sprintf("%.0f KB", float64(b)/KB)
	}
}

// ============================================================
//  1. 概览页
//
//  版面（洋哥指定）：
//    上  = 天气（彩色 + 动画）
//    中  = CPU / 内存 负载 + 上传下载速率
//    下  = 系统信息（主机名 / 局域网 / 公网 IP / 已运行）
//  注意：按洋哥要求，「系统负载」不再出现在本页
// ============================================================

// clamp 数值限幅
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ipOrDash 空地址显示占位符，不编造数据
func ipOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// FullTopPage 全出血顶栏页：顶栏区（0~96）由页面自身接管绘制。
// 「环境」页用它画 Material You 的大时钟 —— 顶栏的 46px 小时钟与大时钟重复。
type FullTopPage interface {
	Page
	FullTop() bool
}

// pageFullTop 页面是否全出血顶栏（非 FullTopPage 一律 false）
func pageFullTop(p Page) bool {
	ft, ok := p.(FullTopPage)
	return ok && ft.FullTop()
}

// trim 中文按字符截断（rune 安全），超长补省略号
func trim(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}
