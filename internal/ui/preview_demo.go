package ui

// ============================================================
//  设计稿预览（开发工具，不参与正式运行）
//
//  为什么要有这个文件：改卡片版面时，「在真机上编译一次、抓屏一次」的
//  循环成本大约 90 秒，而且每次只能看到一个方案 —— 四个方案并排比较
//  要跑四轮。这里把同一条渲染管线（m3Card / cardSkin / 同一套字体与
//  色板）搬进一张画布，一次渲染输出全部候选，A/B 对比才有意义。
//
//  约束：**必须复用正式绘制的函数**。如果这里另写一套「示意图形」，
//  预览和真机就会长得不一样，那这份预览等于骗人。
// ============================================================

import "fmt"

// prvData 预览用示例数据。
//
// 取真机实测值（2026-10-29 env.png 同一时刻）：CPU 12%、四核
// 12/9/14/13、load 0.48/0.42/0.39、47°C。用真实量级而不是 50% 那种
// 中间值，才能看出「低位状态下卡片空不空」——这正是原版最难看的地方。
var prvData = struct {
	CPU   float64
	Cores []float64
	Load  [3]float64
	Temp  float64
}{
	CPU:   12,
	Cores: []float64{12.4, 9.1, 14.2, 13.0},
	Load:  [3]float64{0.48, 0.42, 0.39},
	Temp:  47,
}

// RenderCPUVariantsDemo 渲染四方案对比图（376 × 960，与真机同宽）。
func RenderCPUVariantsDemo(fs *FontSet) *Canvas {
	c := NewCanvas(376, 960)
	d := NewDrawCtx(fs)
	th := d.Theme

	c.FillRectGrad(0, 0, 376, 960, th.BgTop, th.BgBot)

	// ---- 顶部标题 ----
	drawTracked(c, fs, "CPU 卡片 · 四方案", PadX, 44, FzBody, th.Text, 1)
	DrawPartsL(c, fs, PadX, 70, FzTiny, []TPart{
		{S: "同款渲染管线 · 真机字体 · 示例数据为实测值", Col: th.Dim},
	})

	y := 88

	y = prvBlock(c, d, y, "A", "合并总览卡", 168, "删掉顶部重复的小卡，四项合一", prvA)
	y = prvBlock(c, d, y, "B", "系统负载专卡", 128, "保留小卡，只讲 1/5/15 分钟 load", prvB)
	y = prvBlock(c, d, y, "C", "每核 2×2 网格", 172, "四核各占一格，单核数值可读", prvC)
	y = prvBlock(c, d, y, "D", "分段条（最省地方）", 96, "四核压成一根分段条，高度减半", prvD)

	return c
}

// prvBlock 画一个方案区块：块头（编号 + 名称 + 高度 + 一句话说明）+ 卡片
func prvBlock(c *Canvas, d *DrawCtx, y int, tag, name string, h int, desc string,
	draw func(*Canvas, *DrawCtx, int, int)) int {
	th, fs := d.Theme, d.Fonts

	// 编号用方框圈起来，和卡片区分开
	c.FillRoundRect(PadX, y, 26, 26, 8, th.Card)
	DrawTextLC(c, fs, tag, PadX+13, y+5, FzTiny, th.Text)

	drawTracked(c, fs, name, PadX+36, y+19, FzSmall, th.Text, 0)

	// 右侧：卡高标注
	ht := fmt.Sprintf("%dpx", h)
	DrawTextLR(c, fs, ht, PadX+CardW, y+18, FzTiny, th.Dim)

	DrawPartsL(c, fs, PadX+36, y+42, FzTiny, []TPart{{S: desc, Col: th.Dim}})

	draw(c, d, PadX, y+50)
	return y + 50 + h + 18
}

// ------------------------------------------------------------
//
//	A · 合并总览卡
//
//	左栏：64px 大数字 + 核数/温度脚注
//	右栏：四核竖排（核名 · 条 · 百分比），每行都读得出数
//	卡头右侧：load 三档 —— 从「一个没人看得懂的 0.48」变成「1/5/15 分钟」
//
// ------------------------------------------------------------
func prvA(c *Canvas, d *DrawCtx, x, y int) {
	th, fs := d.Theme, d.Fonts
	const h = 168

	m3Card(c, d, x, y, CardW, h, hueBlue, "CPU")
	m3Note(c, fs, x+CardW-m3Pad, y+m3TitleBase, th,
		fmt.Sprintf("负载 %.2f / %.2f / %.2f", prvData.Load[0], prvData.Load[1], prvData.Load[2]))

	// 左栏大数字：基线 +110，64px 墨迹约 +64~+110
	DrawPartsL(c, fs, x+m3Pad, y+110, 64, []TPart{
		{S: fmt.Sprintf("%.0f", prvData.CPU), Col: th.Text, Latin: true},
		{S: "%", Col: th.Sub, Latin: true, Size: FzBody, Gap: 2},
	})
	// 脚注：核数 + 温度（温度读不到时 tempText 给「—」，这里走同一函数）
	DrawPartsL(c, fs, x+m3Pad, y+134, FzTiny, []TPart{
		{S: "4 核", Col: th.Dim},
		{S: " · ", Col: th.Faint},
		{S: tempText(prvData.Temp), Col: th.Dim},
	})

	// 右栏四核竖排：起点 x+156，可用到 x+314
	for i := 0; i < 4 && i < len(prvData.Cores); i++ {
		base := y + 64 + i*28
		cx := x + 156
		drawTracked(c, fs, fmt.Sprintf("C%d", i), cx, base, FzTiny, th.Sub, 0)
		v := prvData.Cores[i]
		c.Bar(cx+26, base-14, 84, 8, 4, clamp(v, 0, 100)/100,
			th.Track, m3Bar(hueBlue, v, 90, th.Danger))
		DrawPartsL(c, fs, cx+118, base, FzTiny, []TPart{
			{S: fmt.Sprintf("%.0f", v), Col: th.Sub, Latin: true},
		})
	}
}

// ------------------------------------------------------------
//
//	B · 系统负载专卡
//
//	三列 load（1/5/15 分钟），每列下面标口径。
//	卡头右侧给「每核 0.12」——load 是进程数，不换算成每核多少，
//	4 核机器上看到 0.48 根本不知道算高算低。
//
// ------------------------------------------------------------
func prvB(c *Canvas, d *DrawCtx, x, y int) {
	th, fs := d.Theme, d.Fonts
	const h = 128

	m3Card(c, d, x, y, CardW, h, hueTeal, "系统负载")
	m3Note(c, fs, x+CardW-m3Pad, y+m3TitleBase, th,
		fmt.Sprintf("4 核 · 每核 %.2f", prvData.Load[0]/4))

	labs := [3]string{"1 分钟", "5 分钟", "15 分钟"}
	for i := 0; i < 3; i++ {
		cx := x + m3Pad + 49 + i*97
		v := fmt.Sprintf("%.2f", prvData.Load[i])
		w := TextWidthL(fs, v, FzMid)
		hh := TextHeightL(fs, v, FzMid)
		DrawTextL(c, fs, v, cx-w/2, y+92-hh, FzMid, th.Text)
		lw := TextWidth(fs, labs[i], FzTiny)
		DrawText(c, fs, labs[i], cx-lw/2, y+112-TextHeight(fs, labs[i], FzTiny), FzTiny, th.Dim)
	}
}

// ------------------------------------------------------------
//
//	C · 每核 2×2 网格
//
//	每格：核名 + 24px 百分比 + 通栏条。四格等宽，扫视路径是 Z 字，
//	比原来四根 6px 细条「散在 292px 里」好读得多。
//
// ------------------------------------------------------------
func prvC(c *Canvas, d *DrawCtx, x, y int) {
	th, fs := d.Theme, d.Fonts
	const h = 172

	m3Card(c, d, x, y, CardW, h, hueTeal, "CPU 负载")
	m3Note(c, fs, x+CardW-m3Pad, y+m3TitleBase, th,
		fmt.Sprintf("负载 %.2f · 4 核", prvData.Load[0]))

	// 格宽 126 + 间距 40：上一版间距 20 太窄，右格百分比和下一格核名
	// 依然会横向连读成「12% C1」。网格分界只能靠留白，不能靠元素本身。
	const cw, ch = 126, 52
	for i := 0; i < 4 && i < len(prvData.Cores); i++ {
		col, row := i%2, i/2
		gx := x + m3Pad + col*(cw+40)
		gy := y + 54 + row*(ch+6)
		v := prvData.Cores[i]

		drawTracked(c, fs, fmt.Sprintf("C%d", i), gx, gy+22, FzTiny, th.Sub, 0)

		// 百分比整体右对齐到格右缘（数字 24px + 单位 17px）
		num := fmt.Sprintf("%.0f", v)
		nw := TextWidthL(fs, num, FzBody)
		pw := TextWidthL(fs, "%", FzTiny)
		rx := gx + cw
		DrawTextL(c, fs, num, rx-nw-pw, gy+24-TextHeightL(fs, num, FzBody), FzBody, th.Text)
		DrawTextL(c, fs, "%", rx-pw, gy+24-TextHeightL(fs, "%", FzTiny), FzTiny, th.Sub)

		c.Bar(gx, gy+36, cw, 8, 4, clamp(v, 0, 100)/100,
			th.Track, m3Bar(hueTeal, v, 90, th.Danger))
	}
}

// ------------------------------------------------------------
//
//	D · 分段条
//
//	四核合成一根 292px 宽的分段条，段间 3px 让底色透出来分界。
//	高度只占一行，省下的 76px 可以还给网络曲线。
//
// ------------------------------------------------------------
func prvD(c *Canvas, d *DrawCtx, x, y int) {
	th, fs := d.Theme, d.Fonts
	const h = 96

	m3Card(c, d, x, y, CardW, h, hueTeal, "CPU 负载")
	m3Note(c, fs, x+CardW-m3Pad, y+m3TitleBase, th,
		fmt.Sprintf("%.2f / %.2f / %.2f", prvData.Load[0], prvData.Load[1], prvData.Load[2]))

	const gap = 3
	segW := (CardW - 2*m3Pad - 3*gap) / 4
	for i := 0; i < 4 && i < len(prvData.Cores); i++ {
		sx := x + m3Pad + i*(segW+gap)
		v := prvData.Cores[i]
		// 四段长度接近时（本例 12/9/14/13），光靠长度分不出谁是谁；
		// 按段序加一级明度梯度，段与段的归属关系一眼可辨。
		col := Lit(m3Bar(hueTeal, v, 90, th.Danger), float64(i)*0.10)
		c.Bar(sx, y+52, segW, 10, 5, clamp(v, 0, 100)/100, th.Track, col)

		lab := fmt.Sprintf("C%d %.0f", i, v)
		w := TextWidthL(fs, lab, FzTiny)
		DrawTextL(c, fs, lab, sx+segW/2-w/2, y+84-TextHeightL(fs, lab, FzTiny), FzTiny, th.Dim)
	}
}
