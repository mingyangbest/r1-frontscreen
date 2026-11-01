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
	Display  DisplayConfig  `yaml:"display"`
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
	NightBrightness int `yaml:"night_brightness"`
	Timeout         int `yaml:"timeout"`
	// Background / TextColor 是给"自定义背景图"预留的字段，**当前不生效**：
	// 面板已撤掉入口，渲染侧也没有读取它们（详见 internal/ui/background.go
	// 顶部说明）。留着是为了以后上这个功能时不必改配置文件格式。
	Background string `yaml:"background"`
	TextColor  string `yaml:"text_color"`
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

// DisplayConfig 屏幕内容本身的设置（与亮度/风扇那类硬件控制分开）。
//
// 全部是"看一眼就知道要不要求改"的开关，没有需要精密调的值 ——
// 这块屏的默认排版是精修过的，这里的旋钮只是让用户能按自己的习惯改，
// 不是让他重新设计。
type DisplayConfig struct {
	// Theme 启动时应用的主题：ink / mono / sand / jade
	Theme string `yaml:"theme" json:"theme"`
	// Clock24 false 用 12 小时制（3:04），true 用 24 小时制（15:04）
	Clock24 bool `yaml:"clock_24h" json:"clock_24h"`
	// Weather 首页右上角天气图标与描述是否显示
	Weather bool `yaml:"weather" json:"weather"`
	// SampleSec 屏幕数字的刷新间隔（秒），只允许 2 / 3 / 5
	SampleSec int `yaml:"sample_seconds" json:"sample_seconds"`
	// PageOrder 全部页面的显示顺序，env/detail/system 的某个排列。
	// 隐藏的页也留在这里 —— 否则重新打开时不知道该插回哪个位置。
	PageOrder []string `yaml:"page_order" json:"page_order"`
	// PagesOn 实际启用的页面（PageOrder 的子集，至少一页）。
	// 屏幕翻页只在启用页之间循环，页签也只画启用的。
	PagesOn []string `yaml:"pages" json:"pages"`
}

// HardwareConfig 硬件节点路径
type HardwareConfig struct {
	BrightnessPath string `yaml:"brightness_path"`
	HwmonPath      string `yaml:"hwmon_path"`
	Framebuffer    string `yaml:"framebuffer"`
}

// normalize 把缺省/越界的显示设置拉回可用值。
//
// 为什么需要它：LoadFrom 是 `var cfg Config` 零值起步再 Unmarshal 的，
// 老配置文件里根本没有 display 段 —— 那样 Clock24/Weather 会被解成 false，
// 用户升级后会发现「12 小时制 + 天气没了」，但配置文件一个字都没动过。
// 用 Theme 是否为空判断"这个段到底存不存在"：不存在就整套回默认；
// 存在就只纠正越界值（主题名写错 → ink，采样间隔填 1 或 7 → 2）。
func (c *Config) normalize() {
	if c.Display.Theme == "" {
		c.Display = DisplayConfig{
			Theme: "ink", Clock24: true, Weather: true, SampleSec: 2,
			PageOrder: []string{"env", "detail", "system"},
			PagesOn:   []string{"env", "detail", "system"},
		}
	}
	// 无条件再过一遍 Normalize：Theme 为空的分支刚填完默认值，
	// 走这里等于自查一遍；非空分支负责把越界值拉回来（含页面两表）
	c.Display.Normalize()
}

// canonicalPages 全部合法页名，顺序即默认顺序
var canonicalPages = []string{"env", "detail", "system"}

// Normalize 把越界的值拉回可用值（不改"整段缺失"的判断，那是 Config 的事）。
// 面板写配置时也走这里，保证落盘的永远是合法值。
func (d *DisplayConfig) Normalize() {
	switch d.Theme {
	case "ink", "mono", "sand", "jade":
	default:
		d.Theme = "ink"
	}
	switch d.SampleSec {
	case 2, 3, 5:
	default:
		d.SampleSec = 2
	}
	d.normalizePages()
}

// normalizePages 把页面顺序收敛成"合法页名的一次排列"、启用表收敛成其子集。
//
// 规则故意偏保守：
//   - 顺序里缺了某页就补到末尾（而不是报错）—— 老配置文件没有这两个
//     字段，Unmarshal 后是空切片，必须能自己长出默认值
//   - 启用表里出现顺序之外的页名直接丢弃（防手改配置写出错别字）
//   - 启用表为空 = 全部启用。面板允许配到"只剩一页"，但不会允许 0 页
//     —— 屏幕总得显示点什么
func (d *DisplayConfig) normalizePages() {
	seen := map[string]bool{}
	order := make([]string, 0, len(canonicalPages))
	for _, n := range d.PageOrder {
		if !isValidPageName(n) || seen[n] {
			continue
		}
		seen[n] = true
		order = append(order, n)
	}
	for _, n := range canonicalPages {
		if !seen[n] {
			order = append(order, n)
		}
	}
	d.PageOrder = order

	seen = map[string]bool{}
	on := make([]string, 0, len(order))
	for _, n := range d.PagesOn {
		if !isValidPageName(n) || seen[n] {
			continue
		}
		seen[n] = true
		on = append(on, n)
	}
	if len(on) == 0 {
		on = append(on, order...)
	}
	d.PagesOn = on
}

// EnabledPages 按 PageOrder 的顺序返回启用的页名（渲染器用这个建页列表）
func (d *DisplayConfig) EnabledPages() []string {
	on := map[string]bool{}
	for _, n := range d.PagesOn {
		on[n] = true
	}
	out := make([]string, 0, len(d.PageOrder))
	for _, n := range d.PageOrder {
		if on[n] {
			out = append(out, n)
		}
	}
	return out
}

// isValidPageName 单个页名是否合法（与 ui.PageNames 保持同一口径，
// 这里不用 import ui —— config 是最底层，反向依赖会成环）
func isValidPageName(name string) bool {
	for _, n := range canonicalPages {
		if n == name {
			return true
		}
	}
	return false
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
	cfg.normalize()
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
//
// 锁序固定为 saveM → mu，任何新增的落盘路径都必须遵守。
// 为什么必须先拿 saveM 再改内存：旧实现是「改内存→放锁→排队取 saveM」，
// 而 s.mu 的解锁顺序与 saveM 的竞争顺序毫无关系，于是后改的值可能先落盘、
// 先改的值反而最后落盘，磁盘留下一个旧快照而内存已是新值 —— 表现为丢更新。
// TestConcurrentUpdate 抓到过「内存态 10002 != 落盘 10048」。
// 现在「取快照」与「落盘」进同一临界区，最后一个写完的必然是最新快照。
func (s *Store) Update(mutate func(*Config)) error {
	s.saveM.Lock()
	defer s.saveM.Unlock()

	s.mu.Lock()
	if mutate != nil {
		mutate(&s.cur)
	}
	snap := s.cur
	s.mu.Unlock()

	return SaveTo(s.path, &snap)
}

// Save 把当前配置落盘。同样先拿 saveM 再取快照，
// 避免一个旧快照盖掉刚由 Update 写下的新值。
func (s *Store) Save() error {
	s.saveM.Lock()
	defer s.saveM.Unlock()

	s.mu.RLock()
	snap := s.cur
	s.mu.RUnlock()

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
		Display: DisplayConfig{Theme: "ink", Clock24: true, Weather: true, SampleSec: 2,
			PageOrder: []string{"env", "detail", "system"},
			PagesOn:   []string{"env", "detail", "system"}},
		Hardware: HardwareConfig{
			BrightnessPath: "/sys/devices/pci0000:00/0000:00:02.0/drm/card0/card0-DSI-1/intel_backlight/brightness",
			HwmonPath:      "/sys/class/hwmon/hwmon3",
			Framebuffer:    "/dev/fb0",
		},
	}
}
