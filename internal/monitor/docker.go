package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"r1-toolbox/internal/logx"
)

// Docker 概览（2026-11-01 洋哥：系统页要一张 Docker 卡，不是只列容器名）。
//
// 读 Docker 的 unix socket（/var/run/docker.sock），两次只读请求：
//   - GET /containers/json?all=1  容器列表（State、Names、compose 项目标签）
//   - GET /images/json            镜像列表（Containers 字段 = 被几个容器引用）
//
// 读不到（socket 不存在 / 无权限 / 非容器环境）时 Docker 置 nil，
// 屏幕显示「未接入 Docker」而不是编一组 0 —— 数据真实性约定同样适用。
// 由后台采样协程每 60 秒调一次；HTTP 在**锁外**完成，最长 3 秒的超时
// 不会挡住渲染取快照。

const dockerSock = "/var/run/docker.sock"

// DockerStats 一屏 Docker 概览。三个维度各自「总数 = 运行/用中 + 停止/闲置」，
// 不做含糊的减法 —— 屏上的数字必须能对上。
type DockerStats struct {
	ContainersTotal   int
	ContainersRunning int
	ContainersExited  int // 非 running 的全部算退出（exited/created/paused）

	// ContainersAbnormal 真正出了问题、需要人看一眼的容器数。
	// 「停止」不等于「异常」：docker compose stop 是用户主动行为，ExitCode 0，
	// 不该报红。判定见 containerAbnormal。
	ContainersAbnormal int

	ImagesTotal  int
	ImagesUsed   int
	ImagesUnused int

	ProjectsTotal   int // 带 com.docker.compose.project 标签的不同项目数
	ProjectsRunning int // 至少有一个容器在运行的项目
	ProjectsStopped int
}

// Healthy 全部容器都没有异常。注意判据是「异常数」而不是「运行数」——
// 停掉的项目不影响健康度，这与飞牛/Docker Desktop 的口径一致。
func (d *DockerStats) Healthy() bool {
	return d != nil && d.ContainersAbnormal == 0
}

// exitedRe 从容器 Status 串里取退出码："Exited (137) 2 days ago" → 137
var exitedRe = regexp.MustCompile(`Exited \((\d+)\)`)

// containerAbnormal 判断一个容器是否「需要人看一眼」。
//
// 2026-11-01 洋哥定的口径：有项目/容器停着，只要不是出错，就算健康。
// 所以这里只看三件事：
//
//	① State 是 restarting / dead —— 在重启循环里或已经死掉；
//	② Status 带 (unhealthy) —— healthcheck 失败；
//	③ 退出码既不是 0（正常结束）也不是 143（SIGTERM，docker stop 的标准信号）。
//	   137 是 SIGKILL（常见于 OOM 被杀）、1 是应用自己报错退出，这些都要报。
//
// paused / created 不算异常：前者是用户主动暂停，后者是建好还没启动。
func containerAbnormal(state, status string) bool {
	switch state {
	case "restarting", "dead":
		return true
	}
	if strings.Contains(status, "(unhealthy)") {
		return true
	}
	if m := exitedRe.FindStringSubmatch(status); m != nil {
		code, err := strconv.Atoi(m[1])
		if err == nil {
			return code != 0 && code != 143
		}
	}
	return false
}

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

// UpdateContainers 刷新 Docker 概览。失败时 Docker=nil、计数=-1（未知），
// 不回退旧值 —— 旧值会让「服务挂了」看起来像「一切正常」。
func (s *SystemMonitor) UpdateContainers() {
	st := fetchDockerStats()

	s.mu.Lock()
	s.Docker = st
	if st != nil {
		s.ContainersRunning, s.ContainersTotal = st.ContainersRunning, st.ContainersTotal
	} else {
		s.ContainersRunning, s.ContainersTotal = -1, -1
	}
	s.mu.Unlock()
}

func dockerGet(path string, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := dockerHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker API 返回 %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// fetchDockerStats 聚合容器/镜像/项目三组计数；任一步失败返回 nil。
func fetchDockerStats() *DockerStats {
	type ctrInfo struct {
		Names  []string          `json:"Names"`
		State  string            `json:"State"`
		Status string            `json:"Status"` // "Up 3 days (healthy)" / "Exited (137) 2 days ago"
		Labels map[string]string `json:"Labels"`
	}
	var ctrs []ctrInfo
	if err := dockerGet("/containers/json?all=1", &ctrs); err != nil {
		logx.Debugf("[docker] 读取容器列表失败: %v", err)
		return nil
	}

	type imgInfo struct {
		Containers int `json:"Containers"` // 被几个容器使用；-1 = Docker 自己也不知道
	}
	var imgs []imgInfo
	if err := dockerGet("/images/json", &imgs); err != nil {
		logx.Debugf("[docker] 读取镜像列表失败: %v", err)
		return nil
	}

	st := &DockerStats{ContainersTotal: len(ctrs), ImagesTotal: len(imgs)}

	// 项目：按 com.docker.compose.project 去重；「运行中」= 至少一个容器 running。
	// map 的值就是"该项目是否有运行中的容器"，一次遍历同时得出两个计数。
	projects := make(map[string]bool)
	for _, c := range ctrs {
		if c.State == "running" {
			st.ContainersRunning++
		} else {
			st.ContainersExited++
		}
		if containerAbnormal(c.State, c.Status) {
			st.ContainersAbnormal++
		}
		if p := c.Labels["com.docker.compose.project"]; p != "" {
			if _, seen := projects[p]; !seen {
				projects[p] = false
			}
			if c.State == "running" {
				projects[p] = true
			}
		}
	}
	st.ProjectsTotal = len(projects)
	for _, running := range projects {
		if running {
			st.ProjectsRunning++
		}
	}
	st.ProjectsStopped = st.ProjectsTotal - st.ProjectsRunning

	// 镜像：Containers>0 即被引用。Containers==0 是「Docker 明确说没人用」，
	// 才算闲置；-1 表示 Docker 自己不知道，归到「用中」（保守，不虚报可清理量）。
	for _, im := range imgs {
		if im.Containers == 0 {
			st.ImagesUnused++
		} else {
			st.ImagesUsed++
		}
	}

	return st
}
