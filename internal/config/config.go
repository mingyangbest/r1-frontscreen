package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	ScreenWidth  = 376
	ScreenHeight = 960
	FrameRate    = 25
)

// ConfigPath 配置文件位置。优先飞牛 NAS 的 TRIM_PKGETC，其次固定路径。
func ConfigPath() string {
	if pkgEtc := os.Getenv("TRIM_PKGETC"); pkgEtc != "" {
		return filepath.Join(pkgEtc, "config.yaml")
	}
	return "/etc/r1-toolbox/config.yaml"
}

type Config struct {
	Screen   ScreenConfig   `yaml:"screen"`
	RGB      RGBConfig      `yaml:"rgb"`
	Fan      FanConfig      `yaml:"fan"`
	Web      WebConfig      `yaml:"web"`
	Hardware HardwareConfig `yaml:"hardware"`
	// Debug 打开明细日志（性能/采样审计）。默认关 —— 见 internal/logx。
	Debug bool `yaml:"debug"`
}

type ScreenConfig struct {
	Width      int `yaml:"width"`
	Height     int `yaml:"height"`
	Brightness int `yaml:"brightness"`
	// NightBrightness 夜间（18:00–06:00，与天气图标同一窗口）自动调暗后的
	// 背光值，0 = 关闭夜间调暗。只在昼夜切换瞬间生效，不覆盖手动调节。
	NightBrightness int    `yaml:"night_brightness"`
	Timeout         int    `yaml:"timeout"`
	Background      string `yaml:"background"`
	TextColor       string `yaml:"text_color"`
}

type RGBConfig struct {
	Mode   string `yaml:"mode"`
	Color  string `yaml:"color"`
	I2CBus string `yaml:"i2c_bus"`
}

type FanConfig struct {
	CPU FanCurve `yaml:"cpu"`
	HDD FanCurve `yaml:"hdd"`
}

type FanCurve struct {
	Mode       string `yaml:"mode"`
	PWMIndex   int    `yaml:"pwm_index"`
	TempSensor string `yaml:"temp_sensor"`
	MinTemp    int    `yaml:"min_temp"`
	MaxTemp    int    `yaml:"max_temp"`
	MinSpeed   int    `yaml:"min_speed"`
	MaxSpeed   int    `yaml:"max_speed"`
}

type WebConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
	// Token 面板访问令牌。为空时 main 启动会自动生成并回写到这里；
	// 所有 /api/* 接口要求请求携带 X-Token 头。
	// 令牌只在启动日志打印一次并显示在屏幕「系统」页 —— 物理接触设备即授权。
	Token string `yaml:"token"`
}

type HardwareConfig struct {
	BrightnessPath string `yaml:"brightness_path"`
	HwmonPath      string `yaml:"hwmon_path"`
	Framebuffer    string `yaml:"framebuffer"`
}

// Load 从默认路径读取配置。文件不存在返回默认值，格式损坏返回 error
// （调用方必须把 error 打出来 —— 静默回落默认值会让用户的亮度、
// 夜间亮度、风扇曲线在毫无提示的情况下全部丢失）。
func Load() (*Config, error) { return LoadFrom(ConfigPath()) }

// LoadFrom 从指定路径读取配置
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultConfig(), nil
		}
		return DefaultConfig(), fmt.Errorf("读取配置 %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("解析配置 %s: %w", path, err)
	}
	return &cfg, nil
}

// ============================================================
//  Store —— 配置的唯一可变持有者
//
// 修复的竞态（-race 实锤，2026-10-28）：一个 HTTP handler 往
// cfg.Screen.Brightness 写值，另一个 handler 的 Save() 正把结构体
// 序列化出去；同时主循环的热重载也在写。三方无锁。
//
// 现在所有读写都必须经过 Store：
//   - Get() 返回**值拷贝**，调用方随便用，不影响内部状态
//   - Update(fn) 在写锁里改，改完立刻以快照落盘
//   - 落盘串行化（saveMu），并发 Update 不会互相覆盖
// ============================================================

type Store struct {
	mu    sync.RWMutex
	path  string
	cur   Config
	saveM sync.Mutex
}

// NewStore 读取配置并返回线程安全的持有者。读失败不返回 error ——
// 设备上配置文件损坏时屏幕必须还能亮，用默认值启动并把错误带走。
func NewStore(path string) (*Store, error) {
	cfg, err := LoadFrom(path)
	s := &Store{path: path}
	if cfg != nil {
		s.cur = *cfg
	} else {
		s.cur = *DefaultConfig()
	}
	return s, err
}

// Get 返回当前配置的值拷贝（Config 全是值类型与 string，浅拷贝即安全）
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Update 在写锁内修改配置，随后把快照落盘。
// mutate 里不要做 IO、不要调用 Store 的任何方法（会自锁）。
func (s *Store) Update(mutate func(*Config)) error {
	s.mu.Lock()
	if mutate != nil {
		mutate(&s.cur)
	}
	snap := s.cur
	s.mu.Unlock()

	s.saveM.Lock()
	defer s.saveM.Unlock()
	return SaveTo(s.path, &snap)
}

// Save 把当前配置落盘
func (s *Store) Save() error {
	s.mu.RLock()
	snap := s.cur
	s.mu.RUnlock()

	s.saveM.Lock()
	defer s.saveM.Unlock()
	return SaveTo(s.path, &snap)
}

// Replace 用外部读到的新配置整体替换（热重载用）。
//
// 只替换「主循环关心且允许热改」的字段，不碰 Web 令牌等运行期状态，
// 否则热重载会把刚生成的令牌冲掉。
func (s *Store) Replace(fresh Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur.Screen.Brightness = fresh.Screen.Brightness
	s.cur.Screen.NightBrightness = fresh.Screen.NightBrightness
	s.cur.Screen.Timeout = fresh.Screen.Timeout
	s.cur.Debug = fresh.Debug
}

// Path 配置文件路径
func (s *Store) Path() string { return s.path }

// ============================================================
//  落盘
// ============================================================

// Save 把当前配置写回默认路径（保留旧文件注释与键序）
func (c *Config) Save() error { return SaveTo(ConfigPath(), c) }

// SaveTo 把配置写回指定路径。
//
// 旧实现是 os.WriteFile 直接截断覆盖，两个后果：
//  1. 写到一半被杀/断电 → 文件只剩半截 YAML，下次启动解析失败；
//  2. 并发保存互相覆盖（已由 Store.saveM 收口）。
//
// 现在写临时文件 + fsync + rename + 目录 fsync：rename 在同一文件系统内
// 是原子的，任何时刻磁盘上要么是旧文件、要么是完整的新文件。
//
// 注释合并逻辑保留（2026-09-27 的 G2 修复）：把新值合并进旧文件的
// yaml.Node 语法树，键序、行注释、引号风格原样保留。
func SaveTo(path string, c *Config) error {
	fresh, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	old, err := os.ReadFile(path)
	if err != nil || len(old) == 0 {
		return WriteFileAtomic(path, fresh)
	}

	var dst yaml.Node
	if err := yaml.Unmarshal(old, &dst); err != nil ||
		dst.Kind != yaml.DocumentNode || len(dst.Content) == 0 ||
		dst.Content[0].Kind != yaml.MappingNode {
		return WriteFileAtomic(path, fresh)
	}

	var src yaml.Node
	if err := yaml.Unmarshal(fresh, &src); err != nil ||
		len(src.Content) == 0 || src.Content[0].Kind != yaml.MappingNode {
		return WriteFileAtomic(path, fresh)
	}

	mergeMappingNode(src.Content[0], dst.Content[0])

	out, err := yaml.Marshal(&dst)
	if err != nil {
		return WriteFileAtomic(path, fresh)
	}
	return WriteFileAtomic(path, out)
}

// WriteFileAtomic 原子写：同目录临时文件 → fsync → rename → fsync 目录。
//
// rename 之后即使掉电，文件系统要么指向旧 inode、要么指向完整的新 inode，
// 不会出现「半截 YAML」。目录 fsync 保证 rename 本身已落盘。
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() {
		f.Close()
		os.Remove(tmp)
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Chmod(0644); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync() // 目录项落盘；失败不影响本次写入（有的文件系统不支持）
		d.Close()
	}
	return nil
}

// mergeMappingNode 把 src 映射里的值合并进 dst 映射（保留 dst 的键序、
// 注释与标量样式）。
//   - dst 已有的键：值用 src 覆盖；标量保留 dst 的 Style（用户写的
//     引号风格不丢），非标量（嵌套映射/序列）整体替换；
//   - dst 缺失的键：连同键值对整体追加（新字段的注释随 src 带入）；
//   - 双方对应位置都是映射时递归合并（fan.cpu/hdd 等小节）。
func mergeMappingNode(src, dst *yaml.Node) {
	dstVal := map[string]*yaml.Node{}
	for i := 0; i+1 < len(dst.Content); i += 2 {
		dstVal[dst.Content[i].Value] = dst.Content[i+1]
	}
	for i := 0; i+1 < len(src.Content); i += 2 {
		sk, sv := src.Content[i], src.Content[i+1]
		dv, ok := dstVal[sk.Value]
		if !ok {
			nk, nv := *sk, *sv
			dst.Content = append(dst.Content, &nk, &nv)
			continue
		}
		if sv.Kind == yaml.MappingNode && dv.Kind == yaml.MappingNode {
			mergeMappingNode(sv, dv)
			continue
		}
		if sv.Kind == yaml.ScalarNode && dv.Kind == yaml.ScalarNode {
			dv.Value = sv.Value
			dv.Tag = sv.Tag
			// dv.Style 保留：不强改为 src 的风格
			continue
		}
		*dv = *sv
	}
}

func DefaultConfig() *Config {
	return &Config{
		Screen: ScreenConfig{Width: 376, Height: 960, Brightness: 36000, Timeout: 300, Background: "", TextColor: "#ffffff"},
		RGB:    RGBConfig{Mode: "solid", Color: "green", I2CBus: "0"},
		Fan: FanConfig{
			CPU: FanCurve{Mode: "auto", PWMIndex: 3, TempSensor: "it8628:temp1", MinTemp: 35, MaxTemp: 75, MinSpeed: 50, MaxSpeed: 255},
			HDD: FanCurve{Mode: "auto", PWMIndex: 2, TempSensor: "it8628:temp2", MinTemp: 40, MaxTemp: 60, MinSpeed: 50, MaxSpeed: 255},
		},
		Web: WebConfig{Enabled: true, Port: 8080},
		Hardware: HardwareConfig{
			BrightnessPath: "/sys/devices/pci0000:00/0000:00:02.0/drm/card0/card0-DSI-1/intel_backlight/brightness",
			HwmonPath:      "/sys/class/hwmon/hwmon3",
			Framebuffer:    "/dev/fb0",
		},
	}
}
