package subscription

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"bigcatv/internal/model"
)

// clashConfig Clash YAML 顶层结构（只关心 proxies）。
type clashConfig struct {
	Proxies []map[string]any `yaml:"proxies"`
}

// ParseClash 解析 Clash YAML 的 proxies 列表。
func ParseClash(data []byte) ([]*model.Node, error) {
	var cfg clashConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("clash yaml 解析失败: %w", err)
	}
	var nodes []*model.Node
	for i, p := range cfg.Proxies {
		n, err := clashProxy(p)
		if err != nil {
			return nil, fmt.Errorf("proxies[%d]: %w", i, err)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func str(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case string:
			return t
		case int:
			return strconv.Itoa(t)
		case float64:
			return strconv.Itoa(int(t))
		}
	}
	return ""
}

func intVal(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case int:
			return t
		case float64:
			return int(t)
		case string:
			i, _ := strconv.Atoi(t)
			return i
		}
	}
	return 0
}

func boolVal(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case bool:
			return t
		case string:
			return strings.EqualFold(t, "true")
		}
	}
	return false
}

func clashProxy(p map[string]any) (*model.Node, error) {
	typ := strings.ToLower(str(p, "type"))
	n := &model.Node{
		Name:   str(p, "name"),
		Server: str(p, "server"),
		Port:   intVal(p, "port"),
	}
	switch typ {
	case "ss", "shadowsocks":
		n.Protocol = model.ProtoShadowsocks
		n.Method = str(p, "cipher")
		n.Password = str(p, "password")
	case "vmess":
		n.Protocol = model.ProtoVMess
		n.UUID = str(p, "uuid")
		n.Network = clashNetwork(p)
		applyClashTLS(n, p)
		applyClashWS(n, p)
		applyClashGRPC(n, p)
	case "vless":
		n.Protocol = model.ProtoVLESS
		n.UUID = str(p, "uuid")
		n.Flow = str(p, "flow")
		n.Network = clashNetwork(p)
		applyClashTLS(n, p)
		applyClashWS(n, p)
		applyClashGRPC(n, p)
	case "trojan":
		n.Protocol = model.ProtoTrojan
		n.Password = str(p, "password")
		n.Network = clashNetwork(p)
		n.TLS = true
		applyClashTLS(n, p)
		applyClashWS(n, p)
		applyClashGRPC(n, p)
	case "hysteria2", "hy2":
		n.Protocol = model.ProtoHysteria2
		n.Password = str(p, "password")
		n.Obfs = str(p, "obfs")
		n.ObfsPassword = str(p, "obfs-password")
		n.SNI = str(p, "sni")
		n.TLS = true
	case "tuic":
		n.Protocol = model.ProtoTUIC
		n.UUID = str(p, "uuid")
		n.Password = str(p, "password")
		n.SNI = str(p, "sni")
		if alpn := p["alpn"]; alpn != nil {
			switch t := alpn.(type) {
			case []any:
				for _, a := range t {
					n.ALPN = append(n.ALPN, fmt.Sprint(a))
				}
			case string:
				n.ALPN = []string{t}
			}
		}
		n.TLS = true
	case "wireguard", "wg":
		n.Protocol = model.ProtoWireGuard
		n.PrivateKey = str(p, "private-key")
		n.PublicKey = str(p, "public-key")
		n.PreSharedKey = str(p, "pre-shared-key")
		if ip := str(p, "ip"); ip != "" {
			n.LocalAddress = strings.Split(ip, ",")
		}
		n.MTU = intVal(p, "mtu")
	case "socks5", "socks":
		n.Protocol = model.ProtoSOCKS
		n.Username = str(p, "username")
		n.Password = str(p, "password")
		if n.Port == 0 {
			n.Port = 1080
		}
	case "http":
		n.Protocol = model.ProtoHTTP
		n.Username = str(p, "username")
		n.Password = str(p, "password")
		n.TLS = boolVal(p, "tls")
	default:
		return nil, fmt.Errorf("不支持的 clash 类型 %q", typ)
	}
	n.Normalize()
	if err := n.Validate(); err != nil {
		return nil, err
	}
	return n, nil
}

func clashNetwork(p map[string]any) model.Network {
	net := strings.ToLower(str(p, "network"))
	switch net {
	case "ws":
		return model.NetWS
	case "grpc":
		return model.NetGRPC
	case "h2":
		return model.NetH2
	default:
		return model.NetTCP
	}
}

func applyClashTLS(n *model.Node, p map[string]any) {
	if boolVal(p, "tls") {
		n.TLS = true
	}
	if sni := str(p, "sni"); sni != "" {
		n.SNI = sni
	} else if sni := str(p, "servername"); sni != "" {
		n.SNI = sni
	}
	if alpn := p["alpn"]; alpn != nil {
		switch t := alpn.(type) {
		case []any:
			for _, a := range t {
				n.ALPN = append(n.ALPN, fmt.Sprint(a))
			}
		case string:
			n.ALPN = []string{t}
		}
	}
	if fp := str(p, "fingerprint"); fp != "" {
		n.Fingerprint = fp
	}
}

func applyClashWS(n *model.Node, p map[string]any) {
	if n.Network != model.NetWS {
		return
	}
	if opts, ok := p["ws-opts"].(map[string]any); ok {
		n.WsPath = str(opts, "path")
		if h, ok := opts["headers"].(map[string]any); ok {
			n.WsHost = str(h, "Host")
		}
	}
}

func applyClashGRPC(n *model.Node, p map[string]any) {
	if n.Network != model.NetGRPC {
		return
	}
	if opts, ok := p["grpc-opts"].(map[string]any); ok {
		n.GrpcService = str(opts, "grpc-service-name")
	}
}
