package monitor

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	netLib "net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemMonitor 采集系统实时指标。
//
// 数据真实性约定（每一屏上的数字都必须能追溯到内核/接口的真实读数）：
//   - 运行时长取 /proc/uptime（系统开机至今），不是本进程启动至今
//   - 网络速率只统计「默认路由所在接口」，绝不聚合所有接口
//     （聚合会把 docker0 / br-* / ovs 桥与物理口重复累加，速率虚高）
//   - 局域网地址取默认路由接口的 IPv4，不取「第一个非回环地址」
//     （那样可能取到 docker0 的 172.17.0.1）
//   - 内存已用 = 总容量 - 可用，保证 已用 + 可用 == 总容量（三项自洽）
//   - 内存总量有两个口径，必须分清：MemTotalGB 是内核可见（16 GB 机器只有
//     15.4 GiB，核显 stolen + 固件预留吃掉了 0.6 GiB），MemPhysGB 才是商品页
//     写的物理安装容量（读 SMBIOS type 17）。百分比只能拿 MemTotalGB 当分母，
//     否则 43% 对不上 已用/总量。
//
// 并发模型（2026-10-28 重构）：采样在独立 goroutine（见 sampler.go），
// 渲染与 Web 各自调 Snapshot() 拿**值拷贝**。所有字段都在 mu 保护下，
// 不存在「一个协程写、另一个协程读」的裸访问 —— -race 实测的
// 7 处 DATA RACE 里，属于本类型的部分由此归零。
type SystemMonitor struct {
	mu sync.RWMutex

	CPUPercent float64
	CPUCores   []float64
	CPUCoreNum int
	CPUTemp    float64
	CPUHistory []float64

	// 静态信息：开机后不会变，启动时读一次即固定，不参与每秒采样。
	// 读不到就是空串，界面显示「—」，绝不编造型号。
	CPUModel string // /proc/cpuinfo 的 model name（如 "Intel(R) N100"）
	Kernel   string // /proc/sys/kernel/osrelease（如 "6.18.18.c1126-trim"）

	MemPercent  float64
	MemUsedGB   float64
	MemTotalGB  float64 // 内核可见总量（MemTotal，已扣除核显/固件预留）
	MemPhysGB   float64 // 物理安装容量（SMBIOS type 17 汇总）；0 = 读不到
	MemAvailGB  float64
	MemHistory  []float64
	MemCacheGB  float64 // buff/cache（Buffers + Cached）
	SwapPercent float64
	SwapUsedGB  float64
	SwapTotalGB float64

	// 网络（仅主网口）
	NetIface    string
	NetSendMBps float64
	NetRecvMBps float64
	NetRxTotal  uint64 // 该网口自开机以来的累计接收字节
	NetTxTotal  uint64 // 该网口自开机以来的累计发送字节
	NetRxToday  uint64 // 今日累计接收字节（跨天自动归零）
	NetTxToday  uint64 // 今日累计发送字节
	NetHistory  [][2]float64

	LocalIP     string
	NetLinkMbps int // 主网口协商速率（Mbit/s），0 = 读不到

	// 系统
	UptimeSec float64 // /proc/uptime，系统开机至今秒数
	LoadAvg   [3]float64
	ProcCount int // 系统当前进程总数

	// Docker 容器（UpdateContainers 每 60s 刷新；-1 = 未知/读不到）
	ContainersRunning int
	ContainersTotal   int

	Disk *DiskMonitor

	// 数据新鲜度：最近一次成功采样的时刻。界面用它显示「更新于 xx:xx:xx」，
	// 采样协程挂掉时用户能看出来，而不是盯着一屏永远不变的数字。
	updatedAt time.Time

	lastRx, lastTx uint64
	lastNetTime    time.Time

	day *netDayState // 当日流量的起算基线
}

// Snapshot 某一时刻的完整读数（值拷贝，可安全跨协程传阅）。
//
// 字段名与 SystemMonitor 一一对应，页面的绘制代码只需把 p.mon 换成
// p.mon.Snapshot()，其余读法完全不变 —— 这样重构不会碰到任何排版坐标。
type Snapshot struct {
	CPUPercent float64
	CPUCores   []float64
	CPUCoreNum int
	CPUTemp    float64
	CPUHistory []float64
	CPUModel   string
	Kernel     string

	MemPercent  float64
	MemUsedGB   float64
	MemTotalGB  float64
	MemPhysGB   float64
	MemAvailGB  float64
	MemHistory  []float64
	MemCacheGB  float64
	SwapPercent float64
	SwapUsedGB  float64
	SwapTotalGB float64

	NetIface    string
	NetSendMBps float64
	NetRecvMBps float64
	NetRxTotal  uint64
	NetTxTotal  uint64
	NetRxToday  uint64
	NetTxToday  uint64
	NetHistory  [][2]float64
	LocalIP     string
	// NetLinkMbps 主网口协商速率（/sys/class/net/<if>/speed，单位 Mbit/s）；
	// 0 = 读不到（链路 down / 虚拟口不报速率）。系统页网络卡的两排信号条
	// 用它点亮 —— 亮几根是真实链路档位，不是随手画的花纹。
	NetLinkMbps int

	UptimeSec float64
	LoadAvg   [3]float64
	ProcCount int

	ContainersRunning int
	ContainersTotal   int

	// Disk 是共享子监控，自身带锁；快照里保留指针即可。
	Disk *DiskMonitor

	// UpdatedAt 本快照的数据采集时刻
	UpdatedAt time.Time
}

// Snapshot 取一份值拷贝。切片深拷贝，调用方拿到后不会被下一次采样改写。
func (s *SystemMonitor) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		CPUPercent: s.CPUPercent,
		CPUCores:   cloneFloats(s.CPUCores),
		CPUCoreNum: s.CPUCoreNum,
		CPUTemp:    s.CPUTemp,
		CPUHistory: cloneFloats(s.CPUHistory),
		CPUModel:   s.CPUModel,
		Kernel:     s.Kernel,

		MemPercent:  s.MemPercent,
		MemUsedGB:   s.MemUsedGB,
		MemTotalGB:  s.MemTotalGB,
		MemPhysGB:   s.MemPhysGB,
		MemAvailGB:  s.MemAvailGB,
		MemHistory:  cloneFloats(s.MemHistory),
		MemCacheGB:  s.MemCacheGB,
		SwapPercent: s.SwapPercent,
		SwapUsedGB:  s.SwapUsedGB,
		SwapTotalGB: s.SwapTotalGB,

		NetIface:    s.NetIface,
		NetSendMBps: s.NetSendMBps,
		NetRecvMBps: s.NetRecvMBps,
		NetRxTotal:  s.NetRxTotal,
		NetTxTotal:  s.NetTxTotal,
		NetRxToday:  s.NetRxToday,
		NetTxToday:  s.NetTxToday,
		NetHistory:  clonePairs(s.NetHistory),
		LocalIP:     s.LocalIP,
		NetLinkMbps: s.NetLinkMbps,

		UptimeSec: s.UptimeSec,
		LoadAvg:   s.LoadAvg,
		ProcCount: s.ProcCount,

		ContainersRunning: s.ContainersRunning,
		ContainersTotal:   s.ContainersTotal,

		Disk:      s.Disk,
		UpdatedAt: s.updatedAt,
	}
}

func cloneFloats(in []float64) []float64 {
	if len(in) == 0 {
		return nil
	}
	out := make([]float64, len(in))
	copy(out, in)
	return out
}

func clonePairs(in [][2]float64) [][2]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make([][2]float64, len(in))
	copy(out, in)
	return out
}

func NewSystemMonitor() *SystemMonitor {
	model, kernel := readStaticInfo()
	return &SystemMonitor{
		CPUHistory: make([]float64, 0, 30),
		MemHistory: make([]float64, 0, 30),
		NetHistory: make([][2]float64, 0, 30),
		Disk:       NewDiskMonitor(),
		CPUModel:   model,
		Kernel:     kernel,
	}
}

// readStaticInfo 读开机后不变化的硬件与内核标识。
//
// 为什么不走 gopsutil：它没有「CPU 型号」这一项（cpu.Info() 在 x86 上
// 也只是回读同一个 /proc/cpuinfo），而内核串要的是 osrelease 原文
// （"6.18.18.c1126-trim" 这种带发行版后缀的版本号，uname -r 等价）。
// 两处都是单次读取，放在构造期做一次，采样循环里零成本。
//
// 读不到就返回空串 —— 设备卡显示「—」。宁可少一行，也不填一个
// 看起来合理的假型号，这块屏的第一原则是数据可追溯。
func readStaticInfo() (cpuModel, kernel string) {
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if !strings.HasPrefix(line, "model name") {
				continue
			}
			if i := strings.IndexByte(line, ':'); i >= 0 {
				cpuModel = strings.TrimSpace(line[i+1:])
			}
			break
		}
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		kernel = strings.TrimSpace(string(b))
	}
	return cpuModel, kernel
}

func (s *SystemMonitor) UpdateCPU() error {
	// ---- 锁外采集 ----
	// gopsutil 的 cpu.Percent 要读 /proc/stat、getCPUTempFromFile 最多读
	// 20 个 hwmon 文件、countProcesses 要遍历 /proc。这些加起来 3~8ms，
	// 全程占着写锁会让渲染的 Snapshot() 一起等 —— 触摸就卡了。
	percent, _ := cpu.Percent(0, false)
	cores, _ := cpu.Percent(0, true)
	n, _ := cpu.Counts(true)
	temp := s.getCPUTempFromFile()
	up := readUptime()
	la, _ := readLoadAvg()
	procs := countProcesses()

	// ---- 锁内只做赋值，微秒级 ----
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(percent) > 0 {
		s.CPUPercent = percent[0]
		s.CPUHistory = append(s.CPUHistory, percent[0])
		if len(s.CPUHistory) > 30 {
			s.CPUHistory = s.CPUHistory[1:]
		}
	}

	// 首次采样可能返回空（没有上一帧基线），不要用空结果覆盖已有的核心数
	if len(cores) > 0 {
		s.CPUCores = cores
	}
	if n > 0 {
		s.CPUCoreNum = n
	}

	// Go 1.19 的 gopsutil 没有 Temperature()，使用文件读取
	s.CPUTemp = temp
	s.UptimeSec = up
	s.LoadAvg = la
	// 注意：/proc/loadavg 第四列的总数是「内核任务数」（含线程），
	// 明显大于真实进程数（实测 1240 vs 376）。要显示「进程」就必须数 /proc 里的 PID 目录。
	s.ProcCount = procs
	s.updatedAt = time.Now()
	return nil
}

// countProcesses 数 /proc 下的纯数字目录，即真实进程数（与 `ps -e` 一致）
func countProcesses() int {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		name := e.Name()
		if len(name) == 0 || name[0] < '0' || name[0] > '9' {
			continue
		}
		n++
	}
	return n
}

// readUptime 读 /proc/uptime 第一列（系统开机至今秒数）
func readUptime() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return v
}

// readLoadAvg 读 /proc/loadavg：前三个是 1/5/15 分钟负载，第四列 "运行数/总进程数"
func readLoadAvg() ([3]float64, int) {
	var la [3]float64
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return la, 0
	}
	f := strings.Fields(string(data))
	if len(f) < 4 {
		return la, 0
	}
	for i := 0; i < 3; i++ {
		la[i], _ = strconv.ParseFloat(f[i], 64)
	}
	if i := strings.Index(f[3], "/"); i >= 0 {
		if n, err := strconv.Atoi(f[3][i+1:]); err == nil {
			return la, n
		}
	}
	return la, 0
}

func (s *SystemMonitor) getCPUTempFromFile() float64 {
	// 智能查找coretemp设备
	for i := 0; i <= 10; i++ {
		hwmonPath := fmt.Sprintf("/sys/class/hwmon/hwmon%d", i)
		namePath := hwmonPath + "/name"

		// 检查是否是coretemp
		if nameData, err := os.ReadFile(namePath); err == nil {
			if strings.TrimSpace(string(nameData)) == "coretemp" {
				// 找到coretemp，读取Package id 0温度 (temp1_input)
				tempPath := hwmonPath + "/temp1_input"
				if data, err := os.ReadFile(tempPath); err == nil {
					if temp, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
						return float64(temp) / 1000.0
					}
				}
			}
		}
	}

	// 备用：thermal_zone
	thermalPaths := []string{
		"/sys/class/thermal/thermal_zone0/temp",
		"/sys/class/thermal/thermal_zone1/temp",
	}
	for _, path := range thermalPaths {
		data, err := os.ReadFile(path)
		if err == nil {
			if temp, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				return float64(temp) / 1000.0
			}
		}
	}
	return 0
}

// ------------------------------------------------------------
//
//	物理安装内存（SMBIOS type 17）
//
// ------------------------------------------------------------
//
// 为什么不能只看 /proc/meminfo：MemTotal 是「内核能用的内存」，核显 stolen
// 显存与固件/ACPI 预留已经被扣掉。R1（16 GB DDR5）实测 MemTotal=15.4 GiB，
// 差额 0.6 GiB 里 124 MB 是 Graphics Stolen、其余是固件预留 —— 不是缩水。
//
// /sys/firmware/dmi/entries/17-*/raw 是裸 SMBIOS 结构，不需要装 dmidecode。
// 注意：SMBIOS 规范里的字段偏移是**从结构首字节算起**（type/len/handle 占
// 0x00~0x03 也算在内），不是从 formatted area 算 —— 这点搞错会读到
// 0x0201=513 MB 这种鬼数字（本机第一版就是踩了这个坑，界面显示「物理 1 GB」）：
//
//	0x00  type(=17)   0x01  length   0x02~0x03  handle
//	0x0C  Size uint16（MB；0=空插槽；0xFFFF=见扩展字段）
//	0x1C  Extended Size uint32（MB）
//
// 读不到（非 x86 / 权限不足 / SMBIOS 未导出）返回 0，界面自行降级，绝不编造。
var (
	physMemOnce sync.Once
	physMemVal  float64
)

func physicalMemGB() float64 {
	physMemOnce.Do(func() { physMemVal = readDMIPhysicalMemGB() })
	return physMemVal
}

func readDMIPhysicalMemGB() float64 {
	dirs, err := filepath.Glob("/sys/firmware/dmi/entries/17-*")
	if err != nil {
		return 0
	}
	var totalMB uint64
	for _, dir := range dirs {
		b, err := os.ReadFile(filepath.Join(dir, "raw"))
		if err != nil || len(b) < 0x0E {
			continue
		}
		size := binary.LittleEndian.Uint16(b[0x0C:])
		var mb uint64
		switch {
		case size == 0x0000: // 该插槽没插内存
			continue
		case size == 0xFFFF: // 超出 32 GB 上限，走扩展字段
			if len(b) < 0x20 {
				continue
			}
			mb = uint64(binary.LittleEndian.Uint32(b[0x1C:]))
		default:
			mb = uint64(size)
		}
		totalMB += mb
	}
	if totalMB == 0 {
		return 0
	}
	return float64(totalMB) / 1024
}

func (s *SystemMonitor) UpdateMemory() error {
	v, err := mem.VirtualMemory()
	if err != nil {
		return err
	}
	swap, swapErr := mem.SwapMemory()
	phys := physicalMemGB()

	total := float64(v.Total)
	avail := float64(v.Available)
	// 用「总 - 可用」作为已用，保证 已用 + 可用 == 总容量
	used := total - avail
	if used < 0 {
		used = 0
	}
	pct := 0.0
	if total > 0 {
		pct = used / total * 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.MemTotalGB = total / (1024 * 1024 * 1024)
	s.MemPhysGB = phys
	s.MemAvailGB = avail / (1024 * 1024 * 1024)
	s.MemUsedGB = used / (1024 * 1024 * 1024)
	s.MemPercent = pct

	s.MemHistory = append(s.MemHistory, pct)
	if len(s.MemHistory) > 30 {
		s.MemHistory = s.MemHistory[1:]
	}

	// 缓存 = `free` 的 buff/cache 列。
	//
	// 别再"顺手"加 Sreclaimable：gopsutil 在 mem_linux.go 末尾有
	// `ret.Cached += ret.Sreclaimable`（为了对齐 free 的输出），这里再加一次
	// 就会重复计入 —— 实测会多报 2.0 GB（9.99 GiB vs free 的 8.02 GiB）。
	// 当前写法与 `free -m` 的 buff/cache 分毫不差。
	s.MemCacheGB = float64(v.Buffers+v.Cached) / (1024 * 1024 * 1024)

	if swapErr == nil {
		s.SwapPercent = swap.UsedPercent
		s.SwapUsedGB = float64(swap.Used) / (1024 * 1024 * 1024)
		s.SwapTotalGB = float64(swap.Total) / (1024 * 1024 * 1024)
	}
	s.updatedAt = time.Now()
	return nil
}

// UpdateNetwork 只统计主网口（默认路由所在接口）的收发速率。
//
// 为什么不聚合所有接口：R1 上 enp2s0 被桥接进 ovs 桥（enp2s0-ovs），
// 同一份流量会同时记在物理口和桥上；再加上 docker0 / br-* 的容器流量，
// 聚合后速率会明显高于真实外网速率。
func (s *SystemMonitor) UpdateNetwork() error {
	// 两次 sysfs 读 + 一次 netlink 查询，合计 <2ms，且 2 秒才有一次。
	// 这里整段持写锁，不拆「锁外采集」—— 拆了反而要额外同步 lastRx/lastTx。
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.NetIface == "" {
		s.NetIface = defaultRouteIface()
		if s.NetIface == "" {
			return nil // 拿不到主网口就不编造数据
		}
		s.LocalIP = primaryIPv4(s.NetIface)
	}

	rx, tx, err := readIfaceCounters(s.NetIface)
	if err != nil {
		return err
	}

	// 链路速率：只在不报值时保留上一次读数（链路 down 时 sysfs 返回 EINVAL）。
	if v := ifaceSpeedMbps(s.NetIface); v > 0 {
		s.NetLinkMbps = v
	}

	// 当日累计（跨天自动归零，服务重启沿用当日基线）
	s.trackDay(rx, tx)

	now := time.Now()
	if s.lastNetTime.IsZero() {
		s.lastNetTime, s.lastRx, s.lastTx = now, rx, tx
		return nil
	}

	if elapsed := now.Sub(s.lastNetTime).Seconds(); elapsed > 0 {
		if rx >= s.lastRx {
			s.NetRecvMBps = float64(rx-s.lastRx) / elapsed / (1024 * 1024)
		} else {
			s.NetRecvMBps = 0 // 计数器回绕/接口重置，不显示负数
		}
		if tx >= s.lastTx {
			s.NetSendMBps = float64(tx-s.lastTx) / elapsed / (1024 * 1024)
		} else {
			s.NetSendMBps = 0
		}
		s.NetHistory = append(s.NetHistory, [2]float64{s.NetSendMBps, s.NetRecvMBps})
		if len(s.NetHistory) > 30 {
			s.NetHistory = s.NetHistory[1:]
		}
	}

	s.lastNetTime, s.lastRx, s.lastTx = now, rx, tx
	s.NetRxTotal, s.NetTxTotal = rx, tx
	s.updatedAt = time.Now()
	return nil
}

// ------------------------------------------------------------
//  当日流量统计
// ------------------------------------------------------------
//
//  sysfs 的 rx_bytes / tx_bytes 是「接口自开机以来」的累计值，要得到「今日」
//  必须自己维护一条起算基线。规则：
//   - 跨天（日期变了）→ 基线重置为当前计数，并落盘
//   - 服务在本日内重启 → 读回落盘的基线继续累加（不会把当天已产生的流量丢掉）
//   - 计数器回绕 / 接口 reset（当前值 < 基线）→ 基线重置
//  基线落盘失败只影响重启后的精度，不影响当前显示。

type netDayState struct {
	Date string `json:"date"`
	Rx   uint64 `json:"rx"`
	Tx   uint64 `json:"tx"`
}

const netDayFile = "/var/lib/r1-toolbox/net-day.json"

func (s *SystemMonitor) trackDay(rx, tx uint64) {
	today := time.Now().Format("2006-01-02")

	switch {
	case s.day == nil:
		// 首次采样：优先沿用当天已落盘的基线（服务重启场景）
		if st, ok := loadNetDay(); ok && st.Date == today && rx >= st.Rx && tx >= st.Tx {
			s.day = &st
		} else {
			s.day = &netDayState{Date: today, Rx: rx, Tx: tx}
			saveNetDay(*s.day)
		}
	case s.day.Date != today || rx < s.day.Rx || tx < s.day.Tx:
		// 跨天 或 计数器重置
		s.day = &netDayState{Date: today, Rx: rx, Tx: tx}
		saveNetDay(*s.day)
	}

	s.NetRxToday = rx - s.day.Rx
	s.NetTxToday = tx - s.day.Tx
	s.NetRxTotal = rx
	s.NetTxTotal = tx
}

func loadNetDay() (netDayState, bool) {
	var st netDayState
	b, err := os.ReadFile(netDayFile)
	if err != nil {
		return st, false
	}
	if json.Unmarshal(b, &st) != nil || st.Date == "" {
		return netDayState{}, false
	}
	return st, true
}

func saveNetDay(st netDayState) {
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(netDayFile), 0755) != nil {
		return
	}
	os.WriteFile(netDayFile, b, 0644)
}

// defaultRouteIface 解析 /proc/net/route 取默认路由接口（Destination == 00000000）
func defaultRouteIface() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if f[1] == "00000000" {
			return f[0]
		}
	}
	return ""
}

// primaryIPv4 取指定网口的第一个 IPv4 地址
func primaryIPv4(iface string) string {
	ifi, err := netLib.InterfaceByName(iface)
	if err != nil {
		return ""
	}
	addrs, err := ifi.Addrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipn, ok := a.(*netLib.IPNet); ok {
			if ip4 := ipn.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return ""
}

// readIfaceCounters 直接读 sysfs 的接口字节计数（比 gopsutil 聚合更可控）
func readIfaceCounters(iface string) (rx, tx uint64, err error) {
	read := func(name string) uint64 {
		b, e := os.ReadFile(filepath.Join("/sys/class/net", iface, "statistics", name))
		if e != nil {
			return 0
		}
		v, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
		return v
	}
	rx = read("rx_bytes")
	tx = read("tx_bytes")
	if rx == 0 && tx == 0 {
		return 0, 0, fmt.Errorf("读取 %s 收发计数失败", iface)
	}
	return rx, tx, nil
}

// ifaceSpeedMbps 主网口协商速率（Mbit/s）。
//
// 虚拟接口（ovs 桥、veth、docker0）不报 speed，链路 down 时内核返回 EINVAL，
// 两种情况都返回 0 —— 界面上就是"一根都没亮"，而不是编一个看起来正常的数字。
// ifaceSpeedMbps 网口协商速率（Mbit/s），0 = 读不到。
//
// 直接读 /sys/class/net/<if>/speed 对普通网口够用，但**对 OVS 网桥无效**：
// 桥接口本身没有物理层，speed 文件存在却读不出值（read 返回 EINVAL）。
// 而 R1 的默认路由恰恰指向桥接口 enp2s0-ovs，真正协商速率在它的物理
// 端口 enp2s0 上（1000）—— 只读桥口的结果是信号条整排不亮，屏幕上
// 看起来像"网线没插"，这是比不显示更糟的错报。
//
// 回退：扫一遍所有**物理**网口（有 device 链接才是真硬件；veth / br-* /
// docker0 这些虚拟口 speed 恒报 10000，会把结论带偏），取 state=up 且
// 速率最大的那个。全程只读 sysfs，2 秒一次，开销在微秒级。
func ifaceSpeedMbps(iface string) int {
	if v := readSpeedFile(iface); v > 0 {
		return v
	}
	ents, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return 0
	}
	best := 0
	for _, e := range ents {
		name := e.Name()
		if name == iface {
			continue
		}
		dir := filepath.Join("/sys/class/net", name)
		if _, err := os.Stat(filepath.Join(dir, "device")); err != nil {
			continue // 虚拟口，跳过
		}
		if b, err := os.ReadFile(filepath.Join(dir, "operstate")); err != nil ||
			strings.TrimSpace(string(b)) != "up" {
			continue
		}
		if v := readSpeedFile(name); v > best {
			best = v
		}
	}
	return best
}

// readSpeedFile 读单个网口的 speed 文件；读不到或非正值一律返回 0。
func readSpeedFile(iface string) int {
	b, err := os.ReadFile(filepath.Join("/sys/class/net", iface, "speed"))
	if err != nil {
		return 0
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func (s *SystemMonitor) UpdateDisk() error {
	return s.Disk.Update()
}