// Package generator 的 xray 部分：统一节点模型 -> Xray 配置 JSON。
//
// 协议兼容矩阵（Xray 不支持的协议）：
//   - hysteria2：xray 无实现
//   - tuic：xray 无实现
//
// 其余 ss / vmess / vless / trojan / wireguard / socks / http 均支持。
package generator

import (
	"encoding/json"
	"fmt"

	"bigcatv/internal/model"
)

// XraySupported 该协议在 xray 内核是否可用。
func XraySupported(p model.Protocol) bool {
	switch p {
	case model.ProtoHysteria2, model.ProtoTUIC:
		return false
	default:
		return true
	}
}

// XrayUnsupportedReason 不可用原因（供 UI 展示）。
func XrayUnsupportedReason(p model.Protocol) string {
	switch p {
	case model.ProtoHysteria2:
		return "xray 内核不支持 hysteria2，请切换 sing-box 内核"
	case model.ProtoTUIC:
		return "xray 内核不支持 tuic，请切换 sing-box 内核"
	default:
		return ""
	}
}

// GenerateXray 生成完整 Xray 配置。
func GenerateXray(nodes []*model.Node, opts Options) (map[string]any, error) {
	if opts.MixedPort == 0 {
		opts.MixedPort = 7890
	}
	var outbounds []any
	for _, n := range nodes {
		if !XraySupported(n.Protocol) {
			return nil, fmt.Errorf("节点 %q: %s", n.DisplayName(), XrayUnsupportedReason(n.Protocol))
		}
		ob, err := xrayOutbound(n)
		if err != nil {
			return nil, fmt.Errorf("节点 %q: %w", n.DisplayName(), err)
		}
		outbounds = append(outbounds, ob)
	}
	outbounds = append(outbounds,
		map[string]any{"protocol": "freedom", "tag": "direct"},
		map[string]any{"protocol": "blackhole", "tag": "block"},
	)

	cfg := map[string]any{
		"log": map[string]any{"loglevel": "info"},
		"inbounds": []any{
			map[string]any{
				"protocol": "socks", "tag": "socks-in",
				"listen": "127.0.0.1", "port": opts.MixedPort,
				"settings": map[string]any{"udp": true},
			},
			map[string]any{
				"protocol": "http", "tag": "http-in",
				"listen": "127.0.0.1", "port": opts.MixedPort + 1,
			},
		},
		"outbounds": outbounds,
		"routing": map[string]any{
			"domainStrategy": "IPIfNonMatch",
			"rules":          xrayBaseRules(opts),
		},
	}
	return cfg, nil
}

// xrayBaseRules 内置规则 + 自定义分流规则 + 默认动作兜底。
func xrayBaseRules(opts Options) []any {
	rules := []any{
		map[string]any{"type": "field", "ip": []string{"geoip:private"}, "outboundTag": "direct"},
		map[string]any{"type": "field", "protocol": []string{"bittorrent"}, "outboundTag": "block"},
	}
	if opts.Routing != nil {
		custom := xrayRouting(opts.Routing, opts.GeoDir)
		merged := append([]any{}, custom...)
		merged = append(merged, rules...)
		// xray 无 final 概念：未命中时走第一条 outbound（即代理）；
		// 默认动作为 direct/block 时追加兜底规则。
		switch actionTag(opts.Routing.DefaultAction) {
		case "direct", "block":
			merged = append(merged, map[string]any{
				"type": "field", "ip": []string{"0.0.0.0/0", "::/0"},
				"outboundTag": actionTag(opts.Routing.DefaultAction),
			})
		}
		return merged
	}
	return rules
}

// MarshalXray 生成并序列化为缩进 JSON。
func MarshalXray(nodes []*model.Node, opts Options) ([]byte, error) {
	cfg, err := GenerateXray(nodes, opts)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func xrayOutbound(n *model.Node) (map[string]any, error) {
	ob := map[string]any{"tag": n.ID}
	switch n.Protocol {
	case model.ProtoShadowsocks:
		ob["protocol"] = "shadowsocks"
		ob["settings"] = map[string]any{
			"servers": []any{map[string]any{
				"address": n.Server, "port": n.Port,
				"method": n.Method, "password": n.Password,
			}},
		}
	case model.ProtoVMess:
		ob["protocol"] = "vmess"
		ob["settings"] = map[string]any{
			"vnext": []any{map[string]any{
				"address": n.Server, "port": n.Port,
				"users": []any{map[string]any{"id": n.UUID, "security": "auto"}},
			}},
		}
		ob["streamSettings"] = xrayStream(n)
	case model.ProtoVLESS:
		ob["protocol"] = "vless"
		user := map[string]any{"id": n.UUID, "encryption": "none"}
		if n.Flow != "" {
			user["flow"] = n.Flow
		}
		ob["settings"] = map[string]any{
			"vnext": []any{map[string]any{
				"address": n.Server, "port": n.Port,
				"users": []any{user},
			}},
		}
		ob["streamSettings"] = xrayStream(n)
	case model.ProtoTrojan:
		ob["protocol"] = "trojan"
		ob["settings"] = map[string]any{
			"servers": []any{map[string]any{
				"address": n.Server, "port": n.Port, "password": n.Password,
			}},
		}
		ob["streamSettings"] = xrayStream(n)
	case model.ProtoSOCKS:
		servers := map[string]any{"address": n.Server, "port": n.Port}
		if n.Username != "" {
			servers["users"] = []any{map[string]any{"user": n.Username, "pass": n.Password}}
		}
		ob["protocol"] = "socks"
		ob["settings"] = map[string]any{"servers": []any{servers}}
	case model.ProtoHTTP:
		servers := map[string]any{"address": n.Server, "port": n.Port}
		if n.Username != "" {
			servers["users"] = []any{map[string]any{"user": n.Username, "pass": n.Password}}
		}
		ob["protocol"] = "http"
		ob["settings"] = map[string]any{"servers": []any{servers}}
	case model.ProtoWireGuard:
		peer := map[string]any{
			"publicKey": n.PublicKey,
			"endpoint":  fmt.Sprintf("%s:%d", n.Server, n.Port),
		}
		if n.PreSharedKey != "" {
			peer["preSharedKey"] = n.PreSharedKey
		}
		settings := map[string]any{
			"secretKey": n.PrivateKey,
			"peers":     []any{peer},
		}
		if len(n.LocalAddress) > 0 {
			settings["address"] = n.LocalAddress
		}
		if n.MTU > 0 {
			settings["mtu"] = n.MTU
		}
		if len(n.Reserved) == 3 {
			settings["reserved"] = n.Reserved
		}
		ob["protocol"] = "wireguard"
		ob["settings"] = settings
	default:
		return nil, fmt.Errorf("xray 不支持协议 %q", n.Protocol)
	}
	return ob, nil
}

// xrayStream 传输层配置。
func xrayStream(n *model.Node) map[string]any {
	ss := map[string]any{}
	switch n.Network {
	case model.NetWS:
		ss["network"] = "ws"
		ws := map[string]any{}
		if n.WsPath != "" {
			ws["path"] = n.WsPath
		}
		if n.WsHost != "" {
			ws["headers"] = map[string]any{"Host": n.WsHost}
		}
		ss["wsSettings"] = ws
	case model.NetGRPC:
		ss["network"] = "grpc"
		ss["grpcSettings"] = map[string]any{"serviceName": n.GrpcService}
	case model.NetH2:
		ss["network"] = "http"
		h := map[string]any{}
		if n.WsPath != "" {
			h["path"] = n.WsPath
		}
		if n.WsHost != "" {
			h["host"] = []string{n.WsHost}
		}
		ss["httpSettings"] = h
	default:
		ss["network"] = "tcp"
	}
	if n.TLS {
		ss["security"] = "tls"
		tls := map[string]any{}
		if n.SNI != "" {
			tls["serverName"] = n.SNI
		}
		if len(n.ALPN) > 0 {
			tls["alpn"] = n.ALPN
		}
		if n.Fingerprint != "" {
			tls["fingerprint"] = n.Fingerprint
		}
		ss["tlsSettings"] = tls
	} else {
		ss["security"] = "none"
	}
	return ss
}
