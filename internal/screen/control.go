// Package screen 是屏幕背光的唯一状态源。
//
// 修复的状态分裂（2026-10-28）：旧实现里「关屏」有两套互不相认的状态 ——
//   - 主循环的 screenOff 布尔值（触摸唤醒、超时息屏、帧率都看它）
//   - Web 面板 /api/screen/off 直接写背光 0，主循环的 screenOff 仍是 false
//
// 于是 Web 息屏后，18:00 的夜间跨界看门狗会把屏幕**重新点亮**；
// 而 /api/screen/on 又硬编码 36000，夜里开屏直接刺眼。
//
// 现在所有改背光的路径（触摸唤醒、超时息屏、界面按钮、Web 面板、
// 夜间跨界、配置热重载）都必须经过 Control，它是一把锁下的一份状态。
package screen

import (
	"sync"

	"r1-toolbox/internal/hardware"
	"r1-toolbox/internal/logx"
)

// Control 背光状态机
type Control struct {
	mu  sync.Mutex
	br  *hardware.BrightnessController
	day int
	// night 夜间亮度（0 = 关闭夜间调暗）
	night int
	off   bool
	// lastNight 上一次观测到的昼夜状态，用于识别跨界瞬间
	lastNight bool

	isNight func() bool
	notify  func(off bool)       // 状态变化通知（渲染器）
	persist func(day, night int) // 手动亮度落盘
}

// New 创建控制器。isNight/notify/persist 允许为 nil（测试时）。
func New(br *hardware.BrightnessController, day, night int, isNight func() bool, notify func(bool), persist func(int, int)) *Control {
	return &Control{
		br:        br,
		day:       day,
		night:     night,
		lastNight: isNight != nil && isNight(),
		isNight:   isNight,
		notify:    notify,
		persist:   persist,
	}
}

// targetLocked 当前时段应生效的亮度（调用方必须持锁）
func (c *Control) targetLocked() int {
	if c.night > 0 && c.isNight != nil && c.isNight() {
		return c.night
	}
	if c.day <= 0 {
		return 36000
	}
	return c.day
}

// Target 当前时段应生效的亮度
func (c *Control) Target() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.targetLocked()
}

// IsOff 屏幕是否处于关闭状态
func (c *Control) IsOff() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.off
}

// Bright 当前背光读数（直接读硬件，读不到返回 -1）
func (c *Control) Bright() int {
	v, err := c.br.Get()
	if err != nil {
		return -1
	}
	return v
}

// Max 硬件背光上限（接口层做入参校验用，不硬编码）
func (c *Control) Max() int { return c.br.Max() }

// Apply 应用一个亮度值（手动调节 / Web 面板）。
//
// v > 0 视为点亮；v == 0 等同关屏。非夜间调节会更新 day，夜间调节更新
// night —— 这样「夜里调到 15000」在第二天夜里仍然是 15000，而不是被
// 日间值悄悄覆盖。更新后会通过 persist 落盘。
func (c *Control) Apply(v int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v <= 0 {
		return c.offLocked("亮度设为 0")
	}
	if err := c.br.Set(v); err != nil {
		return err
	}
	if c.off {
		c.off = false
		c.notifyOff(false)
	}
	if c.isNight != nil && c.isNight() {
		c.night = v
	} else {
		c.day = v
	}
	if c.persist != nil {
		c.persist(c.day, c.night)
	}
	return nil
}

// Off 关屏（超时 / 界面按钮 / Web 面板共用）
func (c *Control) Off(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offLocked(reason)
}

func (c *Control) offLocked(reason string) error {
	if c.off {
		return nil
	}
	if err := c.br.Set(0); err != nil {
		return err
	}
	c.off = true
	c.notifyOff(true)
	logx.Printf("【关屏】%s", reason)
	return nil
}

// Wake 点亮屏幕（触摸唤醒 / Web 面板开屏）。亮度按当前时段取。
func (c *Control) Wake() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.off {
		return
	}
	v := c.targetLocked()
	if err := c.br.Set(v); err != nil {
		logx.Printf("【唤醒】设置背光失败: %v", err)
		return
	}
	c.off = false
	c.notifyOff(false)
	logx.Printf("【唤醒】屏幕点亮（背光 %d）", v)
}

// SyncNight 昼夜跨界时调用：只在状态真正翻转的瞬间调整背光。
// 关屏期间不写背光（唤醒路径会按当时的时段取正确值）。
func (c *Control) SyncNight() {
	c.mu.Lock()
	defer c.mu.Unlock()

	night := c.isNight != nil && c.isNight()
	if night == c.lastNight {
		return
	}
	c.lastNight = night
	if c.off {
		return
	}
	v := c.targetLocked()
	if err := c.br.Set(v); err != nil {
		logx.Printf("【夜间调暗】设置背光失败: %v", err)
		return
	}
	if night {
		logx.Printf("【夜间调暗】进入夜间窗口（18:00–06:00），背光降至 %d", v)
	} else {
		logx.Printf("【夜间调暗】退出夜间窗口，背光恢复 %d", v)
	}
}

// NightDimEnabled 是否启用夜间调暗
func (c *Control) NightDimEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.night > 0
}

// SetDayNight 配置热重载：只更新亮度基准，不立刻改背光
// （下一次跨界或唤醒才生效，避免热重载把用户刚调的值弹回去）。
func (c *Control) SetDayNight(day, night int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if day > 0 {
		c.day = day
	}
	c.night = night
}

// notifyOff 通知渲染器（持锁调用；渲染器侧用原子变量接收，不会反查本结构）
func (c *Control) notifyOff(off bool) {
	if c.notify != nil {
		c.notify(off)
	}
}
