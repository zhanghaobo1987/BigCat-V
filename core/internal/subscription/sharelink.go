// Package subscription 订阅抓取与解析：分享链接 / Clash YAML / sing-box JSON。
package subscription

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"bigcatv/internal/model"
)

// ParseBatch 解析批量分享链接文本（每行一条），跳过空行与注释。
func ParseBatch(text string) ([]*model.Node, []error) {
	var nodes []*model.Node
	var errs []error
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		n, err := ParseShareLink(line)
		if err != nil {
			errs = append(errs, fmt.Errorf("%q: %w", trunc(line, 40), err))
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes, errs
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ParseShareLink 解析单条分享链接。
func ParseShareLink(link string) (*model.Node, error) {
	link = strings.TrimSpace(link)
	scheme := strings.ToLower(strings.SplitN(link, ":", 2)[0])
	switch scheme {
	case "ss":
		return parseSS(link)
	case "vmess":
		return parseVMess(link)
	case "vless":
		return parseVLESS(link)
	case "trojan":
		return parseTrojan(link)
	case "hy2", "hysteria2":
		return parseHysteria2(link)
	case "tuic":
		return parseTUIC(link)
	case "wireguard", "wg":
		return parseWireGuard(link)
	case "socks", "socks5":
		return parseSocks(link, model.ProtoSOCKS)
	case "http", "https":
		// 仅当作代理节点处理时才解析；订阅抓取另走 Fetch
		return parseSocks(link, model.ProtoHTTP)
	default:
		return nil, fmt.Errorf("不支持的协议 %q", scheme)
	}
}

func b64decode(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "#"); i >= 0 {
		s = s[:i]
	}
	// 兼容 urlsafe / 缺 padding
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return base64.StdEncoding.DecodeString(s)
}

func splitFragment(link string) (main, name string) {
	if i := strings.Index(link, "#"); i >= 0 {
		main, name = link[:i], link[i+1:]
		if u, err := url.PathUnescape(name); err == nil {
			name = u
		}
		return main, name
	}
	return link, ""
}

// ---- Shadowsocks ----

func parseSS(link string) (*model.Node, error) {
	main, name := splitFragment(link)
	rest := strings.TrimPrefix(main, "ss://")
	n := &model.Node{Protocol: model.ProtoShadowsocks, Name: name}

	var userinfo, hostport string
	if strings.Contains(rest, "@") {
		parts := strings.SplitN(rest, "@", 2)
		userinfo, hostport = parts[0], parts[1]
		if !strings.Contains(userinfo, ":") {
			// userinfo 整体是 base64(method:password)
			dec, err := b64decode(userinfo)
			if err != nil {
				return nil, fmt.Errorf("ss userinfo base64 解码失败: %w", err)
			}
			userinfo = string(dec)
		}
	} else {
		// 整体 base64(method:password@host:port)
		dec, err := b64decode(rest)
		if err != nil {
			return nil, fmt.Errorf("ss 整体 base64 解码失败: %w", err)
		}
		parts := strings.SplitN(string(dec), "@", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("ss 格式错误")
		}
		userinfo, hostport = parts[0], parts[1]
	}
	mp := strings.SplitN(userinfo, ":", 2)
	if len(mp) != 2 {
		return nil, fmt.Errorf("ss method:password 格式错误")
	}
	n.Method, n.Password = mp[0], mp[1]
	if u, err := url.PathUnescape(n.Password); err == nil {
		n.Password = u
	}
	host, portStr, err := splitHostPort(hostport)
	if err != nil {
		return nil, err
	}
	n.Server = host
	n.Port, err = strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("ss 端口非法: %w", err)
	}
	n.Normalize()
	return n, n.Validate()
}

// ---- VMess ----

type vmessJSON struct {
	Ps   string `json:"ps"`
	Add  string `json:"add"`
	Port string `json:"port"`
	ID   string `json:"id"`
	Aid  string `json:"aid"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	Alpn string `json:"alpn"`
}

func parseVMess(link string) (*model.Node, error) {
	payload := strings.TrimPrefix(link, "vmess://")
	raw, err := b64decode(payload)
	if err != nil {
		return nil, fmt.Errorf("vmess base64 解码失败: %w", err)
	}
	var v vmessJSON
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("vmess json 解析失败: %w", err)
	}
	port, _ := strconv.Atoi(strings.TrimSpace(v.Port))
	n := &model.Node{
		Protocol: model.ProtoVMess,
		Name:     v.Ps,
		Server:   v.Add,
		Port:     port,
		UUID:     v.ID,
		Network:  model.Network(strings.ToLower(v.Net)),
		TLS:      strings.ToLower(v.TLS) == "tls",
		SNI:      v.SNI,
		WsPath:   v.Path,
		WsHost:   v.Host,
		Raw:      link,
	}
	if n.Network == "" {
		n.Network = model.NetTCP
	}
	if v.Alpn != "" {
		n.ALPN = strings.Split(v.Alpn, ",")
	}
	n.Normalize()
	return n, n.Validate()
}

// ---- VLESS / Trojan / Hysteria2 / TUIC（userinfo 风格） ----

func parseUserinfoLink(link string, proto model.Protocol) (*model.Node, error) {
	main, name := splitFragment(link)
	u, err := url.Parse(main)
	if err != nil {
		return nil, fmt.Errorf("URL 解析失败: %w", err)
	}
	n := &model.Node{
		Protocol: proto,
		Name:     name,
		Server:   u.Hostname(),
		Raw:      link,
	}
	if p := u.Port(); p != "" {
		n.Port, _ = strconv.Atoi(p)
	}
	q := u.Query()

	switch proto {
	case model.ProtoVLESS:
		n.UUID = u.User.Username()
		n.Flow = q.Get("flow")
		applyTransport(n, q)
	case model.ProtoTrojan:
		n.Password, _ = url.PathUnescape(u.User.Username())
		// trojan 默认 tls
		if q.Get("security") == "" {
			n.TLS = true
		}
		applyTransport(n, q)
	case model.ProtoHysteria2:
		n.Password, _ = url.PathUnescape(u.User.Username())
		n.SNI = firstNonEmpty(q.Get("sni"), q.Get("peer"))
		n.Obfs = q.Get("obfs")
		n.ObfsPassword = q.Get("obfs-password")
		if up := q.Get("up"); up != "" {
			n.UpMbps, _ = strconv.Atoi(up)
		}
		if down := q.Get("down"); down != "" {
			n.DownMbps, _ = strconv.Atoi(down)
		}
		n.TLS = true
	case model.ProtoTUIC:
		n.UUID = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			n.Password, _ = url.PathUnescape(pw)
		}
		n.SNI = firstNonEmpty(q.Get("sni"), q.Get("peer"))
		if alpn := q.Get("alpn"); alpn != "" {
			n.ALPN = strings.Split(alpn, ",")
		}
		n.ObfsPassword = q.Get("password")
		n.TLS = true
	}
	n.Normalize()
	return n, n.Validate()
}

func applyTransport(n *model.Node, q url.Values) {
	t := strings.ToLower(q.Get("type"))
	switch t {
	case "ws", "websocket":
		n.Network = model.NetWS
	case "grpc":
		n.Network = model.NetGRPC
	case "h2", "http":
		n.Network = model.NetH2
	default:
		n.Network = model.NetTCP
	}
	sec := strings.ToLower(q.Get("security"))
	// security 参数缺省时不覆盖调用方已设的默认值（如 trojan 默认 tls）
	if q.Has("security") {
		n.TLS = sec == "tls" || sec == "reality"
	}
	n.SNI = firstNonEmpty(q.Get("sni"), q.Get("peer"), q.Get("host"))
	if alpn := q.Get("alpn"); alpn != "" {
		n.ALPN = strings.Split(alpn, ",")
	}
	switch n.Network {
	case model.NetWS, model.NetH2:
		n.WsPath = firstNonEmpty(q.Get("path"), q.Get("serviceName"))
		n.WsHost = q.Get("host")
	case model.NetGRPC:
		n.GrpcService = q.Get("serviceName")
	}
	if fp := q.Get("fp"); fp != "" {
		n.Fingerprint = fp
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func parseVLESS(link string) (*model.Node, error) {
	return parseUserinfoLink(link, model.ProtoVLESS)
}
func parseTrojan(link string) (*model.Node, error) {
	return parseUserinfoLink(link, model.ProtoTrojan)
}
func parseHysteria2(link string) (*model.Node, error) {
	return parseUserinfoLink(link, model.ProtoHysteria2)
}
func parseTUIC(link string) (*model.Node, error) {
	return parseUserinfoLink(link, model.ProtoTUIC)
}

// ---- WireGuard ----

func parseWireGuard(link string) (*model.Node, error) {
	main, name := splitFragment(link)
	u, err := url.Parse(main)
	if err != nil {
		return nil, fmt.Errorf("URL 解析失败: %w", err)
	}
	q := u.Query()
	n := &model.Node{
		Protocol:   model.ProtoWireGuard,
		Name:       name,
		Server:     u.Hostname(),
		PrivateKey: q.Get("privateKey"),
		PublicKey:  q.Get("publicKey"),
		Raw:        link,
	}
	if p := u.Port(); p != "" {
		n.Port, _ = strconv.Atoi(p)
	}
	if n.Port == 0 {
		n.Port = 51820
	}
	if psk := q.Get("presharedKey"); psk != "" {
		n.PreSharedKey = psk
	}
	if addr := q.Get("address"); addr != "" {
		n.LocalAddress = strings.Split(addr, ",")
	}
	if mtu := q.Get("mtu"); mtu != "" {
		n.MTU, _ = strconv.Atoi(mtu)
	}
	n.Normalize()
	return n, n.Validate()
}

// ---- SOCKS / HTTP ----

func parseSocks(link string, proto model.Protocol) (*model.Node, error) {
	main, name := splitFragment(link)
	u, err := url.Parse(main)
	if err != nil {
		return nil, fmt.Errorf("URL 解析失败: %w", err)
	}
	n := &model.Node{Protocol: proto, Name: name, Server: u.Hostname(), Raw: link}
	if p := u.Port(); p != "" {
		n.Port, _ = strconv.Atoi(p)
	}
	if n.Port == 0 {
		n.Port = 1080
	}
	if u.User != nil {
		n.Username = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			n.Password = pw
		}
	}
	n.Normalize()
	return n, n.Validate()
}

func splitHostPort(hp string) (string, string, error) {
	// 兼容 IPv6 [::1]:port
	if strings.HasPrefix(hp, "[") {
		end := strings.Index(hp, "]")
		if end < 0 {
			return "", "", fmt.Errorf("IPv6 host 格式错误")
		}
		host := hp[1:end]
		port := strings.TrimPrefix(hp[end+1:], ":")
		return host, port, nil
	}
	i := strings.LastIndex(hp, ":")
	if i < 0 {
		return "", "", fmt.Errorf("缺少端口")
	}
	return hp[:i], hp[i+1:], nil
}
