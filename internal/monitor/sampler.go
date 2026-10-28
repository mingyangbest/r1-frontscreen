package monitor

import (
	"sync/atomic"
	"time"

	"r1-toolbox/internal/logx"
)

// Sampler 把采样从渲染线程里挪出来。
//
// 修复的问题（实测数据）：smartctl 读一块 SATA 盘 359ms、Docker socket
// 最长 3s 超时、CPU 采样 3~8ms。旧实现全跑在渲染主循环里，手指正好在
// 那 0.4~3 秒内划过屏幕，画面就卡住不动 —— 触摸事件也要等这一帧画完
// 才能被处理。
//
// 现在渲染线程每帧只调 Snapshot()（读锁 + 几十个字段拷贝，微秒级），
// 采样在这里按自己的节奏跑，两边通过锁和版本号解耦。
type Sampler struct {
	mon  *SystemMonitor
	ver  atomic.Uint64
	stop chan struct{}
}

// 采样周期
const (
	fastEvery = 2 * time.Second  // CPU / 内存 / 网络：屏幕数字的刷新节奏
	slowEvery = 60 * time.Second // 磁盘 SMART / Docker：物理变化是分钟级
	slowFirst = 5 * time.Second  // 首轮延后，别和启动首帧抢 CPU
)

func NewSampler(mon *SystemMonitor) *Sampler {
	return &Sampler{mon: mon, stop: make(chan struct{})}
}

// Start 启动两条采样协程（非阻塞）
func (s *Sampler) Start() {
	go s.fastLoop()
	go s.slowLoop()
}

// Stop 停止采样（进程退出时调用；不 Stop 也不影响正确性）
func (s *Sampler) Stop() { close(s.stop) }

// Version 数据代数：每完成一轮快速采样自增。
// 主循环比较它就知道「屏幕上的数字要不要重画」，不需要自己判断时间。
func (s *Sampler) Version() uint64 { return s.ver.Load() }

func (s *Sampler) fastLoop() {
	t := time.NewTicker(fastEvery)
	defer t.Stop()

	// 立即采一次：屏幕不用等 2 秒才从 0 跳到真实值
	s.sampleFast()

	for {
		select {
		case <-t.C:
			s.sampleFast()
		case <-s.stop:
			return
		}
	}
}

func (s *Sampler) sampleFast() {
	if err := s.mon.UpdateCPU(); err != nil {
		logx.Debugf("[采样] CPU: %v", err)
	}
	if err := s.mon.UpdateMemory(); err != nil {
		logx.Debugf("[采样] 内存: %v", err)
	}
	if err := s.mon.UpdateNetwork(); err != nil {
		logx.Debugf("[采样] 网络: %v", err)
	}
	s.ver.Add(1)
}

func (s *Sampler) slowLoop() {
	select {
	case <-time.After(slowFirst):
	case <-s.stop:
		return
	}
	s.sampleSlow()

	t := time.NewTicker(slowEvery)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			s.sampleSlow()
		case <-s.stop:
			return
		}
	}
}

func (s *Sampler) sampleSlow() {
	if err := s.mon.UpdateDisk(); err != nil {
		logx.Debugf("[采样] 磁盘: %v", err)
	}
	s.mon.UpdateContainers()
}
