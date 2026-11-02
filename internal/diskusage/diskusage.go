// Package diskusage 磁盘占用分析（路线表 P6）。
//
// 目标：那块 88~89% 的盘，屏上只给百分比，面板要回答「到底是谁占的」。
//
// 实现就是 du + find 两个系统命令，不手写遍历：
//   - du 自动处理硬链接去重（NAS 的媒体库常见硬链接，手写 Walk 很容易算双份）
//   - -x 不跨文件系统（别把 /vol1 的算进 TOSHIBA）
//
// 耗时实测（本机 932G 机械盘、824G 已用）：du 一级目录 4.6 秒。
// 所以同步执行即可，不做异步任务队列 —— 但必须有超时，扫到卡住的
// 网络盘或坏道不能把请求线程挂死。
package diskusage

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"r1-toolbox/internal/execx"
	"r1-toolbox/internal/logx"
)

const (
	dirTimeout  = 90 * time.Second
	fileTimeout = 120 * time.Second
	bigFileMin  = 500 << 20 // 500 MiB，媒体库场景 500M 以下的文件不值得看
	topDirs     = 8
	topFiles    = 10
)

type Dir struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

type File struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

type Result struct {
	Path       string `json:"path"`
	TotalBytes int64  `json:"total_bytes"`
	Dirs       []Dir  `json:"dirs"`
	Files      []File `json:"files"`
	TookMS     int64  `json:"took_ms"`
	Truncated  bool   `json:"truncated"` // 有命令超时/失败，结果是部分的
	Note       string `json:"note,omitempty"`
}

// Scan 扫 root 的一级目录占用与大文件。root 必须由调用方校验过
// （只允许已挂载卷的挂载点，绝不接受任意路径 —— 命令参数注入的入口）。
func Scan(root string) Result {
	t0 := time.Now()
	res := Result{Path: root}

	// ---- 一级目录 ----
	out, err := execx.Run(dirTimeout, "du", "-x", "-B1", "--max-depth=1", root)
	if err != nil {
		res.Truncated = true
		res.Note = "目录统计未完成（超时或权限不足），以下为已扫到的部分"
		logx.Printf("【磁盘分析】du %s: %v", root, err)
	}
	res.Dirs, res.TotalBytes = parseDirs(string(out), root)

	// ---- 大文件 ----
	out2, err2 := execx.Run(fileTimeout, "find", root, "-xdev", "-type", "f",
		"-size", "+500M", "-printf", "%s\t%p\n")
	if err2 != nil {
		res.Truncated = true
		if res.Note == "" {
			res.Note = "大文件扫描未完成，以下为已扫到的部分"
		}
		logx.Printf("【磁盘分析】find %s: %v", root, err2)
	}
	res.Files = parseFiles(string(out2))

	res.TookMS = time.Since(t0).Milliseconds()
	return res
}

// parseDirs 解析 du 输出。每行 "size\tpath"；第一行（或等于 root 的那行）
// 是总量，单独记到 TotalBytes 而不是当作一个目录。
func parseDirs(out, root string) ([]Dir, int64) {
	var total int64
	dirs := make([]Dir, 0, 16)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		tab := strings.IndexByte(line, '\t')
		if tab <= 0 {
			continue
		}
		n, err := strconv.ParseInt(line[:tab], 10, 64)
		if err != nil {
			continue
		}
		path := line[tab+1:]
		if path == root {
			total = n
			continue
		}
		name := path
		if i := strings.LastIndexByte(path, '/'); i >= 0 && i+1 < len(path) {
			name = path[i+1:]
		}
		dirs = append(dirs, Dir{Name: name, Bytes: n})
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Bytes > dirs[j].Bytes })
	if len(dirs) > topDirs {
		dirs = dirs[:topDirs]
	}
	if total == 0 {
		// du 没给出 root 那行（比如超时截断）：用一级目录之和兜底
		for _, d := range dirs {
			total += d.Bytes
		}
	}
	return dirs, total
}

func parseFiles(out string) []File {
	files := make([]File, 0, 16)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		tab := strings.IndexByte(line, '\t')
		if tab <= 0 {
			continue
		}
		n, err := strconv.ParseInt(line[:tab], 10, 64)
		if err != nil || n < bigFileMin {
			continue
		}
		files = append(files, File{Path: line[tab+1:], Bytes: n})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Bytes > files[j].Bytes })
	if len(files) > topFiles {
		files = files[:topFiles]
	}
	return files
}
