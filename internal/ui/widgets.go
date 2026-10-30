package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"r1-toolbox/internal/monitor"
)

// ============================================================
//  字号（针对 376x960 高 DPI 小屏标定）
// ============================================================

const (
	FzTiny  = 17
	FzSmall = 20
	FzBody  = 24
	FzMid   = 30
	FzLarge = 40
	FzBig   = 62
	FzHuge  = 96
	FzMega  = 112 // M3 大时钟（对照效果稿 ~130px 视觉高度，配合当前字体宽度取 112）
)

// 布局常量
const (
	PadX    = 20
	CardW   = 336
	CardRad = 14
)

// 仪表弧开口：下方留 90° 缺口（270° 弧），比整圆更像专业仪表
const (
	gaugeStart = 0.625 // 归一化角度：左下 225°
	gaugeSweep = 0.75  // 270°
)

// ============================================================
//  环形遮罩缓存（同一半径跨帧复用）
// ============================================================

var maskCache = map[[2]int]*RingMask{}

func ringMask(rOut, rIn float64) *RingMask {
	key := [2]int{int(rOut*10 + 0.5), int(rIn*10 + 0.5)}
	if m, ok := maskCache[key]; ok {
		return m
	}
	m := NewRingMask(rOut, rIn)
	maskCache[key] = m
	return m
}

// ============================================================
//  背景
// ============================================================

// DrawBackground 极微的垂直渐变，无光斑、无装饰
func DrawBackground(c *Canvas, th *Theme) {
	c.FillRectGrad(0, 0, c.W, c.H, th.BgTop, th.BgBot)
}

// ============================================================
//  分隔线
// ============================================================

// DrawHairline 两端渐隐的极细分隔线
func DrawHairline(c *Canvas, th *Theme, x, y, w int) {
	if w <= 2 {
		return
	}
	const fade = 0.08
	for i := 0; i < w; i++ {
		t := float64(i) / float64(w-1)
		e := 1.0
		if t < fade {
			e = t / fade
		}
		if t > 1-fade {
			e = (1 - t) / fade
		}
		c.Blend(x+i, y, th.Hairline, e)
	}
}

// ============================================================
//  卡片
// ============================================================

// DrawCard 卡片：一层柔和的「面」+ 淡描边。
//
// 上一版把填充压到 alpha 8 导致信息像浮在黑色虚空里、观感太素；
// 现在 alpha 15 / 描边 26 —— 卡片看得见轮廓，但仍然不抢内容。
func DrawCard(c *Canvas, th *Theme, x, y, w, h int) {
	c.FillRoundRect(x, y, w, h, CardRad, th.Card)
	c.StrokeRoundRect(x, y, w, h, CardRad, 1, th.CardBorder)
}

// DrawCardA 带语义色的卡片：中性「面」+ 极淡的同色色温 + 同色描边。
//
// 这是本轮"每页都有颜色"的主要手法 —— 卡片本身仍然克制，
// 但描边带上所属部件的色相，扫一眼就知道"这张卡讲的是什么"。
// 三项叠加很轻（填充 14 / 描边 64），不会变成彩色色块。
func DrawCardA(c *Canvas, th *Theme, x, y, w, h int, accent RGBA) {
	c.FillRoundRect(x, y, w, h, CardRad, th.Card)
	c.FillRoundRect(x, y, w, h, CardRad, WithA(accent, 15))
	c.StrokeRoundRect(x, y, w, h, CardRad, 1, WithA(accent, 64))
}

// DrawCardTonal M3 tonal 卡片（Material You 三页版）：无描边、
// 卡面 = 语义色的低透明涂层 + 一层极淡的抬亮。
//
// 与 DrawCardA 的区别：tonal 卡**不再靠描边**标记身份，色相直接铺在
// 卡面上（alpha 28，比旧版色温浓一倍），视觉上是一整块安静的彩色面 ——
// 这正是 Material Design 3 的 container 色语言：多彩来自卡与卡之间，
// 克制来自卡内部只有一种色相。
func DrawCardTonal(c *Canvas, th *Theme, x, y, w, h int, accent RGBA) {
	c.FillRoundRect(x, y, w, h, CardRad, WithA(accent, 28))
	c.FillRoundRect(x, y, w, h, CardRad, WithA(th.Text, 5))
}

// DrawTopBarM3 三页版页头：顶部小页名 + 左侧大标题 + 右侧滑动提示。
//
// 精修稿的页头是"两行"：最上面一行居中的小页名（16px 灰，位置指示），
// 下面一行左大标题（36px 白）+ 右提示（17px 灰）。
func DrawTopBarM3(c *Canvas, fs *FontSet, th *Theme, name, hint string) {
	DrawTopBarV2(c, fs, name, hint)
}

// DrawCardTitleT tonal 卡标题：文字直接用卡面色相（提亮保证可读）。
// 无色条 —— 色相已经在卡面上了，再插色条就重复。
func DrawCardTitleT(c *Canvas, fs *FontSet, x, y int, text string, accent RGBA) {
	DrawTextMixL(c, fs, x, y, FzTiny, Lit(accent, 0.45), text)
}

// DrawCardTitle 卡片标题：小号、比正文标签更亮，形成三级文字层次。
// 走混排，让标题里的英文/数字（CPU、RPM、GB）用拉丁细体，与正文一致。
func DrawCardTitle(c *Canvas, fs *FontSet, th *Theme, x, y int, text string) {
	DrawTextMixL(c, fs, x, y, FzTiny, th.Title, text)
}

// DrawCardTitleA 带色标的卡片标题：标题左侧一根 3×11 的短竖条，用语义色。
//
// 极小的一笔颜色，却是整页"色相"的锚点 —— 比给标题文字上色更克制，
// 也比空白的标题更有指向性。标题正文左移 11px 让出色条位置。
func DrawCardTitleA(c *Canvas, fs *FontSet, th *Theme, x, y int, text string, accent RGBA) {
	c.FillRoundRect(x, y+2, 3, 11, 1, WithA(accent, 220))
	DrawTextMixL(c, fs, x+11, y, FzTiny, th.Title, text)
}

// ============================================================
//  大数字排版（"大数字 + 小单位"混排，本设计的核心视觉）
// ============================================================

// DrawNumUnit 绘制「大数字 + 小单位」。
// 数字本身以 cx 精确居中，单位则悬挂在其右下角 —— 仪表需要的是数字在正中。
func DrawNumUnit(c *Canvas, fs *FontSet, cx, cy int, num, unit string,
	numSize, unitSize int, numCol, unitCol RGBA) {

	nim := glyphLatin(fs, numSize, num, numCol)
	nw := nim.inkW()
	nh := nim.inkH()
	if nw == 0 {
		return
	}
	// 数字可见字形水平居中于 cx，垂直中心对齐 cy
	numX := cx - (nim.ix0+nim.ix1)/2
	blitGlyph(c, nim, numX, cy-(nim.iy0+nim.iy1)/2)

	if unit == "" {
		return
	}
	uim := glyphLatin(fs, unitSize, unit, unitCol)
	uw := uim.inkW()
	uh := uim.inkH()
	if uw == 0 {
		return
	}
	// 单位悬挂在数字右侧，底部与数字底部大致齐平
	numRight := numX + nim.ix1 + 1
	numBottom := cy + nh/2
	blitGlyph(c, uim, numRight+8-uim.ix0, numBottom-uh-4-uim.iy0)
}

// DrawNumUnitC 绘制「大数字 + 小单位」，但把两者作为**一个整体**在 cx 水平居中。
//
// 与 DrawNumUnit 的区别：那个让数字严格居中、单位挂在外侧（适合仪表大数字，
// 数字必须钉在正中）；这个把整体居中（适合环内的小数值，比如 76% —— 否则
// 数字会因右侧多出一个 % 而肉眼可见地偏左）。
func DrawNumUnitC(c *Canvas, fs *FontSet, cx, cy int, num, unit string,
	numSize, unitSize int, numCol, unitCol RGBA) {

	nim := glyphLatin(fs, numSize, num, numCol)
	nw := nim.inkW()
	if nw == 0 {
		return
	}

	var uim *glyphImg
	uw := 0
	if unit != "" {
		uim = glyphLatin(fs, unitSize, unit, unitCol)
		uw = uim.inkW()
	}

	const gap = 2
	total := nw + uw
	if uw > 0 {
		total += gap
	}
	left := cx - total/2

	numY := cy - (nim.iy0+nim.iy1)/2
	blitGlyph(c, nim, left-nim.ix0, numY)

	if uw > 0 {
		// 单位底边贴住数字底边再上提 3px：读起来像"上标"，而不是并排的第二行
		uY := numY + nim.iy1 - uim.iy1 - 3
		blitGlyph(c, uim, left+nw+gap-uim.ix0, uY)
	}
}

// ============================================================
//  环形仪表
// ============================================================

// DrawGauge 仪表环：270° 开口弧 + 细环 + 中心大数字
func DrawGauge(c *Canvas, fs *FontSet, th *Theme, cx, cy int, rOut, rIn float64,
	pct float64, col RGBA, heading, big, unit string) {

	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	m := ringMask(rOut, rIn)

	// 底轨：用本部件语义色的极淡版，让整圈都带上色相（而不是灰白轨道）
	track := WithA(col, 48)
	if col == th.Accent || col == th.Accent2 {
		track = th.Track
	}
	c.DrawRingAt(m, cx, cy, gaugeStart, gaugeStart+gaugeSweep, track)
	// 进度弧
	if pct > 0 {
		c.DrawRingAt(m, cx, cy, gaugeStart, gaugeStart+gaugeSweep*pct, col)
	}
	// 弧末端一个小圆点（无光晕）
	if pct > 0.004 && pct < 0.996 {
		a := (gaugeStart + gaugeSweep*pct) * 2 * math.Pi
		rMid := (rOut + rIn) / 2
		ex := float64(cx) + rMid*math.Sin(a)
		ey := float64(cy) - rMid*math.Cos(a)
		c.FillCircle(ex, ey, (rOut-rIn)/2, col)
	}

	// 中心数值：语义色提亮一档 —— 保住色相，同时足够亮
	numCol := th.Text
	if col != th.Accent && col != th.Accent2 {
		numCol = Lit(col, 0.22)
	}
	switch {
	case heading != "" && big != "":
		DrawTextMixC(c, fs, cx, cy-int(rOut*0.54), FzTiny, th.Dim, heading)
		DrawNumUnit(c, fs, cx, cy+12, big, unit, FzHuge, FzMid, numCol, th.Dim)
	case big != "":
		DrawNumUnit(c, fs, cx, cy, big, unit, FzHuge, FzMid, numCol, th.Dim)
	}
}

// DrawMiniRing 迷你环 + 中心数值 + 下方标签。
// valSize 指定中心数值字号（概览/性能页用大号，存储页用小号）。
func DrawMiniRing(c *Canvas, fs *FontSet, th *Theme, cx, cy int, rOut, rIn float64,
	pct float64, col RGBA, label, val string, valSize int) {

	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	m := ringMask(rOut, rIn)
	// 底轨带本部件色相（极淡）
	track := WithA(col, 46)
	if col == th.Accent || col == th.Accent2 {
		track = th.Track
	}
	c.DrawRingAt(m, cx, cy, gaugeStart, gaugeStart+gaugeSweep, track)
	if pct > 0 {
		c.DrawRingAt(m, cx, cy, gaugeStart, gaugeStart+gaugeSweep*pct, col)
	}
	if pct > 0.004 && pct < 0.996 {
		a := (gaugeStart + gaugeSweep*pct) * 2 * math.Pi
		rMid := (rOut + rIn) / 2
		ex := float64(cx) + rMid*math.Sin(a)
		ey := float64(cy) - rMid*math.Cos(a)
		c.FillCircle(ex, ey, (rOut-rIn)/2, col)
	}
	if val != "" {
		vcol := th.Text
		if col != th.Accent && col != th.Accent2 {
			vcol = Lit(col, 0.22)
		}
		// 百分号用小一号字并稍淡：它只是单位的角色，
		// 若和数字同字号同亮度，"76%" 会读成一个粗重的四字块。
		if strings.HasSuffix(val, "%") {
			DrawNumUnitC(c, fs, cx, cy, strings.TrimSuffix(val, "%"), "%",
				valSize, valSize*50/100, vcol, WithA(vcol, 175))
		} else {
			vim := glyphLatin(fs, valSize, val, vcol)
			blitGlyph(c, vim, cx-(vim.ix0+vim.ix1)/2, cy-(vim.iy0+vim.iy1)/2)
		}
	}
	if label != "" {
		DrawTextMixC(c, fs, cx, cy+int(rOut)+10, FzTiny, th.Dim, label)
	}
}

// ============================================================
//  顶部状态栏
// ============================================================

func weekdayCN(t time.Time) string {
	return []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[int(t.Weekday())]
}

// DrawTopBar 顶部：左侧大字时间（HH:MM，不显示秒）+ 日期；右侧当前页名。
//
// 2026-09-27 洋哥要求：① 右上角的「CPU 温度 + 主机名」全部删掉；
// ② 原先居中的页名挪到右上角。
// 于是顶栏只剩两根轴线 —— 左边时间/日期，右边页名，右边界与所有卡片对齐。
// 删掉的 CPU 温度没有丢：性能页的环卡里有「温度」一行，系统页的 CPU 风扇卡
// 里也会显示驱动调速的实测温度；主机名在首页「系统信息」卡里。
//
// 对齐方式：以**大时间可见字形的垂直中心**作为整条顶栏的基准线。
// 不能用行高对齐 —— 字体的行盒上下留白比可见字形大得多，
// 46px 的时间与 24px 的页名按行盒对齐会明显偏上。
func DrawTopBar(c *Canvas, fs *FontSet, th *Theme, now time.Time, mon *monitor.SystemMonitor, pageName string, pageAcc RGBA) {
	DrawTopBarClock(c, fs, th, now)
	DrawTopBarName(c, fs, th, TopBarNameCY(fs, now), pageName, pageAcc)
}

// TopBarNameCY 顶栏页名的垂直基准线：与大时间的**可见字形中心**齐平。
func TopBarNameCY(fs *FontSet, now time.Time) int {
	im := glyphLatin(fs, 46, now.Format("15:04"), RGBA{255, 255, 255, 255})
	return 12 + (im.iy0+im.iy1)/2
}

// DrawTopBarClock 顶栏左侧：大时间 + 日期/星期。
//
// 单独拆出来是因为页名需要参与换页过渡（淡出淡入），而时间不该跟着一起淡 ——
// 一秒不动的时间跟着闪，是比不做过渡更糟的事。
func DrawTopBarClock(c *Canvas, fs *FontSet, th *Theme, now time.Time) {
	tim := glyphLatin(fs, 46, now.Format("15:04"), th.Text)
	blitGlyph(c, tim, PadX, 12)
	// 日期 + 星期（数字用拉丁细体，中文用 CJK）
	DrawTextMixL(c, fs, PadX, 72, FzTiny, th.Sub, now.Format("01-02")+" "+weekdayCN(now))
}

// DrawTopBarName 顶栏右侧：页名（右对齐）+ 名下一条本页语义色的短横线。
func DrawTopBarName(c *Canvas, fs *FontSet, th *Theme, cy int, pageName string, pageAcc RGBA) {
	if pageName == "" {
		return
	}
	nameCol := th.Sub
	if pageAcc.A > 0 {
		nameCol = Lit(pageAcc, 0.40)
	}
	w, inkBot := DrawTextMixRC(c, fs, c.W-PadX, cy, FzBody, nameCol, pageName)
	if pageAcc.A > 0 {
		uw := w
		if uw > 56 {
			uw = 56
		}
		c.FillRoundRect(c.W-PadX-uw, inkBot+9, uw, 2, 1, WithA(pageAcc, 200))
	}
}

// (顶栏不再显示主机名：它只在首页「系统信息」卡里出现一次)

// ============================================================
//  页面指示器
// ============================================================

// DrawDots 页面指示器（三段短横，见 design.go 的 drawTabsV2）。
//
// 精修稿把"圆点"换成了"三段 28×3 的短横"：宽度恒定、当前页那段直接上
// 该页的点缀色。旧版"激活段拉长到 26px、其余 8px"的写法会让整条在翻页时
// 左右漂移，而且 8px 宽的点在 376px 屏上只有 4 个像素的辨识度。
func DrawDots(c *Canvas, accents []RGBA, cur, y int) {
	drawTabsV2(c, accents, cur, y)
}

// DrawDotsT 换页过程中的页签：颜色在 from 与 to 之间插值，长度不变。
func DrawDotsT(c *Canvas, accents []RGBA, from, to int, e float64, y int) {
	drawTabsV2T(c, accents, from, to, e, y)
}

// ============================================================
//  信息行 / 分段控件 / 按钮
// ============================================================

// DrawKV 一行「中文标签 / 数值右对齐」（数值支持中英混排）
func DrawKV(c *Canvas, fs *FontSet, th *Theme, x, y, w int, label, val string, col RGBA) {
	DrawText(c, fs, label, x, y+3, FzTiny, th.Dim)
	DrawTextMixR(c, fs, x+w, y, FzSmall, col, val)
}

// DrawKVLine 同上，但带一条极淡的分隔线（用于列表）
func DrawKVLine(c *Canvas, fs *FontSet, th *Theme, x, y, w int, label, val string, col RGBA) {
	DrawKV(c, fs, th, x, y, w, label, val, col)
	c.FillRect(x, y+38, w, 1, th.Faint)
}

// DrawTabs 下划线式分段控件：激活项文字提亮 + 下方 2px 强调线。
// 返回每项的中心 x 坐标（供点击判定）。
func DrawTabs(c *Canvas, fs *FontSet, th *Theme, x, y, w int, items []string, active int, col RGBA) []int {
	n := len(items)
	if n == 0 {
		return nil
	}
	step := w / n
	centers := make([]int, n)
	for i, it := range items {
		cx := x + step*i + step/2
		centers[i] = cx
		tc := th.Dim
		if i == active {
			tc = th.Text
		}
		DrawTextC(c, fs, it, cx, y, FzSmall, tc)
		if i == active {
			tw := InkWidth(fs, it, FzSmall)
			c.FillRect(cx-tw/2, y+30, tw, 2, col)
		}
	}
	return centers
}

// DrawButton 细描边按钮（方角，不做胶囊）。col 传语义色，按钮就带上色相。
func DrawButton(c *Canvas, fs *FontSet, th *Theme, x, y, w, h int, label string, col RGBA) {
	c.FillRoundRect(x, y, w, h, 8, WithA(col, 22))
	c.StrokeRoundRect(x, y, w, h, 8, 1, WithA(col, 105))
	DrawTextMixC(c, fs, x+w/2, y+(h-TextHeight(fs, label, FzSmall))/2, FzSmall, th.Text, label)
}

// DrawThemeChips 主题选择胶囊。
// 每一格用它**自己主题的强调色**描边与着色 —— 一眼就能看出四套配色分别是什么调子，
// 当前项加填充 + 实边高亮。
func DrawThemeChips(c *Canvas, fs *FontSet, th *Theme, x, y, w, h int, names []string, active string) {
	n := len(names)
	if n == 0 {
		return
	}
	const gap = 8
	cw := (w - gap*(n-1)) / n
	for i, nm := range names {
		cx := x + i*(cw+gap)
		label := ThemeLabel(nm)
		ac := ThemeByName(nm).Accent

		fill := WithA(ac, 12)
		brd := WithA(ac, 60)
		col := WithA(ac, 200)
		if nm == active {
			fill = WithA(ac, 40)
			brd = ac
			col = th.Text
		}
		c.FillRoundRect(cx, y, cw, h, 8, fill)
		c.StrokeRoundRect(cx, y, cw, h, 8, 1, brd)
		DrawTextMixC(c, fs, cx+cw/2, y+(h-TextHeight(fs, label, FzSmall))/2, FzSmall, col, label)
	}
}

// ============================================================
//  进度条
// ============================================================

// DrawBar 细进度条
func DrawBar(c *Canvas, th *Theme, x, y, w, h int, pct float64, col RGBA) {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	r := h / 2
	c.FillRoundRect(x, y, w, h, r, th.Track)
	fw := int(float64(w)*pct + 0.5)
	if fw <= 0 {
		return
	}
	if fw < h {
		fw = h
	}
	c.FillRoundRect(x, y, fw, h, r, col)
}

// ============================================================
//  混排文字（中文用 CJK 字体，数字用拉丁细体）
// ============================================================

// TPart 一段文字。Latin 为 true 时使用拉丁字体族（数字/英文/符号）
type TPart struct {
	S     string
	Col   RGBA
	Latin bool
	// Bold 拉丁粗体（指标大数字用；规范第四节要求数字是全屏最粗的一档）
	Bold bool
	Size int // 0 表示沿用默认字号
	// Gap 段前额外间隙（像素）。混排里靠空格拉开距离是不精确的 ——
	// 空格宽度随字号线性放大，40px 数字旁边的空格能有 10px，
	// 小字旁边只有 4px，同一套写法在不同字号下疏密完全不同。
	// 需要「数字 ↔ 单位」「值 ↔ 标签」这类固定间隙时用它。
	Gap int
}

// drawTracked 逐字绘制并追加字距 track（yBase = 基线，返回总宽）。
//
// 为什么需要它：SDL_ttf 一次渲染整串时字距完全由字体决定，中文标题在
// 18~24px 下调完就是字字相接的一整块墨，看着发闷。这里按字推进并追加
// track 像素，标题才有呼吸感。
//
// 逐字渲染会丢掉拉丁 kerning，所以只用于**标题类短文本**；
// 数字、正文一律走整串渲染，不做字距处理。
func drawTracked(c *Canvas, fs *FontSet, s string, x, yBase, size int, col RGBA, track int) int {
	cx := x
	for _, r := range s {
		ch := string(r)
		fam := famCJK
		if isLatinRune(r) {
			fam = famLatin
		}
		var im *glyphImg
		if fam == famLatin {
			im = glyphLatin(fs, size, ch, col)
		} else {
			im = glyph(fs, size, ch, col)
		}
		if im != nil && im.h > 0 {
			blitGlyph(c, im, cx, yBase-baselineOff(fs, fam, size))
			cx += im.w
		}
		cx += track
	}
	return cx - x
}

// isLatinRune 判断该字符应交给拉丁字体族（数字/英文等）
func isLatinRune(r rune) bool {
	switch {
	case r < 0x2E80: // ASCII、拉丁扩展、希腊、西里尔
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK 标点
		return false
	case r >= 0x3040 && r <= 0x33FF: // 假名、注音、CJK 符号
		return false
	case r >= 0x4E00 && r <= 0x9FFF: // 汉字
		return false
	case r >= 0xF900 && r <= 0xFAFF: // 兼容汉字
		return false
	case r >= 0xFF00: // 全角形式
		return false
	}
	return true
}

// splitParts 把混排字符串按字体族切成若干段
func splitParts(s string, col RGBA) []TPart {
	var out []TPart
	var cur []rune
	curLatin := false
	flush := func() {
		if len(cur) > 0 {
			out = append(out, TPart{S: string(cur), Col: col, Latin: curLatin})
			cur = nil
		}
	}
	for _, r := range s {
		l := isLatinRune(r)
		if len(cur) > 0 && l != curLatin {
			flush()
		}
		curLatin = l
		cur = append(cur, r)
	}
	flush()
	return out
}

// DrawTextMixL 自动按字体族分段、左对齐绘制（yTop 为文字块顶部），返回宽度
func DrawTextMixL(c *Canvas, fs *FontSet, x, yTop, size int, col RGBA, s string) int {
	if s == "" {
		return 0
	}
	parts := splitParts(s, col)
	w, h := measureParts(fs, size, parts)
	DrawPartsL(c, fs, x, yTop+h, size, parts)
	return w
}

// DrawTextMixR 自动分段、右对齐（rx 为右边界）
func DrawTextMixR(c *Canvas, fs *FontSet, rx, yTop, size int, col RGBA, s string) int {
	if s == "" {
		return 0
	}
	parts := splitParts(s, col)
	w, h := measureParts(fs, size, parts)
	DrawPartsL(c, fs, rx-w, yTop+h, size, parts)
	return w
}

// DrawTextMixC 自动分段、水平居中
func DrawTextMixC(c *Canvas, fs *FontSet, cx, yTop, size int, col RGBA, s string) int {
	if s == "" {
		return 0
	}
	parts := splitParts(s, col)
	w, h := measureParts(fs, size, parts)
	DrawPartsL(c, fs, cx-w/2, yTop+h, size, parts)
	return w
}

func partSize(p TPart, def int) int {
	if p.Size > 0 {
		return p.Size
	}
	return def
}

func measureParts(fs *FontSet, def int, parts []TPart) (int, int) {
	w, h := 0, 0
	for _, p := range parts {
		sz := partSize(p, def)
		w += p.Gap
		switch {
		case p.Bold:
			w += TextWidthB(fs, p.S, sz)
			if hh := TextHeightB(fs, p.S, sz); hh > h {
				h = hh
			}
		case p.Latin:
			w += TextWidthL(fs, p.S, sz)
			if hh := TextHeightL(fs, p.S, sz); hh > h {
				h = hh
			}
		default:
			w += TextWidth(fs, p.S, sz)
			if hh := TextHeight(fs, p.S, sz); hh > h {
				h = hh
			}
		}
	}
	return w, h
}

// DrawPartsL 从左边界 x 起绘制混排文字（yBase 为整行底部），返回总宽
func DrawPartsL(c *Canvas, fs *FontSet, x, yBase, def int, parts []TPart) int {
	cx := x
	for _, p := range parts {
		sz := partSize(p, def)
		cx += p.Gap
		switch {
		case p.Bold:
			hh := TextHeightB(fs, p.S, sz)
			DrawTextB(c, fs, p.S, cx, yBase-hh, sz, p.Col)
			cx += TextWidthB(fs, p.S, sz)
		case p.Latin:
			hh := TextHeightL(fs, p.S, sz)
			DrawTextL(c, fs, p.S, cx, yBase-hh, sz, p.Col)
			cx += TextWidthL(fs, p.S, sz)
		default:
			hh := TextHeight(fs, p.S, sz)
			DrawText(c, fs, p.S, cx, yBase-hh, sz, p.Col)
			cx += TextWidth(fs, p.S, sz)
		}
	}
	return cx - x
}

// DrawPartsC 以 cxCenter 水平居中绘制混排文字
func DrawPartsC(c *Canvas, fs *FontSet, cxCenter, yBase, def int, parts []TPart) int {
	w, _ := measureParts(fs, def, parts)
	return DrawPartsL(c, fs, cxCenter-w/2, yBase, def, parts)
}

// DrawPartsR 以 rx 为右边界绘制混排文字（yBase 为整行底部）
func DrawPartsR(c *Canvas, fs *FontSet, rx, yBase, def int, parts []TPart) int {
	w, _ := measureParts(fs, def, parts)
	return DrawPartsL(c, fs, rx-w, yBase, def, parts)
}

// mixedWidth 混排文本的绘制宽度。
//
// 不能对中英混排的串直接调 TextWidth（CJK）或 TextWidthL（拉丁）——
// 它们各自按单一字体度量，而实际绘制是分段换字体的（splitParts），
// 两者会在第二页「交换 0.4 / 4.0 GB」这种数字 + 单位 + 中文的串上对不上，
// 算错就会让进度条压到数字。这里走与绘制完全相同的分段规则。
func mixedWidth(fs *FontSet, size int, s string) int {
	if s == "" {
		return 0
	}
	w, _ := measureParts(fs, size, splitParts(s, RGBA{255, 255, 255, 255}))
	return w
}

// inkBoundsParts 计算混排文本「可见字形」的上下界（相对整块顶部）。
//
// 排版规则必须与 DrawPartsL 一致：各段按自身行盒高度在 yBase 处底对齐，
// 因此第 i 段图像的顶 = blockH - 段行高。这里按同一规则反推，
// 否则算出来的上下界与真正画出来的位置对不上。
func inkBoundsParts(fs *FontSet, def int, parts []TPart) (int, int) {
	_, blockH := measureParts(fs, def, parts)
	top, bot := blockH, 0
	for _, p := range parts {
		sz := partSize(p, def)
		var im *glyphImg
		var hh int
		switch {
		case p.Bold:
			im = glyphBold(fs, sz, p.S, p.Col)
			hh = TextHeightB(fs, p.S, sz)
		case p.Latin:
			im = glyphLatin(fs, sz, p.S, p.Col)
			hh = TextHeightL(fs, p.S, sz)
		default:
			im = glyph(fs, sz, p.S, p.Col)
			hh = TextHeight(fs, p.S, sz)
		}
		if im.inkH() == 0 {
			continue
		}
		y0 := blockH - hh + im.iy0
		y1 := blockH - hh + im.iy1
		if y0 < top {
			top = y0
		}
		if y1 > bot {
			bot = y1
		}
	}
	if bot < top {
		bot = top
	}
	return top, bot
}

// ============================================================
//  墨迹锚版式（精修稿的坐标全是「可见墨迹」坐标）
//
//  DrawPartsL 的 yBase 是**行盒底部**，而行盒里带 ascent 空隙 ——
//  照规范里的「标题墨迹顶 +16」直接当行盒坐标用，每处都会低 3~6px，
//  一排看下来就是"说不上哪里不对但就是不齐"。
//  下面三个以墨迹顶为锚，规范里的数值可以直接抄。
// ============================================================

// inkHeightParts 整行可见墨迹高度
func inkHeightParts(fs *FontSet, def int, parts []TPart) int {
	top, bot := inkBoundsParts(fs, def, parts)
	if bot <= top {
		return 0
	}
	return bot - top
}

// ------------------------------------------------------------
//  基线对齐
//
//  DrawPartsL 是「按位图底边对齐」的：位图底边 = 基线 + 降部，
//  而不同字号的降部比例不同。46px 的降部约 11px、17px 的只有 4px，
//  于是"大数字 + 小单位"按底边对齐时，小单位的基线会比数字低 7px ——
//  表现出来就是「%」永远吊在数字右下方，像掉下去了一样。
//
//  真正的对齐基准是**基线**：每个字号先量一次「H 的墨迹底」当作基线
//  位置（H 没有降部，墨迹底 = 基线），之后所有段都按它落位。
// ------------------------------------------------------------

const (
	famCJK = iota
	famLatin
	famBold
)

type baseKey struct{ fam, size int }

var baseCache = map[baseKey]int{}

// baselineOff 字号 → 「位图顶到基线」的像素数（按字体族各量一次）
func baselineOff(fs *FontSet, fam, size int) int {
	key := baseKey{fam, size}
	if v, ok := baseCache[key]; ok {
		return v
	}
	var im *glyphImg
	switch fam {
	case famBold:
		im = glyphBold(fs, size, "H", colWhite)
	case famLatin:
		im = glyphLatin(fs, size, "H", colWhite)
	default:
		im = glyph(fs, size, "国", colWhite)
	}
	off := 0
	if im != nil && im.inkH() > 0 {
		off = im.iy1
	}
	baseCache[key] = off
	return off
}

func partFam(p TPart) int {
	switch {
	case p.Bold:
		return famBold
	case p.Latin:
		return famLatin
	}
	return famCJK
}

func partGlyph(fs *FontSet, p TPart, sz int) *glyphImg {
	switch {
	case p.Bold:
		return glyphBold(fs, sz, p.S, p.Col)
	case p.Latin:
		return glyphLatin(fs, sz, p.S, p.Col)
	}
	return glyph(fs, sz, p.S, p.Col)
}

// DrawPartsBase 按**基线**绘制：yBase 是那条基线，不是行盒底边。
func DrawPartsBase(c *Canvas, fs *FontSet, x, yBase, def int, parts []TPart) int {
	cx := x
	for _, p := range parts {
		sz := partSize(p, def)
		cx += p.Gap
		off := baselineOff(fs, partFam(p), sz)
		switch {
		case p.Bold:
			DrawTextB(c, fs, p.S, cx, yBase-off, sz, p.Col)
			cx += TextWidthB(fs, p.S, sz)
		case p.Latin:
			DrawTextL(c, fs, p.S, cx, yBase-off, sz, p.Col)
			cx += TextWidthL(fs, p.S, sz)
		default:
			DrawText(c, fs, p.S, cx, yBase-off, sz, p.Col)
			cx += TextWidth(fs, p.S, sz)
		}
	}
	return cx - x
}

// partsInkRise 把「墨迹顶」换算成基线所需的抬升量
func partsInkRise(fs *FontSet, def int, parts []TPart) int {
	rise := 0
	for _, p := range parts {
		sz := partSize(p, def)
		im := partGlyph(fs, p, sz)
		if im == nil || im.inkH() == 0 {
			continue
		}
		if r := baselineOff(fs, partFam(p), sz) - im.iy0; r > rise {
			rise = r
		}
	}
	return rise
}

// DrawPartsTop 左对齐，yInk 为**可见墨迹顶部**
func DrawPartsTop(c *Canvas, fs *FontSet, x, yInk, def int, parts []TPart) int {
	return DrawPartsBase(c, fs, x, yInk+partsInkRise(fs, def, parts), def, parts)
}

// DrawPartsTopR 右对齐（rx 为右边界），yInk 为可见墨迹顶部
func DrawPartsTopR(c *Canvas, fs *FontSet, rx, yInk, def int, parts []TPart) int {
	w, _ := measureParts(fs, def, parts)
	return DrawPartsTop(c, fs, rx-w, yInk, def, parts)
}

// DrawPartsTopC 水平居中，yInk 为可见墨迹顶部
func DrawPartsTopC(c *Canvas, fs *FontSet, cx, yInk, def int, parts []TPart) int {
	w, _ := measureParts(fs, def, parts)
	return DrawPartsTop(c, fs, cx-w/2, yInk, def, parts)
}

// drawTrackedTop 带字距的标题，yInk 为可见墨迹顶部（返回总宽）。
func drawTrackedTop(c *Canvas, fs *FontSet, s string, x, yInk, size int, col RGBA, track int) int {
	if s == "" {
		return 0
	}
	rise := 0
	for _, r := range s {
		fam := famCJK
		if isLatinRune(r) {
			fam = famLatin
		}
		var im *glyphImg
		if fam == famLatin {
			im = glyphLatin(fs, size, string(r), col)
		} else {
			im = glyph(fs, size, string(r), col)
		}
		if im == nil || im.inkH() == 0 {
			continue
		}
		if d := baselineOff(fs, fam, size) - im.iy0; d > rise {
			rise = d
		}
	}
	return drawTracked(c, fs, s, x, yInk+rise, size, col, track)
}

// DrawTextMixRC 混排文本：右边界 rx 对齐，且**可见字形垂直中心**落在 cy 上。//
// 返回 (宽度, 可见字形下沿的 y)。顶栏右上角的页名要求"与左侧大时间齐平"，
// 而时间是 46px、页名是 24px，按行盒对齐会明显偏上 —— 只有按字形对齐才对。
func DrawTextMixRC(c *Canvas, fs *FontSet, rx, cy, size int, col RGBA, s string) (int, int) {
	if s == "" {
		return 0, cy
	}
	parts := splitParts(s, col)
	w, blockH := measureParts(fs, size, parts)
	top, bot := inkBoundsParts(fs, size, parts)
	yTop := cy - (top+bot)/2
	DrawPartsL(c, fs, rx-w, yTop+blockH, size, parts)
	return w, yTop + bot
}

// DrawArrow 画一个小箭头（↑ / ↓）：一根竖杆 + 两笔箭头。
//
// 不用字体里的 U+2191 / U+2193 —— 那两个字形在所选字体里不一定存在，
// 缺字会渲染成豆腐块；用线条画则形状、粗细、位置都可控。
func DrawArrow(c *Canvas, cx, cy, size int, up bool, col RGBA) {
	const t = 2.0
	dir := 1.0
	if up {
		dir = -1
	}
	fx, fy, fs2 := float64(cx), float64(cy), float64(size)
	tipY := fy + dir*fs2
	tailY := fy - dir*fs2
	c.Line(fx, tailY, fx, tipY, t, col)
	head := fs2 * 0.62
	c.Line(fx-head, tipY-dir*head, fx, tipY, t, col)
	c.Line(fx+head, tipY-dir*head, fx, tipY, t, col)
}

// ============================================================
//  速率行（↑ / ↓ + 数值 + 单位，整体右对齐）
// ============================================================

// DrawRateRow 右上角一行速率：箭头 + 数值（大） + 单位（小），右边界 rx，
// 可见字形垂直中心落在 cy。箭头用该方向的语义色，数值提亮一档。
func DrawRateRow(c *Canvas, fs *FontSet, th *Theme, rx, cy int, v float64, col RGBA, up bool) {
	num := fmt.Sprintf("%.2f", v)
	nim := glyphLatin(fs, FzBody, num, Lit(col, 0.28))
	if nim.inkW() == 0 {
		return
	}
	uim := glyphLatin(fs, FzTiny, "MB/s", th.Dim)
	nw, uw := nim.inkW(), uim.inkW()

	numLeft := rx - uw - 6 - nw
	numTop := cy - (nim.iy0+nim.iy1)/2
	blitGlyph(c, nim, numLeft-nim.ix0, numTop)
	// 单位与数值的**基线**对齐（数字无降部，字形下沿即基线）
	numBot := numTop + nim.iy1
	blitGlyph(c, uim, rx-uim.ix1-1, numBot-uim.iy1)
	DrawArrow(c, numLeft-14, cy, 7, up, col)
}

// DrawNumUnitL 左对齐的「大数字 + 小单位」（yBase 为数字底部），返回总宽
func DrawNumUnitL(c *Canvas, fs *FontSet, x, yBase int, num, unit string,
	numSize, unitSize int, numCol, unitCol RGBA) int {

	nim := glyphLatin(fs, numSize, num, numCol)
	nw := nim.inkW()
	if nw == 0 {
		return 0
	}
	blitGlyph(c, nim, x-nim.ix0, yBase-nim.iy1-1)
	if unit == "" {
		return nw
	}
	uim := glyphLatin(fs, unitSize, unit, unitCol)
	uw := uim.inkW()
	uh := uim.inkH()
	if uw > 0 {
		blitGlyph(c, uim, x+nw+8-uim.ix0, yBase-uh-4-uim.iy0)
	}
	return nw + 8 + uw
}
