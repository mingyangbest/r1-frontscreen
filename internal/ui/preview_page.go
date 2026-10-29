package ui

// ============================================================
//  整页预览（开发工具，不参与正式运行）
//
//  和 preview_demo.go 同一个理由：改版面的反馈环不能是
//  「编译一次、抓屏一次」。更麻烦的是这台机器的画面走 KMS/DRM，
//  /dev/fb0 里只有 TTY 残留内容，抓屏这条路本身就不通。
//
//  所以这里**直接调正式页面的 Draw**（EnvPage / DetailPage），
//  预览出来的每个像素都来自真机会跑的那条渲染路径。
//  数据也必须是真数据：monitor 在设备上采样，不在预览里编造 ——
//  编造出来的曲线形状会骗自己，而这块屏的第一原则是数据可追溯。
// ============================================================

import (
	"time"

	"r1-toolbox/internal/hardware"
	"r1-toolbox/internal/monitor"
)

// RenderRealPage 用真实采样数据渲染整页（376 × 960，与真机同尺寸）。
//
// page：0 = 环境页，1 = 详情页。系统页需要 *screen.Control 驱动背光滑条
// 与关屏按钮，离屏环境下没有对应的真实状态，不做预览。
//
// now 由调用方给定：首页大时钟与「夜间窗口」判断都依赖它，
// 传固定值才能得到可比较的两张图。
func RenderRealPage(fs *FontSet, mon *monitor.SystemMonitor, page int, now time.Time) *Canvas {
	c := NewCanvas(376, 960)
	d := NewDrawCtx(fs)
	d.Now = now
	d.T = 0
	// 详情页的风扇行直接调 d.Fan.GetCPUMode()（正式运行时 main 保证非 nil）。
	// 这里补一个：构造只做 hwmon 路径发现，不写硬件。
	d.Fan = hardware.NewFanController()

	// 与 renderer 的页面背景同款渐变，否则卡片边缘的对比度会失真
	th := d.Theme
	c.FillRectGrad(0, 0, 376, 960, th.BgTop, th.BgBot)

	switch page {
	case 0:
		NewEnvPage(mon).Draw(c, d)
	case 1:
		NewDetailPage(mon).Draw(c, d)
	}
	return c
}
