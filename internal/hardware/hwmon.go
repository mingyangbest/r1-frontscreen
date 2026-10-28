package hardware

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"r1-toolbox/internal/execx"
)

// sensors 是一次昂贵的进程调用（实测 30~120ms），而 ScanFans 与 ScanTemps
// 原本各调一次 —— 一次面板请求要跑两遍。这里加 2 秒 TTL 缓存：
// 同一请求里的两次扫描共用一次输出，连续点按钮也不会重复起进程。
var (
	sensorsMu   sync.Mutex
	sensorsAt   time.Time
	sensorsData string
)

const sensorsTTL = 2 * time.Second

func sensorsOutput() (string, error) {
	sensorsMu.Lock()
	defer sensorsMu.Unlock()
	if sensorsData != "" && time.Since(sensorsAt) < sensorsTTL {
		return sensorsData, nil
	}
	out, err := execx.Run(3*time.Second, "sensors")
	if err != nil {
		return "", err
	}
	sensorsData = string(out)
	sensorsAt = time.Now()
	return sensorsData, nil
}

type HwmonScanner struct{}

type FanInfo struct {
	Chip  string
	Label string
	RPM   int
}

type TempInfo struct {
	Chip  string
	Label string
	Temp  float64
}

func NewHwmonScanner(basePath string) *HwmonScanner {
	return &HwmonScanner{}
}

func (h *HwmonScanner) ScanFans() []FanInfo {
	out, err := sensorsOutput()
	if err != nil {
		return []FanInfo{}
	}

	var fans []FanInfo
	lines := strings.Split(out, "\n")
	currentChip := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "-isa-") || strings.Contains(line, "-pci-") {
			currentChip = strings.Split(line, "-")[0]
		}
		if strings.Contains(line, "fan") && strings.Contains(line, "RPM") {
			re := regexp.MustCompile(`(.+?):\s+(\d+)\s+RPM`)
			match := re.FindStringSubmatch(line)
			if len(match) >= 3 {
				rpm, _ := strconv.Atoi(match[2])
				fans = append(fans, FanInfo{
					Chip:  currentChip,
					Label: strings.TrimSpace(match[1]),
					RPM:   rpm,
				})
			}
		}
	}
	return fans
}

func (h *HwmonScanner) ScanTemps() []TempInfo {
	out, err := sensorsOutput()
	if err != nil {
		return []TempInfo{}
	}

	var temps []TempInfo
	lines := strings.Split(out, "\n")
	currentChip := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "-isa-") || strings.Contains(line, "-pci-") {
			currentChip = strings.Split(line, "-")[0]
		}
		if strings.Contains(line, "°C") && !strings.Contains(line, "fan") {
			re := regexp.MustCompile(`(.+?):\s+\+?([\d.]+)°C`)
			match := re.FindStringSubmatch(line)
			if len(match) >= 3 {
				temp, _ := strconv.ParseFloat(match[2], 64)
				temps = append(temps, TempInfo{
					Chip:  currentChip,
					Label: strings.TrimSpace(match[1]),
					Temp:  temp,
				})
			}
		}
	}
	return temps
}

func (h *HwmonScanner) ReadTemp(index int) float64 {
	return 0
}
