// Bark 推送通道。
//
// 用户在面板里填的是**完整前缀**（如 https://api.day.app/你的key），
// 本包只负责拼路径与发请求：
//
//	<BaseURL>/<标题>/<内容>
//
// 设计约束（2026-11-01 与洋哥确认）：
//   - key 存在 config.alerts.bark_url，**接口永不回显**（同面板令牌的口径）
//   - 网络不通时 Push 返回 error，调用方只记日志 —— 推送失败绝不能
//     影响告警判定本身（网络都断了，屏上横幅才是主通道）
package alerts

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Bark URL 提供器做成函数：面板改 key 后立即生效，不用重启引擎
type Bark struct {
	GetURL func() string
	client *http.Client
}

func NewBark(getURL func() string) *Bark {
	return &Bark{GetURL: getURL, client: &http.Client{Timeout: 6 * time.Second}}
}

// Push 发一条。Bark 的 HTTP 200 即认为送达（它自己负责转 APNs）。
func (b *Bark) Push(title, body string) error {
	if b == nil || b.GetURL == nil {
		return fmt.Errorf("bark 未配置")
	}
	base := strings.TrimSpace(b.GetURL())
	if base == "" {
		return fmt.Errorf("bark 未配置")
	}
	if !strings.HasPrefix(base, "https://") {
		// 明文 http 会把 key 暴露给链路上的任何人；用户真有内网自建的
		// 需求也应该用 https + 自签。这里直接拒绝，宁可不推。
		return fmt.Errorf("bark 地址必须是 https://")
	}
	u := strings.TrimRight(base, "/") + "/" +
		url.PathEscape(title) + "/" + url.PathEscape(body)

	resp, err := b.client.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bark 返回 %d", resp.StatusCode)
	}
	return nil
}
