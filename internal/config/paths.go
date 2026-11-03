package config

import (
	"os"
	"path/filepath"
)

// DataDir 运行数据目录（历史库、告警记录、网络日历）。
//
// 飞牛 fpk 通过 R1_DATA_DIR 指向 $TRIM_PKGVAR，卸载时整目录可清干净；
// 命令行安装沿用历史上的 /var/lib/r1-toolbox，行为不变。
func DataDir() string {
	if d := os.Getenv("R1_DATA_DIR"); d != "" {
		return d
	}
	return "/var/lib/r1-toolbox"
}

// DataFile 拼一个运行数据文件的绝对路径。
func DataFile(name string) string {
	return filepath.Join(DataDir(), name)
}
