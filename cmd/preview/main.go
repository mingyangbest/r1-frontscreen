// Command preview 把 UI 设计稿或真实页面离屏渲染成 PNG。
//
// 用法（在设备/带 SDL_ttf 的机器上）：
//
//	go run ./cmd/preview                          # 四方案对比图
//	go run ./cmd/preview -page 0 -out /tmp/env.png      # 环境页（真实数据）
//	go run ./cmd/preview -page 1 -out /tmp/detail.png   # 详情页（真实数据）
//	go run ./cmd/preview -page 2 -out /tmp/sys.png      # 系统页（真实数据）
//	go run ./cmd/preview -all  -out /tmp/shot.png       # 三页一次出，共用同一份快照
//
// 只依赖 SDL_ttf 的字体加载（与正式程序同一套 FontSet），
// 不需要窗口、不需要 framebuffer —— 因此可以在 NAS 上跑，
// 不打断正在显示的界面。
//
// -page 模式会先真采样一段时间：CPU 历史是 2 秒一个点、共 30 点，
// 不预热就只能画出一两个点。预热用 120ms 一拍快速累积形状，
// 数值仍是 /proc 的真实读数，只是时间轴更密。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"time"

	"github.com/veandco/go-sdl2/ttf"

	"r1-toolbox/internal/monitor"
	"r1-toolbox/internal/netinfo"
	"r1-toolbox/internal/ui"
)

func main() {
	out := flag.String("out", "/tmp/cpu-variants.png", "输出 PNG 路径")
	page := flag.Int("page", -1, "渲染真实页面：0=环境 1=详情 2=系统；-1=四方案对比图")
	all := flag.Bool("all", false, "一次采样渲染全部三页（共用同一份快照，曲线才对得上）")
	warm := flag.Int("warm", 30, "真实页面模式的采样预热次数（决定 CPU 曲线长度）")
	interval := flag.Int("interval", 120, "预热采样间隔（毫秒）。出 README 截图时用 2000，与真机 fastEvery 一致，曲线形状才对得上")
	ver := flag.String("version", "dev", "系统页设备卡显示的版本号，与真机保持同一口径（如 v0.7.0）")
	flag.Parse()

	// 主程序是在运行时把 version 赋给 ui.Version 的（cmd/main.go），
	// 预览不走那条路径，得自己赋一次 —— 否则设备卡右上角永远写 "dev"。
	ui.Version = *ver

	if err := ttf.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "SDL_ttf 初始化失败：", err)
		os.Exit(1)
	}
	defer ttf.Quit()

	fs := ui.NewFontSet()
	if fs.Path() == "" {
		fmt.Fprintln(os.Stderr, "找不到中文字体，渲染出来的中文会是空白")
		os.Exit(1)
	}
	defer fs.Close()

	// 三页一次出：必须共用同一份快照，否则曲线形状对不上，
	// 看版式时会误以为"同一台机器两个样"。
	if *all {
		mon := warmUp(*warm, *interval)
		now := time.Now()
		net := fetchPublicIP()
		for i, name := range []string{"-env", "-detail", "-system"} {
			writePNG(*out, name, ui.RenderRealPage(fs, mon, i, now, net))
		}
		return
	}

	var c *ui.Canvas
	if *page < 0 {
		c = ui.RenderCPUVariantsDemo(fs)
	} else {
		if *page > 2 {
			fmt.Fprintln(os.Stderr, "-page 只支持 0（环境）/ 1（详情）/ 2（系统）")
			os.Exit(2)
		}
		var net *netinfo.Service
		if *page == 2 {
			net = fetchPublicIP()
		}
		c = ui.RenderRealPage(fs, warmUp(*warm, *interval), *page, time.Now(), net)
	}
	writePNG(*out, "", c)
}

// fetchPublicIP 起一次真实查询并等待首个结果。
//
// 系统页的「公网」行有三态（获取中/明文/获取失败），如果不实际查一次，
// 截图会一直停在"获取中" —— 而真机上它是几秒内就出结果的。
// 查不到就让它显示「获取失败」，也不编一个假 IP。
func fetchPublicIP() *netinfo.Service {
	net := netinfo.New()
	net.Start()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if pi := net.PublicIP(); !pi.TriedAt.IsZero() {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	return net
}

// warmUp 先真采样若干轮：CPU 历史是 2 秒一点、共 30 点，
// 不预热就只能画出一两个点。数值全是 /proc 的真实读数。
func warmUp(n, intervalMs int) *monitor.SystemMonitor {
	mon := monitor.NewSystemMonitor()
	for i := 0; i < n; i++ {
		_ = mon.UpdateCPU()
		_ = mon.UpdateMemory()
		_ = mon.UpdateNetwork()
		if i < n-1 {
			time.Sleep(time.Duration(intervalMs) * time.Millisecond)
		}
	}
	_ = mon.UpdateDisk()
	// 容器数也要采：不采的话 ContainersRunning 是零值，首页 pill 会显示
	// 「容器 0/0 运行中」—— 和真机（29/30）不一致的截图等于假图。
	mon.UpdateContainers()
	return mon
}

// writePNG 落盘；suffix 非空时插在扩展名之前（"x.png" + "-env" → "x-env.png"）。
func writePNG(path, suffix string, c *ui.Canvas) {
	if suffix != "" {
		path = strings.TrimSuffix(path, ".png") + suffix + ".png"
	}
	img := &image.RGBA{Pix: c.Pix, Stride: c.W * 4, Rect: image.Rect(0, 0, c.W, c.H)}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建文件失败：", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, "编码 PNG 失败：", err)
		os.Exit(1)
	}
	fmt.Printf("✓ %s  %dx%d\n", path, c.W, c.H)
}
