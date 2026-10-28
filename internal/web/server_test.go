package web

// 鉴权路径的测试。2026-10-28 加固的三件事都在这里回归：
//   1. 令牌只认 X-Token 头（?token= 会被访问日志抄走）
//   2. 常量时间比较 + SHA-256 抹平长度差异
//   3. 连续失败 8 次锁 IP 5 分钟

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"r1-toolbox/internal/config"
	"r1-toolbox/internal/hardware"
	"r1-toolbox/internal/monitor"
	"r1-toolbox/internal/screen"
)

func TestTokenEqual(t *testing.T) {
	if !tokenEqual("secret-token", "secret-token") {
		t.Fatal("相同令牌应相等")
	}
	if tokenEqual("secret-token", "wrong-token") {
		t.Fatal("不同令牌不应相等")
	}
	// 长度差异被 SHA-256 抹平：不会 panic、不会相等
	if tokenEqual("secret-token", "x") {
		t.Fatal("长度不同不应相等")
	}
	if tokenEqual("", "secret-token") || tokenEqual("secret-token", "") {
		t.Fatal("空令牌一律拒绝（fail-closed）")
	}
}

func TestAuthGuardLockout(t *testing.T) {
	g := newAuthGuard()

	for i := 0; i < authMaxFails-1; i++ {
		g.fail("10.0.0.1")
	}
	if !g.allow("10.0.0.1") {
		t.Fatal("未达阈值不应锁定")
	}
	g.fail("10.0.0.1") // 第 8 次
	if g.allow("10.0.0.1") {
		t.Fatal("连续失败 8 次后应锁定")
	}
	if !g.allow("10.0.0.2") {
		t.Fatal("其他 IP 不应被牵连")
	}
	g.reset("10.0.0.1")
	if !g.allow("10.0.0.1") {
		t.Fatal("reset 后应放行")
	}
}

// newTestServer 搭一个完整 Server（临时背光文件 + 临时配置 + 零值监控）
func newTestServer(t *testing.T, token string) (*Server, *config.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	bright := hardware.NewBrightnessController(t.TempDir()+"/brightness", 0, 36000)
	store, err := config.NewStore(t.TempDir() + "/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(c *config.Config) { c.Web.Token = token }); err != nil {
		t.Fatal(err)
	}
	mon := monitor.NewSystemMonitor()
	fan := hardware.NewFanController()
	scr := screen.New(bright, 36000, 0, nil, nil, nil)
	return NewServer(mon, fan, hardware.NewRGBController(), bright, store, scr), store
}

func TestAPIRequiresTokenHeader(t *testing.T) {
	s, _ := newTestServer(t, "GOODTOKEN")

	do := func(chooseToken string) int {
		req := httptest.NewRequest("GET", "/api/status", nil)
		if chooseToken != "" {
			req.Header.Set("X-Token", chooseToken)
		}
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		return w.Code
	}

	if code := do(""); code != http.StatusUnauthorized {
		t.Fatalf("无令牌应 401，got %d", code)
	}
	if code := do("BADTOKEN"); code != http.StatusUnauthorized {
		t.Fatalf("错令牌应 401，got %d", code)
	}
	if code := do("GOODTOKEN"); code != http.StatusOK {
		t.Fatalf("对令牌应 200，got %d", code)
	}
}

// TestQueryTokenRejected 旧版允许 ?token=，那是把令牌抄进访问日志。
// 现在必须拒绝 —— 回归测试。
func TestQueryTokenRejected(t *testing.T) {
	s, _ := newTestServer(t, "GOODTOKEN")

	req := httptest.NewRequest("GET", "/api/status?token=GOODTOKEN", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("?token= 传递必须被拒绝（日志泄漏），got %d", w.Code)
	}
}

// TestFailClosedWhenTokenEmpty 令牌为空时所有 /api/* 拒绝，
// 宁可面板不可用也不留开放控制端口。
func TestFailClosedWhenTokenEmpty(t *testing.T) {
	s, _ := newTestServer(t, "")

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("X-Token", "whatever")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("空令牌配置应全拒，got %d", w.Code)
	}
}

// TestAuthGuardLocksRequests 锁定期内连对令牌也拒绝。
func TestAuthGuardLocksRequests(t *testing.T) {
	s, _ := newTestServer(t, "GOODTOKEN")

	// 错 8 次 → IP 被锁
	for i := 0; i < authMaxFails; i++ {
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.Header.Set("X-Token", "WRONG")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("错令牌应 401，got %d", w.Code)
		}
	}
	// 换对令牌也进不来（IP 锁定中）
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("X-Token", "GOODTOKEN")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("锁定期内对令牌也应 401，got %d", w.Code)
	}
}

// TestScreenOffThroughControl Web 关屏走统一状态机：IsOff 必须翻转，
// 而不是只写背光（旧实现的状态分裂回归测试）。
func TestScreenOffThroughControl(t *testing.T) {
	s, _ := newTestServer(t, "GOODTOKEN")

	doPost := func(path string) int {
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("X-Token", "GOODTOKEN")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		return w.Code
	}

	if code := doPost("/api/screen/off"); code != http.StatusOK {
		t.Fatalf("关屏应 200，got %d", code)
	}
	if !s.screen.IsOff() {
		t.Fatal("Web 关屏后状态机仍是「开」—— 状态分裂复现")
	}
}
