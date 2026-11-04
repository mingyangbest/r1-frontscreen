package hardware

// 风扇控制器的测试重点。
//
// 运行期设置（模式/曲线）与调速循环并发访问不产生数据竞争（-race 实锤
// 过的 7 处 DATA RACE 里两处在这条路径上）；CalculateSpeed 的边界行为 ——
// 配置损坏时不能 NaN/越界。
//
// 测试不碰真实 hwmon：SetCPUSpeed 会写文件失败并记日志，属于预期噪声。
//
// 注：这段原本写成「//  1. …」的缩进编号列表，gofmt 1.22 会把它当文档
// 列表重排成 tab 缩进，而设备上的 gofmt 1.19 不认 —— 两边判定不一致，
// CI 的格式门就会卡。改成无缩进的普通段落，任何版本都不再改写。

import (
	"sync"
	"testing"
	"time"
)

// TestFanConcurrentSettings 模拟「Web 面板改设置 + 调速循环每秒读快照」
// 的并发形态。用 go test -race 跑，任何无锁访问都会炸。
func TestFanConcurrentSettings(t *testing.T) {
	f := &FanController{
		cpuMode:       "auto",
		hddMode:       "auto",
		cpuMinTemp:    35,
		cpuMaxTemp:    75,
		hddMinTemp:    40,
		hddMaxTemp:    60,
		cpuTempSensor: "coretemp:Package id 0",
		hddTempSensor: "it8628:temp2",
		lastCPUSpeed:  -1,
		lastHDDSpeed:  -1,
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				f.SetCPUMode("auto")
				f.SetHDDMode("low")
				f.SetCPUTempRange(30+n, 80)
				f.SetHDDTempRange(35, 65)
				_ = f.GetCPUMode()
				_ = f.GetHDDMode()
				_, _ = f.CPUTempRange()
				_, _ = f.HDDTempRange()
			}
		}(i)
	}
	// 模拟 Run() 的读快照路径（只取快照，不写硬件）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 200; j++ {
			f.mu.RLock()
			_, _, _, _ = f.cpuMode, f.hddMode, f.cpuMinTemp, f.hddMaxTemp
			f.mu.RUnlock()
		}
	}()
	wg.Wait()
}

func TestCalculateSpeedBoundaries(t *testing.T) {
	f := &FanController{}

	cases := []struct {
		name                   string
		temp                   float64
		minT, maxT, minS, maxS int
		want                   int
	}{
		{"低于下限取最小", 30, 40, 60, 50, 255, 50},
		{"高于上限取最大", 70, 40, 60, 50, 255, 255},
		{"中点取中值", 50, 40, 60, 50, 255, 152},
		{"配置损坏 max<=min 满速保命", 50, 60, 40, 50, 255, 255},
		{"最低不低于25", 10, 40, 60, 10, 255, 25},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := f.CalculateSpeed(c.temp, c.minT, c.maxT, c.minS, c.maxS)
			if got != c.want {
				t.Fatalf("CalculateSpeed(%v, %d, %d, %d, %d) = %d, want %d",
					c.temp, c.minT, c.maxT, c.minS, c.maxS, got, c.want)
			}
		})
	}
}

func TestFixedSpeed(t *testing.T) {
	if fixedSpeed("low") != 85 {
		t.Fatal("low 应为 85")
	}
	if fixedSpeed("high") != 255 || fixedSpeed("full") != 255 {
		t.Fatal("high/full 应为 255")
	}
	if fixedSpeed("auto") != 170 || fixedSpeed("unknown") != 170 {
		t.Fatal("未知模式按中速 170，不应为 0（风扇停转会烧盘）")
	}
}

// TestGetTempNoFabrication 传感器不存在时 GetTemp 必须如实报告失败，
// 不能返回占位值冒充读数（界面上编 30°C 事故的回归测试）。
func TestGetTempNoFabrication(t *testing.T) {
	f := &FanController{} // 没有任何传感器路径
	if _, ok := f.GetTemp("coretemp:Package id 0"); ok {
		t.Fatal("无传感器路径时 GetTemp 不应成功")
	}
	if _, ok := f.GetTemp("it8628:temp1"); ok {
		t.Fatal("无传感器路径时 GetTemp 不应成功")
	}
	if v := f.TempForControl("coretemp:Package id 0"); v != tempFallback {
		t.Fatalf("调速兜底值应为 %.0f，got %v", tempFallback, v)
	}
}

// TestTempStaleFallback 温度源失联的三段语义：
// 15 秒内沿用上次有效读数，超过才判失联，且失联分支不覆盖上次读数。
func TestTempStaleFallback(t *testing.T) {
	f := &FanController{}
	const sensor = "nope:temp1"

	// 从未读到过 → 直接失联
	if v, stale := f.tempForControl(sensor); !stale || v != tempFallback {
		t.Fatalf("从未读到过应判失联，got (%v, %v)", v, stale)
	}

	// 5 秒前有有效读数 → 沿用，不算失联
	f.lastTemp = map[string]lastReading{
		sensor: {temp: 62, at: time.Now().Add(-5 * time.Second)},
	}
	if v, stale := f.tempForControl(sensor); stale || v != 62 {
		t.Fatalf("15 秒内应沿用旧值 62，got (%v, %v)", v, stale)
	}

	// 20 秒前 → 失联，返回兜底值
	f.lastTemp[sensor] = lastReading{temp: 62, at: time.Now().Add(-20 * time.Second)}
	if v, stale := f.tempForControl(sensor); !stale || v != tempFallback {
		t.Fatalf("超过 15 秒应判失联，got (%v, %v)", v, stale)
	}

	// 关键：失联分支不能回写兜底值，否则下一轮抖动会从 30°C 起算
	if got := f.lastTemp[sensor].temp; got != 62 {
		t.Fatalf("失联不应改写上次有效读数，got %v", got)
	}
}

// TestTempSourcesIndependent CPU 与机箱各存各的最后读数，不能共用一份。
// 共用会让「机箱掉线」把 CPU 的正常读数覆盖掉，反之亦然。
func TestTempSourcesIndependent(t *testing.T) {
	f := &FanController{lastTemp: map[string]lastReading{
		"cpu:temp1": {temp: 55, at: time.Now()},
		"hdd:temp1": {temp: 38, at: time.Now().Add(-30 * time.Second)},
	}}

	if v, stale := f.tempForControl("cpu:temp1"); stale || v != 55 {
		t.Fatalf("CPU 应沿用 55，got (%v, %v)", v, stale)
	}
	if _, stale := f.tempForControl("hdd:temp1"); !stale {
		t.Fatal("机箱读数已过期 30 秒，应判失联")
	}
}

// TestStaleSpeedFloor 失联时抬到中高档，而不是停在曲线最低点。
// 兜底 30°C 对 35–75 的 CPU 曲线算出来只有 50/255，等于「默认机器是凉的」。
func TestStaleSpeedFloor(t *testing.T) {
	f := &FanController{}
	base := f.CalculateSpeed(tempFallback, 35, 75, 50, 255)
	if base != 50 {
		t.Fatalf("兜底温度在曲线最低点应为 50，got %d", base)
	}
	if got := max(base, tempStaleSpeed); got != tempStaleSpeed {
		t.Fatalf("失联应抬到 %d，got %d", tempStaleSpeed, got)
	}
	if tempStaleSpeed >= 255 {
		t.Fatal("失联下限不应等于满转：满了就分不清「故障升档」和「真的过热」")
	}
}
