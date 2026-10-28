package hardware

// 风扇控制器的测试重点：
//  1. 运行期设置（模式/曲线）与调速循环并发访问不产生数据竞争
//     （-race 实锤过的 7 处 DATA RACE 里两处在这条路径上）
//  2. CalculateSpeed 的边界行为 —— 配置损坏时不能 NaN/越界
//
// 测试不碰真实 hwmon：SetCPUSpeed 会写文件失败并记日志，属于预期噪声。

import (
	"sync"
	"testing"
)

// TestFanConcurrentSettings 模拟「Web 面板改设置 + 调速循环每秒读快照」
// 的并发形态。用 go test -race 跑，任何无锁访问都会炸。
func TestFanConcurrentSettings(t *testing.T) {
	f := &FanController{
		cpuMode:      "auto",
		hddMode:      "auto",
		cpuMinTemp:   35,
		cpuMaxTemp:   75,
		hddMinTemp:   40,
		hddMaxTemp:   60,
		cpuTempSensor: "coretemp:Package id 0",
		hddTempSensor: "it8628:temp2",
		lastCPUSpeed: -1,
		lastHDDSpeed: -1,
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
		name                      string
		temp                      float64
		minT, maxT, minS, maxS    int
		want                      int
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
