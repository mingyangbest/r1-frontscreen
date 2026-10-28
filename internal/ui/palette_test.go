package ui

// 色板的可读性纪律，用测试固化。
//
// 背景：2026-10-28 测试评审发现 Dim 文字对 M3 卡片底色只有 2.63:1
// （WCAG AA 小字要求 4.5:1、18px 以上大字 3.0:1）。本次把 Dim 提到
// 3.5+（卡片标签多为 17~20px 小字，全部按 4.5 执行需要牺牲色板层次，
// 按「关键说明文字 ≥3.5」的内部口径执行）。本文件把纪律钉死：
// 以后谁把颜色调暗，CI 直接红。
//
// 对比度按 WCAG 2.x 相对亮度公式计算，对着文字实际所在的背景：
//   - M3 三页的文字全部画在五色 hue 卡容器上 → 对最亮的 amber 底校验
//   - 部分文字直接画在页面背景上 → 对 BgTop 校验

import (
	"math"
	"testing"
)

// relLum WCAG 相对亮度
func relLum(r, g, b uint8) float64 {
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(float64(r)) + 0.7152*lin(float64(g)) + 0.0722*lin(float64(b))
}

func contrast(a, b RGBA) float64 {
	la, lb := relLum(a.R, a.G, a.B), relLum(b.R, b.G, b.B)
	hi, lo := la, lb
	if lb > la {
		hi, lo = lb, la
	}
	return (hi + 0.05) / (lo + 0.05)
}

// m3AmberCard M3 五色卡容器里**最亮**的一个（琥珀底）——
// 对它达标，对其它四张只会更好。与 pages_m3.go 的 hueAmber.bg 保持一致；
// 如果改了那边忘了这边，TestM3PaletteInSync 会拦住。
var m3AmberCard = C(48, 42, 33)

func TestDimContrastOnCards(t *testing.T) {
	for _, name := range ThemeNames() {
		th := ThemeByName(name)
		r := contrast(th.Dim, m3AmberCard)
		if r < 3.5 {
			t.Errorf("主题 %s 的 Dim %v 对琥珀卡底 %.2f < 3.5（调暗前先想清楚标签还要不要人看）",
				name, th.Dim, r)
		}
	}
}

func TestSubAndTextContrast(t *testing.T) {
	for _, name := range ThemeNames() {
		th := ThemeByName(name)
		if r := contrast(th.Sub, m3AmberCard); r < 4.5 {
			t.Errorf("主题 %s 的 Sub 对琥珀卡底 %.2f < 4.5", name, r)
		}
		if r := contrast(th.Text, m3AmberCard); r < 7.0 {
			t.Errorf("主题 %s 的 Text 对琥珀卡底 %.2f < 7.0", name, r)
		}
	}
}

// TestDimContrastOnPageBackground Dim 极少直接落在页背景上（页背景上
// 只有 Text/Sub 层级的大字），阈值按 4.5 执行。
func TestDimContrastOnPageBackground(t *testing.T) {
	for _, name := range ThemeNames() {
		th := ThemeByName(name)
		if r := contrast(th.Dim, th.BgTop); r < 4.5 {
			t.Errorf("主题 %s 的 Dim 对页背景 %.2f < 4.5", name, r)
		}
	}
}

// TestWarnDangerStandOut 状态色必须一眼可辨（告警被淹没 = 没有告警）。
func TestWarnDangerStandOut(t *testing.T) {
	for _, name := range ThemeNames() {
		th := ThemeByName(name)
		if r := contrast(th.Warn, m3AmberCard); r < 3.0 {
			t.Errorf("主题 %s 的 Warn 对卡底 %.2f < 3.0", name, r)
		}
		if r := contrast(th.Danger, m3AmberCard); r < 3.0 {
			t.Errorf("主题 %s 的 Danger 对卡底 %.2f < 3.0", name, r)
		}
	}
}
