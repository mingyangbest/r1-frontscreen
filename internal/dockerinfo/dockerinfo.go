// Package dockerinfo Docker 只读查询（路线表 P5）。
//
// 与 monitor/docker.go 的分工：那边只数个数（屏幕上的 Docker 卡），
// 这边给面板提供**列表与日志**。两者都用 unix socket 直连，
// 但不共享 client —— monitor 在采样协程里、本包在 web 协程里，
// 各持各的连接池，互不阻塞。
//
// **只读**：只调 GET 两类接口（列表 / 日志），没有任何启停写操作。
// 这是与洋哥定的边界：面板围着这块屏转，Docker 编排交给 Portainer。
package dockerinfo

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const sock = "/var/run/docker.sock"

// port 容器端口映射（Docker API 的 Ports 元素）。
type port struct {
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

// Container 面板用的容器信息（字段都来自 /containers/json）。
type Container struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`   // running / exited / created / paused / restarting
	Status  string `json:"status"`  // 人话状态，如 "Up 2 hours" / "Exited (0) 3 days ago"
	Project string `json:"project"` // compose 项目（纯 docker run 的为空）
	Ports   string `json:"ports"`   // 精简端口，如 "5433→8000"
	Health  string `json:"health"`  // healthy / unhealthy / ""（无 healthcheck）
}

// client 每次调用新建一个短连接 client：调用频率低（面板手动刷新），
// 复用连接池的收益抵不上"租约过期/连接泄漏"的排查成本。
func client() *http.Client {
	return &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
	}
}

func get(path string) ([]byte, error) {
	c := client()
	resp, err := c.Get("http://docker" + path)
	if err != nil {
		return nil, fmt.Errorf("连不上 docker.sock: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return b, nil
}

// List 列出全部容器（含已停止），按「运行中优先、其次名称」排序。
func List() ([]Container, error) {
	b, err := get("/containers/json?all=1")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID     string            `json:"Id"`
		Names  []string          `json:"Names"`
		Image  string            `json:"Image"`
		State  string            `json:"State"`
		Status string            `json:"Status"`
		Labels map[string]string `json:"Labels"`
		Ports  []port            `json:"Ports"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("解析容器列表: %w", err)
	}

	out := make([]Container, 0, len(raw))
	for _, r := range raw {
		name := ""
		if len(r.Names) > 0 {
			name = strings.TrimPrefix(r.Names[0], "/")
		}
		out = append(out, Container{
			ID:      r.ID[:min(12, len(r.ID))],
			Name:    name,
			Image:   r.Image,
			State:   r.State,
			Status:  r.Status,
			Project: r.Labels["com.docker.compose.project"],
			Ports:   compactPorts(r.Ports),
		})
	}
	// 运行中优先，同状态按名字 —— 面板列表顺序稳定，不随 API 顺序跳动
	sortContainers(out)
	return out, nil
}

// Logs 取容器最近 tail 行日志（stdout+stderr）。
//
// Docker 日志是 **8 字节帧头 + payload** 的多路复用流（stderr/stdout 分开）。
// 容器开了 TTY 时又是原始流 —— 两种都要能解，所以先探帧头合法性。
func Logs(id string, tail int) (string, error) {
	if tail <= 0 || tail > 2000 {
		tail = 200
	}
	b, err := get(fmt.Sprintf("/containers/%s/logs?stdout=1&stderr=1&timestamps=0&tail=%d", id, tail))
	if err != nil {
		return "", err
	}
	return stripANSI(demux(b)), nil
}

func demux(b []byte) string {
	var out bytes.Buffer
	orig := b
	for len(b) >= 8 {
		typ := b[0]
		if typ != 0 && typ != 1 && typ != 2 {
			return string(orig) // 不是多路复用流（TTY 容器），原样返回
		}
		n := int(binary.BigEndian.Uint32(b[4:8]))
		if n < 0 || 8+n > len(b) {
			return string(orig)
		}
		out.Write(b[8 : 8+n])
		b = b[8+n:]
	}
	if out.Len() == 0 {
		return string(orig)
	}
	return out.String()
}

// ansiRe 终端颜色/样式转义序列。面板是 HTML <pre>，不吃 ANSI ——
// 不去掉的话日志里全是 [33m 这类乱码（实测 AvDB 的 Python 日志满屏）。
var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func compactPorts(ps []port) string {
	// 去重：同一端口对在 API 里会出现两条（IPv4 + IPv6 各一条），
	// 不去重会渲染成「5433→8000 5433→8000」。
	var parts []string
	seen := map[string]bool{}
	for _, p := range ps {
		var v string
		if p.PublicPort > 0 {
			v = fmt.Sprintf("%d→%d", p.PublicPort, p.PrivatePort)
		} else {
			v = fmt.Sprintf("%d", p.PrivatePort)
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		parts = append(parts, v)
	}
	if len(parts) > 3 {
		parts = append(parts[:3], "…")
	}
	return strings.Join(parts, " ")
}

func sortContainers(cs []Container) {
	rank := func(s string) int {
		switch s {
		case "running":
			return 0
		case "restarting":
			return 1
		case "paused":
			return 2
		default:
			return 3
		}
	}
	// 插入排序：30 个量级，比引入 sort 包 + 闭包更直白
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0; j-- {
			a, b := cs[j-1], cs[j]
			if rank(a.State) < rank(b.State) || (rank(a.State) == rank(b.State) && a.Name <= b.Name) {
				break
			}
			cs[j-1], cs[j] = cs[j], cs[j-1]
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
