package monitor

import "testing"

// TestUnescapeLabel lsblk 会把非 ASCII 卷标转义成字面 \xNN，
// 屏上横幅和面板存储卡直接显示它，用户看到的是一串反斜杠。
func TestUnescapeLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		{`\xe5\xaa\x92\xe4\xbd\x93\xe5\xba\x93`, `媒体库`}, // 媒体库
		{`trim-fd5c:0`, `trim-fd5c:0`},
		{``, ``},
		{`abc\x41`, `abcA`},
		{`bad\xzz`, `bad\xzz`}, // 非法转义原样保留
		{`tail\x4`, `tail\x4`}, // 截断的转义也不吞字符
		{`中文卷标`, `中文卷标`},       // 已经是原文，不应被二次处理
	}
	for _, c := range cases {
		if got := unescapeLabel(c.in); got != c.want {
			t.Fatalf("unescapeLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
