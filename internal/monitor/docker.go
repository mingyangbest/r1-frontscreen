package monitor

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"r1-toolbox/internal/logx"
)

// 容器运行状态（Material You 三页版新增）。
//
// 读 Docker 的 unix socket（/var/run/docker.sock）数容器：
//   - 只读不写，一次 GET /containers/json?all=1
//   - 读不到（socket 不存在 / 无权限 / 非容器环境）时置 -1，
//     屏幕上显示「—」而不是编一个数字 —— 数据真实性约定同样适用
//   - 由后台采样协程每 60 秒调一次；HTTP 在**锁外**完成，最长 3 秒的超时
//     不会挡住渲染取快照（旧实现跑在渲染线程里，手指正好划过就会顿一下）

const dockerSock = "/var/run/docker.sock"

var dockerHTTPClient = &http.Client{
	Timeout: 3 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			// 地址写死 unix socket：请求 URL 的 host 只是占位
			var d net.Dialer
			return d.DialContext(ctx, "unix", dockerSock)
		},
	},
}

// UpdateContainers 刷新 Docker 容器计数。失败时置 -1（未知），不回退旧值。
func (s *SystemMonitor) UpdateContainers() {
	run, total := fetchContainerCounts()

	s.mu.Lock()
	s.ContainersRunning, s.ContainersTotal = run, total
	s.mu.Unlock()
}

// fetchContainerCounts 返回 (-1,-1) 表示未知
func fetchContainerCounts() (int, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://docker/containers/json?all=1", nil)
	if err != nil {
		return -1, -1
	}
	resp, err := dockerHTTPClient.Do(req)
	if err != nil {
		logx.Debugf("[docker] 读取容器列表失败: %v", err)
		return -1, -1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logx.Debugf("[docker] 容器列表返回 %d", resp.StatusCode)
		return -1, -1
	}

	type ctrInfo struct {
		State string `json:"State"`
	}
	var ctrs []ctrInfo
	if json.NewDecoder(resp.Body).Decode(&ctrs) != nil {
		return -1, -1
	}

	running := 0
	for _, c := range ctrs {
		if c.State == "running" {
			running++
		}
	}
	return running, len(ctrs)
}
