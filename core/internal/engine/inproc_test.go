//go:build mobile || android || ios

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

var sbCfg = `{"log":{"level":"warn"},"inbounds":[{"type":"socks","tag":"socks-in","listen":"127.0.0.1","listen_port":18081}],"outbounds":[{"type":"direct","tag":"direct"}]}`
var xrCfg = `{"log":{"loglevel":"warning"},"inbounds":[{"port":18082,"protocol":"socks","settings":{}}],"outbounds":[{"protocol":"freedom"}]}`

func writeCfg(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInProcessSingBox(t *testing.T) {
	e, err := NewInProcessSingBox()
	if err != nil {
		t.Fatal(err)
	}
	p := writeCfg(t, "sb.json", sbCfg)
	if err := e.Check(p); err != nil {
		t.Fatalf("check: %v", err)
	}
	if err := e.Start(p); err != nil {
		t.Fatalf("start: %v", err)
	}
	if s := e.Snapshot(); s.Status != StatusRunning {
		t.Fatalf("status=%v", s.Status)
	}
	if v, _ := e.Version(); v == "" {
		t.Fatal("empty version")
	}
	if err := e.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if s := e.Snapshot(); s.Status != StatusStopped {
		t.Fatalf("status=%v", s.Status)
	}
	// 非法配置应被 Check 拒绝
	bad := writeCfg(t, "bad.json", `{"inbounds":[{"type":"nope"}]}`)
	if err := e.Check(bad); err == nil {
		t.Fatal("bad config should fail check")
	}
}

func TestInProcessXray(t *testing.T) {
	e, err := NewInProcessXray()
	if err != nil {
		t.Fatal(err)
	}
	p := writeCfg(t, "x.json", xrCfg)
	if err := e.Check(p); err != nil {
		t.Fatalf("check: %v", err)
	}
	if err := e.Reload(p); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if s := e.Snapshot(); s.Status != StatusRunning {
		t.Fatalf("status=%v", s.Status)
	}
	if v, _ := e.Version(); v == "" {
		t.Fatal("empty version")
	}
	if err := e.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	bad := writeCfg(t, "bad.json", `{"inbounds":[{"port":"xx"}]}`)
	if err := e.Check(bad); err == nil {
		t.Fatal("bad config should fail check")
	}
}
