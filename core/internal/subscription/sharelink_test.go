package subscription

import (
	"encoding/base64"
	"strings"
	"testing"

	"bigcatv/internal/model"
)

func mustParse(t *testing.T, link string) *model.Node {
	t.Helper()
	n, err := ParseShareLink(link)
	if err != nil {
		t.Fatalf("解析失败 %q: %v", link, err)
	}
	return n
}

func TestParseSS(t *testing.T) {
	// ss://base64(method:password)@host:port#name
	n := mustParse(t, "ss://YWVzLTI1Ni1nY206cGFzc3dvcmQxMjM=@127.0.0.1:8388#SS%E6%B5%8B%E8%AF%95")
	if n.Protocol != model.ProtoShadowsocks || n.Method != "aes-256-gcm" || n.Password != "password123" {
		t.Fatalf("ss 解析错误: %+v", n)
	}
	if n.Port != 8388 || n.Name != "SS测试" {
		t.Fatalf("ss host/port/name 错误: %+v", n)
	}
}

func TestParseVMess(t *testing.T) {
	raw := `{"ps":"vmess测试","add":"127.0.0.1","port":"10086","id":"b831696f-3b6b-4b6b-8b6b-1b6b6b6b6b6b","aid":"0","net":"ws","type":"none","host":"example.com","path":"/vmess","tls":"tls","sni":"example.com"}`
	link := "vmess://" + base64.StdEncoding.EncodeToString([]byte(raw))
	n := mustParse(t, link)
	if n.Protocol != model.ProtoVMess || n.Network != model.NetWS || !n.TLS {
		t.Fatalf("vmess 解析错误: %+v", n)
	}
	if n.WsPath != "/vmess" || n.WsHost != "example.com" || n.SNI != "example.com" {
		t.Fatalf("vmess 传输层错误: %+v", n)
	}
}

func TestParseVLESS(t *testing.T) {
	n := mustParse(t, "vless://b831696f-3b6b-4b6b-8b6b-1b6b6b6b6b6b@127.0.0.1:443?encryption=none&security=tls&sni=example.com&type=ws&path=%2Fvless&flow=xtls-rprx-vision#VLESS%E6%B5%8B%E8%AF%95")
	if n.Protocol != model.ProtoVLESS || n.Flow != "xtls-rprx-vision" || !n.TLS {
		t.Fatalf("vless 解析错误: %+v", n)
	}
	if n.WsPath != "/vless" || n.SNI != "example.com" {
		t.Fatalf("vless 传输层错误: %+v", n)
	}
}

func TestParseTrojan(t *testing.T) {
	n := mustParse(t, "trojan://mypassword@127.0.0.1:443?sni=example.com&type=grpc&serviceName=gun#Trojan%E6%B5%8B%E8%AF%95")
	if n.Protocol != model.ProtoTrojan || n.Password != "mypassword" || !n.TLS {
		t.Fatalf("trojan 解析错误: %+v", n)
	}
	if n.Network != model.NetGRPC || n.GrpcService != "gun" {
		t.Fatalf("trojan grpc 错误: %+v", n)
	}
}

func TestParseHysteria2(t *testing.T) {
	n := mustParse(t, "hysteria2://hy2pass@127.0.0.1:443?sni=example.com&obfs=salamander&obfs-password=obfspw#HY2%E6%B5%8B%E8%AF%95")
	if n.Protocol != model.ProtoHysteria2 || n.Obfs != "salamander" || n.ObfsPassword != "obfspw" {
		t.Fatalf("hysteria2 解析错误: %+v", n)
	}
}

func TestParseTUIC(t *testing.T) {
	n := mustParse(t, "tuic://b831696f-3b6b-4b6b-8b6b-1b6b6b6b6b6b:tuicpass@127.0.0.1:443?sni=example.com&alpn=h3#TUIC%E6%B5%8B%E8%AF%95")
	if n.Protocol != model.ProtoTUIC || n.UUID == "" || n.Password != "tuicpass" {
		t.Fatalf("tuic 解析错误: %+v", n)
	}
}

func TestParseBatch(t *testing.T) {
	text := strings.Join([]string{
		"# 注释行",
		"",
		"ss://YWVzLTI1Ni1nY206cGFzc3dvcmQxMjM=@127.0.0.1:8388#SS1",
		"trojan://pw@127.0.0.1:443#TJ1",
		"not-a-link",
	}, "\n")
	nodes, errs := ParseBatch(text)
	if len(nodes) != 2 {
		t.Fatalf("期望 2 个节点，得到 %d（errs=%v）", len(nodes), errs)
	}
	if len(errs) != 1 {
		t.Fatalf("期望 1 个错误，得到 %v", errs)
	}
	// ID 稳定性：同一链接两次解析 ID 相同
	a := mustParse(t, "ss://YWVzLTI1Ni1nY206cGFzc3dvcmQxMjM=@127.0.0.1:8388#SS1")
	b := mustParse(t, "ss://YWVzLTI1Ni1nY206cGFzc3dvcmQxMjM=@127.0.0.1:8388#SS1")
	if a.ID != b.ID {
		t.Fatalf("节点 ID 不稳定: %s vs %s", a.ID, b.ID)
	}
}

func TestParseClash(t *testing.T) {
	yml := `
proxies:
  - name: "clash-ss"
    type: ss
    server: 127.0.0.1
    port: 8388
    cipher: aes-256-gcm
    password: "password123"
  - name: "clash-vless"
    type: vless
    server: 127.0.0.1
    port: 443
    uuid: b831696f-3b6b-4b6b-8b6b-1b6b6b6b6b6b
    tls: true
    sni: example.com
    network: ws
    ws-opts:
      path: /vless
      headers:
        Host: example.com
`
	nodes, err := ParseClash([]byte(yml))
	if err != nil {
		t.Fatalf("clash 解析失败: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("期望 2 个节点，得到 %d", len(nodes))
	}
	if nodes[0].Protocol != model.ProtoShadowsocks || nodes[1].Protocol != model.ProtoVLESS {
		t.Fatalf("clash 协议映射错误: %+v", nodes)
	}
	if nodes[1].WsPath != "/vless" || nodes[1].WsHost != "example.com" {
		t.Fatalf("clash ws-opts 错误: %+v", nodes[1])
	}
}

func TestSniffBase64Batch(t *testing.T) {
	inner := "ss://YWVzLTI1Ni1nY206cGFzc3dvcmQxMjM=@127.0.0.1:8388#SS1\n"
	batch := base64.StdEncoding.EncodeToString([]byte(inner))
	nodes, err := SniffAndParse([]byte(batch))
	if err != nil {
		t.Fatalf("嗅探解析失败: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("期望 1 个节点，得到 %d", len(nodes))
	}
}
