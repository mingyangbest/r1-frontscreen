package screen

// 背光状态机测试。用临时文件模拟 sysfs brightness 节点，
// 不需要真实硬件就能验证「关屏/唤醒/夜间跨界」的组合行为。
//
// 这里固化的是 2026-10-28 修复的状态分裂事故：
//   - Web 息屏后夜间看门狗把屏重新点亮（旧实现）
//   - 夜里开屏硬编码 36000 刺眼（旧实现）

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"r1-toolbox/internal/hardware"
)

// newTestControl 搭一个带临时背光文件的控制器。
// isNight 由测试显式给定（不依赖真实时钟）。
func newTestControl(t *testing.T, night int, isNight func() bool) (*Control, func() int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "brightness")
	if err := os.WriteFile(path, []byte("36000"), 0644); err != nil {
		t.Fatal(err)
	}
	br := hardware.NewBrightnessController(path, 0, 36000)
	c := New(br, 36000, night, isNight, nil, nil)
	return c, func() int {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读背光文件: %v", err)
		}
		v, err := strconv.Atoi(string(b))
		if err != nil {
			t.Fatalf("背光文件内容非法: %q", b)
		}
		return v
	}
}

// TestOffBlocksNightSync 核心回归：关屏后昼夜跨界不得点亮屏幕。
func TestOffBlocksNightSync(t *testing.T) {
	day := false
	c, read := newTestControl(t, 12000, func() bool { return day })

	c.Off("测试")
	if v := read(); v != 0 {
		t.Fatalf("关屏后背光应为 0，got %d", v)
	}
	day = true // 跨入夜间
	c.SyncNight()
	if v := read(); v != 0 {
		t.Fatalf("关屏期间 SyncNight 把屏点亮了（背光 %d）—— 夜间看门狗复活事故复现", v)
	}
	if !c.IsOff() {
		t.Fatal("关屏状态丢失")
	}
}

// TestWakeUsesNightValue 夜里唤醒要用夜间亮度，不是硬编码 36000。
func TestWakeUsesNightValue(t *testing.T) {
	c, read := newTestControl(t, 12000, func() bool { return true })

	c.Off("测试")
	c.Wake()
	if v := read(); v != 12000 {
		t.Fatalf("夜间唤醒背光应为 12000，got %d", v)
	}
}

// TestWakeUsesDayValue 白天唤醒用日间亮度。
func TestWakeUsesDayValue(t *testing.T) {
	c, read := newTestControl(t, 12000, func() bool { return false })

	c.Off("测试")
	c.Wake()
	if v := read(); v != 36000 {
		t.Fatalf("日间唤醒背光应为 36000，got %d", v)
	}
}

// TestSyncNightOnlyOnTransition 跨界只生效一次，不周期拉回。
func TestSyncNightOnlyOnTransition(t *testing.T) {
	day := false
	c, read := newTestControl(t, 12000, func() bool { return day })

	c.SyncNight() // 无跨界（还是白天），什么都不该发生
	if v := read(); v != 36000 {
		t.Fatalf("未跨界不应改背光，got %d", v)
	}
	day = true
	c.SyncNight() // 跨入夜间 → 降亮度
	if v := read(); v != 12000 {
		t.Fatalf("跨入夜间应降至 12000，got %d", v)
	}
	c.SyncNight() // 已经是夜间，再调不应有任何写入
	if v := read(); v != 12000 {
		t.Fatalf("重复 SyncNight 不应改背光，got %d", v)
	}
}

// TestApplyZeroTurnsOff Apply(0) 等同关屏（Web 面板亮度滑到 0 的路径）。
func TestApplyZeroTurnsOff(t *testing.T) {
	c, read := newTestControl(t, 12000, func() bool { return false })

	if err := c.Apply(0); err != nil {
		t.Fatalf("Apply(0): %v", err)
	}
	if !c.IsOff() || read() != 0 {
		t.Fatal("Apply(0) 应关屏")
	}
	c.Wake()
	if v := read(); v != 36000 {
		t.Fatalf("唤醒后应恢复日间亮度，got %d", v)
	}
}

// TestApplyNightUpdatesNightBucket 夜间 Apply 更新的是夜间基准，
// 第二天夜里仍然是这个值，不被日间值覆盖。
func TestApplyNightUpdatesNightBucket(t *testing.T) {
	isNight := false
	c, read := newTestControl(t, 12000, func() bool { return isNight })

	// 白天调 15000 → 属于日间桶
	if err := c.Apply(15000); err != nil {
		t.Fatal(err)
	}
	if v := read(); v != 15000 {
		t.Fatalf("白天 Apply(15000) 应立即生效，got %d", v)
	}
	// 入夜 → 切到夜间基准 12000
	isNight = true
	c.SyncNight()
	if v := read(); v != 12000 {
		t.Fatalf("入夜应用夜间基准 12000（白天调的 15000 属于日间桶），got %d", v)
	}
	// 夜间再调 → 更新夜间桶
	if err := c.Apply(9000); err != nil {
		t.Fatal(err)
	}
	if v := read(); v != 9000 {
		t.Fatalf("夜间 Apply(9000) 应生效，got %d", v)
	}
}
