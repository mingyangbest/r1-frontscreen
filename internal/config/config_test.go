package config

// 配置层的测试重点不是「能存能读」，而是两个真实事故场景：
//  1. 进程写到一半被杀/断电 → 磁盘上不能出现半截 YAML（原子性）
//  2. 多个 handler 并发保存 → 后写的不能把先写的覆盖成更旧的混合体
//
// 这些测试配合 go test -race 跑。

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSaveToRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := DefaultConfig()
	cfg.Screen.Brightness = 12345
	cfg.Screen.NightBrightness = 6789
	cfg.Web.Token = "ABCD2345"
	cfg.Fan.HDD.MinTemp = 42

	if err := SaveTo(path, cfg); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got.Screen.Brightness != 12345 || got.Screen.NightBrightness != 6789 ||
		got.Web.Token != "ABCD2345" || got.Fan.HDD.MinTemp != 42 {
		t.Fatalf("往返不一致: %+v", got)
	}
}

// TestSaveToNoTempLeft 原子写完成后目录里不允许有临时文件残留。
// 残留的 .tmp 文件会在 /etc 里越积越多。
func TestSaveToNoTempLeft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	for i := 0; i < 5; i++ {
		if err := SaveTo(path, DefaultConfig()); err != nil {
			t.Fatalf("SaveTo #%d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config.yaml.tmp") {
			t.Errorf("临时文件残留: %s", e.Name())
		}
	}
}

// TestConcurrentUpdate 并发 50 个 Update，每次写不同的亮度。
// 结束后文件必须能被完整解析，且亮度是 50 个值之一 ——
// 旧实现（非原子写）在这里会随机解析失败。
func TestConcurrentUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := SaveTo(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			_ = store.Update(func(c *Config) {
				c.Screen.Brightness = 10000 + v
			})
		}(i)
	}
	wg.Wait()

	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("并发写入后文件损坏: %v", err)
	}
	b := got.Screen.Brightness
	if b < 10000 || b >= 10000+n {
		t.Fatalf("亮度 %d 不在写入的值域内", b)
	}
	// Store 内存态必须与最后一次 Update 一致（串行化保证不丢更新）
	if store.Get().Screen.Brightness != b {
		t.Fatalf("内存态 %d != 落盘 %d，存在丢更新",
			store.Get().Screen.Brightness, b)
	}
}

// TestSaveToPreservesComments 用户手写的注释与键序不能被保存动作抹掉。
func TestSaveToPreservesComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := "# 我的手写注释\nscreen:\n  brightness: 36000 # 行内注释\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Screen.Brightness = 28000
	if err := SaveTo(path, cfg); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "我的手写注释") || !strings.Contains(s, "行内注释") {
		t.Fatalf("注释丢失:\n%s", s)
	}
	if !strings.Contains(s, "28000") {
		t.Fatalf("新值未写入:\n%s", s)
	}
}

func TestLoadFromMissing(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("缺文件应回落默认值且无错误, got %v", err)
	}
	if cfg.Screen.Brightness != 36000 {
		t.Fatalf("默认亮度错误: %d", cfg.Screen.Brightness)
	}
}

func TestLoadFromBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	// 半截 YAML（模拟断电）必须显式报错，不许静默回落
	if err := os.WriteFile(path, []byte("screen:\n  brightness: [1,2"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(path); err == nil {
		t.Fatal("损坏的配置必须返回错误")
	}
}
