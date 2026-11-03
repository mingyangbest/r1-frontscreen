package hardware

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DiscoverBrightnessPath 在 /sys/class/backlight 下找第一个可用的 brightness 节点。
//
// /sys/class/backlight/* 是稳定软链，不随 PCI 路径变化 —— 配置里存的是
// 完整绝对路径（.../card1-DSI-1/intel_backlight/brightness），换台机器
// 可能失效，而失效是静默的：写不进去，用户只看到亮度调不动。
func DiscoverBrightnessPath() string {
	entries, err := os.ReadDir("/sys/class/backlight")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		p := filepath.Join("/sys/class/backlight", e.Name(), "brightness")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ReadMaxBrightness 读硬件背光上限（同目录的 max_brightness），
// 读不到时返回 fallback。R1 上实测为 96000。
func ReadMaxBrightness(brightnessPath string, fallback int) int {
	if brightnessPath == "" {
		return fallback
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(brightnessPath), "max_brightness"))
	if err != nil {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
