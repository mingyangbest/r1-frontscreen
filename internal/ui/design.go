package ui

import (
	"math"
	"sync"
)

// ============================================================
//  设计规范 v2（2026-10-30 洋哥给的精修稿）
//
//  这份文件是**唯一**的取值来源：色板、圆角、边距、字阶、卡片渐变、
//  图表网格、页签、药丸、信号条都在这里定义，页面代码只许引用不许另写。
//  上一版的问题不是配色难看，而是每个卡片各写各的 18/22/24，一排看下来
//  左边线参差不齐 —— 把数值收拢到一处，是"看起来专业"最省力的做法。
//
//  规范原文（2 倍图数值，落到设备像素除以 2）：
//    画布 376×960 · 页边距 48→24 · 卡片间距 24→12~20 · 圆角 40→20
//    卡内左右留白 24→12 · 大卡标题 36→18 · 右上元信息 32→16
//    小卡数字 88→44 · 图线 3→1.5 · 网格 3 条 10/255 · 脉冲点 r8→4 + 光晕 r22→11
//    进度条高 10→5 · 页签 56×6 间距 24 → 28×3 间距 12 · 药丸圆角 40→20
//
//  卡面渐变（规范第三节）：顶部 = 点缀色 × 22%，底部 = 卡底 #0D0E12。
//  参考图逐像素采样核对过：CPU 历史卡顶部 (30,37,33)，SAGE(143,174,158)×0.22
//  = (31,38,35)；内存卡顶部 (40,32,32)，ROSE(192,154,154)×0.22 = (42,34,34)。
//  不是"低透明度涂层"——涂层会随底色变化，渐变不会。
// ============================================================

// ---------------- 色板（锁死） ----------------

var (
	colBg      = RGBA{8, 8, 10, 255}   // #08080A 整屏底色
	colCardBot = RGBA{13, 14, 18, 255} // #0D0E12 卡片渐变底部

	colSlate = RGBA{147, 167, 196, 255} // #93A7C4 CPU 卡 / 控制卡 / 上传
	colRose  = RGBA{192, 154, 154, 255} // #C09A9A 内存卡
	colSand  = RGBA{201, 168, 120, 255} // #C9A878 温度卡 / 药丸圆点 / 详情页签
	colSage  = RGBA{143, 174, 158, 255} // #8FAE9E CPU 历史曲线 / 环境页签
	colMint  = RGBA{127, 191, 165, 255} // #7FBFA5 网络 / 磁盘 / 下载 / 令牌圆点
	colCoral = RGBA{198, 140, 130, 255} // #C68C82 详情页 C1 核心条
	colPale  = RGBA{150, 168, 182, 255} // #96A8B6 详情页 C2 核心条

	colWhite = RGBA{255, 255, 255, 255}
	// colData 指标数字的常态色。
	//
	// 不用纯白：376×960 这块屏上 #FFFFFF 会"发光"，一排大数字看久了扎眼。
	// 先降到 #E6E6E6 还是偏亮（洋哥 2026-10-31：粗体数字太白了，不够高级），
	// 再降一档到 #D4D4D4 —— 比纯白柔一大截，又仍明显亮于次白 #C9C9C9，
	// 大数字和副文案之间还留得住层次。时钟和卡片标题仍用纯白：
	// 它们是"信息层次"的顶端，不是被读数的对象。
	colData  = RGBA{212, 212, 212, 255}
	colSub   = RGBA{201, 201, 201, 255} // #C9C9C9 次要文字
	colDim   = RGBA{138, 138, 138, 255} // #8A8A8A 第三级文字
	colTrack = RGBA{42, 42, 46, 255}    // #2A2A2E 进度条 / 滑杆轨道
	colTab   = RGBA{58, 58, 58, 255}    // #3A3A3A 未激活页签

	colBtnBg = RGBA{48, 36, 36, 255}    // #302424 关闭屏幕按钮填充
	colBtnTx = RGBA{208, 160, 160, 255} // #D0A0A0
	colPill  = RGBA{22, 23, 27, 255}    // #16171B 底部药丸
)

// valColor 指标读数着色（阈值是洋哥 2026-10-30 定的）：
//
//	< 50  → 偏灰白（常态，别让正常值抢占注意力）
//	>= 50 → 沙金（留意）
//	>= 80 → 珊瑚（告警）
//
// 首页三张小卡、详情页各核百分比、内存、两个温度都走这一条 ——
// 同一件事在哪儿都是同一个颜色，不用每张卡各记一套规则。
// 磁盘是例外：只分"<80 常态 / >=80 黄"两档，见磁盘卡里的注释。
//
// 先四舍五入再判档：屏上写的是 "%.0f"，49.6 会显示成 50，
// 要是拿 49.6 去判就会画出"白字的 50"—— 显示值和颜色必须同源。
func valColor(v float64) RGBA {
	switch r := math.Round(v); {
	case r >= 80:
		return colCoral
	case r >= 50:
		return colSand
	}
	return colData
}

// barColor 进度条填充色（阈值是洋哥 2026-10-31 定的）：
//
//	< 50  → 绿（常态）
//	>= 50 → 沙金（留意）
//	>= 80 → 珊瑚（告警）
//
// 原先四核各有一个"身份色"（灰蓝/珊瑚/浅灰蓝/灰绿），但 C1 那个珊瑚正好
// 撞上 >=80 的告警红 —— 45% 也是红的，红色就不再意味着"出事了"。所以
// 条的颜色只编码状态、不编码身份：几号核看左边的 C0~C3 就够了。
// 数字仍走 valColor（<50 是偏灰白），整行数字全绿反而看不出层级。
func barColor(v float64) RGBA {
	switch r := math.Round(v); {
	case r >= 80:
		return colCoral
	case r >= 50:
		return colSand
	}
	return colMint
}

// diskBarColor 磁盘进度条填充色。
//
// 磁盘只有一档阈值（洋哥定的 80%，没有 CPU 那档 50，也不要红色）：
// <80 绿（正常）、>=80 沙金（该清理了）。右端百分比在同一个 80% 处变黄，
// 条和数字同步跳色 —— 否则会出现"86% 的黄字配一条绿条"这种自相矛盾的画面。
func diskBarColor(v float64) RGBA {
	if math.Round(v) >= 80 {
		return colSand
	}
	return colMint
}

// ---------------- 版式（设备像素） ----------------

const (
	DPadX  = 24  // 页边距
	DCardW = 328 // 卡片宽（376 - 2×24）
	DRad   = 20  // 卡片圆角
	DPad   = 12  // 卡内左右留白
	DGap   = 20  // 卡片纵向间距
	DTopH  = 128 // 非全出血页的页头高度（页名 + 标题）

	// 字号。中文字宽≈字号、拉丁数字墨迹高≈0.72 字号，
	// 所以这几个数字是照参考图墨迹高度反推的，不是拍脑袋。
	FzClock = 100 // 大时钟
	FzTitle = 32  // 页标题（「详情」「系统」）
	FzPage  = 40  // 系统页设备卡里的主机名（实测参考图墨迹 30px，36px 只有 27）
	FzCard  = 18  // 卡标题
	FzMini  = 15  // 小卡标题（100px 宽的卡放 18 会顶到右内边距）
	FzNum   = 46  // 小卡数字
	FzData  = 48  // 详情/系统页大数字
	FzUnit  = 17  // 单位
	FzMeta  = 16  // 元信息 / 副文案
)

// mixBlack 把颜色与黑按 alpha 混合（alpha=0.22 即规范里的「点缀色 × 22%」）
func mixBlack(c RGBA, a float64) RGBA {
	f := func(x uint8) uint8 { return uint8(float64(x)*a + 0.5) }
	return RGBA{f(c.R), f(c.G), f(c.B), 255}
}

// ============================================================
//  卡面（烘焙缓存）
// ============================================================

type cardFaceKey struct {
	w, h    int
	r, g, b uint8
}

var (
	faceMu    sync.Mutex
	faceCache = map[cardFaceKey]*Sprite{}
)

// cardFace 卡片底：顶部点缀色×22% → 底部 #0D0E12 的纵向线性渐变 + 20 圆角。
//
// 烘焙成 Sprite 而不是每帧逐像素算：屏幕上卡片是面积最大的元素，
// 一次 blit 换掉每帧三十万次浮点插值，这是 CPU ≤5% 约束下的唯一打法。
func cardFace(w, h int, accent RGBA) *Sprite {
	key := cardFaceKey{w, h, accent.R, accent.G, accent.B}
	faceMu.Lock()
	if sp, ok := faceCache[key]; ok {
		faceMu.Unlock()
		return sp
	}
	faceMu.Unlock()

	m := roundMaskFor(w, h, DRad)
	sp := &Sprite{W: w, H: h, Pix: make([]byte, w*h*4)}
	top := mixBlack(accent, 0.22)
	den := float64(h - 1)
	if den <= 0 {
		den = 1
	}
	for y := 0; y < h; y++ {
		col := Lerp(top, colCardBot, float64(y)/den)
		row := y * w
		for x := 0; x < w; x++ {
			cov := m.cov[row+x]
			if cov == 0 {
				continue
			}
			p := (row + x) * 4
			sp.Pix[p] = col.R
			sp.Pix[p+1] = col.G
			sp.Pix[p+2] = col.B
			sp.Pix[p+3] = cov
		}
	}
	faceMu.Lock()
	if len(faceCache) > 64 {
		faceCache = map[cardFaceKey]*Sprite{}
	}
	faceCache[key] = sp
	faceMu.Unlock()
	return sp
}

// DCard 画一张卡（含按压脉冲），返回卡内左缘 x（= 标题起点前一位）。
func DCard(c *Canvas, d *DrawCtx, x, y, w, h int, accent RGBA) {
	c.DrawSprite(cardFace(w, h, accent), x, y)
	if ov := d.PressOverlay(x, y, w, h); ov > 0 {
		c.FillRoundRect(x, y, w, h, DRad, WithA(colWhite, uint8(ov*255+0.5)))
	}
}

// CardPadY 卡面在「内容框」上下各外扩的高度（设备像素）。
//
// 参考图的卡片比「标题顶到内容底」这个内容框上下各高 4px。之前直接按内容框
// 画卡面，整屏每张卡都矮 8px —— 单看没问题，和参考图并排一比就是"卡片更瘪"。
const CardPadY = 4

// DCardBox 画卡片本体：参数是**内容框**（卡内所有元素都按它算坐标），
// 卡面在内容框基础上向上、向下各扩 CardPadY。
func DCardBox(c *Canvas, d *DrawCtx, x, y, w, h int, accent RGBA) {
	DCard(c, d, x, y-CardPadY, w, h+2*CardPadY, accent)
}

// DCardTitle 卡标题：18px、墨迹顶 yInk、左内边距对齐。
// 颜色走卡片点缀色（规范：详情/系统页卡标题带色相，环境页两张主卡用白色）。
func DCardTitle(c *Canvas, fs *FontSet, x, yInk int, text string, col RGBA) int {
	return drawTrackedTop(c, fs, text, x+DPad, yInk, FzCard, col, 0)
}

// DMetaR 卡头右上元信息：16px、右对齐、与标题**同墨迹中心**。
func DMetaR(c *Canvas, fs *FontSet, xRight, yInk int, text string, col RGBA) int {
	parts := splitParts(text, col)
	h := inkHeightParts(fs, FzMeta, parts)
	return DrawPartsTopR(c, fs, xRight-DPad, yInk+(FzCard-h)/2, FzMeta, parts)
}

// ============================================================
//  页头
// ============================================================

// DrawTopBarV2 非全出血页的页头：左上大标题 + 右上手势提示。
//
// 顶部居中的小页名已删（洋哥 2026-10-31：每页最上面那两个字要去掉）——
// 它的「位置指示」功能和底部三点页签完全重复，环境页那两个字更是零信息量。
// 删掉后省出的顶栏高度给了卡间隙（20 → 28），大标题从 84 上移到 56 补位。
func DrawTopBarV2(c *Canvas, fs *FontSet, name, hint string) {
	DrawPartsTop(c, fs, DPadX, 56, FzTitle, splitParts(name, colWhite))
	if hint != "" {
		DrawPartsTopR(c, fs, DPadX+DCardW, 62, FzUnit, splitParts(hint, colDim))
	}
}

// hintOf 页头右上角的手势提示。
//
// 系统页这一条原本写「滑左回首页」—— 但向左滑到底其实是循环回第一页，
// 提示反而在描述一个不存在的动作。这里改成提示「上滑换主题」：
// 换主题是个隐藏手势，不给提示没人会发现。
func hintOf(name string) string {
	switch name {
	case "详情":
		return "滑左翻页"
	case "系统":
		return "上滑换主题"
	}
	return ""
}

// ============================================================
//  页签（三段短横）
// ============================================================

const (
	DTabW   = 28
	DTabH   = 3
	DTabGap = 12
	DTabY   = 908
	// DSysTabY 系统页的页签 y：这一页比另两页多一行令牌药丸
	// （药丸 874..913），页签被推到 937..939。参考图三栏实体一致。
	DSysTabY = 937
)

// tabYOf 取某页页签的 y：页面自己报过就用它的，否则用默认 908。
func tabYOf(p Page) int {
	if t, ok := p.(TabYer); ok {
		return t.TabY()
	}
	return DTabY
}

// drawTabsV2 三段等长短横，当前页那段用该页点缀色，其余 #3A3A3A。
func drawTabsV2(c *Canvas, accents []RGBA, cur, y int) {
	n := len(accents)
	if n == 0 {
		return
	}
	total := n*DTabW + (n-1)*DTabGap
	x := (c.W - total) / 2
	for i := 0; i < n; i++ {
		col := colTab
		if i == cur {
			col = accents[i]
		}
		c.FillRoundRect(x, y, DTabW, DTabH, DTabH/2, col)
		x += DTabW + DTabGap
	}
}

// drawTabsV2T 换页过渡：颜色在两页点缀色之间插值（长度不变，整条不漂移）。
func drawTabsV2T(c *Canvas, accents []RGBA, from, to int, e float64, y int) {
	n := len(accents)
	if n == 0 {
		return
	}
	if from == to || from < 0 || to < 0 || from >= n || to >= n || e <= 0 {
		drawTabsV2(c, accents, to, y)
		return
	}
	if e >= 1 {
		drawTabsV2(c, accents, to, y)
		return
	}
	total := n*DTabW + (n-1)*DTabGap
	x := (c.W - total) / 2
	for i := 0; i < n; i++ {
		col := colTab
		switch i {
		case from:
			col = Lerp(accents[from], colTab, e)
		case to:
			col = Lerp(colTab, accents[to], e)
		}
		c.FillRoundRect(x, y, DTabW, DTabH, DTabH/2, col)
		x += DTabW + DTabGap
	}
}

// ============================================================
//  药丸（底部状态条）
// ============================================================

// DPill 一只药丸：填充 #16171B、无描边、圆角半高。
func DPill(c *Canvas, x, y, w, h int) {
	c.FillRoundRect(x, y, w, h, h/2, colPill)
}

// DPillDot 药丸左侧圆点。
//
// 半径 4.3（直径 ≈9）：参考图逐列量出来是 9px 宽（x 36..44），
// 上一版 3.2 只有 6.4px，在药丸里像个针孔。
func DPillDot(c *Canvas, cx, cy int, col RGBA) {
	c.FillCircle(float64(cx), float64(cy), 4.3, col)
}

// ============================================================
//  信号条
// ============================================================

// sigPattern 16 根竖条的固定起伏（规范：高度按固定起伏 pattern，不随数据变）。
//
// 2026-10-30 从参考图逐根量出：公网那一组 16 根全都画了出来（只是颜色分亮/暗），
// 高度是 [8,12,16,20,16,12] 的六拍循环 —— 8,12,16,20,16,12,8,12,16,20,16,12,8,12,16,20。
// 上一版 {6,9,12,15,18,20,17,13,10,14,17,12,8,11,15,19} 是单向爬坡再跌，
// 亮 11 根时最高只到 11px，而参考图同一组最高 20px —— 整排条矮了快一半。
var sigPattern = [16]int{8, 12, 16, 20, 16, 12, 8, 12, 16, 20, 16, 12, 8, 12, 16, 20}

// DSigBars 画 n 根信号条，右边界 xRight、底部 yBase；前 lit 根点亮。
//
// lit 是**唯一的数据入口**：条高永远是同一条起伏曲线（装饰），
// 亮几根才表示状态 —— 这样"信号强弱"不会被误读成一个精确读数。
func DSigBars(c *Canvas, xRight, yBase, lit int, accent RGBA) {
	const n = 16
	const bw, gap = 4, 3
	x := xRight - (n*bw + (n-1)*gap)
	for i := 0; i < n; i++ {
		h := sigPattern[i]
		col := colTrack
		if i < lit {
			col = accent
		}
		c.FillRoundRect(x, yBase-h, bw, h, 1, col)
		x += bw + gap
	}
}

// ============================================================
//  滑杆
// ============================================================

// DSlider 亮度滑杆：轨道灰 + 填充色 + 白色圆钮。
func DSlider(c *Canvas, x, y, w, h int, pct float64, fill RGBA) {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	c.Bar(x, y, w, h, h/2, pct, colTrack, fill)
	c.FillCircle(float64(x)+float64(w)*pct, float64(y)+float64(h)/2, 6.5, colWhite)
}
