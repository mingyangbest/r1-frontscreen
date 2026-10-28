package ui

// ============================================================
//  Material You 三页版（2026-10-28 洋哥拍板）
//
//  设计原则（对照 Google Nest Hub / M3 tonal 语言）：
//   1. 每张卡一个低饱和色相 —— 多彩来自卡与卡之间，克制来自卡内部
//      只有一种色相（DrawCardTonal：色相铺卡面、无描边）
//   2. 满版排版：内容铺满 96~896 内容区，不做大片留白
//   3. 大数字 + 细进度条取代双环仪表；CPU 数据完整上屏（总量+每核）
//   4. 公网 IP 半遮蔽（maskPubIP），截图外发不泄密
//   5. 数据真实性约定不变：读不到的显示 —，绝不编造
//
//  三页：
//   环境（fullTop，大时钟+天气融合） → 详情 → 系统
// ============================================================

import (
	"fmt"

	"r1-toolbox/internal/hardware"
	"r1-toolbox/internal/monitor"
)

const appVersion = "v0.4.0"

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

// tonalCard 画一张 M3 tonal 卡 + 左上角色相标题（三页版的标准卡头）
func tonalCard(c *Canvas, fs *FontSet, x, y, w, h int, accent RGBA, title string) {
	DrawCardTonal(c, themeInk, x, y, w, h, accent)
	DrawCardTitleT(c, fs, x+22, y+22, title, accent)
}

// ============================================================
//  第 1 页 · 环境（fullTop：大时钟 + 天气融合，六块内容满版）
// ============================================================

type EnvPage struct{ mon *monitor.SystemMonitor }

func NewEnvPage(mon *monitor.SystemMonitor) *EnvPage { return &EnvPage{mon: mon} }

func (p *EnvPage) PageName() string { return "环境" }
func (p *EnvPage) FullTop() bool    { return true }

// 首页纵向布局。底边统一落在 818，与详情/系统页对齐；
// 卡间距 16px，满版不留大片空白。
//
// N100 是 4 核 4 线程（实机核数以 coreCount 为准），每核条单行排布。
const (
	envTopH   = 152 // 大时钟 + 日期行（fullTop 接管区，renderer 按此高度做分钟局部重画）
	envCardsY = 156
	envCardsH = 148
	envCoresY = 320
	envCoresH = 120
	envNetY   = 456
	envNetH   = 170
	envDiskY  = 642
	envDiskH  = 120
	envPillY  = 778
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
		tim := glyphLatin(fs, FzHuge, d.Now.Format("15:04"), th.Text)
		blitGlyph(c, tim, PadX+4, 6)

		wx := d.WeatherNow()
		kind := wx.Kind
		if kind == "" {
			kind = "overcast"
		}
		wxAcc := weatherAccent(kind)

		parts := []TPart{
			{S: d.Now.Format("01-02") + " " + weekdayCN(d.Now), Col: th.Sub},
		}
		if wx.OK {
			parts = append(parts,
				TPart{S: "  ·  ", Col: th.Faint},
				TPart{S: wx.Desc, Col: Lit(wxAcc, 0.30)},
				TPart{S: "  ", Col: th.Faint},
				TPart{S: fmt.Sprintf("%.0f°", wx.TempC), Col: th.Sub, Latin: true},
			)
		}
		DrawPartsL(c, fs, PadX+4, 138, FzSmall, parts)

		// 天气图标（右上）：昼夜由 isNight 内部判断，静态相位
		DrawWeatherIcon(c, PadX+CardW-52, 64, 40, kind, 2.856)
	}

	// ---------------- 三张指标小卡（CPU 橙 / 内存青 / 温度黄）----------------
	if c.ClipVisible(PadX, envCardsY, CardW, envCardsH) {
		type mini struct {
			label string
			txt   string
			pct   float64
			acc   RGBA
			fill  RGBA
		}
		cpu := clamp(d.Ease("env.cpu", m.CPUPercent), 0, 100)
		mem := clamp(d.Ease("env.mem", m.MemPercent), 0, 100)
		temp := clamp(d.Ease("env.temp", m.CPUTemp), 0, 100)
		cards := []mini{
			{"CPU", fmt.Sprintf("%.0f%%", cpu), cpu / 100, P.Cpu, th.OnLoad(cpu)},
			{"内存", fmt.Sprintf("%.0f%%", mem), mem / 100, P.Mem, th.OnLoad(mem)},
			{"温度", fmt.Sprintf("%.0f°", m.CPUTemp), temp / 100, P.Fan, th.OnTemp(P.Fan, temp)},
		}
		for i, mc := range cards {
			cx := PadX + i*(104+12)
			DrawCardTonal(c, th, cx, envCardsY, 104, envCardsH, mc.acc)
			DrawTextMixL(c, fs, cx+18, envCardsY+26, FzTiny, Lit(mc.acc, 0.45), mc.label)
			DrawTextL(c, fs, mc.txt, cx+18, envCardsY+48, FzMid, th.Text)
			c.Bar(cx+18, envCardsY+108, 68, 6, 3, mc.pct, th.Track, mc.fill)
		}
		_ = temp
	}

	// ---------------- CPU 负载（每核迷你条，单行）----------------
	if c.ClipVisible(PadX, envCoresY, CardW, envCoresH) {
		tonalCard(c, fs, PadX, envCoresY, CardW, envCoresH, P.Sys, "CPU 负载")
		DrawTextMixRC(c, fs, PadX+CardW-22, envCoresY+22, FzTiny, th.Dim,
			fmt.Sprintf("负载 %.2f", m.LoadAvg[0]))

		cores := m.CPUCores
		n := coreCount(m)
		if n > 4 {
			n = 4
		}
		cellW := 74
		for i := 0; i < n; i++ {
			cx := PadX + 22 + i*cellW
			DrawTextL(c, fs, fmt.Sprintf("C%d", i), cx, envCoresY+56, FzTiny, th.Dim)
			v := 0.0
			if i < len(cores) {
				v = clamp(cores[i], 0, 100)
			}
			c.Bar(cx, envCoresY+86, 62, 5, 2, v/100, th.Track, th.OnLoad(v))
		}
	}

	// ---------------- 网络（双速率 + 双曲线）----------------
	if c.ClipVisible(PadX, envNetY, CardW, envNetH) {
		tonalCard(c, fs, PadX, envNetY, CardW, envNetH, P.NetDown, "网络")
		up := d.Ease("env.up", m.NetSendMBps)
		dn := d.Ease("env.dn", m.NetRecvMBps)
		DrawRateRow(c, fs, th, PadX+CardW-22, envNetY+30, up, P.NetUp, true)
		DrawRateRow(c, fs, th, PadX+CardW-22, envNetY+58, dn, P.NetDown, false)

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
		c.Curve(PadX+22, envNetY+78, CardW-44, 74, recv, scale, 60, P.NetDown, 2, 0.26)
		c.Curve(PadX+22, envNetY+78, CardW-44, 74, send, scale, 60, P.NetUp, 2, 0)
	}

	// ---------------- 存储（容量最大的卷）----------------
	if c.ClipVisible(PadX, envDiskY, CardW, envDiskH) {
		tonalCard(c, fs, PadX, envDiskY, CardW, envDiskH, P.DiskSSD, "存储")
		vols := m.Disk.GetVolumes()
		if len(vols) == 0 {
			DrawText(c, fs, "未检测到存储卷", PadX+22, envDiskY+62, FzSmall, th.Dim)
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
					PadX+CardW-22, envDiskY+22, FzTiny, th.Dim)
			}
			DrawTextMixL(c, fs, PadX+22, envDiskY+50, FzSmall, th.Text,
				trim(big.VolName, 8))
			c.Bar(PadX+22, envDiskY+78, CardW-44, 8, 4, big.UsedPercent/100,
				th.Track, th.OnLoad(big.UsedPercent))
			DrawTextMixL(c, fs, PadX+22, envDiskY+94, FzTiny, th.Dim,
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

// coreCount 逻辑核心数（页 1 每核条用；挪自旧概览页）
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
	dtMemY, dtMemH   = 292, 140
	dtDiskY, dtDiskH = 448, 180
	dtTempY, dtTempH = 644, 176
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

	// ---------------- CPU 历史 · 60 秒（大曲线）----------------
	if c.ClipVisible(PadX, dtCpuY, CardW, dtCpuH) {
		tonalCard(c, fs, PadX, dtCpuY, CardW, dtCpuH, P.Cpu, "CPU 历史 · 60 秒")
		peak := 0.0
		for _, v := range m.CPUHistory {
			if v > peak {
				peak = v
			}
		}
		DrawTextMixRC(c, fs, PadX+CardW-22, dtCpuY+22, FzTiny, th.Dim,
			fmt.Sprintf("峰值 %.0f%%", peak))
		c.Curve(PadX+22, dtCpuY+48, CardW-44, 92, m.CPUHistory,
			autoTop(m.CPUHistory, 20), 60, P.Cpu, 2, 0.20)
		// DrawPartsL 的 y 是基线：40px 数字的 ink 顶 ≈ y-32，别压住曲线
		DrawPartsL(c, fs, PadX+22, dtCpuY+166, FzSmall, []TPart{
			{S: "当前 ", Col: th.Dim},
			{S: fmt.Sprintf("%.0f%%", m.CPUPercent), Col: th.Sub, Latin: true},
		})
	}

	// ---------------- 内存（绝对量 + 缓存/Swap）----------------
	if c.ClipVisible(PadX, dtMemY, CardW, dtMemH) {
		tonalCard(c, fs, PadX, dtMemY, CardW, dtMemH, P.Mem, "内存")
		DrawTextLR(c, fs, fmt.Sprintf("%.0f%%", m.MemPercent),
			PadX+CardW-22, dtMemY+22, FzTiny, th.Dim)
		DrawPartsL(c, fs, PadX+22, dtMemY+88, FzLarge, []TPart{
			{S: fmt.Sprintf("%.1f", m.MemUsedGB), Col: th.Text, Latin: true},
			{S: " GB", Col: th.Dim, Latin: true},
			{S: "  /  ", Col: th.Faint, Latin: true},
			{S: fmt.Sprintf("%.0f GB", m.MemTotalGB), Col: th.Sub, Latin: true},
		})
		c.Bar(PadX+22, dtMemY+102, CardW-44, 8, 4, m.MemPercent/100,
			th.Track, th.OnLoad(m.MemPercent))
		DrawTextMixL(c, fs, PadX+22, dtMemY+118, FzTiny, th.Dim,
			fmt.Sprintf("缓存 %.1f GB · Swap %.1f / %.0f GB",
				m.MemCacheGB, m.SwapUsedGB, m.SwapTotalGB))
	}

	// ---------------- 磁盘（逐盘容量 + 温度）----------------
	if c.ClipVisible(PadX, dtDiskY, CardW, dtDiskH) {
		tonalCard(c, fs, PadX, dtDiskY, CardW, dtDiskH, P.DiskSSD, "磁盘")
		vols := m.Disk.GetVolumes()
		if len(vols) == 0 {
			DrawText(c, fs, "未检测到存储卷", PadX+22, dtDiskY+70, FzSmall, th.Dim)
		} else {
			for i, v := range vols {
				if i >= 3 {
					break
				}
				vy := dtDiskY + 50 + i*44
				DrawTextMixL(c, fs, PadX+22, vy, FzSmall, th.Text, trim(v.VolName, 8))
				if v.Temp > 0 {
					DrawTextLR(c, fs, fmt.Sprintf("%.0f°C", v.Temp),
						PadX+CardW-22, vy, FzTiny, th.Dim)
				}
				c.Bar(PadX+22, vy+22, CardW-44, 8, 4, v.UsedPercent/100,
					th.Track, P.DiskColor(v.DeviceType))
				_ = vy
			}
		}
	}

	// ---------------- 温度与风扇 ----------------
	if c.ClipVisible(PadX, dtTempY, CardW, dtTempH) {
		tonalCard(c, fs, PadX, dtTempY, CardW, dtTempH, P.Fan, "温度与风扇")
		cpuT := clamp(d.Ease("dt.ct", m.CPUTemp), 0, 120)
		diskT := clamp(d.Ease("dt.dt", m.Disk.MaxDiskTemp()), 0, 120)
		DrawPartsL(c, fs, PadX+22, dtTempY+86, FzLarge, []TPart{
			{S: fmt.Sprintf("%.0f°", cpuT), Col: th.Text, Latin: true},
		})
		DrawTextMixL(c, fs, PadX+110, dtTempY+62, FzSmall, th.Dim, "CPU")
		DrawPartsL(c, fs, PadX+190, dtTempY+86, FzLarge, []TPart{
			{S: fmt.Sprintf("%.0f°", diskT), Col: th.Text, Latin: true},
		})
		DrawTextMixL(c, fs, PadX+272, dtTempY+62, FzSmall, th.Dim, "硬盘")
		c.Bar(PadX+22, dtTempY+112, 100, 6, 3, cpuT/100, th.Track, th.OnTemp(P.Fan, m.CPUTemp))
		c.Bar(PadX+190, dtTempY+112, 100, 6, 3, diskT/100, th.Track, th.OnTemp(P.Fan, diskT))

		cpuRPM, boxRPM := p.fanSpeeds(d)
		DrawTextMixL(c, fs, PadX+22, dtTempY+140, FzTiny, th.Dim,
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

// fanModeCN 风扇模式中文名（挪自旧系统页）
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
//  第 3 页 · 系统（设备 / 网络身份 / 上下行速度大卡 / 控制）
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
	syDevY, syDevH   = 104, 116
	syNetY, syNetH   = 236, 108
	sySpdY, sySpdH   = 360, 196
	syCtrlY, syCtrlH = 572, 164
	syPillY          = 752
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

	// ---------------- 设备 ----------------
	if c.ClipVisible(PadX, syDevY, CardW, syDevH) {
		tonalCard(c, fs, PadX, syDevY, CardW, syDevH, P.Sys, "设备")
		DrawTextLR(c, fs, appVersion, PadX+CardW-22, syDevY+22, FzTiny, th.Dim)
		DrawTextMixL(c, fs, PadX+22, syDevY+42, FzLarge, th.Text, trim(d.Host, 14))
		DrawTextMixL(c, fs, PadX+22, syDevY+92, FzSmall, th.Dim,
			fmt.Sprintf("运行 %s · 进程 %d", formatUptime(m.UptimeSec), m.ProcCount))
	}

	// ---------------- 网络身份（公网 IP 打码）----------------
	if c.ClipVisible(PadX, syNetY, CardW, syNetH) {
		tonalCard(c, fs, PadX, syNetY, CardW, syNetH, P.NetDown, "网络身份")
		iface := m.NetIface
		if iface == "" {
			iface = "—"
		}
		DrawKV(c, fs, th, PadX+22, syNetY+42, CardW-44,
			"局域网 "+iface, ipOrDash(m.LocalIP), th.Text)
		pub := "获取中"
		if d.Net != nil {
			if pi := d.Net.PublicIP(); pi.OK && pi.IP != "" {
				pub = maskPubIP(pi.IP)
			}
		}
		DrawKV(c, fs, th, PadX+22, syNetY+74, CardW-44, "公网", pub, th.Text)
	}

	// ---------------- 上传 · 下载（速度大卡）----------------
	if c.ClipVisible(PadX, sySpdY, CardW, sySpdH) {
		tonalCard(c, fs, PadX, sySpdY, CardW, sySpdH, P.NetUp, "上传 · 下载")
		up := clamp(d.Ease("sy.up", m.NetSendMBps), 0, 1e9)
		dn := clamp(d.Ease("sy.dn", m.NetRecvMBps), 0, 1e9)

		DrawTextMixL(c, fs, PadX+22, sySpdY+52, FzTiny, Lit(P.NetUp, 0.40), "上传 · MB/s")
		DrawPartsL(c, fs, PadX+22, sySpdY+104, FzLarge, []TPart{
			{S: fmtMBps(up), Col: th.Text, Latin: true},
		})
		DrawTextMixL(c, fs, PadX+190, sySpdY+52, FzTiny, Lit(P.NetDown, 0.40), "下载 · MB/s")
		DrawPartsL(c, fs, PadX+190, sySpdY+104, FzLarge, []TPart{
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
		c.Curve(PadX+22, sySpdY+118, CardW-44, 38, recv, scale, 60, P.NetDown, 2, 0.26)
		c.Curve(PadX+22, sySpdY+118, CardW-44, 38, send, scale, 60, P.NetUp, 2, 0)

		DrawPartsL(c, fs, PadX+22, sySpdY+184, FzSmall, []TPart{
			{S: "今日收 " + fmtBytes(m.NetRxToday), Col: th.Sub},
			{S: "  ·  ", Col: th.Faint},
			{S: "发 " + fmtBytes(m.NetTxToday), Col: th.Sub},
		})
	}

	// ---------------- 控制（亮度可点调 + 关屏按钮）----------------
	if c.ClipVisible(PadX, syCtrlY, CardW, syCtrlH) {
		tonalCard(c, fs, PadX, syCtrlY, CardW, syCtrlH, P.Cpu, "控制")
		cur, err := p.br.Get()
		pct := 0.0
		if err == nil && p.br.Max() > 0 {
			pct = float64(cur) / float64(p.br.Max())
		}
		DrawTextMixL(c, fs, PadX+22, syCtrlY+42, FzSmall, th.Text, "亮度")
		c.Bar(PadX+90, syCtrlY+50, 160, 6, 3, pct, th.Track, P.Cpu)
		DrawTextLR(c, fs, fmt.Sprintf("%.0f%%", pct*100),
			PadX+CardW-22, syCtrlY+42, FzSmall, th.Text)
		DrawTextMixL(c, fs, PadX+22, syCtrlY+72, FzTiny, th.Dim,
			"点左半降低 · 右半升高")

		// 关屏按钮（保留 v0.1.0 以来的一键关屏能力）
		c.FillRoundRect(PadX+22, syCtrlY+104, CardW-44, 40, 20, WithA(th.Danger, 40))
		DrawTextC(c, fs, "关闭屏幕", PadX+22+(CardW-44)/2, syCtrlY+116, FzSmall, Lit(th.Danger, 0.25))
	}

	// ---------------- 服务状态 pill ----------------
	if c.ClipVisible(PadX, syPillY, CardW, 40) {
		c.FillRoundRect(PadX, syPillY, 200, 40, 20, WithA(th.Text, 10))
		c.FillCircle(PadX+22, syPillY+20, 4, P.NetDown)
		DrawTextMixL(c, fs, PadX+34, syPillY+12, FzTiny, th.Sub,
			"服务运行中 · 夜间调暗已启用")
	}
}

// Tap 触屏：亮度行左半 -10% / 右半 +10%；关屏按钮触发 OnScreenOff。
func (p *SysPage) Tap(x, y int, d *DrawCtx) {
	// 关屏按钮（先判：关屏按钮在下方，与亮度行不重叠）
	if x >= PadX+22 && x <= PadX+CardW-22 && y >= syCtrlY+104 && y <= syCtrlY+144 {
		if d.OnScreenOff != nil {
			d.OnScreenOff()
		}
		return
	}
	// 亮度行
	if y >= syCtrlY+30 && y <= syCtrlY+70 && x >= PadX && x <= PadX+CardW {
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
