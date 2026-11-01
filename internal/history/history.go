// Package history 24 小时指标历史（路线表 P4）。
//
// 设计口径（2026-11-02 与洋哥对齐）：
//   - SQLite 单文件 + WAL；**每分钟一行**（该分钟内的均值 + 峰值），
//     24 小时只有 1440 行 —— 不是什么"时序数据库"，就是一张够用的小表。
//   - 写入端：Add 每 2 秒被喂一个瞬时样本，在内存里按自然分钟聚合，
//     满一分钟才落一行 INSERT。**不在采样热路径上写库**（磁盘 IO 会拖
//     采样协程，之前 smartctl 卡 359ms 就拖死过触摸的教训）。
//   - 查询端：保留 24 小时（超期 DELETE），查询时降采样到约 240 点
//     —— 面板一个折线图看不出 1440 个点的差别，少传点也快。
//   - 不做：查询语言、导出、区间对比（那是 Grafana 的活，N100 上
//     也跑不动两套重的）。
package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"r1-toolbox/internal/logx"
)

// Retain 保留窗口。只留 24 小时 —— 7d/30d 等真有人天天看再加。
const Retain = 24 * time.Hour

// Path 历史库文件。与告警记录（alerts.LogPath）同目录：都是"程序自己
// 攒的运行时数据"，清一色放 /var/lib 下，不碰 /etc（配置）与 /tmp（会被清）。
const Path = "/var/lib/r1-toolbox/history.db"

// Sample 一分钟聚合行。六个物理量各带峰值：
// CPU/内存/温度/磁盘（最大卷占用%）、网络收/发（MB/s）。
type Sample struct {
	TS       int64   `json:"ts"` // 该分钟的起始 Unix 秒
	CPU      float64 `json:"cpu"`
	CPUMax   float64 `json:"cpu_max"`
	Mem      float64 `json:"mem"`
	MemMax   float64 `json:"mem_max"`
	Temp     float64 `json:"temp"`
	TempMax  float64 `json:"temp_max"`
	Disk     float64 `json:"disk"`
	DiskMax  float64 `json:"disk_max"`
	NetRx    float64 `json:"net_rx"`
	NetRxMax float64 `json:"net_rx_max"`
	NetTx    float64 `json:"net_tx"`
	NetTxMax float64 `json:"net_tx_max"`
}

type Store struct {
	db   *sql.DB
	path string

	mu  sync.Mutex
	cur *minute // 正在累计的这一分钟
}

type minute struct {
	start int64 // Unix 秒（分钟起点）
	n     int
	s     Sample // Sum 在各字段上累计（Max 字段存最大值）
}

// Open 打开（必要时创建）历史库。目录不存在会创建。
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建历史库目录: %w", err)
	}
	dsn := "file:" + path + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开历史库: %w", err)
	}
	// SQLite 单文件：并发写只会互相等，串行化最省心
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("建表: %w", err)
	}
	s := &Store{db: db, path: path}
	if err := s.prune(); err != nil {
		logx.Printf("【历史】清理旧数据失败: %v", err)
	}
	return s, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS samples (
  ts       INTEGER PRIMARY KEY,
  cpu      REAL, cpu_max  REAL,
  mem      REAL, mem_max  REAL,
  temp     REAL, temp_max REAL,
  disk     REAL, disk_max REAL,
  net_rx   REAL, net_rx_max REAL,
  net_tx   REAL, net_tx_max REAL
);`

func (s *Store) Close() error { return s.db.Close() }

// Add 喂一个瞬时样本（2 秒一次）。跨分钟时把上一分钟落库。
//
// 纯内存操作，绝大多数调用只做几次加法 —— 放到采样协程旁边也不碍事。
func (s *Store) Add(v Sample) {
	now := time.Now().Unix()
	start := now - now%60

	s.mu.Lock()
	if s.cur != nil && s.cur.start != start {
		done := s.cur
		s.cur = nil
		s.mu.Unlock()
		s.flush(done) // 落库在锁外：SQLite 写可能慢，别挡住 Add
		s.mu.Lock()
	}
	if s.cur == nil {
		s.cur = &minute{start: start}
	}
	c := s.cur
	c.n++
	c.s.CPU += v.CPU
	c.s.Mem += v.Mem
	c.s.Temp += v.Temp
	c.s.Disk += v.Disk
	c.s.NetRx += v.NetRx
	c.s.NetTx += v.NetTx
	c.s.CPUMax = maxf(c.s.CPUMax, v.CPU)
	c.s.MemMax = maxf(c.s.MemMax, v.Mem)
	c.s.TempMax = maxf(c.s.TempMax, v.Temp)
	c.s.DiskMax = maxf(c.s.DiskMax, v.Disk)
	c.s.NetRxMax = maxf(c.s.NetRxMax, v.NetRx)
	c.s.NetTxMax = maxf(c.s.NetTxMax, v.NetTx)
	s.mu.Unlock()
}

// flush 落一行（均值 + 峰值）。
func (s *Store) flush(m *minute) {
	if m == nil || m.n == 0 {
		return
	}
	n := float64(m.n)
	_, err := s.db.Exec(`INSERT OR REPLACE INTO samples
	  (ts, cpu, cpu_max, mem, mem_max, temp, temp_max, disk, disk_max, net_rx, net_rx_max, net_tx, net_tx_max)
	  VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.start, m.s.CPU/n, m.s.CPUMax, m.s.Mem/n, m.s.MemMax,
		m.s.Temp/n, m.s.TempMax, m.s.Disk/n, m.s.DiskMax,
		m.s.NetRx/n, m.s.NetRxMax, m.s.NetTx/n, m.s.NetTxMax)
	if err != nil {
		logx.Printf("【历史】写入失败: %v", err)
		return
	}
	s.prune()
}

// prune 删掉超过保留窗口的行。每分钟一次，扫的是主键，代价可忽略。
func (s *Store) prune() error {
	cut := time.Now().Add(-Retain).Unix()
	_, err := s.db.Exec(`DELETE FROM samples WHERE ts < ?`, cut)
	return err
}

// Query 取最近 hours 小时，降采样到至多 target 个点。
//
// 合并规则：均值字段取均值、峰值字段取最大值 —— 降采样不能让峰值
// 变小，否则"凌晨三点那个尖峰"会在图上凭空消失。
func (s *Store) Query(hours int, target int) ([]Sample, error) {
	if hours <= 0 {
		hours = 24
	}
	if target <= 0 {
		target = 240
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
	rows, err := s.db.Query(`SELECT ts, cpu, cpu_max, mem, mem_max, temp, temp_max,
	  disk, disk_max, net_rx, net_rx_max, net_tx, net_tx_max
	  FROM samples WHERE ts >= ? ORDER BY ts`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []Sample
	for rows.Next() {
		var v Sample
		if err := rows.Scan(&v.TS, &v.CPU, &v.CPUMax, &v.Mem, &v.MemMax,
			&v.Temp, &v.TempMax, &v.Disk, &v.DiskMax,
			&v.NetRx, &v.NetRxMax, &v.NetTx, &v.NetTxMax); err != nil {
			return nil, err
		}
		all = append(all, v)
	}
	return downsample(all, target), rows.Err()
}

func downsample(in []Sample, target int) []Sample {
	if len(in) == 0 || len(in) <= target {
		return in
	}
	step := (len(in) + target - 1) / target
	out := make([]Sample, 0, target)
	for i := 0; i < len(in); i += step {
		j := i + step
		if j > len(in) {
			j = len(in)
		}
		g := in[i:j]
		var acc Sample
		acc.TS = g[0].TS
		for _, v := range g {
			acc.CPU += v.CPU
			acc.Mem += v.Mem
			acc.Temp += v.Temp
			acc.Disk += v.Disk
			acc.NetRx += v.NetRx
			acc.NetTx += v.NetTx
			acc.CPUMax = maxf(acc.CPUMax, v.CPUMax)
			acc.MemMax = maxf(acc.MemMax, v.MemMax)
			acc.TempMax = maxf(acc.TempMax, v.TempMax)
			acc.DiskMax = maxf(acc.DiskMax, v.DiskMax)
			acc.NetRxMax = maxf(acc.NetRxMax, v.NetRxMax)
			acc.NetTxMax = maxf(acc.NetTxMax, v.NetTxMax)
		}
		k := float64(len(g))
		acc.CPU /= k
		acc.Mem /= k
		acc.Temp /= k
		acc.Disk /= k
		acc.NetRx /= k
		acc.NetTx /= k
		out = append(out, acc)
	}
	return out
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
