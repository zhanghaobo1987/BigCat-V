package subscription

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bigcatv/internal/model"
)

// FetchOptions 订阅抓取选项。
type FetchOptions struct {
	Timeout   time.Duration
	UserAgent string
}

// DefaultFetchOptions 默认抓取选项。
func DefaultFetchOptions() FetchOptions {
	return FetchOptions{
		Timeout:   15 * time.Second,
		UserAgent: "BigCatV/0.1",
	}
}

// Fetch 抓取订阅 URL，自动嗅探载荷类型并解析为节点列表。
func Fetch(rawURL string, opts FetchOptions) ([]*model.Node, error) {
	if opts.Timeout == 0 {
		opts = DefaultFetchOptions()
	}
	client := &http.Client{Timeout: opts.Timeout}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", opts.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("抓取失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP 状态码 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	return SniffAndParse(body)
}

// SniffAndParse 嗅探字节流类型并解析：
//  1. JSON -> sing-box 配置；2. YAML(含 proxies:) -> Clash；
//  3. 否则按 base64 批量分享链接处理（兼容明文逐行链接）。
func SniffAndParse(body []byte) ([]*model.Node, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("订阅内容为空")
	}
	// 1. JSON（sing-box 配置或单条 vmess 风格）
	if trimmed[0] == '{' {
		if nodes, err := ParseSingBox(trimmed); err == nil && len(nodes) > 0 {
			return nodes, nil
		}
		return nil, fmt.Errorf("JSON 载荷无法解析为 sing-box 配置")
	}
	// 2. Clash YAML
	if bytes.Contains(trimmed, []byte("proxies:")) {
		return ParseClash(trimmed)
	}
	// 3. base64 批量或明文逐行
	text := string(trimmed)
	if decoded, err := b64decodeAll(text); err == nil {
		text = decoded
	}
	nodes, errs := ParseBatch(text)
	if len(nodes) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("无有效节点：%v", errs[0])
	}
	return nodes, nil
}

// b64decodeAll 尝试整体 base64 解码（机场常见整包 base64）。
func b64decodeAll(s string) (string, error) {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=", r) {
			return r
		}
		return -1
	}, strings.TrimSpace(s))
	if len(clean) < 16 || len(clean)%4 != 0 {
		return "", fmt.Errorf("not base64 batch")
	}
	dec, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return "", err
	}
	out := string(dec)
	if !strings.Contains(out, "://") {
		return "", fmt.Errorf("decoded 无分享链接特征")
	}
	return out, nil
}
