package ui

// ============================================================
//  三页版（2026-10-30 按精修设计规范重排）
//
//  这一版与上一版的区别不是"换了配色"，而是**坐标来源换了**：
//  所有 y 都来自参考图的逐像素量测（2 倍图 ÷ 2），再落到 design.go 的
//  令牌上；不再有"看着差不多"的偏移量。
//
//  三条硬约束（来自规范，违反哪一条都会立刻看出来）：
//   1. 卡面无描边：卡片靠"顶部点缀色×22% → 底部 #0D0E12"的纵向渐变分层。
//      加一条 1px 描边，整屏立刻变回上一版那种"塑料面板"。
//   2. 字阶三档：时钟 100 细体 / 数字 46~48 粗体 / 正文 16~18 常规。
//      数字不用粗体，大屏上会显得没骨头；标题不用细体，会跟正文糊在一起。
//   3. 图表三线网格 + 面积线性淡出到 0 + 末端脉冲点。旧版"0.26 恒定填充"
//      在卡底留下一片灰雾，是"AI 味"的主要来源之一。
//
//  数据口径不变：读不到显示「—」，绝不编造。
//  三页：环境（大时钟） → 详情 → 系统
// ============================================================

import (
	"fmt"
	"time"

	"r1-toolbox/internal/monitor"
	"r1-toolbox/internal/screen"
)

// Version 屏幕「设备」卡右上角显示的版本号。
//
// 由 cmd/main 通过 -ldflags "-X r1-toolbox/internal/ui.Version=v0.6.0"
// 注入（见 Makefile），默认 "dev"。硬编码版本号会与 git tag 对不上 ——
// 屏幕写着 v0.5.0、跑的是哪次提交只有天知道。
var Version = "dev"

// fmtMBps 速率文本：位数随量级收缩，避免 0.04 那样难读的数字
func fmtMBps(v float64) string {
	switch {
	case v >= 10:
		return fmt.Sprintf("%.0f", v)
	case v >= 1:
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}

// tempText 温度文本。读不到就是「—」，绝不显示 0°
// （旧实现 clamp(0,120) 会把「传感器读不到」画成一个结结实实的 0°C）。
func tempText(v float64) string {
	if v <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f°", v)
}

// fitText 按像素宽度裁字，超宽补省略号。
//
// 中文字宽是字号量级，14 个字符的 36px 主机名能铺 500+px —— 直接
// 硬截字符数挡不住溢出。这里量真实墨迹宽度，保证画出来的一定放得下。
func fitText(fs *FontSet, s string, size, maxW int) string {
	if maxW <= 0 || mixedWidth(fs, size, s) <= maxW {
		return s
	}
	rs := []rune(s)
	for i := len(rs) - 1; i > 0; i-- {
		cand := string(rs[:i]) + "…"
		if mixedWidth(fs, size, cand) <= maxW {
			return cand
		}
	}
	return "…"
}

// numFit 在给定宽度内给「数字 + 单位」挑一个放得下的字号。
//
// 小卡只有 100px 宽，46px 的「73%」刚好舒服，但 CPU 真跑到 100% 时
// 三个数字会顶出卡片右边缘。宁可这一张的数字小一号，也不能让数字被卡边切掉。
func numFit(fs *FontSet, num, unit string, maxW, start int) int {
	for _, sz := range []int{start, start * 15 / 16, start * 7 / 8, start * 3 / 4} {
		parts := []TPart{
			{S: num, Col: colData, Latin: true, Bold: true},
			{S: unit, Col: colDim, Latin: true, Size: FzUnit, Gap: 2},
		}
		if w, _ := measureParts(fs, sz, parts); w <= maxW {
			return sz
		}
	}
	return start * 3 / 4
}

// drawRateRow 网络速率行：↑/↓ + 数字（28 常规）+ 单位（16 灰），右边界对齐。
func drawRateRow(c *Canvas, fs *FontSet, xRight, yInk int, v float64, col RGBA, up bool) {
	parts := []TPart{
		{S: fmtMBps(v), Col: col, Latin: true},
		{S: "MB/s", Col: colDim, Latin: true, Size: FzMeta, Gap: 5},
	}
	w, _ := measureParts(fs, FzRate, parts)
	DrawPartsTopR(c, fs, xRight, yInk, FzRate, parts)
	// 箭头与数字之间留 10px：贴着数字会读成"↑0.08"这个整体
	DrawArrow(c, xRight-w-11, yInk+13, 6, up, col)
}

// FzRate 网络速率数字字号（参考图墨迹高 20px → 字号 28）
const FzRate = 28

// FzDate 日期天气行字号。
//
// 参考图墨迹高 17.5、整行宽 207.5；22px 画出来宽 233（宽了 12%），
// 20px 落到 212 —— 同一句「10月29日 周四 · 雾 16°」，字符数完全对得上。
const FzDate = 20

// FzCurrent 曲线卡左下「当前 xx%」的字号。
//
// 这一行不是「元信息」而是读数：参考图里「当」墨迹 17px（=18 号），
// 比右上角「峰值 100%」的 15px（=16 号）高出一档，两处颜色同为灰但大小不同。
const FzCurrent = 18

// ============================================================
//  第 1 页 · 环境
// ============================================================

type EnvPage struct{ mon *monitor.SystemMonitor }

func NewEnvPage(mon *monitor.SystemMonitor) *EnvPage { return &EnvPage{mon: mon} }

func (p *EnvPage) PageName() string { return "环境" }
func (p *EnvPage) FullTop() bool    { return true }

// 首页纵向布局（device px，全部来自参考图量测）
//
//	页名        y=12   16px 居中
//	大时钟      墨迹顶 108（字号 100 细体）
//	日期天气    墨迹顶 202（24px）
//	三张小卡    y=236 h=98
//	CPU 历史    y=356 h=208
//	网络        y=586 h=222
//	药丸        y=838 h=40
const (
	envTopH    = 232
	envClockY  = 108
	envDateY   = 202
	envCardsY  = 236
	envCardsH  = 98
	envMiniW   = 101
	envMiniGap = 12
	envHistY   = 356
	envHistH   = 208
	envNetY    = 586
	envNetH    = 222
	envPillY   = 838
	envPillH   = 40
)

func (p *EnvPage) DataRects(d *DrawCtx) []Rect {
	// 大时钟区不在列：分钟由 renderer 的 fullTop 分支单独局部重画；
	// 天气快照变化走全局的天气戳全屏重画。
	return []Rect{
		{DPadX - 6, envCardsY - 6, DCardW + 12, envCardsH + 12},
		{DPadX - 6, envHistY - 6, DCardW + 12, envHistH + 12},
		{DPadX - 6, envNetY - 6, DCardW + 12, envNetH + 12},
		{DPadX - 6, envPillY - 6, DCardW + 12, envPillH + 12},
	}
}

// CardRects 入场编排单元：时钟块 → 三张小卡逐张 → 大卡自上而下 → 药丸。
func (p *EnvPage) CardRects() []Rect {
	const W = 376
	return []Rect{
		{0, 0, W, envTopH},
		{DPadX, envCardsY, envMiniW, envCardsH},
		{DPadX + envMiniW + envMiniGap, envCardsY, envMiniW, envCardsH},
		{DPadX + 2*(envMiniW+envMiniGap), envCardsY, envMiniW, envCardsH},
		{DPadX, envHistY, DCardW, envHistH},
		{DPadX, envNetY, DCardW, envNetH},
		{DPadX, envPillY, DCardW, envPillH},
	}
}

func (p *EnvPage) Draw(c *Canvas, d *DrawCtx) {
	fs := d.Fonts
	// 取一份值拷贝：采样在后台协程跑，直接读 p.mon 的字段是数据竞争。
	m := p.mon.Snapshot()

	// ---------------- 页名 + 大时钟 + 日期天气 ----------------
	if c.ClipVisible(0, 0, c.W, envTopH) {
		DrawPageLabel(c, fs, p.PageName())

		// 时钟加粗（洋哥 2026-10-30：首页时间要更"实"）。规范第四节写的是
		// 「最细体」，实机看细体 100px 在深底上有点飘，粗体更有压场感。
		DrawPartsTop(c, fs, DPadX, envClockY, FzClock,
			[]TPart{{S: d.Now.Format("15:04"), Col: colWhite, Latin: true, Bold: true}})

		wx := d.WeatherNow()
		kind := wx.Kind
		if kind == "" {
			kind = "overcast"
		}
		wxAcc := weatherAccent(kind)

		parts := []TPart{
			{S: d.Now.Format("01月02日") + " " + weekdayCN(d.Now), Col: colDim},
		}
		if wx.OK {
			parts = append(parts,
				TPart{S: "  ·  ", Col: colTab},
				TPart{S: wx.Desc, Col: Lit(wxAcc, 0.30)},
				TPart{S: "  ", Col: colTab},
				TPart{S: fmt.Sprintf("%.0f°", wx.TempC), Col: colDim, Latin: true},
			)
		}
		DrawPartsTop(c, fs, DPadX, envDateY, FzDate, parts)

		// 天气图标（右上，昼夜由 isNight 内部判断）。
		// r 从 16 放到 21（洋哥 2026-10-30：图标太小）。放大后弯月约 21×22、
		// 太阳光晕约 1.25r≈26，中心 x=335 时右沿 361 仍在 376 屏内。
		DrawWeatherIcon(c, DPadX+DCardW-17, 90, 21, kind, 2.856)
	}

	// ---------------- 三张指标小卡 ----------------
	if c.ClipVisible(DPadX, envCardsY, DCardW, envCardsH) {
		type mini struct {
			label string
			txt   string
			unit  string
			v     float64
			hue   RGBA
		}
		cpu := clamp(d.Ease("env.cpu", m.CPUPercent), 0, 100)
		mem := clamp(d.Ease("env.mem", m.MemPercent), 0, 100)
		temp := clamp(d.Ease("env.temp", m.CPUTemp), 0, 120)
		// 温度读不到时画「—」，不画 0°
		tempStr, tempUnit := "—", ""
		if temp > 0 {
			tempStr, tempUnit = fmt.Sprintf("%.0f", temp), "°"
		}
		cards := []mini{
			{"CPU", fmt.Sprintf("%.0f", cpu), "%", cpu, colSlate},
			{"内存", fmt.Sprintf("%.0f", mem), "%", mem, colRose},
			{"温度", tempStr, tempUnit, temp, colSand},
		}
		for i, mc := range cards {
			cx := DPadX + i*(envMiniW+envMiniGap)
			DCardBox(c, d, cx, envCardsY, envMiniW, envCardsH, mc.hue)
			// 标签：卡片点缀色，15px，墨迹顶 +13。
			// 18px 时「内存」墨迹 17px 宽 33px，参考图是 14.5 / 28 —— 小卡只有
			// 100 宽，18px 的标题会跟数字抢地方，参考图这里本来就降了一档。
			drawTrackedTop(c, fs, mc.label, cx+DPad, envCardsY+13, FzMini, mc.hue, 0)
			// 数字：墨迹顶 +48，粗体；颜色按读数走阈值（<50 偏灰白 / >=50 沙金
			// / >=80 珊瑚），放不下自动降一档
			sz := numFit(fs, mc.txt, mc.unit, envMiniW-2*DPad, FzNum)
			DrawPartsTop(c, fs, cx+DPad, envCardsY+48, sz, []TPart{
				{S: mc.txt, Col: valColor(mc.v), Latin: true, Bold: true},
				{S: mc.unit, Col: colDim, Latin: true, Size: FzUnit, Gap: 2},
			})
			// 进度条：+85，高 4，内缩 12（参考图 321..324，3.5px 四舍五入到 4）
			c.Bar(cx+DPad, envCardsY+85, envMiniW-2*DPad, 4, 2,
				clamp(mc.v, 0, 100)/100, colTrack, mc.hue)
		}
	}

	// ---------------- CPU 历史 · 60 秒 ----------------
	// 采样 2 秒一点、窗口 30 点 = 60 秒 —— 标题口径必须与 sampler.fastEvery
	// 一致，否则就是在屏上报一个假的时间跨度。
	if c.ClipVisible(DPadX, envHistY, DCardW, envHistH) {
		DCardBox(c, d, DPadX, envHistY, DCardW, envHistH, colSage)
		DCardTitle(c, fs, DPadX, envHistY+13, "CPU 历史 · 60 秒", colWhite)
		peak := 0.0
		for _, v := range m.CPUHistory {
			if v > peak {
				peak = v
			}
		}
		DMetaR(c, fs, DPadX+DCardW, envHistY+16, fmt.Sprintf("峰值 %.0f%%", peak), colDim)

		// 曲线框 408..518（h=110）：三条网格实测在 435.75 / 463.25 / 490.75，
		// 等距 27.5 → 框高 110、框顶 408。框底 518 之下 17px 才是「当前」。
		c.PlotFloor(DPadX+DPad, envHistY+52, DCardW-2*DPad, 110, colSage)
		c.Graph(DPadX+DPad, envHistY+52, DCardW-2*DPad, 110,
			m.CPUHistory, autoTop(m.CPUHistory, 20), 60, colSage, 0.12)

		// 当前值：墨迹顶 +179（曲线底 +140 之下 39），与卡底留 17。
		// 数值同样走阈值色 —— 它和上面三张小卡说的是同一件事。
		cpuNow := d.Ease("env.cpu", m.CPUPercent)
		DrawPartsTop(c, fs, DPadX+DPad, envHistY+179, FzCurrent, []TPart{
			{S: "当前", Col: colDim},
			{S: fmt.Sprintf("%.0f%%", cpuNow), Col: valColor(cpuNow), Latin: true, Gap: 6},
		})
	}

	// ---------------- 网络（双速率 + 双曲线）----------------
	if c.ClipVisible(DPadX, envNetY, DCardW, envNetH) {
		DCardBox(c, d, DPadX, envNetY, DCardW, envNetH, colMint)
		DCardTitle(c, fs, DPadX, envNetY+13, "网络", colWhite)

		up := d.Ease("env.up", m.NetSendMBps)
		dn := d.Ease("env.dn", m.NetRecvMBps)

		send := make([]float64, 0, len(m.NetHistory))
		recv := make([]float64, 0, len(m.NetHistory))
		scale := 0.1
		for _, v := range m.NetHistory {
			send = append(send, v[0])
			recv = append(recv, v[1])
			if v[0] > scale {
				scale = v[0]
			}
			if v[1] > scale {
				scale = v[1]
			}
		}
		scale *= 1.15
		// 曲线框 658..776（h=118）：网格实测 688 / 717 / 747，等距 29.5。
		// 框顶比第一行速率还高一截 —— 速率文字是压在曲线区右上角的，不是分区排列。
		// 底板只铺一次；下行先画（带面积 + 脉冲点），上行后画成细线压在网格之上。
		c.PlotFloor(DPadX+DPad, envNetY+72, DCardW-2*DPad, 118, colMint)
		c.Graph(DPadX+DPad, envNetY+72, DCardW-2*DPad, 118, recv, scale, 60, colMint, 0.12)
		c.Curve(DPadX+DPad, envNetY+72, DCardW-2*DPad, 118, send, scale, 60, colSlate, 1.5, 0)

		// 速率文字最后画：它压在曲线区右上角（参考图就是这个结构），
		// 先画会被曲线穿过去 —— 真跑起来总有一次峰值正好落在右端。
		// 两行墨迹顶实测 600 / 627 → 行距 27；第一行与「网络」标题同一水平线
		// （标题 601..618、上行 600..618），不是"标题一行、速率另起一行"。
		drawRateRow(c, fs, DPadX+DCardW-DPad, envNetY+14, up, colSlate, true)
		drawRateRow(c, fs, DPadX+DCardW-DPad, envNetY+41, dn, colMint, false)
	}

	// ---------------- 底部药丸（容器 / 运行时长）----------------
	if c.ClipVisible(DPadX, envPillY, DCardW, envPillH) {
		// 绿点 = Docker 可达且全部运行；读不到 = 灰点 + —
		dot := colDim
		if m.ContainersRunning >= 0 {
			dot = colMint
			if m.ContainersRunning != m.ContainersTotal {
				dot = colSand
			}
		}
		DPill(c, DPadX, envPillY, DCardW, envPillH)
		// 圆点实测圆心 x=42（直径 8.5），文字起点 55、墨迹顶 853.5
		DPillDot(c, DPadX+18, envPillY+envPillH/2, dot)

		txt := fmt.Sprintf("容器 %d/%d 运行中",
			m.ContainersRunning, m.ContainersTotal)
		if m.ContainersRunning < 0 {
			txt = "容器 —"
		}
		DrawPartsTop(c, fs, DPadX+31, envPillY+15, FzMeta, []TPart{
			{S: txt, Col: colSub},
			{S: "·", Col: colTab, Gap: 8},
			{S: "已运行 " + formatUptime(m.UptimeSec), Col: colSub, Gap: 8},
		})
	}
}

// coreCount 逻辑核心数
func coreCount(m monitor.Snapshot) int {
	if m.CPUCoreNum > 0 {
		return m.CPUCoreNum
	}
	return len(m.CPUCores)
}

// ============================================================
//  第 2 页 · 详情
// ============================================================

type DetailPage struct{ mon *monitor.SystemMonitor }

func NewDetailPage(mon *monitor.SystemMonitor) *DetailPage {
	return &DetailPage{mon: mon}
}

func (p *DetailPage) PageName() string { return "详情" }

const (
	dtCpuY, dtCpuH   = 136, 182
	dtMemY, dtMemH   = 338, 160
	dtDiskY, dtDiskH = 518, 160
	dtTempY, dtTempH = 698, 150
)

func (p *DetailPage) DataRects(d *DrawCtx) []Rect {
	return []Rect{
		{DPadX - 6, dtCpuY - 6, DCardW + 12, dtCpuH + 12},
		{DPadX - 6, dtMemY - 6, DCardW + 12, dtMemH + 12},
		{DPadX - 6, dtDiskY - 6, DCardW + 12, dtDiskH + 12},
		{DPadX - 6, dtTempY - 6, DCardW + 12, dtTempH + 12},
	}
}

// CardRects 入场编排单元：四张大卡自上而下依次浮入
func (p *DetailPage) CardRects() []Rect {
	return []Rect{
		{DPadX, dtCpuY, DCardW, dtCpuH},
		{DPadX, dtMemY, DCardW, dtMemH},
		{DPadX, dtDiskY, DCardW, dtDiskH},
		{DPadX, dtTempY, DCardW, dtTempH},
	}
}

// 详情页四核条原先各有一个身份色（C0 灰蓝 / C1 珊瑚 / C2 浅灰蓝 / C3 灰绿）。
// 2026-10-31 起改成**只按负载着色**（<50 绿 / >=50 黄 / >=80 红），见 barColor：
// C1 的珊瑚和告警红撞色，"变红"就没有信号意义了；哪个核看左边 C0~C3 标签即可。

func (p *DetailPage) Draw(c *Canvas, d *DrawCtx) {
	fs := d.Fonts
	m := p.mon.Snapshot()

	// ---------------- CPU 负载（每核明细）----------------
	// 首页只留「趋势」，每核分布这种细节归详情页：四行、条长 205px，
	// 两个核差 10% 时长度差 20px，一眼可辨。
	if c.ClipVisible(DPadX, dtCpuY, DCardW, dtCpuH) {
		DCardBox(c, d, DPadX, dtCpuY, DCardW, dtCpuH, colMint)
		DCardTitle(c, fs, DPadX, dtCpuY+13, "CPU 负载", colSlate)
		DMetaR(c, fs, DPadX+DCardW, dtCpuY+16,
			fmt.Sprintf("1m %.2f · 5m %.2f", m.LoadAvg[0], m.LoadAvg[1]), colDim)

		cores := m.CPUCores
		n := coreCount(m)
		if n > 4 {
			n = 4
		}
		for i := 0; i < n; i++ {
			// 首行中心 +58，行距 36：第四行中心 +166，条底 +169，卡底 +182
			cy := dtCpuY + 58 + i*36
			v := 0.0
			if i < len(cores) {
				v = clamp(cores[i], 0, 100)
			}
			DrawPartsTop(c, fs, DPadX+DPad, cy-6, FzUnit,
				[]TPart{{S: fmt.Sprintf("C%d", i), Col: colSub, Latin: true}})
			// 条的颜色只按负载走阈值（洋哥 2026-10-31：<50 绿 / >=50 黄 / >=80 红），
			// 不再用"该核身份色" —— 理由见文件里 coreBarCols 那段说明。
			c.Bar(DPadX+51, cy-6, 205, 5, 2, v/100, colTrack, barColor(v))
			DrawPartsTopR(c, fs, DPadX+DCardW-DPad, cy-6, FzUnit,
				[]TPart{{S: fmt.Sprintf("%.0f%%", v), Col: valColor(v), Latin: true}})
		}
	}

	// ---------------- 内存 ----------------
	if c.ClipVisible(DPadX, dtMemY, DCardW, dtMemH) {
		DCardBox(c, d, DPadX, dtMemY, DCardW, dtMemH, colRose)
		tw := DCardTitle(c, fs, DPadX, dtMemY+13, "内存", colRose)
		// 标题后紧跟物理容量：16 GB 的机器内核只报 15.4 GiB（核显/固件预留），
		// 两个数字都摆出来，才不会让人以为内存"缩水"了
		if m.MemPhysGB > 0 {
			DrawPartsTop(c, fs, DPadX+DPad+tw+10, dtMemY+17, FzMeta,
				splitParts(fmt.Sprintf("物理 %.0f GB", m.MemPhysGB), colDim))
		}
		memPct := clamp(d.Ease("dt.mp", m.MemPercent), 0, 100)
		DMetaR(c, fs, DPadX+DCardW, dtMemY+16, fmt.Sprintf("%.0f%%", memPct), valColor(memPct))

		// 大数字：已用容粗体、总量细体次白 —— 一眼分得清"占了多少 / 共多少"。
		// 墨迹顶 +58，字底 +93；单位降两档用 Gap 顶住，不靠空格。
		// 已用容按占用率着色（>=50 沙金 / >=80 珊瑚），低于阈值时是偏灰白。
		DrawPartsTop(c, fs, DPadX+DPad, dtMemY+58, FzData, []TPart{
			{S: fmt.Sprintf("%.1f", d.Ease("dt.mu", m.MemUsedGB)), Col: valColor(memPct), Latin: true, Bold: true},
			{S: "GB", Col: colDim, Latin: true, Size: FzUnit, Gap: 3},
			{S: "/", Col: colDim, Latin: true, Size: FzUnit, Gap: 10},
			{S: fmt.Sprintf("%.1f", m.MemTotalGB), Col: colSub, Latin: true, Gap: 10},
			{S: "GB", Col: colDim, Latin: true, Size: FzUnit, Gap: 3},
		})
		c.Bar(DPadX+DPad, dtMemY+100, DCardW-2*DPad, 5, 2,
			memPct/100, colTrack, colRose)

		DrawPartsTop(c, fs, DPadX+DPad, dtMemY+128, FzMeta, []TPart{
			{S: "缓存", Col: colDim},
			{S: fmt.Sprintf("%.1f GB", m.MemCacheGB), Col: colDim, Latin: true, Gap: 5},
			{S: "·", Col: colTab, Gap: 9},
			{S: "Swap", Col: colDim, Latin: true, Gap: 9},
			{S: fmt.Sprintf("%.1f / %.0f GB", m.SwapUsedGB, m.SwapTotalGB),
				Col: colDim, Latin: true, Gap: 5},
		})
	}

	// ---------------- 磁盘（逐盘容量 + 温度）----------------
	if c.ClipVisible(DPadX, dtDiskY, DCardW, dtDiskH) {
		DCardBox(c, d, DPadX, dtDiskY, DCardW, dtDiskH, colMint)
		DCardTitle(c, fs, DPadX, dtDiskY+13, "磁盘", colMint)
		vols := m.Disk.GetVolumes()
		if len(vols) == 0 {
			DrawPartsTop(c, fs, DPadX+DPad, dtDiskY+56, FzCard,
				splitParts("未检测到存储卷", colDim))
		} else {
			// 三行：首行中心 +59、行距 36。条从 +86 起、宽 170，
			// 名字列与条列分开，三行的条起点才是对齐的。
			for i, v := range vols {
				if i >= 3 {
					break
				}
				cy := dtDiskY + 59 + i*36
				DrawPartsTop(c, fs, DPadX+DPad, cy-6, FzUnit,
					splitParts(trim(v.VolName, 5), colSub))
				c.Bar(DPadX+86, cy-7, 170, 5, 2, v.UsedPercent/100, colTrack, diskBarColor(v.UsedPercent))
				// 右端显示**已用百分比**，不再显示温度（洋哥 2026-10-30）。
				// 温度在「温度与风扇」卡里已经有硬盘那一档，这里重复是噪音；
				// 使用率才是"这块盘要不要清理"的直接答案。
				// 磁盘只有两档：<80 常态、>=80 沙金（洋哥只说黄，没要红）。
				pctCol := valColor(v.UsedPercent)
				if pctCol == colCoral {
					pctCol = colSand
				}
				DrawPartsTopR(c, fs, DPadX+DCardW-DPad, cy-6, FzUnit,
					[]TPart{{S: fmt.Sprintf("%.0f%%", v.UsedPercent), Col: pctCol, Latin: true}})
			}
		}
	}

	// ---------------- 温度与风扇 ----------------
	if c.ClipVisible(DPadX, dtTempY, DCardW, dtTempH) {
		DCardBox(c, d, DPadX, dtTempY, DCardW, dtTempH, colSand)
		DCardTitle(c, fs, DPadX, dtTempY+13, "温度与风扇", colSand)

		cpuT := clamp(d.Ease("dt.ct", m.CPUTemp), 0, 120)
		diskT := clamp(d.Ease("dt.disk", m.Disk.MaxDiskTemp()), 0, 120)
		// 双列：数字墨迹顶 +61（48px 粗体），标签落在数字的视觉中线上。
		// 温度同样走阈值着色：>=50 沙金、>=80 珊瑚（洋哥 2026-10-30）。
		nw := DrawPartsTop(c, fs, DPadX+DPad, dtTempY+61, FzData, []TPart{
			{S: tempText(cpuT), Col: valColor(cpuT), Latin: true, Bold: true},
		})
		DrawPartsTop(c, fs, DPadX+DPad+nw+7, dtTempY+76, FzUnit,
			[]TPart{{S: "CPU", Col: colDim, Latin: true}})
		dw := DrawPartsTop(c, fs, DPadX+168, dtTempY+61, FzData, []TPart{
			{S: tempText(diskT), Col: valColor(diskT), Latin: true, Bold: true},
		})
		DrawPartsTop(c, fs, DPadX+168+dw+7, dtTempY+76, FzUnit,
			[]TPart{{S: "硬盘", Col: colDim}})

		cpuRPM, boxRPM := p.fanSpeeds(d)
		DrawPartsTop(c, fs, DPadX+DPad, dtTempY+113, FzMeta, []TPart{
			{S: "风扇", Col: colDim},
			{S: "CPU", Col: colDim, Latin: true, Gap: 8},
			{S: cpuRPM, Col: colSub, Latin: true, Gap: 4},
			{S: "·", Col: colTab, Gap: 7},
			{S: "机箱", Col: colDim, Gap: 7},
			{S: boxRPM, Col: colSub, Latin: true, Gap: 4},
			{S: "·", Col: colTab, Gap: 7},
			{S: fanModeCN(d.Fan.GetCPUMode()), Col: colDim, Gap: 7},
		})
	}
}

// fanSpeeds 风扇转速文本（纯数字；0 视为读不到 → —；单位由调用行统一给出）
func (p *DetailPage) fanSpeeds(d *DrawCtx) (string, string) {
	cpuRPM, boxRPM := 0, 0
	if d.Fan != nil {
		cpuRPM, boxRPM = d.Fan.GetFanSpeeds()
	}
	f := func(v int) string {
		if v <= 0 {
			return "—"
		}
		return fmt.Sprintf("%d", v)
	}
	return f(cpuRPM), f(boxRPM)
}

// fanModeCN 风扇模式中文名
func fanModeCN(m string) string {
	switch m {
	case "auto":
		return "自动"
	case "manual":
		return "手动"
	case "full":
		return "全速"
	default:
		return m
	}
}

// ============================================================
//  第 3 页 · 系统
// ============================================================

type SysPage struct {
	mon *monitor.SystemMonitor
	scr *screen.Control
}

func NewSysPage(mon *monitor.SystemMonitor, scr *screen.Control) *SysPage {
	return &SysPage{mon: mon, scr: scr}
}

func (p *SysPage) PageName() string { return "系统" }

// 系统页四张卡 + 药丸。
//
// 2026-10-30 重新按参考图逐像素校准：四张卡的卡面分别是
// 132..291.5 / 304..451.5 / 464..671.5 / 684..861.5（高 160/148/208/178），
// 卡间隙恒定 12.5。上一版为了把药丸拉回 838 而把每张卡各收 6~9px，
// 结果第二张卡起就整体上移 ——「卡片高度对齐」是硬指标，不能为了
// 三页底部齐平去改单页布局（系统页本来就比另两页长）。
const (
	syDevY, syDevH   = 136, 152
	syNetY, syNetH   = 308, 140
	sySpdY, sySpdH   = 468, 200
	syCtrlY, syCtrlH = 688, 170
	syPillY          = 874
)

// TabY 系统页页签比另两页低 29px：这一页多一行令牌药丸（874..913），
// 页签被推到 937。参考图三栏实测如此，不是绘制误差。
func (p *SysPage) TabY() int { return DSysTabY }

func (p *SysPage) DataRects(d *DrawCtx) []Rect {
	return []Rect{
		{DPadX - 6, syDevY - 6, DCardW + 12, syDevH + 12},
		{DPadX - 6, syNetY - 6, DCardW + 12, syNetH + 12},
		{DPadX - 6, sySpdY - 6, DCardW + 12, sySpdH + 12},
		{DPadX - 6, syCtrlY - 6, DCardW + 12, syCtrlH + 12},
		{DPadX - 6, syPillY - 6, DCardW + 12, 40 + 12},
	}
}

// CardRects 入场编排单元：四张大卡 + 底部药丸
func (p *SysPage) CardRects() []Rect {
	return []Rect{
		{DPadX, syDevY, DCardW, syDevH},
		{DPadX, syNetY, DCardW, syNetH},
		{DPadX, sySpdY, DCardW, sySpdH},
		{DPadX, syCtrlY, DCardW, syCtrlH},
		{DPadX, syPillY, DCardW, 40},
	}
}

// linkBars 链路速率 → 信号条点亮根数。
//
// 16 根条只有"几根亮"这一个自由度，所以映射必须粗、必须单调：
// 10M=5、100M=8、1G=11、2.5G=14、10G=16。正好对得上参考图里 1G 口的 11 根。
func linkBars(mbps int) int {
	switch {
	case mbps >= 10000:
		return 16
	case mbps >= 2500:
		return 14
	case mbps >= 1000:
		return 11
	case mbps >= 100:
		return 8
	case mbps >= 10:
		return 5
	case mbps > 0:
		return 3
	}
	return 0
}

func (p *SysPage) Draw(c *Canvas, d *DrawCtx) {
	fs := d.Fonts
	m := p.mon.Snapshot()

	// ---------------- 设备 ----------------
	if c.ClipVisible(DPadX, syDevY, DCardW, syDevH) {
		DCardBox(c, d, DPadX, syDevY, DCardW, syDevH, colSlate)
		DCardTitle(c, fs, DPadX, syDevY+13, "设备", colSlate)
		DMetaR(c, fs, DPadX+DCardW, syDevY+16, Version, colDim)

		// 主机名：36px 细体，按**像素宽度**裁剪（14 个中文字能铺 500+px）
		DrawPartsTop(c, fs, DPadX+DPad, syDevY+49, FzPage,
			splitParts(fitText(fs, d.Host, FzPage, DCardW-2*DPad), colWhite))

		// 两行规格：只放**读得到真值**的字段，读不到给「—」。
		// 内存写物理安装容量（SMBIOS），与商品页 16 GB 一致；
		// 运行态「已用 / 内核可见」仍在详情页内存卡，两边不混。
		model := m.CPUModel
		if model == "" {
			model = "—"
		}
		DrawPartsTop(c, fs, DPadX+DPad, syDevY+91, FzCard, []TPart{
			{S: "飞牛 fnOS", Col: colDim},
			{S: fitText(fs, model, FzCard, 150), Col: colDim, Latin: true, Gap: 9},
			{S: fmt.Sprintf("%d 核", coreCount(m)), Col: colDim, Gap: 9},
		})

		memTxt := fmt.Sprintf("%.1f GB", m.MemTotalGB)
		if m.MemPhysGB > 0 {
			memTxt = fmt.Sprintf("%.0f GB", m.MemPhysGB)
		}
		kernel := m.Kernel
		if kernel == "" {
			kernel = "—"
		}
		DrawPartsTop(c, fs, DPadX+DPad, syDevY+115, FzMeta, []TPart{
			{S: "内核", Col: colDim},
			{S: fitText(fs, kernel, FzMeta, 190), Col: colDim, Latin: true, Gap: 7},
			{S: "·", Col: colTab, Gap: 8},
			{S: memTxt, Col: colDim, Latin: true, Gap: 8},
		})
	}

	// ---------------- 网络（链路速率 / 公网可达性，信号条表达）----------------
	if c.ClipVisible(DPadX, syNetY, DCardW, syNetH) {
		DCardBox(c, d, DPadX, syNetY, DCardW, syNetH, colMint)
		DCardTitle(c, fs, DPadX, syNetY+13, "网络", colMint)

		// 局域网：**显示本机在该网口上的 IPv4**（洋哥 2026-10-30：局域网和公网
		// IP 都要如实）。接口名 enp2s0-ovs 回答不了"这台机器在哪、该怎么连它"，
		// IP 才回答得了；网口名在「详情 → 系统」里另有位置。
		//
		// 右侧那两排 16 根信号条已按要求去掉（洋哥 2026-10-30）—— 它们原本
		// 用根数表示协商速率/公网可达，但速率文本本来就在下一张卡里、
		// 公网可达看有没有 IP 更直接，两块装饰反而把卡挤成"左文右图"。
		// 去掉后按「标题 / 两行 / 卡底」三段仍等距重新排：卡面 304..452、
		// 标题墨迹 321..338、两行各 17 高 → 三段留白各 26.7，
		// 于是两行墨迹顶落在 365 / 408（相对内容框 +57 / +100）。
		// 标签靠左、地址靠右 —— 两端对齐（洋哥 2026-10-31）。
		//
		// 上一版是"标签左对齐 + 地址从固定列起"：地址确实在同一条竖线上，
		// 但右边留了 150px 空白，卡片看着没被撑开，等于没对齐。
		// 现在两行都各自贴住卡内左右边距，左边缘齐、右边缘也齐。
		lan := m.LocalIP
		if lan == "" {
			lan = "—"
		}
		DrawPartsTop(c, fs, DPadX+DPad, syNetY+57, FzCard,
			[]TPart{{S: "局域网", Col: colDim}})
		DrawPartsTopR(c, fs, DPadX+DCardW-DPad, syNetY+57, FzCard,
			[]TPart{{S: fitText(fs, lan, FzCard, 200), Col: colSub, Latin: true}})

		// 公网：只有"通 / 不通 / 还在查"三态 —— 有 IP 就是通，
		// 取不到就直说"获取失败"，不用信号条去暗示强弱。
		pub, pubCol := "获取中", colDim
		if d.Net != nil {
			pi := d.Net.PublicIP()
			if pi.OK && pi.IP != "" {
				pub, pubCol = pi.IP, colSub
			} else if !pi.TriedAt.IsZero() {
				pub, pubCol = "获取失败", colDim
			}
		}
		DrawPartsTop(c, fs, DPadX+DPad, syNetY+100, FzCard,
			[]TPart{{S: "公网", Col: colDim}})
		DrawPartsTopR(c, fs, DPadX+DCardW-DPad, syNetY+100, FzCard,
			[]TPart{{S: fitText(fs, pub, FzCard, 200), Col: pubCol, Latin: true}})
	}

	// ---------------- 上传 · 下载 ----------------
	if c.ClipVisible(DPadX, sySpdY, DCardW, sySpdH) {
		DCardBox(c, d, DPadX, sySpdY, DCardW, sySpdH, colMint)
		DCardTitle(c, fs, DPadX, sySpdY+13, "上传 · 下载", colMint)
		up := clamp(d.Ease("sy.up", m.NetSendMBps), 0, 1e9)
		dn := clamp(d.Ease("sy.dn", m.NetRecvMBps), 0, 1e9)

		// 两列：左 上传、右 下载，各占半卡
		DrawPartsTop(c, fs, DPadX+DPad, sySpdY+41, FzMeta,
			[]TPart{{S: "上传", Col: colDim}})
		uw := DrawPartsTop(c, fs, DPadX+DPad, sySpdY+72, FzNum, []TPart{
			{S: fmtMBps(up), Col: colData, Latin: true, Bold: true},
		})
		DrawPartsTop(c, fs, DPadX+DPad+uw+6, sySpdY+86, FzMeta,
			[]TPart{{S: "MB/s", Col: colDim, Latin: true}})

		DrawPartsTop(c, fs, DPadX+176, sySpdY+41, FzMeta,
			[]TPart{{S: "下载", Col: colDim}})
		dw := DrawPartsTop(c, fs, DPadX+176, sySpdY+72, FzNum, []TPart{
			{S: fmtMBps(dn), Col: colData, Latin: true, Bold: true},
		})
		DrawPartsTop(c, fs, DPadX+176+dw+6, sySpdY+86, FzMeta,
			[]TPart{{S: "MB/s", Col: colDim, Latin: true}})

		send := make([]float64, 0, len(m.NetHistory))
		recv := make([]float64, 0, len(m.NetHistory))
		scale := 0.1
		for _, v := range m.NetHistory {
			send = append(send, v[0])
			recv = append(recv, v[1])
			if v[0] > scale {
				scale = v[0]
			}
			if v[1] > scale {
				scale = v[1]
			}
		}
		scale *= 1.15
		// 曲线框 577..626（h=50）：三条网格实测 589 / 602 / 614.5，等距 12.75。
		// 上一版 h=46 时网格间距 11.5，三条线整体偏上 3px —— 网格间距=框高/4，
		// 所以框高必须按"网格间距 × 4"反推，不能只看框顶。
		c.PlotFloor(DPadX+DPad, sySpdY+109, DCardW-2*DPad, 50, colMint)
		c.Graph(DPadX+DPad, sySpdY+109, DCardW-2*DPad, 50, recv, scale, 60, colMint, 0.12)
		c.Curve(DPadX+DPad, sySpdY+109, DCardW-2*DPad, 50, send, scale, 60, colSlate, 1.5, 0)

		DrawPartsTop(c, fs, DPadX+DPad, sySpdY+175, FzMeta, []TPart{
			{S: "今日", Col: colDim},
			{S: "收 " + fmtBytes(m.NetRxToday), Col: colDim, Gap: 7},
			{S: "·", Col: colTab, Gap: 8},
			{S: "发 " + fmtBytes(m.NetTxToday), Col: colDim, Gap: 8},
		})
	}

	// ---------------- 控制（亮度滑杆 + 关屏按钮）----------------
	if c.ClipVisible(DPadX, syCtrlY, DCardW, syCtrlH) {
		DCardBox(c, d, DPadX, syCtrlY, DCardW, syCtrlH, colSlate)
		DCardTitle(c, fs, DPadX, syCtrlY+13, "控制", colSlate)

		// 亮度走 screen.Control：它是背光的唯一状态源，
		// 直接读硬件会让"Web 已息屏"这件事在界面上不可见。
		cur := p.scr.Bright()
		pct := 0.0
		if cur > 0 && p.scr.Max() > 0 {
			pct = float64(cur) / float64(p.scr.Max())
		}
		// 亮度行墨迹顶实测 737（卡顶 684 → +53）；滑杆 87..277 长 190，
		// 上一版 108..258 只有 150 —— 参考图那条杆明显更长，末端顶到「52%」左边 32px 处。
		DrawPartsTop(c, fs, DPadX+DPad, syCtrlY+49, FzCard,
			[]TPart{{S: "亮度", Col: colSub}})
		DSlider(c, DPadX+63, syCtrlY+51, 190, 4, pct, colSlate)
		DrawPartsTopR(c, fs, DPadX+DCardW-DPad, syCtrlY+49, FzCard,
			[]TPart{{S: fmt.Sprintf("%.0f%%", pct*100), Col: colSub, Latin: true}})

		DrawPartsTop(c, fs, DPadX+DPad, syCtrlY+75, FzMeta,
			[]TPart{{S: "点左半降值 · 右半升高", Col: colDim}})

		// 关屏按钮：暗玫瑰底 + 灰粉字，禁止实心正红、禁止描边
		c.FillRoundRect(DPadX+DPad, syCtrlY+100, DCardW-2*DPad, 38, 19, colBtnBg)
		DrawPartsTopC(c, fs, DPadX+DCardW/2, syCtrlY+115, FzCard,
			[]TPart{{S: "关闭屏幕", Col: colBtnTx}})
	}

	// ---------------- 面板令牌 + 数据新鲜度 ----------------
	//
	// 左边是 Web 面板令牌：屏幕挂在自家墙上，能看到屏幕的人本来就有
	// 物理接触权限 —— 令牌显示在这里比藏在 journal 里更符合威胁模型。
	// 右边是**数据时间戳**：采样协程万一挂掉，屏幕数字会停在最后一秒，
	// 只有这里能看出来。
	if c.ClipVisible(DPadX, syPillY, DCardW, 40) {
		DPill(c, DPadX, syPillY, 166, 40)
		DPillDot(c, DPadX+16, syPillY+20, colMint)
		tok := d.WebToken
		// 令牌**明文不上屏**：参考图这一格写的是「令牌」+ 一串小信号条，
		// 令牌值本身没有出现。墙上挂着的屏幕谁路过都能看见，把面板令牌
		// 印在上面等于把钥匙贴门上。这里只报"设没设置"，值去 Web 面板看。
		DrawPartsTop(c, fs, DPadX+28, syPillY+12, FzMeta,
			splitParts("令牌", colSub))
		lit := 0
		if tok != "" {
			lit = 6
		}
		DTokBars(c, DPadX+66, syPillY+24, lit, colMint)

		DPill(c, DPadX+178, syPillY, DCardW-178, 40)
		upd := "—"
		// 圆点是中性灰（参考图实测 92,92,92）：它只是"数据时间戳"的标记，
		// 不是状态灯。沙金那一档留给环境页的容器药丸（29/30 → 沙金）。
		// 超过 5 个采样周期没更新才转沙金警示 —— 那时屏幕数字已不可信。
		dot := colDim
		if !m.UpdatedAt.IsZero() {
			upd = m.UpdatedAt.Format("15:04:05")
			if time.Since(m.UpdatedAt) > 10*time.Second {
				dot = colSand
			}
		}
		DPillDot(c, DPadX+194, syPillY+20, dot)
		DrawPartsTop(c, fs, DPadX+206, syPillY+12, FzMeta,
			splitParts("数据 "+upd, colDim))
	}
}

// DTokBars 令牌药丸里那串小信号条：9 根、宽 4、间距 3，前 lit 根点亮。
//
// 高度是参考图逐列量出来的 [6,9,11,8,11,7,9,7,10]（不是规整的等差）。
// 亮色就是**纯 MINT**（实心像素实测 127,190,165）；先前量到 (71,102,92)
// 是取了条子最左列的半透明抗锯齿边，据此调成半透明反而把整排压暗了。
func DTokBars(c *Canvas, x, yBase, lit int, accent RGBA) {
	hs := [9]int{6, 9, 11, 8, 11, 7, 9, 7, 10}
	for i, h := range hs {
		col := colTrack
		if i < lit {
			col = accent
		}
		c.FillRoundRect(x+i*7, yBase-h, 4, h, 1, col)
	}
}

// Tap 触屏：亮度滑条左半 -10% / 右半 +10%；关屏按钮触发 OnScreenOff。
func (p *SysPage) Tap(x, y int, d *DrawCtx) {
	// 关屏按钮（先判：按钮在下方，与亮度行不重叠）
	if x >= DPadX+DPad && x <= DPadX+DCardW-DPad && y >= syCtrlY+100 && y <= syCtrlY+138 {
		if d.OnScreenOff != nil {
			d.OnScreenOff()
		}
		return
	}
	// 亮度行
	if y >= syCtrlY+36 && y <= syCtrlY+76 && x >= DPadX && x <= DPadX+DCardW {
		if p.scr == nil {
			return
		}
		max := p.scr.Max()
		cur := p.scr.Bright()
		if cur < 0 || max <= 0 {
			return
		}
		step := max / 10
		if step < 1 {
			step = 1
		}
		if x < DPadX+DCardW/2 {
			cur -= step
		} else {
			cur += step
		}
		if cur < 2000 {
			cur = 2000
		}
		if cur > max {
			cur = max
		}
		if p.scr.Apply(cur) == nil {
			d.BumpData()
		}
	}
}
