package execx

import (
	"strings"
	"testing"
	"time"
)

func TestRunOK(t *testing.T) {
	out, err := Run(5*time.Second, "echo", "hello")
	if err != nil {
		t.Fatalf("Run echo: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Fatalf("输出不符: %q", out)
	}
}

// TestRunTimeout 半死命令必须在超时处被放弃，且耗时 ≈ timeout 而不是命令时长。
func TestRunTimeout(t *testing.T) {
	start := time.Now()
	_, err := Run(300*time.Millisecond, "sleep", "5")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("超时未报错")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("超时耗时 %v，双保险失效（Wait 被卡住）", elapsed)
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("错误信息应说明超时: %v", err)
	}
}

func TestRunNotFound(t *testing.T) {
	if _, err := Run(time.Second, "definitely-not-exists-cmd-xyz"); err == nil {
		t.Fatal("不存在的命令应报错")
	}
}
