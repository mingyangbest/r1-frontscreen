package ui

import "testing"

// 2026-11-22 洋哥定的三色规则，用测试把阈值和边界钉死。
// 低=湖蓝 colLake / 中=沙金 colSand / 高=珊瑚 colCoral。
// 边界一律"先四舍五入后判档"，和屏上 %.0f 的显示值同源。

func TestLoadColorThresholds(t *testing.T) {
	cases := []struct {
		v    float64
		want RGBA
		name string
	}{
		{0, colLake, "0%"},
		{49.4, colLake, "49.4% 显示 49"},
		{49.6, colSand, "49.6% 显示 50"},
		{50, colSand, "50% 下边界"},
		{79.4, colSand, "79.4% 显示 79"},
		{79.6, colCoral, "79.6% 显示 80"},
		{80, colCoral, "80% 下边界"},
		{100, colCoral, "100%"},
	}
	for _, c := range cases {
		if got := loadColor(c.v); got != c.want {
			t.Errorf("loadColor(%v) [%s] = %v, want %v", c.v, c.name, got, c.want)
		}
	}
}

func TestTempColorThresholds(t *testing.T) {
	cases := []struct {
		v    float64
		want RGBA
		name string
	}{
		{0, colDim, "读不到，画—不染湖蓝"},
		{69.4, colLake, "69.4° 显示 69"},
		{69.6, colSand, "69.6° 显示 70"},
		{70, colSand, "70° 下边界"},
		{89.4, colSand, "89.4° 显示 89"},
		{89.6, colCoral, "89.6° 显示 90"},
		{90, colCoral, "90° 下边界"},
		{120, colCoral, "120°"},
	}
	for _, c := range cases {
		if got := tempColor(c.v); got != c.want {
			t.Errorf("tempColor(%v) [%s] = %v, want %v", c.v, c.name, got, c.want)
		}
	}
}

// 两套阈值必须独立：60 是正常温度(湖蓝)，却是留意负载(沙金)。
func TestLoadAndTempThresholdsAreIndependent(t *testing.T) {
	if got := loadColor(60); got != colSand {
		t.Errorf("loadColor(60) = %v, want colSand", got)
	}
	if got := tempColor(60); got != colLake {
		t.Errorf("tempColor(60) = %v, want colLake", got)
	}
}

// 条与数值同源
func TestBarColorMatchesLoadColor(t *testing.T) {
	for _, v := range []float64{0, 49.6, 50, 79.6, 80, 100} {
		if got, want := barColor(v), loadColor(v); got != want {
			t.Errorf("barColor(%v)=%v != loadColor(%v)=%v", v, got, v, want)
		}
	}
}
