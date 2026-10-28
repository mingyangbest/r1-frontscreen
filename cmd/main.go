package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"r1-toolbox/internal/config"
	"r1-toolbox/internal/hardware"
	"r1-toolbox/internal/logx"
	"r1-toolbox/internal/monitor"
	"r1-toolbox/internal/netinfo"
	"r1-toolbox/internal/screen"
	"r1-toolbox/internal/ui"
	"r1-toolbox/internal/watchdog"
	"r1-toolbox/internal/web"
)

// 版本信息由 -ldflags 注入（见 Makefile）：
//
//	go build -ldflags "-X main.version=v0.6.0 -X main.commit=a1b2c3d"
//
// 硬编码在源码里的话，屏幕上的版本号与 git tag 迟早对不上。
var (
	version = "dev"
	commit  = "dev"
)

// genWebToken 生成 8 位面板令牌。字符集剔除 0/O/1/I/L 等易混字符，
// 屏幕小字上抄写不会认错。
func genWebToken() (string, error) {
	const alphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i, v := range b {
		b[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(b), nil
}

const (
	// 帧率两档。
	//
	// 页面滑动单独给 60fps：376px 宽的位移在 0.38s 内完成，
	// 30fps 只有 11 帧、每帧跳 34px —— 无论缓动怎么调都会"一顿一顿"。
	//
	// 静止（既没有换页也没有缓动）8fps 心跳，配合跳帧机制，
	// 静态页 8 帧里 7 帧什么都不画，纯粹保触摸响应。
	fpsSlide = 60
	fpsEase  = 12
	fpsIdle  = 8
)

func main() {
	logx.Printf("R1 Toolbox %s (%s) 启动中...", version, commit)

	// ---------------- 配置 ----------------
	store, err := config.NewStore(config.ConfigPath())
	if err != nil {
		// 读不出来时用默认值继续（屏幕必须能亮），但错误必须打出来 ——
		// 静默回落会让用户的亮度/风扇曲线在毫无提示的情况下全部丢失。
		logx.Printf("配置读取失败，已用默认值启动: %v", err)
	}
	cfg := store.Get()

	if cfg.Debug || os.Getenv("R1_DEBUG") == "1" {
		logx.SetDebug(true)
		logx.Printf("明细日志已开启（性能/采样审计）")
	}

	ui.Version = version

	// ---------------- 硬件 ----------------
	mon := monitor.NewSystemMonitor()

	fanCtrl := hardware.NewFanController()
	fanCtrl.SetCPUTempRange(cfg.Fan.CPU.MinTemp, cfg.Fan.CPU.MaxTemp)
	fanCtrl.SetHDDTempRange(cfg.Fan.HDD.MinTemp, cfg.Fan.HDD.MaxTemp)
	fanCtrl.SetCPUMode(cfg.Fan.CPU.Mode)
	fanCtrl.SetHDDMode(cfg.Fan.HDD.Mode)
	// 机箱风扇的自动调速改为跟踪**真实盘体温度**（多盘取最高）。
	// 数据源就是硬盘页用的那个 DiskMonitor —— 屏幕上看到的盘温与调速用的盘温
	// 永远同源，不会出现"界面 37°C、风扇却按 48°C 吹"这种对不上的情况。
	fanCtrl.SetHDDTempSource(func() float64 { return mon.Disk.MaxDiskTemp() })
	fanCtrl.Init()

	rgbCtrl := hardware.NewRGBController()
	rgbCtrl.SetMode(cfg.RGB.Mode, cfg.RGB.Color)

	brightness := hardware.NewBrightnessController(cfg.Hardware.BrightnessPath, 0, 96000)

	// ---------------- 屏幕状态机 ----------------
	//
	// 渲染器依赖 screenCtl（系统页的亮度滑条要读写它），screenCtl 又需要
	// 一个回调控渲染器 —— 用 atomic 指针打破这个循环，顺便保证回调
	// 在不同协程里读到的是同一个渲染器实例。
	var rendererPtr atomic.Pointer[ui.Renderer]

	screenCtl := screen.New(brightness,
		cfg.Screen.Brightness, cfg.Screen.NightBrightness,
		ui.IsNight,
		func(off bool) {
			if r := rendererPtr.Load(); r != nil {
				r.SetScreenOff(off)
			}
		},
		func(day, night int) {
			// 手动亮度落盘：夜里调的亮度记到 night，白天调的记到 day，
			// 这样跨界之后用户的选择还在。
			if err := store.Update(func(c *config.Config) {
				c.Screen.Brightness = day
				c.Screen.NightBrightness = night
			}); err != nil {
				logx.Printf("保存亮度配置失败: %v", err)
			}
		})

	// ---------------- Web 令牌 ----------------
	if cfg.Web.Token == "" {
		if tok, gerr := genWebToken(); gerr == nil {
			cfg.Web.Token = tok
			if err := store.Update(func(c *config.Config) { c.Web.Token = tok }); err != nil {
				logx.Printf("保存面板令牌失败: %v", err)
			}
			logx.Printf("已生成 Web 面板令牌（显示在屏幕「系统」页底部）")
		} else {
			logx.Printf("生成 Web 令牌失败，面板将不可用: %v", gerr)
		}
	}
	if !cfg.Web.Enabled {
		logx.Printf("Web 面板已在配置中禁用")
	} else if cfg.Web.Token == "" {
		logx.Printf("警告: 无令牌，Web 面板不会启动（fail-closed）")
	}

	// ---------------- 渲染器 ----------------
	renderer, err := ui.NewRenderer(mon, fanCtrl, screenCtl)
	if err != nil {
		logx.Printf("渲染器初始化失败: %v", err)
		os.Exit(1)
	}
	defer renderer.Cleanup()
	rendererPtr.Store(renderer)
	renderer.Ctx.WebToken = store.Get().Web.Token

	// 启动即按昼夜定亮度：夜里重启服务不再被白天亮度闪一下眼
	brightness.Set(screenCtl.Target())
	logx.Printf("背光初始值 %d（%s）", screenCtl.Target(), dayNightText())

	go fanCtrl.Run()

	// ---------------- 后台采样 ----------------
	//
	// CPU/内存/网络 2 秒、磁盘/Docker 60 秒，全部在独立协程里跑。
	// 渲染线程每帧只取一次快照（值拷贝），smartctl 的 359ms 与
	// Docker socket 的 3 秒超时再也卡不到触摸响应。
	sampler := monitor.NewSampler(mon)
	sampler.Start()
	defer sampler.Stop()

	// ---------------- 外网信息（公网 IP / 天气）----------------
	netSvc := netinfo.New()
	netSvc.Start()
	renderer.Ctx.Net = netSvc

	// 系统页协议：关闭屏幕按钮走统一状态机
	renderer.Ctx.OnScreenOff = func() { screenCtl.Off("界面按钮") }

	// ---------------- Web 面板 ----------------
	if cfg.Web.Enabled && cfg.Web.Token != "" {
		webServer := web.NewServer(mon, fanCtrl, rgbCtrl, brightness, store, screenCtl)
		go func() {
			addr := fmt.Sprintf(":%d", cfg.Web.Port)
			logx.Printf("Web 面板启动在 %s", addr)
			if err := webServer.Run(addr); err != nil {
				logx.Printf("Web 服务器错误: %v", err)
			}
		}()
	}

	// ---------------- 触摸 ----------------
	useSDLTouch := false
	touchHandler, err := ui.NewTouchHandlerWithI2C()
	if err != nil {
		logx.Printf("I2C触摸屏直连不可用（R1固件预期行为），已回退SDL触摸: %v", err)
		touchHandler = ui.NewTouchHandler()
		useSDLTouch = true
	} else {
		logx.Println("使用 I2C 触摸屏")
		defer touchHandler.Close()
	}

	startTime := time.Now()
	lastTouchTime := time.Now()
	screenTimeout := time.Duration(cfg.Screen.Timeout) * time.Second
	if screenTimeout <= 0 {
		logx.Println("屏幕模式: 常亮（不自动息屏）")
	}

	slideDur := time.Second / fpsSlide
	idleDur := time.Second / fpsIdle
	easeDur := time.Second / fpsEase

	configReloadTicker := time.NewTicker(5 * time.Second)
	defer configReloadTicker.Stop()

	// 处理点击：派发给当前页（若支持）
	dispatchTap := func(x, y int) {
		if screenCtl.IsOff() {
			return
		}
		if tp, ok := renderer.CurrentPage().(ui.Tappable); ok {
			tp.Tap(x, y, renderer.Ctx)
			// 点按会改页面状态（风扇模式、亮度读数等），这些不经数据采样，
			// 跳帧机制感知不到 —— 显式强制下一帧全屏重绘，反馈才不会"卡 2 秒"。
			renderer.Invalidate()
		}
	}

	logx.Printf("初始化完成，共 %d 个页面", renderer.PageCount())
	watchdog.Ready()

	// ---------------- 主循环 ----------------
	running := true
	// 唤醒屏幕的那一次触摸序列要整体吞掉：手指按下时唤醒、抬起时才产生"点击"，
	// 若不吞掉，唤醒动作会顺带触发底下的按钮（例如又按到「关闭屏幕」）。
	var swallowUntil time.Time

	lastDataGen := uint64(0)
	lastNightCheck := time.Now()
	var lastBeat time.Time
	beatEvery := watchdog.Interval()

	for running {
		frameStart := time.Now()

		// ---------------- SDL 事件 ----------------
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			if _, ok := event.(*sdl.QuitEvent); ok {
				logx.Println("收到退出信号")
				running = false
				continue
			}

			// 触摸/鼠标事件是 SDL 唯一可能把指针重新显示出来的时机，这里保持关闭。
			if v, _ := sdl.ShowCursor(sdl.QUERY); v == sdl.ENABLE {
				sdl.ShowCursor(sdl.DISABLE)
				logx.Println("鼠标指针被重新打开，已再次隐藏")
			}

			gesture, tap := touchHandler.HandleEvent(event)

			// 消化唤醒序列：收到抬起事件即解除，另有 2 秒兜底
			if swallowUntil.After(frameStart) {
				switch e := event.(type) {
				case *sdl.TouchFingerEvent:
					if e.Type == sdl.FINGERUP {
						swallowUntil = time.Time{}
					}
				case *sdl.MouseButtonEvent:
					if e.Type == sdl.MOUSEBUTTONUP {
						swallowUntil = time.Time{}
					}
				}
				continue
			}

			woke := false
			if useSDLTouch {
				switch event.(type) {
				case *sdl.TouchFingerEvent, *sdl.MouseButtonEvent:
					lastTouchTime = time.Now()
					if screenCtl.IsOff() {
						screenCtl.Wake()
						woke = true
						swallowUntil = frameStart.Add(2 * time.Second)
					}
				}
			}
			if woke {
				continue
			}

			switch gesture {
			case ui.GestureSwipeLeft:
				renderer.SwitchPage(1)
			case ui.GestureSwipeRight:
				renderer.SwitchPage(-1)
			case ui.GestureSwipeUp:
				renderer.NextTheme()
			}

			if tap != nil {
				dispatchTap(int(tap.X), int(tap.Y))
			}
		}

		// ---------------- I2C 触摸 ----------------
		if !useSDLTouch && touchHandler != nil {
			if g := touchHandler.ReadI2CTouch(); g != ui.GestureNone {
				lastTouchTime = time.Now()
				screenCtl.Wake()
				switch g {
				case ui.GestureSwipeLeft:
					renderer.SwitchPage(1)
				case ui.GestureSwipeRight:
					renderer.SwitchPage(-1)
				case ui.GestureSwipeUp:
					renderer.NextTheme()
				}
			}
			if tap := touchHandler.ReadTap(); tap != nil {
				lastTouchTime = time.Now()
				dispatchTap(int(tap.X), int(tap.Y))
			}
		}

		// ---------------- 配置热重载 ----------------
		select {
		case <-configReloadTicker.C:
			if fresh, err := config.LoadFrom(store.Path()); err == nil {
				screenTimeout = time.Duration(fresh.Screen.Timeout) * time.Second
				screenCtl.SetDayNight(fresh.Screen.Brightness, fresh.Screen.NightBrightness)
				store.Replace(*fresh)
				if fresh.Debug != cfg.Debug {
					logx.SetDebug(fresh.Debug)
					cfg.Debug = fresh.Debug
				}
			}
		default:
		}

		// ---------------- 夜间自动调暗（每 30s 检查一次昼夜切换）----------------
		//
		// 只在切换瞬间调一次，不做周期拉回 —— 夜间通过 Web 面板手动调的
		// 亮度不会被覆盖，一直保留到下一次跨界。状态全在 screenCtl 里，
		// Web 息屏时这里不会再"顺手把屏点亮"。
		if time.Since(lastNightCheck) >= 30*time.Second {
			lastNightCheck = frameStart
			screenCtl.SyncNight()
		}

		// ---------------- 数据变化 → 通知重绘 ----------------
		//
		// 采样在后台跑，主循环只比较版本号。版本变了说明屏幕上的数字
		// 该刷新了，退一次全屏（或让页面走局部矩形）。
		if v := sampler.Version(); v != lastDataGen {
			lastDataGen = v
			renderer.Ctx.BumpData()
		}

		// ---------------- 自动息屏（timeout 为 0 时常亮）----------------
		if screenTimeout > 0 && !screenCtl.IsOff() && time.Since(lastTouchTime) > screenTimeout {
			screenCtl.Off("超时自动息屏")
		}

		// ---------------- 看门狗心跳 ----------------
		//
		// 主循环活着就等于心跳。若某天某个调用把主循环卡死，systemd 会在
		// WatchdogSec 后杀掉进程重启 —— 而不是让屏幕定格在一小时前的画面。
		if time.Since(lastBeat) >= beatEvery {
			lastBeat = frameStart
			watchdog.Watchdog()
		}

		// ---------------- 渲染 ----------------
		// Tick 放在判定之前：换页刚被触发的那一帧就要立刻切到 60fps，
		// 否则第一帧仍按静止档的 125ms 间隔走，动画开场就少掉两帧。
		renderer.Ctx.Now = time.Now()
		renderer.Ctx.T = frameStart.Sub(startTime).Seconds()
		renderer.Tick(frameStart)
		switching := renderer.Animating()
		animating := switching || renderer.Ctx.NeedsAnim()
		renderer.Ctx.BeginFrame()
		renderer.Render(renderer.Ctx)

		// ---------------- 帧率控制 ----------------
		//
		// 滑动 60fps（位移敏感）> 数值缓动 12fps（0.16s 过渡落 2 帧）>
		// 静止 8fps 心跳（配合跳帧，静态页基本零开销，只保触摸响应）。
		target := idleDur
		if screenCtl.IsOff() {
			target = time.Second // 息屏时画面全黑，无需高频重绘
		} else if switching {
			target = slideDur // 滑动：最高优先级
		} else if animating {
			target = easeDur
		}
		if spent := time.Since(frameStart); spent < target {
			time.Sleep(target - spent)
		}
	}

	logx.Println("R1 Toolbox 退出")
}

// dayNightText 当前昼夜状态的中文描述（仅用于启动日志）
func dayNightText() string {
	if ui.IsNight() {
		return "夜间窗口"
	}
	return "日间"
}
