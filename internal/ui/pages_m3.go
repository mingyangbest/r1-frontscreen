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
const m3Num = 48 // 卡内大数字

type m3Hue struct{ bg, hi RGBA }

var (
	hueBlue   = m3Hue{RGBA{26, 42, 80, 255}, RGBA{130, 170, 255, 255}}   // CPU / 设备
	huePurple = m3Hue{RGBA{48, 35, 86, 255}, RGBA{186, 150, 255, 255}}   // 内存 / 控制 / 存储
	hueAmber  = m3Hue{RGBA{76, 55, 22, 255}, RGBA{255, 188, 104, 255}}   // 温度
	hueGreen  = m3Hue{RGBA{19, 54, 36, 255}, RGBA{106, 218, 156, 255}}   // 网络 / 磁盘
	hueTeal   = m3Hue{RGBA{15, 54, 57, 255}, RGBA{90, 216, 208, 255}}    // CPU 负载 / 上下行
)

// m3Bar 卡内进度条：常态跟随卡色相，≥pctDanger 才变红（效果稿注释约定）
func m3Bar(hue m3Hue, v, pctDanger float64, danger RGBA) RGBA {
	if v >= pctDanger {
		return danger
	}
	return hue.hi
}

// m3Card 实色容器卡 + 色相标题（三页版标准卡头）
func m3Card(c *Canvas, fs *FontSet, x, y, w, h int, hue m3Hue, title string) {
	c.FillRoundRect(x, y, w, h, m3Rad, hue.bg)
	DrawTextMixL(c, fs, x+24, y+42, FzBody, hue.hi, title)
}

// m3Note 卡头右侧的小注（FzTiny，弱化灰）
func m3Note(c *Canvas, fs *FontSet, xRight, y int, th *Theme, text string) {
	DrawTextMixRC(c, fs, xRight, y, FzTiny, th.Dim, text)
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

// 首页纵向布局。时钟区加高到 184（FzMega 112 的字号空间），
// 卡间距 14px，满版不留大片空白。
// 锚定纪律（实机踩坑结论，2026-10-28）：
//   DrawTextL / DrawTextMixL / DrawTextLR / DrawTextMixRC —— y 是**行盒顶**
//   DrawPartsL —— y 是**基线**（40px 数字 ink 顶 ≈ y-32）
// 排版时两者不可混用同一 y 值，否则文字与进度条互相穿插。
const (
	envTopH   = 184
	envCardsY = 200
	envCardsH = 150
	envCoresY = 364
	envCoresH = 118
	envNetY   = 496
	envNetH   = 166
	envDiskY  = 676
	envDiskH  = 134
	envPillY  = 820
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
	if c.ClipVisible(PadX, envCardsY, CardW, envCardsH) {
		type mini struct {
			label string
			txt   string
			v     float64
			hue   m3Hue
		}
		cpu := clamp(d.Ease("env.cpu", m.CPUPercent), 0, 100)
		mem := clamp(d.Ease("env.mem", m.MemPercent), 0, 100)
		temp := clamp(d.Ease("env.temp", m.CPUTemp), 0, 120)
		cards := []mini{
			{"CPU", fmt.Sprintf("%.0f%%", cpu), cpu, hueBlue},
			{"内存", fmt.Sprintf("%.0f%%", mem), mem, huePurple},
			{"温度", fmt.Sprintf("%.0f°", temp), temp, hueAmber},
		}
		for i, mc := range cards {
			cx := PadX + i*(104+12)
			c.FillRoundRect(cx, envCardsY, 104, envCardsH, m3Rad, mc.hue.bg)
			DrawTextMixL(c, fs, cx+18, envCardsY+42, FzBody, mc.hue.hi, mc.label)
			DrawTextL(c, fs, mc.txt, cx+18, envCardsY+70, m3Num, th.Text)
			c.Bar(cx+18, envCardsY+126, 68, 8, 4, clamp(mc.v, 0, 100)/100,
				th.Track, m3Bar(mc.hue, mc.v, 90, th.Danger))
		}
	}

	// ---------------- CPU 负载（每核迷你条，单行）----------------
	if c.ClipVisible(PadX, envCoresY, CardW, envCoresH) {
		m3Card(c, fs, PadX, envCoresY, CardW, envCoresH, hueTeal, "CPU 负载")
		m3Note(c, fs, PadX+CardW-24, envCoresY+40, th,
			fmt.Sprintf("负载 %.2f", m.LoadAvg[0]))

		cores := m.CPUCores
		n := coreCount(m)
		if n > 4 {
			n = 4
		}
		cellW := 74
		for i := 0; i < n; i++ {
			cx := PadX + 24 + i*cellW
			DrawTextL(c, fs, fmt.Sprintf("C%d", i), cx, envCoresY+68, FzTiny, th.Dim)
			v := 0.0
			if i < len(cores) {
				v = clamp(cores[i], 0, 100)
			}
			c.Bar(cx, envCoresY+90, 62, 6, 3, v/100,
				th.Track, m3Bar(hueTeal, v, 90, th.Danger))
		}
	}

	// ---------------- 网络（双速率 + 双曲线）----------------
	if c.ClipVisible(PadX, envNetY, CardW, envNetH) {
		m3Card(c, fs, PadX, envNetY, CardW, envNetH, hueGreen, "网络")
		up := d.Ease("env.up", m.NetSendMBps)
		dn := d.Ease("env.dn", m.NetRecvMBps)
		DrawRateRow(c, fs, th, PadX+CardW-24, envNetY+44, up, P.NetUp, true)
		DrawRateRow(c, fs, th, PadX+CardW-24, envNetY+74, dn, P.NetDown, false)

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
		c.Curve(PadX+24, envNetY+92, CardW-48, 58, recv, scale, 60, P.NetDown, 2, 0.26)
		c.Curve(PadX+24, envNetY+92, CardW-48, 58, send, scale, 60, P.NetUp, 2, 0)
	}

	// ---------------- 存储（容量最大的卷）----------------
	if c.ClipVisible(PadX, envDiskY, CardW, envDiskH) {
		m3Card(c, fs, PadX, envDiskY, CardW, envDiskH, huePurple, "存储")
		vols := m.Disk.GetVolumes()
		if len(vols) == 0 {
			DrawText(c, fs, "未检测到存储卷", PadX+24, envDiskY+72, FzSmall, th.Dim)
		} else {
			// 按总容量降序取最大卷 —— /vol1 这类主数据卷排在系统盘前面
			big := vols[0]
			for _, v := range vols[1:] {
				if v.TotalGB > big.TotalGB {
					big = v
				}
			}
			if big.Temp > 0 {
				DrawTextLR(c, fs, fmt.Sprintf("%.0f°C", big.Temp),
					PadX+CardW-24, envDiskY+40, FzTiny, th.Dim)
			}
			DrawTextMixL(c, fs, PadX+24, envDiskY+74, FzSmall, th.Text,
				trim(big.VolName, 8))
			c.Bar(PadX+24, envDiskY+100, CardW-48, 8, 4, big.UsedPercent/100,
				th.Track, m3Bar(huePurple, big.UsedPercent, 90, th.Danger))
			DrawTextMixL(c, fs, PadX+24, envDiskY+112, FzTiny, th.Dim,
				fmt.Sprintf("%.0f GB / %.0f GB · %.0f%%",
					big.UsedGB, big.TotalGB, big.UsedPercent))
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
		DrawTextMixL(c, fs, PadX+34, envPillY+12, FzTiny, th.Sub, ptxt)

		c.FillRoundRect(PadX+170, envPillY, CardW-170, envPillH, envPillH/2, WithA(th.Text, 10))
		DrawTextMixL(c, fs, PadX+192, envPillY+12, FzTiny, th.Sub,
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
	dtCpuY, dtCpuH   = 104, 172
	dtMemY, dtMemH   = 292, 156
	dtDiskY, dtDiskH = 464, 180
	dtTempY, dtTempH = 660, 176
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
		m3Note(c, fs, PadX+CardW-24, dtCpuY+40, th,
			fmt.Sprintf("峰值 %.0f%%", peak))
		// 曲线区淡网格（对照效果稿的水平细线）
		for _, gy := range []int{22, 46, 70} {
			DrawHairline(c, th, PadX+24, dtCpuY+56+gy, CardW-48)
		}
		c.Curve(PadX+24, dtCpuY+56, CardW-48, 88, m.CPUHistory,
			autoTop(m.CPUHistory, 20), 60, hueBlue.hi, 2, 0.20)
		// DrawPartsL 的 y 是基线：40px 数字的 ink 顶 ≈ y-32，别压住曲线
		DrawPartsL(c, fs, PadX+24, dtCpuY+166, FzSmall, []TPart{
			{S: "当前 ", Col: th.Dim},
			{S: fmt.Sprintf("%.0f%%", m.CPUPercent), Col: th.Sub, Latin: true},
		})
	}

	// ---------------- 内存（绝对量 + 缓存/Swap，紫卡）----------------
	if c.ClipVisible(PadX, dtMemY, CardW, dtMemH) {
		m3Card(c, fs, PadX, dtMemY, CardW, dtMemH, huePurple, "内存")
		m3Note(c, fs, PadX+CardW-24, dtMemY+40, th,
			fmt.Sprintf("%.0f%%", m.MemPercent))
		DrawPartsL(c, fs, PadX+24, dtMemY+114, FzLarge, []TPart{
			{S: fmt.Sprintf("%.1f", m.MemUsedGB), Col: th.Text, Latin: true},
			{S: " GB", Col: th.Dim, Latin: true},
			{S: "  /  ", Col: th.Faint, Latin: true},
			{S: fmt.Sprintf("%.0f GB", m.MemTotalGB), Col: th.Sub, Latin: true},
		})
		c.Bar(PadX+24, dtMemY+126, CardW-48, 8, 4, m.MemPercent/100,
			th.Track, m3Bar(huePurple, m.MemPercent, 90, th.Danger))
		DrawTextMixL(c, fs, PadX+24, dtMemY+132, FzTiny, th.Dim,
			fmt.Sprintf("缓存 %.1f GB · Swap %.1f / %.0f GB",
				m.MemCacheGB, m.SwapUsedGB, m.SwapTotalGB))
	}

	// ---------------- 磁盘（逐盘容量 + 温度，绿卡）----------------
	if c.ClipVisible(PadX, dtDiskY, CardW, dtDiskH) {
		m3Card(c, fs, PadX, dtDiskY, CardW, dtDiskH, hueGreen, "磁盘")
		vols := m.Disk.GetVolumes()
		if len(vols) == 0 {
			DrawText(c, fs, "未检测到存储卷", PadX+24, dtDiskY+82, FzSmall, th.Dim)
		} else {
			for i, v := range vols {
				if i >= 3 {
					break
				}
				vy := dtDiskY + 72 + i*36
				DrawTextMixL(c, fs, PadX+24, vy, FzSmall, th.Text, trim(v.VolName, 8))
				if v.Temp > 0 {
					DrawTextLR(c, fs, fmt.Sprintf("%.0f°C", v.Temp),
						PadX+CardW-24, vy, FzTiny, th.Dim)
				}
				c.Bar(PadX+24, vy+26, CardW-48, 6, 3, v.UsedPercent/100,
					th.Track, m3Bar(hueGreen, v.UsedPercent, 90, th.Danger))
			}
		}
	}

	// ---------------- 温度与风扇（琥珀卡）----------------
	if c.ClipVisible(PadX, dtTempY, CardW, dtTempH) {
		m3Card(c, fs, PadX, dtTempY, CardW, dtTempH, hueAmber, "温度与风扇")
		cpuT := clamp(d.Ease("dt.ct", m.CPUTemp), 0, 120)
		diskT := clamp(d.Ease("dt.dt", m.Disk.MaxDiskTemp()), 0, 120)
		DrawPartsL(c, fs, PadX+24, dtTempY+114, FzLarge, []TPart{
			{S: fmt.Sprintf("%.0f°", cpuT), Col: th.Text, Latin: true},
		})
		DrawTextMixL(c, fs, PadX+110, dtTempY+110, FzSmall, th.Dim, "CPU")
		DrawPartsL(c, fs, PadX+190, dtTempY+114, FzLarge, []TPart{
			{S: fmt.Sprintf("%.0f°", diskT), Col: th.Text, Latin: true},
		})
		DrawTextMixL(c, fs, PadX+272, dtTempY+110, FzSmall, th.Dim, "硬盘")
		c.Bar(PadX+24, dtTempY+134, 100, 8, 4, clamp(cpuT, 0, 100)/100,
			th.Track, m3Bar(hueAmber, cpuT, 82, th.Danger))
		c.Bar(PadX+190, dtTempY+134, 100, 8, 4, clamp(diskT, 0, 100)/100,
			th.Track, m3Bar(hueAmber, diskT, 82, th.Danger))

		cpuRPM, boxRPM := p.fanSpeeds(d)
		DrawTextMixL(c, fs, PadX+24, dtTempY+152, FzTiny, th.Dim,
			fmt.Sprintf("CPU %s · 机箱 %s（%s）",
				cpuRPM, boxRPM, fanModeCN(d.Fan.GetCPUMode())))
	}
}

// fanSpeeds 风扇转速文本（0 视为读不到 → —）
func (p *DetailPage) fanSpeeds(d *DrawCtx) (string, string) {
	cpuRPM, boxRPM := 0, 0
	if d.Fan != nil {
		cpuRPM, boxRPM = d.Fan.GetFanSpeeds()
	}
	f := func(v int) string {
		if v <= 0 {
			return "—"
		}
		return fmt.Sprintf("%d RPM", v)
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
	syNetY, syNetH   = 260, 132
	sySpdY, sySpdH   = 408, 200
	syCtrlY, syCtrlH = 624, 164
	syPillY          = 804
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
		DrawTextLR(c, fs, appVersion, PadX+CardW-24, syDevY+38, FzTiny, th.Dim)
		DrawTextMixL(c, fs, PadX+24, syDevY+66, FzLarge, th.Text, trim(d.Host, 14))
		DrawTextMixL(c, fs, PadX+24, syDevY+112, FzSmall, th.Dim,
			fmt.Sprintf("飞牛 fnOS · %d 核 · %.0f GB 内存",
				coreCount(m), m.MemTotalGB))
	}

	// ---------------- 网络（绿卡，行内「标签 + 值」；公网应用内已打码）----------------
	if c.ClipVisible(PadX, syNetY, CardW, syNetH) {
		m3Card(c, fs, PadX, syNetY, CardW, syNetH, hueGreen, "网络")
		iface := m.NetIface
		if iface == "" {
			iface = "—"
		}
		DrawPartsL(c, fs, PadX+24, syNetY+100, FzSmall, []TPart{
			{S: "局域网 " + iface + "  ", Col: th.Sub},
			{S: ipOrDash(m.LocalIP), Col: th.Text, Latin: true},
		})
		pub := "获取中"
		if d.Net != nil {
			if pi := d.Net.PublicIP(); pi.OK && pi.IP != "" {
				pub = maskPubIP(pi.IP)
			}
		}
		DrawPartsL(c, fs, PadX+24, syNetY+128, FzSmall, []TPart{
			{S: "公网  ", Col: th.Sub},
			{S: pub, Col: th.Text, Latin: true},
		})
	}

	// ---------------- 上传 · 下载（速度大卡，青卡）----------------
	if c.ClipVisible(PadX, sySpdY, CardW, sySpdH) {
		m3Card(c, fs, PadX, sySpdY, CardW, sySpdH, hueTeal, "上传 · 下载")
		up := clamp(d.Ease("sy.up", m.NetSendMBps), 0, 1e9)
		dn := clamp(d.Ease("sy.dn", m.NetRecvMBps), 0, 1e9)

		DrawTextMixL(c, fs, PadX+24, sySpdY+66, FzTiny, th.Dim, "上传 · MB/s")
		DrawPartsL(c, fs, PadX+24, sySpdY+122, FzLarge, []TPart{
			{S: fmtMBps(up), Col: th.Text, Latin: true},
		})
		DrawTextMixL(c, fs, PadX+190, sySpdY+66, FzTiny, th.Dim, "下载 · MB/s")
		DrawPartsL(c, fs, PadX+190, sySpdY+122, FzLarge, []TPart{
			{S: fmtMBps(dn), Col: th.Text, Latin: true},
		})

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
		c.Curve(PadX+24, sySpdY+134, CardW-48, 38, recv, scale, 60, P.NetDown, 2, 0.26)
		c.Curve(PadX+24, sySpdY+134, CardW-48, 38, send, scale, 60, P.NetUp, 2, 0)

		DrawPartsL(c, fs, PadX+24, sySpdY+194, FzSmall, []TPart{
			{S: "今日收 " + fmtBytes(m.NetRxToday), Col: th.Sub},
			{S: "  ·  ", Col: th.Faint},
			{S: "发 " + fmtBytes(m.NetTxToday), Col: th.Sub},
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
		DrawTextMixL(c, fs, PadX+24, syCtrlY+66, FzSmall, th.Text, "亮度")
		sliderX, sliderW := PadX+92, 158
		c.Bar(sliderX, syCtrlY+70, sliderW, 8, 4, pct, th.Track, huePurple.hi)
		c.FillCircle(float64(sliderX+int(float64(sliderW)*pct)), syCtrlY+74, 7, th.Text)
		DrawTextLR(c, fs, fmt.Sprintf("%.0f%%", pct*100),
			PadX+CardW-24, syCtrlY+66, FzSmall, th.Text)
		DrawTextMixL(c, fs, PadX+24, syCtrlY+94, FzTiny, th.Dim,
			"点左半降低 · 右半升高")

		// 关屏按钮（保留 v0.1.0 以来的一键关屏能力）
		c.FillRoundRect(PadX+24, syCtrlY+110, CardW-48, 42, 21, WithA(th.Danger, 40))
		DrawTextC(c, fs, "关闭屏幕", PadX+24+(CardW-48)/2, syCtrlY+132, FzSmall, Lit(th.Danger, 0.25))
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
	if x >= PadX+24 && x <= PadX+CardW-24 && y >= syCtrlY+112 && y <= syCtrlY+154 {
		if d.OnScreenOff != nil {
			d.OnScreenOff()
		}
		return
	}
	// 亮度行
	if y >= syCtrlY+58 && y <= syCtrlY+94 && x >= PadX && x <= PadX+CardW {
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
