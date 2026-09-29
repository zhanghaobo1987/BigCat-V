package mobile

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatal("Version() 为空")
	}
	if strings.Contains(v, "\n") {
		t.Fatalf("Version() 含换行: %q", v)
	}
}

func TestSetLogLevel(t *testing.T) {
	if err := SetLogLevel("debug"); err != "" {
		t.Fatal(err)
	}
	if LogLevel() != "debug" {
		t.Fatal("LogLevel 应为 debug")
	}
	if err := SetLogLevel("nope"); err == "" {
		t.Fatal("非法级别应返回错误文本")
	}
	if err := SetLogLevel("info"); err != "" {
		t.Fatal(err)
	}
}

func TestStartStop(t *testing.T) {
	if Running() {
		t.Fatal("初始应为未运行")
	}
	if err := Start("127.0.0.1:17891", t.TempDir()); err != "" {
		t.Fatalf("Start: %s", err)
	}
	if !Running() {
		t.Fatal("Start 后应为运行中")
	}
	// 重复启动应拒绝
	if err := Start("127.0.0.1:17892", t.TempDir()); err == "" {
		t.Fatal("重复 Start 应返回错误文本")
	}
	// API 全链路冒烟
	time.Sleep(200 * time.Millisecond)
	resp, err := http.Get("http://127.0.0.1:17891/api/v1/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"version"`) {
		t.Fatalf("status 异常: %d %s", resp.StatusCode, body)
	}
	if err := Stop(); err != "" {
		t.Fatalf("Stop: %s", err)
	}
	if Running() {
		t.Fatal("Stop 后应为未运行")
	}
	// 幂等
	if err := Stop(); err != "" {
		t.Fatalf("重复 Stop: %s", err)
	}
}
