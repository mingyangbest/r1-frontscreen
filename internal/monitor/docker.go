package monitor

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strings"
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

// UpdateContainers 刷新 Docker 容器计数与运行中的容器名。
// 失败时置 -1（未知），不回退旧值。
func (s *SystemMonitor) UpdateContainers() {
	run, total, names := fetchContainers()

	s.mu.Lock()
	s.ContainersRunning, s.ContainersTotal = run, total
	s.Containers = names
	s.mu.Unlock()
}

// fetchContainers 返回 (-1,-1,nil) 表示未知
func fetchContainers() (int, int, []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://docker/containers/json?all=1", nil)
	if err != nil {
		return -1, -1, nil
	}
	resp, err := dockerHTTPClient.Do(req)
	if err != nil {
		logx.Debugf("[docker] 读取容器列表失败: %v", err)
		return -1, -1, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logx.Debugf("[docker] 容器列表返回 %d", resp.StatusCode)
		return -1, -1, nil
	}

	// Names 是 ["/容器名"] 形式（带前导斜杠），State 是 running/exited/created…
	type ctrInfo struct {
		Names []string `json:"Names"`
		State string   `json:"State"`
	}
	var ctrs []ctrInfo
	if json.NewDecoder(resp.Body).Decode(&ctrs) != nil {
		return -1, -1, nil
	}

	running := 0
	names := make([]string, 0, len(ctrs))
	for _, c := range ctrs {
		if c.State != "running" {
			continue
		}
		running++
		if len(c.Names) > 0 {
			names = append(names, strings.TrimPrefix(c.Names[0], "/"))
		}
	}
	// 名字排序后再上屏：Docker API 的顺序不保证稳定，屏幕每 60 秒跳一次
	// 顺序会让人以为容器在重启。
	sort.Strings(names)
	return running, len(ctrs), names
}
