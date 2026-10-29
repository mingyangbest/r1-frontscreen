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

	"r1-toolbox/internal/config"
	"r1-toolbox/internal/hardware"
	"r1-toolbox/internal/monitor"
	"r1-toolbox/internal/netinfo"
	"r1-toolbox/internal/screen"
)

// 背光节点走 /sys/class 软链：真机配置里写死的是 drm/card1/...，
// 而 card 编号会随驱动探头顺序变化（card0 / card1 都见过），软链最稳。
// 只在系统页预览时用来读真实亮度（只读，不写硬件）。
const previewBrightPath = "/sys/class/backlight/intel_backlight/brightness"

// RenderRealPage 用真实采样数据渲染整页（376 × 960，与真机同尺寸）。
//
// page：0 = 环境页，1 = 详情页，2 = 系统页。
//
// 系统页要读真实背光（滑条位置）、真实令牌（底部 pill）和公网 IP
// （net 可为 nil，则显示「获取中」），这些只能在设备上取；全程只读。
//
// now 由调用方给定：首页大时钟与「夜间窗口」判断都依赖它，
// 传固定值才能得到可比较的几张图。
func RenderRealPage(fs *FontSet, mon *monitor.SystemMonitor, page int, now time.Time,
	net *netinfo.Service) *Canvas {
	c := NewCanvas(376, 960)
	d := NewDrawCtx(fs)
	d.Now = now
	d.T = 0
	// 详情页的风扇行直接调 d.Fan.GetCPUMode()（正式运行时 main 保证非 nil）。
	// 这里补一个：构造只做 hwmon 路径发现，不写硬件。
	d.Fan = hardware.NewFanController()
	d.Net = net
	// 令牌：真机读的是同一份配置。截图里写「未设置」而真机有令牌，
	// 这张图就不算真机实况了。
	if cfg, err := config.Load(); err == nil {
		d.WebToken = cfg.Web.Token
	}

	// 与 renderer 的页面背景同款渐变，否则卡片边缘的对比度会失真
	th := d.Theme
	c.FillRectGrad(0, 0, 376, 960, th.BgTop, th.BgBot)

	var p Page
	switch page {
	case 0:
		p = NewEnvPage(mon)
	case 1:
		p = NewDetailPage(mon)
	case 2:
		br := hardware.NewBrightnessController(previewBrightPath, 0, 96000)
		// day/night 传 0：系统页只显示「当前读数」，不走 Target() 那条时段逻辑
		scr := screen.New(br, 0, 0, nil, nil, nil)
		p = NewSysPage(mon, scr)
	default:
		return c
	}
	p.Draw(c, d)

	// 页头（页名 + 右侧手势提示）由 renderer 在页面之外画，非全出血页才有。
	// 少了它，系统页截图顶部会空出一整块，和真机对不上。
	if !pageFullTop(p) {
		if n, ok := p.(Named); ok {
			DrawTopBarM3(c, fs, th, n.PageName(), hintOf(n.PageName()))
		}
	}

	// 页签圆点由 renderer 在页面之外单独画（y=918）。少了它，预览图
	// 和真机全屏截图差一截，拿去做 README 配图会"看着不像"。
	DrawDots(c, pageAccents, page, 918)
	return c
}
