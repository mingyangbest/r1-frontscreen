// Command preview 把 UI 设计稿离屏渲染成 PNG，用于方案对比。
//
// 用法（在设备/带 SDL_ttf 的机器上）：
//
//	go run ./cmd/preview -out /tmp/cpu-variants.png
//
// 只依赖 SDL_ttf 的字体加载（与正式程序同一套 FontSet），
// 不需要窗口、不需要 framebuffer —— 因此可以在 NAS 上跑，
// 不打断正在显示的界面。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/veandco/go-sdl2/ttf"

	"r1-toolbox/internal/ui"
)

func main() {
	out := flag.String("out", "/tmp/cpu-variants.png", "输出 PNG 路径")
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

	c := ui.RenderCPUVariantsDemo(fs)

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
