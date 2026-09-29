package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"bigcatv/internal/model"
)

// singBoxConfig sing-box 配置中我们关心的部分。
type singBoxConfig struct {
	Outbounds []map[string]any `json:"outbounds"`
}

// ParseSingBox 解析 sing-box JSON 配置的 outbounds 为统一节点。
func ParseSingBox(data []byte) ([]*model.Node, error) {
	var cfg singBoxConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("sing-box json 解析失败: %w", err)
	}
	var nodes []*model.Node
	for i, ob := range cfg.Outbounds {
		typ, _ := ob["type"].(string)
		switch strings.ToLower(typ) {
		case "shadowsocks", "vmess", "vless", "trojan", "hysteria2", "tuic", "wireguard", "socks", "http":
			// 可转换为节点的类型
		default:
			continue // selector/urltest/direct/block/dns 等跳过
		}
		n, err := singBoxOutbound(ob)
		if err != nil {
			return nil, fmt.Errorf("outbounds[%d]: %w", i, err)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func sbStr(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func sbInt(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return 0
}

func sbMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]any); ok {
			return mm
		}
	}
	return nil
}

func singBoxOutbound(ob map[string]any) (*model.Node, error) {
	typ := strings.ToLower(sbStr(ob, "type"))
	n := &model.Node{
		Name:   sbStr(ob, "tag"),
		Server: sbStr(ob, "server"),
		Port:   sbInt(ob, "server_port"),
	}
	switch typ {
	case "shadowsocks":
		n.Protocol = model.ProtoShadowsocks
		n.Method = sbStr(ob, "method")
		n.Password = sbStr(ob, "password")
	case "vmess":
		n.Protocol = model.ProtoVMess
		n.UUID = sbStr(ob, "uuid")
		applySBTLS(n, sbMap(ob, "tls"))
		applySBTransport(n, sbMap(ob, "transport"))
	case "vless":
		n.Protocol = model.ProtoVLESS
		n.UUID = sbStr(ob, "uuid")
		n.Flow = sbStr(ob, "flow")
		applySBTLS(n, sbMap(ob, "tls"))
		applySBTransport(n, sbMap(ob, "transport"))
	case "trojan":
		n.Protocol = model.ProtoTrojan
		n.Password = sbStr(ob, "password")
		applySBTLS(n, sbMap(ob, "tls"))
		applySBTransport(n, sbMap(ob, "transport"))
	case "hysteria2":
		n.Protocol = model.ProtoHysteria2
		n.Password = sbStr(ob, "password")
		if o := sbMap(ob, "obfs"); o != nil {
			n.Obfs = sbStr(o, "type")
			n.ObfsPassword = sbStr(o, "password")
		}
		applySBTLS(n, sbMap(ob, "tls"))
	case "tuic":
		n.Protocol = model.ProtoTUIC
		n.UUID = sbStr(ob, "uuid")
		n.Password = sbStr(ob, "password")
		applySBTLS(n, sbMap(ob, "tls"))
	case "wireguard":
		n.Protocol = model.ProtoWireGuard
		n.PrivateKey = sbStr(ob, "private_key")
		if peer, ok := ob["peer_public_key"].(string); ok {
			n.PublicKey = peer
		}
		n.PreSharedKey = sbStr(ob, "pre_shared_key")
		if addrs, ok := ob["local_address"].([]any); ok {
			for _, a := range addrs {
				n.LocalAddress = append(n.LocalAddress, fmt.Sprint(a))
			}
		}
		n.MTU = sbInt(ob, "mtu")
	case "socks":
		n.Protocol = model.ProtoSOCKS
		n.Username = sbStr(ob, "username")
		n.Password = sbStr(ob, "password")
	case "http":
		n.Protocol = model.ProtoHTTP
		n.Username = sbStr(ob, "username")
		n.Password = sbStr(ob, "password")
		if t := sbMap(ob, "tls"); t != nil && t["enabled"] == true {
			n.TLS = true
		}
	}
	n.Normalize()
	if err := n.Validate(); err != nil {
		return nil, err
	}
	return n, nil
}

func applySBTLS(n *model.Node, tls map[string]any) {
	if tls == nil {
		return
	}
	if en, _ := tls["enabled"].(bool); en {
		n.TLS = true
	}
	n.SNI = sbStr(tls, "server_name")
	if alpn, ok := tls["alpn"].([]any); ok {
		for _, a := range alpn {
			n.ALPN = append(n.ALPN, fmt.Sprint(a))
		}
	}
	n.Fingerprint = sbStr(tls, "utls")
	if fp, ok := tls["utls"].(map[string]any); ok {
		n.Fingerprint = sbStr(fp, "fingerprint")
	}
}

func applySBTransport(n *model.Node, tr map[string]any) {
	if tr == nil {
		n.Network = model.NetTCP
		return
	}
	switch strings.ToLower(sbStr(tr, "type")) {
	case "ws":
		n.Network = model.NetWS
		n.WsPath = sbStr(tr, "path")
		if h, ok := tr["headers"].(map[string]any); ok {
			n.WsHost = sbStr(h, "Host")
		}
	case "grpc":
		n.Network = model.NetGRPC
		n.GrpcService = sbStr(tr, "service_name")
	case "http":
		n.Network = model.NetH2
	default:
		n.Network = model.NetTCP
	}
}
