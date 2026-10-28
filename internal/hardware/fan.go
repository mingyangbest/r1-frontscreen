package hardware

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"r1-toolbox/internal/logx"
)

// tempFallback 传感器读不到时**仅供调速**使用的保守值。
//
// 它绝不会出现在界面上：界面走 GetTemp 的 ok=false 显示「—」。
// 旧实现把这个 30 当成真实读数返回，于是温度读数失败时屏上赫然写着
// 「30°C」，而实际上一个传感器都没读到 —— 这是明确的编数据。
const tempFallback = 30.0

type FanController struct {
	it8628Path   string
	coretempPath string
	cpuPWMIndex  int
	hddPWMIndex  int

	// mu 保护下面这些运行期可变设置。
	//
	// -race 实锤（2026-10-28）：Web 的 /api/fan 与 /api/fan/curve 在写
	// cpuMode / 曲线上下限，而 Run() 协程同时每秒读它们算转速 ——
	// 7 处 DATA RACE 里有两处就在这条路径上（handleFanCurve→SetHDDTempRange
	// 对 FanController.Run）。表现是「风扇设置偶尔设了不生效」。
	mu            sync.RWMutex
	cpuMode       string
	hddMode       string
	cpuMinTemp    int
	cpuMaxTemp    int
	hddMinTemp    int
	hddMaxTemp    int
	cpuTempSensor string
	hddTempSensor string
	// hddTempSource 由主程序注入：返回「真实硬盘温度」（多盘取最高）。
	//
	// 这是 2026-09-27 洋哥提出的修正。原来机箱风扇读的是 it8628:temp2 ——
	// 那颗传感器焊在主板上（硬盘上方），实测常年在 47~48°C，而硬盘本体
	// 只有 37°C（SATA）/ 50°C（NVMe）。被控量本身偏高 10°C，再配上
	// 30~45°C 的曲线，结果就是**永远顶在 255 满速**，模式切了也听不出差别。
	// 现在改为读盘体温度，且与硬盘页显示的是同一个数据源（monitor.DiskMonitor），
	// 屏幕上的数字和调速用的数字永远一致。
	// 注入为空或读不到时，回退到 hddTempSensor（至少比失控安全）。
	hddTempSource func() float64

	// speedMu 串行化「写 pwm」与 lastSpeed 去重。
	// 目前只有 Run() 一个写者，但 SetCPUSpeed 是导出方法，
	// 将来 Web 面板直接调也不会踩到。
	speedMu      sync.Mutex
	lastCPUSpeed int
	lastHDDSpeed int
}

func NewFanController() *FanController {
	it8628 := DiscoverIT8628Path()
	coretemp := DiscoverCoretempPath()

	logx.Debugf("发现 IT8628: %s", it8628)
	logx.Debugf("发现 Coretemp: %s", coretemp)

	return &FanController{
		it8628Path:   it8628,
		coretempPath: coretemp,
		cpuPWMIndex:  3,
		hddPWMIndex:  2,
		cpuMode:      "auto",
		hddMode:      "auto",
		cpuMinTemp:   35,
		cpuMaxTemp:   75,
		// 机箱风扇曲线：40°C 起调、60°C 满速。
		// 盘体 37°C → 最低档；50°C（本机 NVMe 常态）→ 约 152/255；60°C → 满速。
		// 想看更安静就把 MaxTemp 调大，更保守就调小（配置项 fan.hdd.min_temp/max_temp）。
		hddMinTemp:    40,
		hddMaxTemp:    60,
		cpuTempSensor: "coretemp:Package id 0",
		hddTempSensor: "it8628:temp2", // 仅作兜底：盘温读不到时才用它
		lastCPUSpeed:  -1,
		lastHDDSpeed:  -1,
	}
}

// SetHDDTempSource 注入真实硬盘温度来源（由 main 接 monitor.DiskMonitor）
func (f *FanController) SetHDDTempSource(fn func() float64) {
	f.mu.Lock()
	f.hddTempSource = fn
	f.mu.Unlock()
}

func (f *FanController) Init() error {
	if f.it8628Path == "" {
		return fmt.Errorf("未找到 IT8628 芯片")
	}

	logx.Debugf("设置 PWM%d 为手动模式", f.cpuPWMIndex)
	f.setPWMEnable(f.cpuPWMIndex, 1)

	logx.Debugf("设置 PWM%d 为手动模式", f.hddPWMIndex)
	f.setPWMEnable(f.hddPWMIndex, 1)

	return nil
}

func (f *FanController) setPWMEnable(pwmIndex int, mode int) error {
	path := filepath.Join(f.it8628Path, fmt.Sprintf("pwm%d_enable", pwmIndex))
	logx.Debugf("写入 %s = %d", path, mode)
	return os.WriteFile(path, []byte(strconv.Itoa(mode)), 0644)
}

func (f *FanController) SetCPUMode(mode string) {
	f.mu.Lock()
	changed := f.cpuMode != mode
	f.cpuMode = mode
	f.mu.Unlock()
	if changed {
		logx.Printf("CPU 风扇模式: %s", mode)
	}
}

func (f *FanController) SetHDDMode(mode string) {
	f.mu.Lock()
	changed := f.hddMode != mode
	f.hddMode = mode
	f.mu.Unlock()
	if changed {
		logx.Printf("HDD 风扇模式: %s", mode)
	}
}

func (f *FanController) GetCPUMode() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cpuMode
}

func (f *FanController) GetHDDMode() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.hddMode
}

// GetFanSpeeds 返回 (CPU风扇RPM, 机箱风扇RPM)。
//
// 直接读 IT8628 的 fan*_input 真实转速 —— 界面上的「自动/低速/中速/高速」
// 只是模式名，转速才是可被验证的事实（写 pwmN 之后转速会实际变化）。
// 读不到返回 0，调用方据此决定是否显示。
func (f *FanController) GetFanSpeeds() (int, int) {
	if f.it8628Path == "" {
		return 0, 0
	}
	readRPM := func(idx int) int {
		if idx <= 0 {
			return 0
		}
		b, err := os.ReadFile(filepath.Join(f.it8628Path, fmt.Sprintf("fan%d_input", idx)))
		if err != nil {
			return 0
		}
		v, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || v < 0 {
			return 0
		}
		return v
	}
	return readRPM(f.cpuPWMIndex), readRPM(f.hddPWMIndex)
}

// CurrentPWMDuty 返回当前实际写入的 PWM 占空比 0-255（读硬件，不是读模式名）
func (f *FanController) CurrentPWMDuty() (int, int) {
	read := func(idx int) int {
		if idx <= 0 {
			return -1
		}
		b, err := os.ReadFile(filepath.Join(f.it8628Path, fmt.Sprintf("pwm%d", idx)))
		if err != nil {
			return -1
		}
		v, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			return -1
		}
		return v
	}
	return read(f.cpuPWMIndex), read(f.hddPWMIndex)
}

func (f *FanController) SetCPUTempRange(minTemp, maxTemp int) {
	f.mu.Lock()
	f.cpuMinTemp, f.cpuMaxTemp = minTemp, maxTemp
	f.mu.Unlock()
}

func (f *FanController) SetHDDTempRange(minTemp, maxTemp int) {
	f.mu.Lock()
	f.hddMinTemp, f.hddMaxTemp = minTemp, maxTemp
	f.mu.Unlock()
}

// CPUTempRange / HDDTempRange 供 Web 回读（避免让 handler 直接摸字段）
func (f *FanController) CPUTempRange() (int, int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cpuMinTemp, f.cpuMaxTemp
}

func (f *FanController) HDDTempRange() (int, int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.hddMinTemp, f.hddMaxTemp
}

func (f *FanController) SetCPUSpeed(speed int) error {
	if speed < 0 {
		speed = 0
	}
	if speed > 255 {
		speed = 255
	}

	f.speedMu.Lock()
	defer f.speedMu.Unlock()

	if f.lastCPUSpeed == speed {
		return nil
	}

	path := filepath.Join(f.it8628Path, fmt.Sprintf("pwm%d", f.cpuPWMIndex))
	err := os.WriteFile(path, []byte(strconv.Itoa(speed)), 0644)
	if err == nil {
		f.lastCPUSpeed = speed
		logx.Debugf("CPU 风扇 → %d/255", speed)
	} else {
		logx.Printf("CPU 风扇写入失败: %v", err)
	}
	return err
}

func (f *FanController) SetHDDSpeed(speed int) error {
	if speed < 0 {
		speed = 0
	}
	if speed > 255 {
		speed = 255
	}

	f.speedMu.Lock()
	defer f.speedMu.Unlock()

	if f.lastHDDSpeed == speed {
		return nil
	}

	path := filepath.Join(f.it8628Path, fmt.Sprintf("pwm%d", f.hddPWMIndex))
	err := os.WriteFile(path, []byte(strconv.Itoa(speed)), 0644)
	if err == nil {
		f.lastHDDSpeed = speed
		logx.Debugf("机箱风扇 → %d/255", speed)
	} else {
		logx.Printf("机箱风扇写入失败: %v", err)
	}
	return err
}

// ------------------------------------------------------------
//  温度读取
// ------------------------------------------------------------
//
// 🔴 语义约定（2026-10-28 修正）：
//   - GetTemp 返回 (读数, 是否读成功)。读不到就 ok=false，
//     界面显示「—」。**不允许**用 30.0 这种占位值冒充真实读数。
//   - TempForControl 只给调速用：读不到时返回保守值，保证风扇不会停。

// GetTemp 读取指定传感器；ok=false 表示这次读取失败
func (f *FanController) GetTemp(sensor string) (float64, bool) {
	if sensor == "" {
		return f.getCoretempPackage()
	}

	parts := strings.SplitN(sensor, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	chip, label := parts[0], parts[1]

	switch chip {
	case "coretemp":
		return f.getCoretempByLabel(label)
	case "it8628", "it87":
		return f.getIT8628TempByLabel(label)
	case "nvme":
		return f.getNVMeTempByLabel(label)
	}
	return 0, false
}

// TempForControl 供调速使用：读不到时返回保守值，绝不返回 0（会让风扇停）
func (f *FanController) TempForControl(sensor string) float64 {
	if v, ok := f.GetTemp(sensor); ok {
		return v
	}
	return tempFallback
}

func (f *FanController) getCoretempPackage() (float64, bool) {
	if f.coretempPath == "" {
		return 0, false
	}
	b, err := os.ReadFile(filepath.Join(f.coretempPath, "temp1_input"))
	if err != nil {
		return 0, false
	}
	t, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || t <= 0 {
		return 0, false
	}
	return float64(t) / 1000.0, true
}

func (f *FanController) getCoretempByLabel(label string) (float64, bool) {
	if f.coretempPath == "" {
		return 0, false
	}
	if label == "Package id 0" {
		return f.getCoretempPackage()
	}

	for i := 1; i <= 10; i++ {
		labelData, err := os.ReadFile(filepath.Join(f.coretempPath, fmt.Sprintf("temp%d_label", i)))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(labelData)) != label {
			continue
		}
		data, err := os.ReadFile(filepath.Join(f.coretempPath, fmt.Sprintf("temp%d_input", i)))
		if err != nil {
			continue
		}
		t, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || t <= 0 {
			continue
		}
		return float64(t) / 1000.0, true
	}
	return 0, false
}

func (f *FanController) getIT8628TempByLabel(label string) (float64, bool) {
	if f.it8628Path == "" {
		return 0, false
	}
	tempIndex := 0
	if strings.HasPrefix(label, "temp") {
		fmt.Sscanf(label, "temp%d", &tempIndex)
	}
	if tempIndex <= 0 {
		return 0, false
	}
	data, err := os.ReadFile(filepath.Join(f.it8628Path, fmt.Sprintf("temp%d_input", tempIndex)))
	if err != nil {
		return 0, false
	}
	t, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || t <= 0 {
		return 0, false
	}
	return float64(t) / 1000.0, true
}

func (f *FanController) getNVMeTempByLabel(label string) (float64, bool) {
	nvmePath := DiscoverNVMePath()
	if nvmePath == "" {
		return 0, false
	}
	data, err := os.ReadFile(filepath.Join(nvmePath, "temp1_input"))
	if err != nil {
		return 0, false
	}
	t, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || t <= 0 {
		return 0, false
	}
	return float64(t) / 1000.0, true
}

// ------------------------------------------------------------
//  调速主循环
// ------------------------------------------------------------

func (f *FanController) Run() {
	for {
		f.tick()
		time.Sleep(time.Second)
	}
}

// tick 一次调速。
//
// 持锁只做「取快照」这一件事，温度读取与 pwm 写入都在锁外 ——
// 否则 Web 面板一次改曲线就得等 3 个 hwmon 文件读完。
func (f *FanController) tick() {
	f.mu.RLock()
	cpuSensor := f.cpuTempSensor
	hddSensor := f.hddTempSensor
	cpuMode := f.cpuMode
	hddMode := f.hddMode
	cpuMin, cpuMax := f.cpuMinTemp, f.cpuMaxTemp
	hddMin, hddMax := f.hddMinTemp, f.hddMaxTemp
	hddSrc := f.hddTempSource
	f.mu.RUnlock()

	cpuTemp := f.TempForControl(cpuSensor)

	// 机箱风扇被控量 = 真实盘体温度（多盘取最高），取不到才回退主板传感器
	hddTemp := 0.0
	if hddSrc != nil {
		hddTemp = hddSrc()
	}
	if hddTemp <= 0 {
		hddTemp = f.TempForControl(hddSensor)
	}

	cpuSpeed := fixedSpeed(cpuMode)
	if cpuMode == "auto" {
		cpuSpeed = f.CalculateSpeed(cpuTemp, cpuMin, cpuMax, 50, 255)
	}
	hddSpeed := fixedSpeed(hddMode)
	if hddMode == "auto" {
		hddSpeed = f.CalculateSpeed(hddTemp, hddMin, hddMax, 50, 255)
	}

	f.SetCPUSpeed(cpuSpeed)
	f.SetHDDSpeed(hddSpeed)
}

// fixedSpeed 手动模式对应的占空比（未知模式按中速，不按 0）
func fixedSpeed(mode string) int {
	switch mode {
	case "low":
		return 85
	case "high", "full":
		return 255
	default:
		return 170
	}
}

func (f *FanController) CalculateSpeed(temp float64, minTemp, maxTemp, minSpeed, maxSpeed int) int {
	if maxTemp <= minTemp { // 配置损坏（min>=max）时不产生 NaN
		return maxSpeed
	}
	if temp <= float64(minTemp) {
		return max(minSpeed, 25)
	}
	if temp >= float64(maxTemp) {
		return maxSpeed
	}
	ratio := (temp - float64(minTemp)) / float64(maxTemp-minTemp)
	speed := int(float64(minSpeed) + ratio*float64(maxSpeed-minSpeed))
	return max(max(minSpeed, 25), min(speed, maxSpeed))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
