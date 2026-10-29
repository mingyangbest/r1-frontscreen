// Command preview 把 UI 设计稿或真实页面离屏渲染成 PNG。
//
// 用法（在设备/带 SDL_ttf 的机器上）：
//
//	go run ./cmd/preview                       # 四方案对比图
//	go run ./cmd/preview -page 0 -out /tmp/env.png      # 环境页（真实数据）
//	go run ./cmd/preview -page 1 -out /tmp/detail.png   # 详情页（真实数据）
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
	"time"

	"github.com/veandco/go-sdl2/ttf"

	"r1-toolbox/internal/monitor"
	"r1-toolbox/internal/ui"
)

func main() {
	out := flag.String("out", "/tmp/cpu-variants.png", "输出 PNG 路径")
	page := flag.Int("page", -1, "渲染真实页面：0=环境 1=详情；-1=四方案对比图")
	warm := flag.Int("warm", 30, "真实页面模式的采样预热次数（决定 CPU 曲线长度）")
	flag.Parse()

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

	var c *ui.Canvas
	if *page < 0 {
		c = ui.RenderCPUVariantsDemo(fs)
	} else {
		if *page > 1 {
			fmt.Fprintln(os.Stderr, "暂不支持预览该页（系统页依赖真实背光状态）")
			os.Exit(2)
		}
		mon := monitor.NewSystemMonitor()
		for i := 0; i < *warm; i++ {
			_ = mon.UpdateCPU()
			_ = mon.UpdateMemory()
			_ = mon.UpdateNetwork()
			if i < *warm-1 {
				time.Sleep(120 * time.Millisecond)
			}
		}
		_ = mon.UpdateDisk()
		c = ui.RenderRealPage(fs, mon, *page, time.Now())
	}

	img := &image.RGBA{Pix: c.Pix, Stride: c.W * 4, Rect: image.Rect(0, 0, c.W, c.H)}
	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建文件失败：", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, "编码 PNG 失败：", err)
		os.Exit(1)
	}
	fmt.Printf("✓ %s  %dx%d  cjk=%s\n", *out, c.W, c.H, fs.Path())
}
