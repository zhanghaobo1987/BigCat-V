package generator

import (
	"encoding/json"
	"testing"

	"bigcatv/internal/model"
)

func sampleNodes() []*model.Node {
	nodes := []*model.Node{
		{Protocol: model.ProtoShadowsocks, Name: "ss", Server: "127.0.0.1", Port: 8388, Method: "aes-256-gcm", Password: "pw"},
		{Protocol: model.ProtoVLESS, Name: "vless", Server: "127.0.0.1", Port: 443, UUID: "b831696f-3b6b-4b6b-8b6b-1b6b6b6b6b6b", TLS: true, SNI: "example.com", Network: model.NetWS, WsPath: "/vless"},
		{Protocol: model.ProtoHysteria2, Name: "hy2", Server: "127.0.0.1", Port: 443, Password: "pw", TLS: true, SNI: "example.com"},
	}
	for _, n := range nodes {
		n.Normalize()
	}
	return nodes
}

func TestGenerateSingBox(t *testing.T) {
	data, err := Marshal(sampleNodes(), DefaultOptions())
	if err != nil {
		t.Fatalf("sing-box 生成失败: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("生成的不是合法 JSON: %v", err)
	}
	obs := cfg["outbounds"].([]any)
	// 3 节点 + selector + urltest + direct + block + dns = 8
	if len(obs) != 8 {
		t.Fatalf("期望 8 个 outbounds，得到 %d", len(obs))
	}
}

func TestGenerateXray(t *testing.T) {
	nodes := sampleNodes()[:2] // 去掉 hy2
	data, err := MarshalXray(nodes, DefaultOptions())
	if err != nil {
		t.Fatalf("xray 生成失败: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("生成的不是合法 JSON: %v", err)
	}
	obs := cfg["outbounds"].([]any)
	if len(obs) != 4 { // 2 节点 + direct + block
		t.Fatalf("期望 4 个 outbounds，得到 %d", len(obs))
	}
	if cfg["routing"] == nil {
		t.Fatal("缺少 routing")
	}
}

func TestGenerateXrayRejectsHy2(t *testing.T) {
	_, err := MarshalXray(sampleNodes(), DefaultOptions())
	if err == nil {
		t.Fatal("xray 生成器应拒绝 hysteria2 节点")
	}
}

func TestXraySupportedMatrix(t *testing.T) {
	cases := map[model.Protocol]bool{
		model.ProtoShadowsocks: true,
		model.ProtoVMess:       true,
		model.ProtoVLESS:       true,
		model.ProtoTrojan:      true,
		model.ProtoWireGuard:   true,
		model.ProtoSOCKS:       true,
		model.ProtoHTTP:        true,
		model.ProtoHysteria2:   false,
		model.ProtoTUIC:        false,
	}
	for p, want := range cases {
		if got := XraySupported(p); got != want {
			t.Fatalf("XraySupported(%s) = %v，期望 %v", p, got, want)
		}
	}
}
