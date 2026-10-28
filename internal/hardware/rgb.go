package hardware

import (
	"fmt"
	"sync"
	"time"

	"r1-toolbox/internal/execx"
	"r1-toolbox/internal/logx"
)

// 单条 i2cset 的超时。正常 5~15ms；I2C 总线被别的驱动占住时会卡住，
// 而这条路径由 Web 请求同步触发，卡住就是一个永远不返回的 HTTP 请求。
const i2cTimeout = 2 * time.Second

type RGBController struct {
	bus string

	// mu 串行化整段寄存器写入序列 —— 两位用户同时点面板时，
	// 两条 SetMode 交错写 0x90/0x92-0x94 会拼出一个谁都没选过的颜色。
	mu sync.Mutex
}

func NewRGBController() *RGBController {
	return &RGBController{bus: "0"}
}

func (r *RGBController) i2cSet(register, value int) error {
	_, err := execx.Run(i2cTimeout, "i2cset", "-f", "-y", r.bus, "0x26",
		fmt.Sprintf("0x%02x", register), fmt.Sprintf("0x%02x", value))
	return err
}

func (r *RGBController) SetMode(mode, color string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	logx.Printf("设置 RGB 灯: mode=%s color=%s", mode, color)

	// 重置RGB灯状态
	r.i2cSet(0x90, 0x0)
	r.i2cSet(0x92, 0)
	r.i2cSet(0x93, 0)
	r.i2cSet(0x94, 0)
	time.Sleep(200 * time.Millisecond)

	switch mode {
	case "off":
		return r.i2cSet(0x90, 0x0)

	case "rainbow":
		return r.i2cSet(0x90, 0x2)

	case "breathing":
		rgb := getColorRGB(color)
		r.i2cSet(0x92, rgb[0])
		r.i2cSet(0x93, rgb[1])
		r.i2cSet(0x94, rgb[2])
		time.Sleep(100 * time.Millisecond)
		return r.i2cSet(0x90, 0x3)

	case "solid":
		rgb := getColorRGB(color)
		r.i2cSet(0x92, rgb[0])
		r.i2cSet(0x93, rgb[1])
		r.i2cSet(0x94, rgb[2])
		time.Sleep(100 * time.Millisecond)
		return r.i2cSet(0x90, 0x1)
	}

	return nil
}

func getColorRGB(color string) [3]int {
	colors := map[string][3]int{
		"red":    {255, 0, 0},
		"orange": {255, 136, 0},
		"yellow": {255, 255, 0},
		"green":  {0, 255, 0},
		"cyan":   {0, 255, 255},
		"blue":   {0, 0, 255},
		"purple": {255, 0, 255},
		"white":  {255, 255, 255},
	}

	if rgb, ok := colors[color]; ok {
		return rgb
	}
	return [3]int{0, 255, 0} // 默认绿色
}
