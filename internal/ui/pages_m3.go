package ui

// ============================================================
//  Material You 三页版（2026-10-28 洋哥拍板，10-28 深夜按效果稿复刻质感）
//
//  质感复刻要点（第一版走样的根因与修正）：
//   1. 卡面 = **实色深色相容器**（hue.bg），不是「语义色低透明涂层」——
//      橙色 alpha 28 铺在灰底上就是发褐的泥色，这是第一版质感跑样主因
//   2. 时钟 FzMega=112（第一版 96 偏小），卡片标题 FzBody、卡内大数字 48
//   3. 圆角 m3Rad=26（第一版沿用 CardRad=14，明显不够「M3」）
//   4. bar 颜色跟随所在卡的色相，≥90% 才变红（效果稿右侧注释原文）
//   5. 背景压到近纯黑（themeInk 同步调整），把彩色容器衬出来
//
//  排版纪律（2026-10-28 二改：全部改基线锚 + 统一令牌，必须遵守）：
//   - 卡内左右留白只有 m3Pad=22 一个值；标题基线固定 y+40、色相、
//     FzSmall 20 + 1px 字距（drawTracked），右上小注 m3Note 与标题**同基线**
//   - 所有文字一律**基线锚**（DrawPartsL / drawTracked）；位图顶锚要额外
//     补偿 ascent 空隙，是上一版文字压条、压标题的根源，已全页弃用
//   - 数字旁的单位降两档字号并用 TPart.Gap 顶住，不靠空格 —— 空格宽度
//     随字号线性放大，40px 旁边的一个空格有 10px，疏密完全不受控
//   - 节奏：内容首行基线 +74~+92、进度条顶 = 文字基线 +12、
//     任何墨迹距卡底 ≥ 12、卡间距统一 14
//
//  其余约定不变：满版排版、公网 IP 打码、读不到显示 — 绝不编造。
//  三页：环境（fullTop 大时钟） → 详情 → 系统
// ============================================================

import (
	"fmt"

	"r1-toolbox/internal/hardware"
	"r1-toolbox/internal/monitor"
)

const appVersion = "v0.5.0"

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

// ------------------------------------------------------------
//  M3 hue 容器（对照效果稿取色）
// ------------------------------------------------------------

const m3Rad = 26

// 卡内左右留白：全站唯一值。之前每张卡各写 18/22/24，
// 一排卡看下来左边线参差不齐，是"不齐"观感的主要来源之一。
const m3Pad = 22

type m3Hue struct{ bg, hi RGBA }

// 低饱和 hue 容器（2026-10-28 二改，洋哥要「低饱和」）。
//
// 上一版 bg 饱和 42~55%、hi 色饱和接近 100% —— 在近黑背景上像涂了荧光漆，
// 颜色抢过数据本身。这一版把 bg 压到 16~23%、hi 压到 13~20%：
// 五种色相依然一眼可辨（蓝/紫/琥珀/绿/青），但整体是灰调里的彩色，
// 亮度靠明度拉开而不是靠饱和，这才撑得住「高级」。
var (
	hueBlue   = m3Hue{RGBA{33, 40, 53, 255}, RGBA{152, 175, 208, 255}} // CPU / 设备
	huePurple = m3Hue{RGBA{40, 35, 50, 255}, RGBA{182, 168, 202, 255}} // 内存 / 控制 / 存储
	hueAmber  = m3Hue{RGBA{48, 42, 33, 255}, RGBA{200, 180, 148, 255}} // 温度
	hueGreen  = m3Hue{RGBA{31, 43, 39, 255}, RGBA{146, 186, 168, 255}} // 网络 / 磁盘
	hueTeal   = m3Hue{RGBA{29, 43, 45, 255}, RGBA{138, 184, 188, 255}} // CPU 负载 / 上下行
)

// m3Bar 卡内进度条：常态跟随卡色相，≥pctDanger 才变红（效果稿注释约定）
func m3Bar(hue m3Hue, v, pctDanger float64, danger RGBA) RGBA {
	if v >= pctDanger {
		return danger
	}
	return hue.hi
}

// ---------------- 卡内排版令牌 ----------------
//
// 之前每张卡各写各的坐标，标题忽高忽低、内容忽松忽紧，是"排版不齐"的根。
// 现在所有卡共用一套纵向节奏（相对卡顶，单位 px）：
//
//	标题基线            +40   （FzSmall 20，字距 1px，墨迹 ≈ +25~+40）
//	内容首行基线        +74 ~ +92
//	进度条顶            文字基线 +12
//	任何墨迹距卡底      ≥ 14
//
// 全部用**基线锚**（DrawPartsL / drawTracked）定位 —— 位图顶锚要额外补偿
// ascent 空隙，是之前文字压条、压标题的根源，已全部弃用。
const (
	m3TitleBase = 40 // 标题基线（相对卡顶）
)

// m3Card 容器卡 + 标题（FzSmall + 1px 字距，基线 y+m3TitleBase）
func m3Card(c *Canvas, fs *FontSet, x, y, w, h int, hue m3Hue, title string) {
	c.FillRoundRect(x, y, w, h, m3Rad, hue.bg)
	drawTracked(c, fs, title, x+m3Pad, y+m3TitleBase, FzSmall, hue.hi, 1)
}

// m3Note 卡头右侧小注：**与标题同基线**右对齐（之前用中心锚，且比标题低 8px，
// 一左一右两个元素基线不齐，扫一眼就觉得歪）
func m3Note(c *Canvas, fs *FontSet, xRight, yBase int, th *Theme, text string) {
	parts := splitParts(text, th.Dim)
	w, _ := measureParts(fs, FzTiny, parts)
	DrawPartsL(c, fs, xRight-w, yBase, FzTiny, parts)
}

// hintOf 页面顶部右侧的滑动提示（对照效果稿）
func hintOf(name string) string {
	switch name {
	case "详情":
		return "滑左翻页"
	case "系统":
		return "滑左回首页"
	}
	return ""
}

// ============================================================
//  第 1 页 · 环境（fullTop：大时钟 + 天气融合，六块内容满版）
// ============================================================

type EnvPage struct{ mon *monitor.SystemMonitor }

func NewEnvPage(mon *monitor.SystemMonitor) *EnvPage { return &EnvPage{mon: mon} }

func (p *EnvPage) PageName() string { return "环境" }
func (p *EnvPage) FullTop() bool    { return true }

// 首页纵向布局。
//
// 所有高度由「实测墨迹 + 固定层间距」反推，不再凭感觉给：
//
//	标题墨迹       +19~+36（FzSmall 20 / 基线 +40）
//	标题底→内容顶   20
//	内容底→bar 顶   12
//	bar 底→下一层   16
//	末层底→卡底     18
const (
	envTopH   = 184
	envCardsY = 200
	envCardsH = 122
	envCoresY = 336
	envCoresH = 102
	envNetY   = 452
	envNetH   = 158
	envDiskY  = 624
	envDiskH  = 137
	envPillY  = 782
	envPillH  = 40
)

func (p *EnvPage) DataRects(d *DrawCtx) []Rect {
	// 大时钟区不在列：分钟由 renderer 的 fullTop 分支单独局部重画；
	// 天气快照变化走全局的天气戳全屏重画。
	return []Rect{
		{PadX - 6, envCardsY - 6, CardW + 12, envCardsH + 12},
		{PadX - 6, envCoresY - 6, CardW + 12, envCoresH + 12},
		{PadX - 6, envNetY - 6, CardW + 12, envNetH + 12},
		{PadX - 6, envDiskY - 6, CardW + 12, envDiskH + 12},
		{PadX - 6, envPillY - 6, CardW + 12, envPillH + 12},
	}
}

func (p *EnvPage) Draw(c *Canvas, d *DrawCtx) {
	th, fs := d.Theme, d.Fonts
	m := p.mon

	// ---------------- 大时钟 + 日期天气行（顶栏接管区）----------------
	if c.ClipVisible(0, 0, c.W, envTopH) {
		tim := glyphLatin(fs, FzMega, d.Now.Format("15:04"), th.Text)
		blitGlyph(c, tim, PadX, 10)

		wx := d.WeatherNow()
		kind := wx.Kind
		if kind == "" {
			kind = "overcast"
		}
		wxAcc := weatherAccent(kind)

		parts := []TPart{
			{S: d.Now.Format("01月02日") + " " + weekdayCN(d.Now), Col: th.Sub},
		}
		if wx.OK {
			parts = append(parts,
				TPart{S: "  ·  ", Col: th.Faint},
				TPart{S: wx.Desc, Col: Lit(wxAcc, 0.30)},
				TPart{S: "  ", Col: th.Faint},
				TPart{S: fmt.Sprintf("%.0f°", wx.TempC), Col: th.Sub, Latin: true},
			)
		}
		DrawPartsL(c, fs, PadX+2, 166, FzBody, parts)

		// 天气图标（时钟右上，圆心+半径语义）：昼夜由 isNight 内部判断
		DrawWeatherIcon(c, PadX+CardW-46, 44, 32, kind, 2.856)
	}

	// ---------------- 三张指标小卡（CPU 蓝 / 内存 紫 / 温度 琥珀）----------------
	// 104px 窄卡，左右各留 14：可用 76px。
	// 数字 36px + 单位 17px（Gap 3）："100%" 实测 71px，刚好不顶边；
	// 40px 会到 78px 直接溢出，这是上一版文字贴卡边的原因。
	if c.ClipVisible(PadX, envCardsY, CardW, envCardsH) {
		type mini struct {
			label string
			txt   string
			unit  string
			v     float64
			hue   m3Hue
		}
		cpu := clamp(d.Ease("env.cpu", m.CPUPercent), 0, 100)
		mem := clamp(d.Ease("env.mem", m.MemPercent), 0, 100)
		temp := clamp(d.Ease("env.temp", m.CPUTemp), 0, 120)
		cards := []mini{
			{"CPU", fmt.Sprintf("%.0f", cpu), "%", cpu, hueBlue},
			{"内存", fmt.Sprintf("%.0f", mem), "%", mem, huePurple},
			{"温度", fmt.Sprintf("%.0f", temp), "°", temp, hueAmber},
		}
		for i, mc := range cards {
			cx := PadX + i*(104+12)
			c.FillRoundRect(cx, envCardsY, 104, envCardsH, m3Rad, mc.hue.bg)
			// 窄卡内边距 12（外框左缘与其他卡对齐，内部不必用 22）
			drawTracked(c, fs, mc.label, cx+12, envCardsY+40, FzSmall, mc.hue.hi, 1)
			// 38px 数字基线 +91（墨迹 [57, 86]）；"100%" 实测 78px，可用宽 80
			// 单位用 FzSmall + Gap 1 紧贴数字：17px 时 % 太小又离得远，
			// 看起来像挂在数字右下角的脚注（2026-10-28 抓屏第 2 轮修正）
			DrawPartsL(c, fs, cx+12, envCardsY+91, 36, []TPart{
				{S: mc.txt, Col: th.Text, Latin: true},
				{S: mc.unit, Col: th.Sub, Latin: true, Size: FzBody, Gap: 1},
			})
			// bar 顶 +98（数字墨迹底 +86 之下 12），底 +104，卡底 +122 → 18 留白
			c.Bar(cx+12, envCardsY+98, 80, 6, 3, clamp(mc.v, 0, 100)/100,
				th.Track, m3Bar(mc.hue, mc.v, 90, th.Danger))
		}
	}

	// ---------------- CPU 负载（每核迷你条，单行）----------------
	if c.ClipVisible(PadX, envCoresY, CardW, envCoresH) {
		m3Card(c, fs, PadX, envCoresY, CardW, envCoresH, hueTeal, "CPU 负载")
		m3Note(c, fs, PadX+CardW-m3Pad, envCoresY+m3TitleBase, th,
			fmt.Sprintf("负载 %.2f", m.LoadAvg[0]))

		cores := m.CPUCores
		n := coreCount(m)
		if n > 4 {
			n = 4
		}
		cellW := 71 // (328 - 22*2) / 4 = 71
		for i := 0; i < n; i++ {
			cx := PadX + m3Pad + i*cellW
			// DrawTextL 位图顶 +51 → 墨迹 [56, 67]（17px 字号 ascent 空隙 5px）
			DrawTextL(c, fs, fmt.Sprintf("C%d", i), cx, envCoresY+51, FzTiny, th.Sub)
			v := 0.0
			if i < len(cores) {
				v = clamp(cores[i], 0, 100)
			}
			// 条顶 +78（标签墨迹底 +67 之下 11），高 6 底 +84，卡底 +102 → 18
			c.Bar(cx, envCoresY+78, 62, 6, 3, v/100,
				th.Track, m3Bar(hueTeal, v, 90, th.Danger))
		}
	}

	// ---------------- 网络（双速率 + 双曲线）----------------
	if c.ClipVisible(PadX, envNetY, CardW, envNetH) {
		m3Card(c, fs, PadX, envNetY, CardW, envNetH, hueGreen, "网络")
		up := d.Ease("env.up", m.NetSendMBps)
		dn := d.Ease("env.dn", m.NetRecvMBps)
		// DrawRateRow 的 cy 是数字墨迹垂直中心：两行 cy +54 / +78
		// （墨迹 [44,63] / [68,87]），与标题墨迹 [19,36] 错开不叠
		DrawRateRow(c, fs, th, PadX+CardW-m3Pad, envNetY+58, up, P.NetUp, true)
		DrawRateRow(c, fs, th, PadX+CardW-m3Pad, envNetY+82, dn, P.NetDown, false)

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
		// 曲线顶 +96（行2 墨迹底 +87 之下 9px，图形元素不套用 16 的文字间距），
		// 高 44，底 +140，卡底 +158 → 18
		c.Curve(PadX+m3Pad, envNetY+100, CardW-2*m3Pad, 40, recv, scale, 60, P.NetDown, 2, 0.26)
		c.Curve(PadX+m3Pad, envNetY+100, CardW-2*m3Pad, 40, send, scale, 60, P.NetUp, 2, 0)
	}

	// ---------------- 存储（容量最大的卷）----------------
	if c.ClipVisible(PadX, envDiskY, CardW, envDiskH) {
		m3Card(c, fs, PadX, envDiskY, CardW, envDiskH, huePurple, "存储")
		vols := m.Disk.GetVolumes()
		if len(vols) == 0 {
			DrawText(c, fs, "未检测到存储卷", PadX+m3Pad, envDiskY+68, FzSmall, th.Dim)
		} else {
			// 按总容量降序取最大卷 —— /vol1 这类主数据卷排在系统盘前面
			big := vols[0]
			for _, v := range vols[1:] {
				if v.TotalGB > big.TotalGB {
					big = v
				}
			}
			if big.Temp > 0 {
				m3Note(c, fs, PadX+CardW-m3Pad, envDiskY+m3TitleBase, th,
					fmt.Sprintf("%.0f°C", big.Temp))
			}
			// 卷名行盒顶 +52 → 墨迹 [56,73] / bar 顶 +85（底 +91）/
			// 容量基线 +119（墨迹 [107,119]），卡底 +137 → 18
			DrawTextMixL(c, fs, PadX+m3Pad, envDiskY+52, FzSmall, th.Text,
				trim(big.VolName, 8))
			c.Bar(PadX+m3Pad, envDiskY+85, CardW-2*m3Pad, 6, 3, big.UsedPercent/100,
				th.Track, m3Bar(huePurple, big.UsedPercent, 90, th.Danger))
			DrawPartsL(c, fs, PadX+m3Pad, envDiskY+119, FzTiny, []TPart{
				{S: fmt.Sprintf("%.0f", big.UsedGB), Col: th.Sub, Latin: true},
				{S: "/", Col: th.Dim, Latin: true, Gap: 7},
				{S: fmt.Sprintf("%.0f GB", big.TotalGB), Col: th.Dim, Latin: true, Gap: 7},
				{S: fmt.Sprintf("%.0f%%", big.UsedPercent), Col: th.Dim, Latin: true, Gap: 10},
			})
		}
	}

	// ---------------- 底部双 pill（容器 / 运行时长）----------------
	if c.ClipVisible(PadX, envPillY, CardW, envPillH) {
		// 容器 pill：绿点 = Docker 可达且全部运行；读不到 = 灰点 + —
		ptxt, dot := "容器 —", RGBA{150, 155, 162, 255}
		if m.ContainersRunning >= 0 {
			ptxt = fmt.Sprintf("容器 %d/%d 运行中", m.ContainersRunning, m.ContainersTotal)
			if m.ContainersRunning == m.ContainersTotal {
				dot = P.NetDown
			} else {
				dot = th.Warn
			}
		}
		c.FillRoundRect(PadX, envPillY, 158, envPillH, envPillH/2, WithA(th.Text, 10))
		c.FillCircle(PadX+22, envPillY+20, 4, dot)
		DrawTextMixL(c, fs, PadX+34, envPillY+11, FzTiny, th.Sub, ptxt)

		c.FillRoundRect(PadX+170, envPillY, CardW-170, envPillH, envPillH/2, WithA(th.Text, 10))
		DrawTextMixL(c, fs, PadX+192, envPillY+11, FzTiny, th.Sub,
			"已运行 "+formatUptime(m.UptimeSec))
	}
}

// coreCount 逻辑核心数（页 1 每核条用）
func coreCount(m *monitor.SystemMonitor) int {
	if m.CPUCoreNum > 0 {
		return m.CPUCoreNum
	}
	return len(m.CPUCores)
}

// ============================================================
//  第 2 页 · 详情（CPU 历史大曲线 / 内存明细 / 逐盘 / 温度风扇）
// ============================================================

type DetailPage struct{ mon *monitor.SystemMonitor }

func NewDetailPage(mon *monitor.SystemMonitor) *DetailPage {
	return &DetailPage{mon: mon}
}

func (p *DetailPage) PageName() string { return "详情" }

const (
	dtCpuY, dtCpuH   = 104, 183
	dtMemY, dtMemH   = 301, 155
	dtDiskY, dtDiskH = 470, 211
	dtTempY, dtTempH = 695, 155
)

func (p *DetailPage) DataRects(d *DrawCtx) []Rect {
	return []Rect{
		{PadX - 6, dtCpuY - 6, CardW + 12, dtCpuH + 12},
		{PadX - 6, dtMemY - 6, CardW + 12, dtMemH + 12},
		{PadX - 6, dtDiskY - 6, CardW + 12, dtDiskH + 12},
		{PadX - 6, dtTempY - 6, CardW + 12, dtTempH + 12},
	}
}

func (p *DetailPage) Draw(c *Canvas, d *DrawCtx) {
	th, fs := d.Theme, d.Fonts
	m := p.mon

	// ---------------- CPU 历史 · 60 秒（大曲线，蓝卡）----------------
	if c.ClipVisible(PadX, dtCpuY, CardW, dtCpuH) {
		m3Card(c, fs, PadX, dtCpuY, CardW, dtCpuH, hueBlue, "CPU 历史 · 60 秒")
		peak := 0.0
		for _, v := range m.CPUHistory {
			if v > peak {
				peak = v
			}
		}
		m3Note(c, fs, PadX+CardW-m3Pad, dtCpuY+m3TitleBase, th,
			fmt.Sprintf("峰值 %.0f%%", peak))
		// 曲线区：顶 +56（标题墨迹底 +36 之下 20），高 76，底 +132
		for _, gy := range []int{25, 50} {
			DrawHairline(c, th, PadX+m3Pad, dtCpuY+56+gy, CardW-2*m3Pad)
		}
		c.Curve(PadX+m3Pad, dtCpuY+56, CardW-2*m3Pad, 76, m.CPUHistory,
			autoTop(m.CPUHistory, 20), 60, hueBlue.hi, 2, 0.20)
		// 当前值墨迹顶 +148（曲线底 +132 之下 16）→ 基线 +165，卡底 +183 → 18
		DrawPartsL(c, fs, PadX+m3Pad, dtCpuY+165, FzSmall, []TPart{
			{S: "当前", Col: th.Dim},
			{S: fmt.Sprintf("%.0f%%", m.CPUPercent), Col: th.Sub, Latin: true, Gap: 6},
		})
	}

	// ---------------- 内存（绝对量 + 缓存/Swap，紫卡）----------------
	if c.ClipVisible(PadX, dtMemY, CardW, dtMemH) {
		m3Card(c, fs, PadX, dtMemY, CardW, dtMemH, huePurple, "内存")
		m3Note(c, fs, PadX+CardW-m3Pad, dtMemY+m3TitleBase, th,
			fmt.Sprintf("%.0f%%", m.MemPercent))
		// 大数字基线 +89（墨迹 [56,89]）；单位降两档并用 Gap 顶住，不留大空格
		DrawPartsL(c, fs, PadX+m3Pad, dtMemY+89, FzLarge, []TPart{
			{S: fmt.Sprintf("%.1f", m.MemUsedGB), Col: th.Text, Latin: true},
			{S: "GB", Col: th.Sub, Latin: true, Size: FzSmall, Gap: 4},
			{S: "/", Col: th.Dim, Latin: true, Size: FzBody, Gap: 10},
			{S: fmt.Sprintf("%.0f", m.MemTotalGB), Col: th.Sub, Latin: true, Gap: 10},
			{S: "GB", Col: th.Dim, Latin: true, Size: FzSmall, Gap: 4},
		})
		// bar 顶 +101（数字墨迹底 +89 之下 12），底 +107
		c.Bar(PadX+m3Pad, dtMemY+101, CardW-2*m3Pad, 6, 3, m.MemPercent/100,
			th.Track, m3Bar(huePurple, m.MemPercent, 90, th.Danger))
		// 缓存行墨迹顶 +123（bar 底 +107 之下 16）→ 基线 +137，卡底 +155 → 18
		DrawPartsL(c, fs, PadX+m3Pad, dtMemY+137, FzTiny, []TPart{
			{S: "缓存", Col: th.Dim},
			{S: fmt.Sprintf("%.1f GB", m.MemCacheGB), Col: th.Dim, Latin: true, Gap: 5},
			{S: "·", Col: th.Track, Gap: 9},
			{S: "Swap", Col: th.Dim, Latin: true, Gap: 9},
			{S: fmt.Sprintf("%.1f / %.0f GB", m.SwapUsedGB, m.SwapTotalGB),
				Col: th.Dim, Latin: true, Gap: 5},
		})
	}

	// ---------------- 磁盘（逐盘容量 + 温度，绿卡）----------------
	if c.ClipVisible(PadX, dtDiskY, CardW, dtDiskH) {
		m3Card(c, fs, PadX, dtDiskY, CardW, dtDiskH, hueGreen, "磁盘")
		vols := m.Disk.GetVolumes()
		if len(vols) == 0 {
			DrawText(c, fs, "未检测到存储卷", PadX+m3Pad, dtDiskY+68, FzSmall, th.Dim)
		} else {
			// 每组 51px（bar 底 → 下一行墨迹顶 16）：
			// 卷名基线 73/124/175（行盒顶 = 基线-21），bar 顶 = 基线+12，高 5
			for i, v := range vols {
				if i >= 3 {
					break
				}
				vy := dtDiskY + 73 + i*51
				DrawTextMixL(c, fs, PadX+m3Pad, vy-21, FzSmall, th.Text, trim(v.VolName, 8))
				if v.Temp > 0 {
					DrawTextLR(c, fs, fmt.Sprintf("%.0f°C", v.Temp),
						PadX+CardW-m3Pad, vy-17, FzTiny, th.Dim)
				}
				c.Bar(PadX+m3Pad, vy+12, CardW-2*m3Pad, 5, 2, v.UsedPercent/100,
					th.Track, m3Bar(hueGreen, v.UsedPercent, 90, th.Danger))
			}
		}
	}

	// ---------------- 温度与风扇（琥珀卡）----------------
	if c.ClipVisible(PadX, dtTempY, CardW, dtTempH) {
		m3Card(c, fs, PadX, dtTempY, CardW, dtTempH, hueAmber, "温度与风扇")
		cpuT := clamp(d.Ease("dt.ct", m.CPUTemp), 0, 120)
		diskT := clamp(d.Ease("dt.disk", m.Disk.MaxDiskTemp()), 0, 120)
		// 双列：列1 x=m3Pad，列2 x=168。
		// 数字基线 +89；**标签单独画、基线抬 6**（同基线时 17px 标签吊在
		// 大数字下沿，视觉上下坠），抬 6 后落在数字的视觉中线上。
		nw := DrawPartsL(c, fs, PadX+m3Pad, dtTempY+89, FzLarge, []TPart{
			{S: fmt.Sprintf("%.0f°", cpuT), Col: th.Text, Latin: true},
		})
		DrawPartsL(c, fs, PadX+m3Pad+nw+7, dtTempY+83, FzTiny,
			[]TPart{{S: "CPU", Col: th.Sub, Latin: true}})
		dw := DrawPartsL(c, fs, PadX+168, dtTempY+89, FzLarge, []TPart{
			{S: fmt.Sprintf("%.0f°", diskT), Col: th.Text, Latin: true},
		})
		DrawPartsL(c, fs, PadX+168+dw+7, dtTempY+83, FzTiny,
			[]TPart{{S: "硬盘", Col: th.Sub}})
		c.Bar(PadX+m3Pad, dtTempY+101, 124, 6, 3, clamp(cpuT, 0, 100)/100,
			th.Track, m3Bar(hueAmber, cpuT, 82, th.Danger))
		c.Bar(PadX+168, dtTempY+101, 124, 6, 3, clamp(diskT, 0, 100)/100,
			th.Track, m3Bar(hueAmber, diskT, 82, th.Danger))

		// 风扇行墨迹顶 +123（bar 底 +107 之下 16）→ 基线 +137，卡底 +155 → 18
		cpuRPM, boxRPM := p.fanSpeeds(d)
		DrawPartsL(c, fs, PadX+m3Pad, dtTempY+137, FzTiny, []TPart{
			{S: "风扇", Col: th.Dim},
			{S: "CPU", Col: th.Dim, Latin: true, Gap: 8},
			{S: cpuRPM, Col: th.Sub, Latin: true, Gap: 4},
			{S: "·", Col: th.Track, Gap: 7},
			{S: "机箱", Col: th.Dim, Gap: 7},
			{S: boxRPM, Col: th.Sub, Latin: true, Gap: 4},
			{S: "·", Col: th.Track, Gap: 7},
			{S: fanModeCN(d.Fan.GetCPUMode()), Col: th.Dim, Gap: 7},
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
//  第 3 页 · 系统（设备 / 网络 / 上下行速度大卡 / 控制）
// ============================================================

type SysPage struct {
	mon *monitor.SystemMonitor
	br  *hardware.BrightnessController
}

func NewSysPage(mon *monitor.SystemMonitor, br *hardware.BrightnessController) *SysPage {
	return &SysPage{mon: mon, br: br}
}

func (p *SysPage) PageName() string { return "系统" }

const (
	syDevY, syDevH   = 104, 140
	syNetY, syNetH   = 258, 129
	sySpdY, sySpdH   = 401, 198
	syCtrlY, syCtrlH = 613, 173
	syPillY          = 800
)

func (p *SysPage) DataRects(d *DrawCtx) []Rect {
	return []Rect{
		{PadX - 6, syDevY - 6, CardW + 12, syDevH + 12},
		{PadX - 6, syNetY - 6, CardW + 12, syNetH + 12},
		{PadX - 6, sySpdY - 6, CardW + 12, sySpdH + 12},
		{PadX - 6, syCtrlY - 6, CardW + 12, syCtrlH + 12},
		{PadX - 6, syPillY - 6, CardW + 12, 40 + 12},
	}
}

func (p *SysPage) Draw(c *Canvas, d *DrawCtx) {
	th, fs := d.Theme, d.Fonts
	m := p.mon

	// ---------------- 设备（蓝卡）----------------
	if c.ClipVisible(PadX, syDevY, CardW, syDevH) {
		m3Card(c, fs, PadX, syDevY, CardW, syDevH, hueBlue, "设备")
		m3Note(c, fs, PadX+CardW-m3Pad, syDevY+m3TitleBase, th, appVersion)
		// 主机名基线 +85（墨迹 [55,85]）—— 标题墨迹底 +36 之下 19
		DrawPartsL(c, fs, PadX+m3Pad, syDevY+85, FzLarge,
			splitParts(trim(d.Host, 14), th.Text))
		// 副标题墨迹顶 +105（主机名墨迹底 +85 之下 20）→ 基线 +122，卡底 +140 → 18
		DrawPartsL(c, fs, PadX+m3Pad, syDevY+122, FzSmall, []TPart{
			{S: "飞牛 fnOS", Col: th.Dim},
			{S: fmt.Sprintf("%d 核", coreCount(m)), Col: th.Dim, Gap: 9},
			{S: fmt.Sprintf("%.0f GB", m.MemTotalGB), Col: th.Dim, Latin: true, Gap: 9},
		})
	}

	// ---------------- 网络（绿卡，行内「标签 + 值」；公网应用内已打码）----------------
	if c.ClipVisible(PadX, syNetY, CardW, syNetH) {
		m3Card(c, fs, PadX, syNetY, CardW, syNetH, hueGreen, "网络")
		iface := m.NetIface
		if iface == "" {
			iface = "—"
		}
		// 两行基线 +74 / +111（墨迹 [57,78] / [94,111]），卡底 +129 → 18
		DrawPartsL(c, fs, PadX+m3Pad, syNetY+74, FzSmall, []TPart{
			{S: "局域网", Col: th.Dim},
			{S: iface, Col: th.Dim, Latin: true, Gap: 7},
			{S: ipOrDash(m.LocalIP), Col: th.Text, Latin: true, Gap: 12},
		})
		pub := "获取中"
		if d.Net != nil {
			if pi := d.Net.PublicIP(); pi.OK && pi.IP != "" {
				pub = maskPubIP(pi.IP)
			}
		}
		DrawPartsL(c, fs, PadX+m3Pad, syNetY+111, FzSmall, []TPart{
			{S: "公网", Col: th.Dim},
			{S: pub, Col: th.Sub, Latin: true, Gap: 12},
		})
	}

	// ---------------- 上传 · 下载（速度大卡，青卡）----------------
	if c.ClipVisible(PadX, sySpdY, CardW, sySpdH) {
		m3Card(c, fs, PadX, sySpdY, CardW, sySpdH, hueTeal, "上传 · 下载")
		up := clamp(d.Ease("sy.up", m.NetSendMBps), 0, 1e9)
		dn := clamp(d.Ease("sy.dn", m.NetRecvMBps), 0, 1e9)

		// 两行结构：标签基线 +66（FzTiny 墨迹 [54,66]），数字基线 +108（FzLarge 墨迹 [75,108]）。
		// 左右两组各占半卡 138px，"0.01" + MB/s(17px) 实测约 118，不越界。
		DrawPartsL(c, fs, PadX+m3Pad, sySpdY+66, FzTiny,
			[]TPart{{S: "上传", Col: th.Dim}})
		uw := DrawPartsL(c, fs, PadX+m3Pad, sySpdY+108, FzLarge, []TPart{
			{S: fmtMBps(up), Col: th.Text, Latin: true},
		})
		DrawPartsL(c, fs, PadX+m3Pad+uw+6, sySpdY+102, FzTiny,
			[]TPart{{S: "MB/s", Col: th.Sub, Latin: true}})
		DrawPartsL(c, fs, PadX+186, sySpdY+66, FzTiny,
			[]TPart{{S: "下载", Col: th.Dim}})
		dw := DrawPartsL(c, fs, PadX+186, sySpdY+108, FzLarge, []TPart{
			{S: fmtMBps(dn), Col: th.Text, Latin: true},
		})
		DrawPartsL(c, fs, PadX+186+dw+6, sySpdY+102, FzTiny,
			[]TPart{{S: "MB/s", Col: th.Sub, Latin: true}})

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
		// 曲线顶 +116（数字墨迹底 +108 之下 8），高 34，底 +150
		c.Curve(PadX+m3Pad, sySpdY+116, CardW-2*m3Pad, 34, recv, scale, 60, P.NetDown, 2, 0.26)
		c.Curve(PadX+m3Pad, sySpdY+116, CardW-2*m3Pad, 34, send, scale, 60, P.NetUp, 2, 0)

		// 今日行墨迹顶 +166（曲线底 +150 之下 16）→ 基线 +180，卡底 +198 → 18
		DrawPartsL(c, fs, PadX+m3Pad, sySpdY+180, FzTiny, []TPart{
			{S: "今日", Col: th.Dim},
			{S: "收 " + fmtBytes(m.NetRxToday), Col: th.Dim, Gap: 7},
			{S: "·", Col: th.Track, Gap: 8},
			{S: "发 " + fmtBytes(m.NetTxToday), Col: th.Dim, Gap: 8},
		})
	}

	// ---------------- 控制（紫卡：亮度滑条带圆钮 + 关屏按钮）----------------
	if c.ClipVisible(PadX, syCtrlY, CardW, syCtrlH) {
		m3Card(c, fs, PadX, syCtrlY, CardW, syCtrlH, huePurple, "控制")
		cur, err := p.br.Get()
		pct := 0.0
		if err == nil && p.br.Max() > 0 {
			pct = float64(cur) / float64(p.br.Max())
		}
		// 亮度行：文字基线 +73（墨迹 [56,73]），滑条中心 +64 与其墨迹中心对齐
		DrawPartsL(c, fs, PadX+m3Pad, syCtrlY+73, FzSmall,
			[]TPart{{S: "亮度", Col: th.Sub}})
		sliderX, sliderW := PadX+84, 150
		c.Bar(sliderX, syCtrlY+61, sliderW, 6, 3, pct, th.Track, huePurple.hi)
		c.FillCircle(float64(sliderX+int(float64(sliderW)*pct)), syCtrlY+64, 6, th.Sub)
		// 百分比**基线锚**右对齐：与「亮度」同基线（上一版用位图顶锚，低了 6px）
		DrawPartsR(c, fs, PadX+CardW-m3Pad, syCtrlY+73, FzSmall,
			[]TPart{{S: fmt.Sprintf("%.0f%%", pct*100), Col: th.Sub, Latin: true}})
		// 提示行墨迹顶 +89（亮度墨迹底 +73 之下 16）→ 基线 +103
		DrawPartsL(c, fs, PadX+m3Pad, syCtrlY+103, FzTiny,
			[]TPart{{S: "点左半降低 · 右半升高", Col: th.Dim}})

		// 关屏按钮（保留 v0.1.0 以来的一键关屏能力）：顶 +117，高 44，底 +161
		c.FillRoundRect(PadX+m3Pad, syCtrlY+117, CardW-2*m3Pad, 44, 22, WithA(th.Danger, 34))
		DrawTextC(c, fs, "关闭屏幕", PadX+CardW/2, syCtrlY+126, FzSmall, Lit(th.Danger, 0.25))
	}

	// ---------------- 服务状态 pill ----------------
	if c.ClipVisible(PadX, syPillY, CardW, 40) {
		c.FillRoundRect(PadX, syPillY, 200, 40, 20, WithA(th.Text, 10))
		c.FillCircle(PadX+22, syPillY+20, 4, P.NetDown)
		DrawTextMixL(c, fs, PadX+34, syPillY+12, FzTiny, th.Sub,
			"服务运行中 · 夜间调暗已启用")
	}
}

// Tap 触屏：亮度滑条左半 -10% / 右半 +10%；关屏按钮触发 OnScreenOff。
func (p *SysPage) Tap(x, y int, d *DrawCtx) {
	// 关屏按钮（先判：按钮在下方，与亮度行不重叠）
	if x >= PadX+m3Pad && x <= PadX+CardW-m3Pad && y >= syCtrlY+117 && y <= syCtrlY+161 {
		if d.OnScreenOff != nil {
			d.OnScreenOff()
		}
		return
	}
	// 亮度行
	if y >= syCtrlY+50 && y <= syCtrlY+110 && x >= PadX && x <= PadX+CardW {
		if p.br == nil {
			return
		}
		max := p.br.Max()
		cur, err := p.br.Get()
		if err != nil {
			return
		}
		step := max / 10
		if step < 1 {
			step = 1
		}
		if x < PadX+CardW/2 {
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
		if p.br.Set(cur) == nil {
			d.BumpData()
		}
	}
}
