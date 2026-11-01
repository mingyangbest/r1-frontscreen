package ui

import (
	"embed"
	"os"
	"path/filepath"
	"sync"
)

// 拉丁字体：IBM Plex Sans（OFL 1.1，见同目录 OFL.txt）。
//
// 为什么换掉 Cantarell（2026-11-01 实测）：
//   - Cantarell 的数字是**比例宽度**：100px 下 "1"=42px、"8"=58px，
//     "15:31"=218px 而 "08:05"=259px，差 41px。时钟每小时换数字、百分比
//     从 88% 变 100% 时，宽度都在跳 —— 仪表盘最忌讳这个。
//   - 试过 Inter：更糟（"1"=41 vs "8"=62，差 44px）。
//   - IBM Plex Sans 是唯一实测**完全等宽**的（十个数码全是 60px/100px），
//     而且它本来就是给仪器面板做设计语汇的，字重也齐。
//
// 嵌入二进制而不是依赖系统安装：Debian 12 的源里没有 IBM Plex，开源用户
// 把二进制拷过去就该能跑，不能让人先去装字体。三个字重共约 600KB。
//
//go:embed assets/fonts/IBMPlexSans-Thin.ttf assets/fonts/IBMPlexSans-Light.ttf assets/fonts/IBMPlexSans-Bold.ttf
var embeddedFonts embed.FS

// embeddedFontNames 按上线顺序列出要落盘的字体文件名
var embeddedFontNames = []string{
	"IBMPlexSans-Thin.ttf",
	"IBMPlexSans-Light.ttf",
	"IBMPlexSans-Bold.ttf",
}

var (
	fontDirOnce sync.Once
	fontDirPath string
)

// ensureFontDir 把嵌入的字体解到临时目录，返回目录路径。
//
// 为什么落盘而不是走 SDL_ttf 的内存加载：go-sdl2 的 ttf 包没有稳定的
// RWops 打开接口，落盘最省事且进程内只做一次（480KB，微秒级）。
// 文件已存在就跳过 —— 重启服务不会重复写盘。
func ensureFontDir() string {
	fontDirOnce.Do(func() {
		dir := filepath.Join(os.TempDir(), "r1-toolbox-fonts")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return // 落盘失败就返回空，NewFontSet 会退回系统字体
		}
		for _, name := range embeddedFontNames {
			dst := filepath.Join(dir, name)
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			b, err := embeddedFonts.ReadFile("assets/fonts/" + name)
			if err != nil {
				continue
			}
			// 先写临时文件再 rename：并发启动时不会读到半截文件
			tmp := dst + ".tmp"
			if err := os.WriteFile(tmp, b, 0o644); err != nil {
				continue
			}
			_ = os.Rename(tmp, dst)
		}
		fontDirPath = dir
	})
	return fontDirPath
}

// embeddedFontPath 拼一个嵌入字体的落地路径；目录不可用时返回空串
func embeddedFontPath(name string) string {
	dir := ensureFontDir()
	if dir == "" {
		return ""
	}
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}
