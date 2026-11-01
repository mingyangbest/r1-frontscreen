package ui

// ============================================================
//  自定义背景图（**功能暂缓，当前没有任何调用点**）
//
//  2026-10-28 安全加固时删掉了 /api/background（它能往 /etc 写文件），
//  前端那两个按钮（背景图片 / 文字颜色）随之失效；2026-11-01 重做面板时
//  已把它们从界面撤掉。同日洋哥决定「背景图功能暂时不上」。
//
//  所以这里保留的只是解码/编码工具，**不是坏掉的功能**：
//  真要上这个功能时，需要一起解决的三件事（别只做上传接口）：
//    1. 落盘位置换成 /var/lib/r1-toolbox/，别碰 /etc
//    2. 卡片是 15/255 的白色半透明，压在照片上会糊成一层雾 ——
//       要么提不透明度，要么加暗化层，否则数字读不清
//    3. Screen.TextColor 现在能存但没用；一旦启用，它和卡片标题的
//       语义色（sage/rose/mint/sand）会打架，得先定清楚作用范围
// ============================================================

import (
	"github.com/veandco/go-sdl2/sdl"
	"image"
	"image/jpeg"
	"os"
)

func LoadBackground(path string, width, height int32) (*sdl.Surface, error) {
	if path == "" {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	return imageToSurface(img)
}

func imageToSurface(img image.Image) (*sdl.Surface, error) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	surface, err := sdl.CreateRGBSurface(0, int32(w), int32(h), 32, 0, 0, 0, 0)
	if err != nil {
		return nil, err
	}

	surface.Lock()
	defer surface.Unlock()

	pixels := surface.Pixels()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			idx := (y*w + x) * 4
			pixels[idx] = byte(b >> 8)
			pixels[idx+1] = byte(g >> 8)
			pixels[idx+2] = byte(r >> 8)
			pixels[idx+3] = byte(a >> 8)
		}
	}

	return surface, nil
}

func SaveImage(img image.Image, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return jpeg.Encode(file, img, &jpeg.Options{Quality: 90})
}
