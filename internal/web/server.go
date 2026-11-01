package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"r1-toolbox/internal/config"
	"r1-toolbox/internal/hardware"
	"r1-toolbox/internal/logx"
	"r1-toolbox/internal/monitor"
	"r1-toolbox/internal/screen"
)

type Server struct {
	router     *gin.Engine
	monitor    *monitor.SystemMonitor
	fanCtrl    *hardware.FanController
	rgbCtrl    *hardware.RGBController
	brightness *hardware.BrightnessController
	config     *config.Store
	screen     *screen.Control
	guard      *authGuard

	// OnDisplay 显示设置变更后的回调（主程序注入：把改动立刻推到屏幕上）。
	// 为 nil 时只落盘，重启后生效 —— 面板不该因为屏幕没接上就拒绝保存。
	OnDisplay func(config.DisplayConfig)

	// Preview 实时预览：按页名离屏重绘一帧 PNG（主程序注入渲染器闭包）。
	// 为 nil 时 /api/preview 返回 501 —— 预览依赖屏幕在跑，纯 headless
	// 场景（Docker 容器运行）给不了。
	Preview func(page string) ([]byte, error)
}

// NewServer 组装 Web 面板。
//
// 与旧版的差别（2026-10-28 加固）：
//   - config 由 *config.Config 换成 *config.Store（并发读写收口）
//   - 关屏/开屏/调亮度统一走 *screen.Control（不再直接写背光）
//   - 删掉 /api/background 与 /api/textcolor：新 UI 用内置渐变背景，
//     LoadBackground 是空实现，上传接口等于一个能往 /etc 写文件的洞
func NewServer(mon *monitor.SystemMonitor, fan *hardware.FanController, rgb *hardware.RGBController,
	bright *hardware.BrightnessController, store *config.Store, scr *screen.Control) *Server {

	gin.SetMode(gin.ReleaseMode)
	// 不用 gin.Default()：它的访问日志会把完整 URL（含 query）写进 journal，
	// 而面板令牌过去允许 ?token= 传递 —— 等于把令牌抄进日志。改用自建中间件。
	router := gin.New()
	router.Use(gin.Recovery())

	s := &Server{
		router:     router,
		monitor:    mon,
		fanCtrl:    fan,
		rgbCtrl:    rgb,
		brightness: bright,
		config:     store,
		screen:     scr,
		guard:      newAuthGuard(),
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	router := s.router

	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")

		// /api/* 全部要求令牌（只认 X-Token 头）。
		// 页面外壳（/ 与 /assets）保持开放 —— 它不含任何数据，
		// 浏览器首次访问时由内嵌脚本引导输入令牌。
		if strings.HasPrefix(c.Request.URL.Path, "/api/") && !s.authorized(c) {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}

		start := time.Now()
		c.Next()
		// 只记 method / path / status / 耗时；**不记 query**（防令牌泄漏进日志）
		logx.Debugf("[web] %s %s %d %v", c.Request.Method, c.Request.URL.Path,
			c.Writer.Status(), time.Since(start).Round(time.Millisecond))
	})

	distFS := getDistFS()
	router.GET("/assets/*filepath", func(c *gin.Context) {
		c.FileFromFS(c.Request.URL.Path, distFS)
	})

	router.GET("/", s.handleIndex)
	router.GET("/api/status", s.handleStatus)
	router.GET("/api/config", s.handleConfig)
	router.GET("/api/display", s.handleDisplayGet)
	router.GET("/api/preview", s.handlePreview)
	router.POST("/api/display", s.handleDisplaySet)
	router.GET("/api/hwmon/scan", s.handleHwmonScan)
	router.POST("/api/brightness", s.handleBrightness)
	router.POST("/api/screen/timeout", s.handleScreenTimeout)
	router.POST("/api/screen/off", s.handleScreenOff)
	router.POST("/api/screen/on", s.handleScreenOn)
	router.POST("/api/rgb", s.handleRGB)
	router.POST("/api/fan", s.handleFan)
	router.POST("/api/fan/curve", s.handleFanCurve)
	router.POST("/api/fan/mapping", s.handleFanMapping)
}

func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

// ------------------------------------------------------------
//  鉴权
// ------------------------------------------------------------

// authorized 校验面板令牌。
//
// 安全细节：
//   - 只接受 X-Token 头。旧版还接受 ?token=，而 query 会被访问日志原样
//     抄进 journal —— 令牌写在 URL 里迟早漏。
//   - 用 SHA-256 后做常量时间比较：直接 == 比较会在第一个不同字节处返回，
//     理论上可被计时攻击逐位试探；先哈希还能顺带抹平长度差异。
//   - 连续失败 8 次锁该 IP 5 分钟，防止局域网里有人挂着字典慢慢试。
func (s *Server) authorized(c *gin.Context) bool {
	want := s.config.Get().Web.Token
	if want == "" {
		// 没有令牌就 fail-closed：宁可面板不可用，也不留一个开放的控制端口
		return false
	}
	ip := c.ClientIP()
	if !s.guard.allow(ip) {
		return false
	}
	if tokenEqual(c.GetHeader("X-Token"), want) {
		s.guard.reset(ip)
		return true
	}
	s.guard.fail(ip)
	return false
}

func tokenEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// authGuard 按 IP 记失败次数
type authGuard struct {
	mu    sync.Mutex
	fails map[string]*failState
}

type failState struct {
	count int
	until time.Time
}

const (
	authMaxFails = 8
	authLockFor  = 5 * time.Minute
)

func newAuthGuard() *authGuard {
	return &authGuard{fails: map[string]*failState{}}
}

func (g *authGuard) allow(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.fails[ip]
	if !ok {
		return true
	}
	if time.Now().Before(st.until) {
		return false
	}
	// 锁定期已过，清掉记录重新计数
	if !st.until.IsZero() {
		delete(g.fails, ip)
	}
	return true
}

func (g *authGuard) fail(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.fails[ip]
	if !ok {
		st = &failState{}
		g.fails[ip] = st
	}
	st.count++
	if st.count >= authMaxFails {
		st.until = time.Now().Add(authLockFor)
		st.count = 0
		logx.Printf("【安全】%s 连续 %d 次令牌错误，已锁定 %v", ip, authMaxFails, authLockFor)
	}
}

func (g *authGuard) reset(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fails, ip)
}

// tokenBootstrap 注入到 index.html 的 fetch 拦截器：
// 所有请求自动带上 localStorage 里的令牌；收到 401 时弹出输入框
// （令牌见设备屏幕「系统」页），输入成功即重试。
const tokenBootstrap = `<script>
(function(){
  var KEY='r1tb_token';
  var orig=window.fetch.bind(window);
  window.fetch=function(input,init){
    init=init||{};
    var h=new Headers(init.headers||{});
    h.set('X-Token',localStorage.getItem(KEY)||'');
    return orig(input,Object.assign({},init,{headers:h})).then(function(r){
      if(r.status!==401) return r;
      var t=prompt('首次使用：请输入面板令牌。\n在 NAS 上执行 sudo grep token /etc/r1-toolbox/config.yaml 可查看。');
      if(!t) return r;
      localStorage.setItem(KEY,t.trim());
      return window.fetch(input,init);
    });
  };
})();
</script>`

func withTokenBootstrap(html string) string {
	if i := strings.Index(html, "</head>"); i >= 0 {
		return html[:i] + tokenBootstrap + html[i:]
	}
	return html + tokenBootstrap
}

func (s *Server) handleIndex(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	htmlContent, _ := distFS.ReadFile("dist/index.html")
	if len(htmlContent) > 0 {
		c.Data(200, "text/html; charset=utf-8", []byte(withTokenBootstrap(string(htmlContent))))
	} else {
		c.String(200, withTokenBootstrap(htmlTemplate))
	}
}

func (s *Server) handleStatus(c *gin.Context) {
	snap := s.monitor.Snapshot()
	cfg := s.config.Get()

	// 温度读不到就是 null，让前端自己决定怎么显示 —— 不编一个 30
	cpuTemp, cpuOK := s.fanCtrl.GetTemp(cfg.Fan.CPU.TempSensor)
	hddTemp, hddOK := s.fanCtrl.GetTemp(cfg.Fan.HDD.TempSensor)

	resp := gin.H{
		"cpu":        snap.CPUPercent,
		"memory":     snap.MemPercent,
		"brightness": s.screen.Bright(),
		"screen_off": s.screen.IsOff(),
		"updated_at": snap.UpdatedAt,
	}
	if cpuOK {
		resp["cpu_temp"] = cpuTemp
	}
	if hddOK {
		resp["hdd_temp"] = hddTemp
	}
	c.JSON(200, resp)
}

// handleConfig 返回面板初始化所需的**真实配置**。
//
// 2026-11-01：旧面板打开时把「PWM 3 / 启动 35°C / 全速 75°C」硬编码在前端，
// 设备上实际的曲线是别的值 —— 面板一打开就在说谎，用户照着调反而把好的改成坏的。
// 这里只回显可调项，**绝不返回 web.token**（令牌只走屏幕/配置文件）。
func (s *Server) handleConfig(c *gin.Context) {
	cfg := s.config.Get()

	fan := func(f config.FanCurve) gin.H {
		return gin.H{
			"mode":        f.Mode,
			"pwm_index":   f.PWMIndex,
			"temp_sensor": f.TempSensor,
			"min_temp":    f.MinTemp,
			"max_temp":    f.MaxTemp,
		}
	}

	c.JSON(200, gin.H{
		"screen": gin.H{
			"brightness":       cfg.Screen.Brightness,
			"brightness_max":   s.brightness.Max(),
			"night_brightness": cfg.Screen.NightBrightness,
			"timeout":          cfg.Screen.Timeout,
		},
		"fan": gin.H{"cpu": fan(cfg.Fan.CPU), "hdd": fan(cfg.Fan.HDD)},
		"rgb": gin.H{"mode": cfg.RGB.Mode, "color": cfg.RGB.Color},
		"web": gin.H{"port": cfg.Web.Port, "token_set": cfg.Web.Token != ""},
	})
}

func (s *Server) handleDisplayGet(c *gin.Context) {
	c.JSON(200, s.config.Get().Display)
}

// handleDisplaySet 保存显示设置并立刻应用。
//
// 校验放在 config.DisplayConfig.Normalize 里而不是复制一份规则：
// 面板能写什么，启动时就能读什么，两边永远同一套约束。
func (s *Server) handleDisplaySet(c *gin.Context) {
	var req config.DisplayConfig
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	req.Normalize()

	if err := s.config.Update(func(cfg *config.Config) { cfg.Display = req }); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if s.OnDisplay != nil {
		s.OnDisplay(req)
	}
	c.JSON(200, gin.H{"success": true, "display": req})
}

// handlePreview 返回指定页的实时预览 PNG。
//
// 页名不合法或未启用都返回 400（面板只渲染启用的页，出现 400 说明
// 两边状态不同步 —— 比如用户在另一台设备上刚把页关了）。
func (s *Server) handlePreview(c *gin.Context) {
	if s.Preview == nil {
		c.JSON(501, gin.H{"error": "预览不可用（屏幕未在运行）"})
		return
	}
	page := c.Query("page")
	png, err := s.Preview(page)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.Data(200, "image/png", png)
}

func (s *Server) handleHwmonScan(c *gin.Context) {
	cfg := s.config.Get()
	scanner := hardware.NewHwmonScanner(cfg.Hardware.HwmonPath)
	c.JSON(200, gin.H{
		"fans":  scanner.ScanFans(),
		"temps": scanner.ScanTemps(),
	})
}

func (s *Server) handleScreenOff(c *gin.Context) {
	s.screen.Off("Web 面板")
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) handleScreenOn(c *gin.Context) {
	// 不再硬编码 36000：Control 会按当前时段（日间/夜间）取正确亮度
	s.screen.Wake()
	c.JSON(200, gin.H{"success": true, "brightness": s.screen.Bright()})
}

func (s *Server) handleBrightness(c *gin.Context) {
	var req struct {
		Value int `json:"value"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	// 入参校验（2026-09-27 修复）：越界值曾被原样写入配置持久化 ——
	// 实测 {"value":-500} 会把屏幕熄灭且重启后仍为 0 亮度（自残状态）。
	if req.Value < 0 || req.Value > s.brightness.Max() {
		c.JSON(400, gin.H{"error": "value out of range"})
		return
	}
	if err := s.screen.Apply(req.Value); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) handleScreenTimeout(c *gin.Context) {
	var req struct {
		Timeout int `json:"timeout"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Timeout < 0 {
		c.JSON(400, gin.H{"error": "timeout must be >= 0"})
		return
	}
	if err := s.config.Update(func(cfg *config.Config) { cfg.Screen.Timeout = req.Timeout }); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) handleRGB(c *gin.Context) {
	var req struct {
		Mode  string `json:"mode"`
		Color string `json:"color"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := s.rgbCtrl.SetMode(req.Mode, req.Color); err != nil {
		// RGB 是装饰性功能，硬件不在也不该让整个请求失败
		logx.Debugf("[web] RGB 写入失败: %v", err)
	}
	if err := s.config.Update(func(cfg *config.Config) {
		cfg.RGB.Mode = req.Mode
		cfg.RGB.Color = req.Color
	}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) handleFan(c *gin.Context) {
	var req struct {
		Type string `json:"type"`
		Mode string `json:"mode"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := s.config.Update(func(cfg *config.Config) {
		if req.Type == "cpu" {
			cfg.Fan.CPU.Mode = req.Mode
		} else {
			cfg.Fan.HDD.Mode = req.Mode
		}
	}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// 先落盘再动硬件：落盘失败时不要让屏幕显示「已切到手动」而硬盘没切
	if req.Type == "cpu" {
		s.fanCtrl.SetCPUMode(req.Mode)
	} else {
		s.fanCtrl.SetHDDMode(req.Mode)
	}
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) handleFanCurve(c *gin.Context) {
	var req struct {
		Type    string `json:"type"`
		MinTemp int    `json:"min_temp"`
		MaxTemp int    `json:"max_temp"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	// 曲线自洽性：min 必须小于 max，且都落在物理合理域 [0,120]°C。
	// 修复前 {"min_temp":90,"max_temp":30} 会被照单全收，调速行为未定义。
	if req.MinTemp < 0 || req.MinTemp > 120 || req.MaxTemp < 0 || req.MaxTemp > 120 || req.MinTemp >= req.MaxTemp {
		c.JSON(400, gin.H{"error": "invalid curve: require 0 <= min_temp < max_temp <= 120"})
		return
	}
	if err := s.config.Update(func(cfg *config.Config) {
		if req.Type == "cpu" {
			cfg.Fan.CPU.MinTemp, cfg.Fan.CPU.MaxTemp = req.MinTemp, req.MaxTemp
		} else {
			cfg.Fan.HDD.MinTemp, cfg.Fan.HDD.MaxTemp = req.MinTemp, req.MaxTemp
		}
	}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if req.Type == "cpu" {
		s.fanCtrl.SetCPUTempRange(req.MinTemp, req.MaxTemp)
	} else {
		s.fanCtrl.SetHDDTempRange(req.MinTemp, req.MaxTemp)
	}
	c.JSON(200, gin.H{"success": true})
}

func (s *Server) handleFanMapping(c *gin.Context) {
	var req struct {
		Type       string `json:"type"`
		PWMIndex   int    `json:"pwm_index"`
		TempSensor string `json:"temp_sensor"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.PWMIndex < 1 || req.PWMIndex > 5 {
		c.JSON(400, gin.H{"error": "pwm_index must be 1..5"})
		return
	}
	if err := s.config.Update(func(cfg *config.Config) {
		if req.Type == "cpu" {
			cfg.Fan.CPU.PWMIndex = req.PWMIndex
			cfg.Fan.CPU.TempSensor = req.TempSensor
		} else {
			cfg.Fan.HDD.PWMIndex = req.PWMIndex
			cfg.Fan.HDD.TempSensor = req.TempSensor
		}
	}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "note": "PWM 映射需重启服务后生效"})
}
