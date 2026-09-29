// Package model 定义 BigCat V 的统一节点模型（内部 IR）。
//
// 所有输入（分享链接、Clash YAML、sing-box JSON、手动录入）先归一化为 Node，
// 再由 generator 翻译为 sing-box outbound。新增协议只需扩展 parser 与
// generator 分支，不动 API 与 UI。
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Protocol 代理协议。
type Protocol string

const (
	ProtoShadowsocks Protocol = "shadowsocks"
	ProtoVMess       Protocol = "vmess"
	ProtoVLESS       Protocol = "vless"
	ProtoTrojan      Protocol = "trojan"
	ProtoHysteria2   Protocol = "hysteria2"
	ProtoTUIC        Protocol = "tuic"
	ProtoWireGuard   Protocol = "wireguard"
	ProtoSOCKS       Protocol = "socks"
	ProtoHTTP        Protocol = "http"
)

// Network 传输网络。
type Network string

const (
	NetTCP  Network = "tcp"
	NetWS   Network = "ws"
	NetGRPC Network = "grpc"
	NetH2   Network = "h2"
	NetHTTP Network = "http"
	NetQUIC Network = "quic"
)

// Node 统一节点模型。
type Node struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Protocol Protocol `json:"protocol"`
	Server   string   `json:"server"`
	Port     int      `json:"port"`

	// 认证
	UUID     string `json:"uuid,omitempty"`     // vmess / vless / trojan / tuic
	Password string `json:"password,omitempty"` // ss / trojan / hysteria2 / tuic / socks / http
	Method   string `json:"method,omitempty"`   // shadowsocks 加密方式
	Username string `json:"username,omitempty"` // socks / http

	// 传输层
	Network     Network  `json:"network,omitempty"`
	TLS         bool     `json:"tls,omitempty"`
	SNI         string   `json:"sni,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"` // uTLS 指纹
	WsPath      string   `json:"ws_path,omitempty"`
	WsHost      string   `json:"ws_host,omitempty"`
	GrpcService string   `json:"grpc_service,omitempty"`
	Flow        string   `json:"flow,omitempty"` // vless xtls-rprx-vision 等

	// hysteria2 / tuic 特有
	Obfs         string `json:"obfs,omitempty"`
	ObfsPassword string `json:"obfs_password,omitempty"`
	UpMbps       int    `json:"up_mbps,omitempty"`
	DownMbps     int    `json:"down_mbps,omitempty"`

	// wireguard 特有
	PrivateKey   string   `json:"private_key,omitempty"`
	PublicKey    string   `json:"public_key,omitempty"`
	PreSharedKey string   `json:"psk,omitempty"`
	LocalAddress []string `json:"local_address,omitempty"`
	MTU          int      `json:"mtu,omitempty"`
	Reserved     []int    `json:"reserved,omitempty"`

	// 元信息
	Group     string `json:"group,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"` // -1=未测/失败，>=0 毫秒
	Raw       string `json:"raw,omitempty"`        // 原始分享链接（可回放）
}

// Normalize 补全派生字段：空 Name 用 server:port，计算稳定 ID。
func (n *Node) Normalize() {
	n.Server = strings.TrimSpace(n.Server)
	if n.Name == "" {
		n.Name = fmt.Sprintf("%s:%d", n.Server, n.Port)
	}
	if n.SNI == "" && n.TLS {
		n.SNI = n.Server
	}
	n.ID = n.HashID()
}

// HashID 基于归一化关键字段的稳定哈希，用于订阅更新去重。
func (n *Node) HashID() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%s|%s|%s|%s|%s",
		n.Protocol, n.Server, n.Port,
		n.UUID, n.Password, n.Method, n.Network, n.SNI)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum)[:16]
}

// DisplayName UI 展示名。
func (n *Node) DisplayName() string {
	if n.Group != "" {
		return fmt.Sprintf("[%s] %s", n.Group, n.Name)
	}
	return n.Name
}

// Validate 基础合法性检查。
func (n *Node) Validate() error {
	if n.Server == "" {
		return fmt.Errorf("server 为空")
	}
	if n.Port <= 0 || n.Port > 65535 {
		return fmt.Errorf("端口 %d 非法", n.Port)
	}
	switch n.Protocol {
	case ProtoShadowsocks:
		if n.Method == "" || n.Password == "" {
			return fmt.Errorf("shadowsocks 需要 method 与 password")
		}
	case ProtoVMess, ProtoVLESS:
		if n.UUID == "" {
			return fmt.Errorf("%s 需要 uuid", n.Protocol)
		}
	case ProtoTrojan:
		if n.Password == "" {
			return fmt.Errorf("trojan 需要 password")
		}
	case ProtoHysteria2, ProtoTUIC:
		if n.Password == "" && n.UUID == "" {
			return fmt.Errorf("%s 需要 password/uuid", n.Protocol)
		}
	case ProtoWireGuard:
		if n.PrivateKey == "" || n.PublicKey == "" {
			return fmt.Errorf("wireguard 需要 private_key 与 peer public_key")
		}
	case ProtoSOCKS, ProtoHTTP:
		// 无需认证也可
	default:
		return fmt.Errorf("未知协议 %q", n.Protocol)
	}
	return nil
}
