package monitor

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"
)

// 容器运行状态（Material You 三页版新增）。
//
// 读 Docker 的 unix socket（/var/run/docker.sock）数容器：
//   - 只读不写，一次 GET /containers/json?all=1
//   - 读不到（socket 不存在 / 无权限 / 非容器环境）时保持 -1，
//     屏幕上显示「—」而不是编一个数字 —— 数据真实性约定同样适用
//   - 由主采样循环每 60 秒调一次，不占用 2 秒数据帧的预算

var dockerHTTPClient = &http.Client{
	Timeout: 3 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			// 地址写死 unix socket：请求 URL 的 host 只是占位
			return net.Dial("unix", "/var/run/docker.sock")
		},
	},
}

// UpdateContainers 刷新 Docker 容器计数。失败时置 -1（未知），不回退旧值。
func (s *SystemMonitor) UpdateContainers() {
	s.ContainersRunning, s.ContainersTotal = -1, -1

	req, err := http.NewRequestWithContext(context.Background(),
		"GET", "http://docker/containers/json?all=1", nil)
	if err != nil {
		return
	}
	resp, err := dockerHTTPClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	type ctrInfo struct {
		State string `json:"State"`
	}
	var ctrs []ctrInfo
	if json.NewDecoder(resp.Body).Decode(&ctrs) != nil {
		return
	}

	running := 0
	for _, c := range ctrs {
		if c.State == "running" {
			running++
		}
	}
	s.ContainersRunning = running
	s.ContainersTotal = len(ctrs)
}
