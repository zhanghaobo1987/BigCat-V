// Package generator 将统一节点模型翻译为 sing-box 配置 JSON。
package generator

import (
	"encoding/json"
	"fmt"

	"bigcatv/internal/model"
)

// Options 生成选项。
type Options struct {
	// MixedPort 本地 mixed 入口端口（HTTP+SOCKS5）。
	MixedPort int
	// Tun 是否生成 tun 入口。
	Tun bool
	// TunStack tun 堆栈：gvisor / system。
	TunStack string
	// GeoIPPath / GeositePath 规则数据库路径（为空则跳过相关规则）。
	GeoIPPath   string
	GeositePath string
	// Routing 自定义分流配置（nil=不启用）。
	Routing *model.RoutingConfig
	// GeoDir geo 资源目录（.srs / .dat 存放处）。
	GeoDir string
	// DefaultOutbound 默认出站 tag：节点 ID，或 "proxy"/"auto"。
	DefaultOutbound string
}

func DefaultOptions() Options {
	return Options{MixedPort: 7890, TunStack: "gvisor"}
}

// Generate 生成完整 sing-box 配置（map 形式，可直接 json.Marshal）。
func Generate(nodes []*model.Node, opts Options) (map[string]any, error) {
	if opts.MixedPort == 0 {
		opts.MixedPort = 7890
	}
	var outbounds []any
	var tags []string
	for _, n := range nodes {
		ob, err := outbound(n)
		if err != nil {
			return nil, fmt.Errorf("节点 %q: %w", n.DisplayName(), err)
		}
		outbounds = append(outbounds, ob)
		tags = append(tags, n.ID)
	}
	outbounds = append(outbounds,
		map[string]any{"type": "selector", "tag": "proxy", "outbounds": tags},
		map[string]any{"type": "urltest", "tag": "auto", "outbounds": tags,
			"url": "https://www.gstatic.com/generate_204", "interval": "5m"},
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
		map[string]any{"type": "dns", "tag": "dns-out"},
	)

	inbounds := []any{
		map[string]any{
			"type": "mixed", "tag": "mixed-in",
			"listen": "127.0.0.1", "listen_port": opts.MixedPort,
		},
	}
	if opts.Tun {
		stack := opts.TunStack
		if stack == "" {
			stack = "gvisor"
		}
		inbounds = append(inbounds, map[string]any{
			"type": "tun", "tag": "tun-in",
			"interface_name": "bigcatv0",
			"address":        []string{"172.19.0.1/30"},
			"stack":          stack,
			"auto_route":     true,
			"strict_route":   true,
		})
	}

	final := opts.DefaultOutbound
	if final == "" {
		final = "proxy"
	}
	baseRules := []any{
		map[string]any{"protocol": "dns", "outbound": "dns-out"},
		map[string]any{"ip_cidr": []string{"224.0.0.0/4", "ff00::/8"}, "outbound": "block"},
		map[string]any{"ip_is_private": true, "outbound": "direct"},
	}
	route := map[string]any{"rules": baseRules, "final": final}
	// 自定义分流：用户规则插在 dns 规则之后、内置规则之前，优先于内置直连/拦截
	if opts.Routing != nil {
		ruleSets, custom := singboxRouting(opts.Routing, opts.GeoDir)
		if len(ruleSets) > 0 {
			route["rule_set"] = ruleSets
		}
		merged := []any{baseRules[0]}
		merged = append(merged, custom...)
		merged = append(merged, baseRules[1:]...)
		route["rules"] = merged
		route["final"] = actionTag(opts.Routing.DefaultAction)
	}
	cfg := map[string]any{
		"log":       map[string]any{"level": "info", "timestamp": true},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     route,
	}
	// 可选：geo 规则
	if opts.GeoIPPath != "" {
		cfg["route"].(map[string]any)["geoip"] =
			map[string]any{"path": opts.GeoIPPath}
		rules := cfg["route"].(map[string]any)["rules"].([]any)
		rules = append([]any{
			map[string]any{"geoip": []string{"private", "cn"}, "outbound": "direct"},
		}, rules...)
		cfg["route"].(map[string]any)["rules"] = rules
	}
	return cfg, nil
}

// Marshal 生成并序列化为缩进 JSON。
func Marshal(nodes []*model.Node, opts Options) ([]byte, error) {
	cfg, err := Generate(nodes, opts)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// outbound 单节点 -> sing-box outbound。
func outbound(n *model.Node) (map[string]any, error) {
	ob := map[string]any{"type": string(n.Protocol), "tag": n.ID}
	switch n.Protocol {
	case model.ProtoShadowsocks:
		ob["server"] = n.Server
		ob["server_port"] = n.Port
		ob["method"] = n.Method
		ob["password"] = n.Password
	case model.ProtoVMess:
		ob["server"] = n.Server
		ob["server_port"] = n.Port
		ob["uuid"] = n.UUID
		ob["security"] = "auto"
		applyTLS(ob, n)
		applyTransport(ob, n)
	case model.ProtoVLESS:
		ob["server"] = n.Server
		ob["server_port"] = n.Port
		ob["uuid"] = n.UUID
		if n.Flow != "" {
			ob["flow"] = n.Flow
		}
		applyTLS(ob, n)
		applyTransport(ob, n)
	case model.ProtoTrojan:
		ob["server"] = n.Server
		ob["server_port"] = n.Port
		ob["password"] = n.Password
		applyTLS(ob, n)
		applyTransport(ob, n)
	case model.ProtoHysteria2:
		ob["server"] = n.Server
		ob["server_port"] = n.Port
		ob["password"] = n.Password
		if n.Obfs != "" {
			ob["obfs"] = map[string]any{"type": n.Obfs, "password": n.ObfsPassword}
		}
		applyTLS(ob, n)
		if n.UpMbps > 0 {
			ob["up_mbps"] = n.UpMbps
		}
		if n.DownMbps > 0 {
			ob["down_mbps"] = n.DownMbps
		}
	case model.ProtoTUIC:
		ob["server"] = n.Server
		ob["server_port"] = n.Port
		ob["uuid"] = n.UUID
		ob["password"] = n.Password
		applyTLS(ob, n)
	case model.ProtoWireGuard:
		ob["server"] = n.Server
		ob["server_port"] = n.Port
		ob["private_key"] = n.PrivateKey
		ob["peer_public_key"] = n.PublicKey
		if n.PreSharedKey != "" {
			ob["pre_shared_key"] = n.PreSharedKey
		}
		if len(n.LocalAddress) > 0 {
			ob["local_address"] = n.LocalAddress
		}
		if n.MTU > 0 {
			ob["mtu"] = n.MTU
		}
		if len(n.Reserved) == 3 {
			ob["reserved"] = n.Reserved
		}
	case model.ProtoSOCKS:
		ob["server"] = n.Server
		ob["server_port"] = n.Port
		ob["version"] = "5"
		if n.Username != "" {
			ob["username"] = n.Username
			ob["password"] = n.Password
		}
	case model.ProtoHTTP:
		ob["server"] = n.Server
		ob["server_port"] = n.Port
		if n.Username != "" {
			ob["username"] = n.Username
			ob["password"] = n.Password
		}
		if n.TLS {
			ob["tls"] = map[string]any{"enabled": true, "server_name": n.SNI}
		}
	default:
		return nil, fmt.Errorf("不支持生成协议 %q", n.Protocol)
	}
	return ob, nil
}

func applyTLS(ob map[string]any, n *model.Node) {
	if !n.TLS {
		return
	}
	tls := map[string]any{"enabled": true}
	if n.SNI != "" {
		tls["server_name"] = n.SNI
	}
	if len(n.ALPN) > 0 {
		tls["alpn"] = n.ALPN
	}
	if n.Fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": n.Fingerprint}
	}
	// hysteria2/tuic 默认不校验证书交由 sing-box 处理；此处保持默认行为
	ob["tls"] = tls
}

func applyTransport(ob map[string]any, n *model.Node) {
	switch n.Network {
	case model.NetWS:
		tr := map[string]any{"type": "ws"}
		if n.WsPath != "" {
			tr["path"] = n.WsPath
		}
		if n.WsHost != "" {
			tr["headers"] = map[string]any{"Host": n.WsHost}
		}
		ob["transport"] = tr
	case model.NetGRPC:
		tr := map[string]any{"type": "grpc"}
		if n.GrpcService != "" {
			tr["service_name"] = n.GrpcService
		}
		ob["transport"] = tr
	case model.NetH2:
		ob["transport"] = map[string]any{"type": "http"}
	}
}
