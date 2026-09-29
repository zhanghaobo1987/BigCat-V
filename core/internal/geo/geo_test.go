package geo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Update 下载期间，Status/SetAutoUpdate 不应被阻塞。
func TestUpdateDoesNotBlockStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Write([]byte("slow"))
	}))
	defer srv.Close()

	u := &Updater{
		dir:    t.TempDir(),
		assets: []Asset{{Name: "slow.srs", URL: srv.URL}},
		client: &http.Client{Timeout: 10 * time.Second},
		set:    Settings{AutoUpdate: true, IntervalHours: 24, Files: map[string]*FileMeta{}},
	}
	u.set.Files["slow.srs"] = &FileMeta{Name: "slow.srs", URL: srv.URL}

	done := make(chan error, 1)
	go func() { done <- u.Update(context.Background(), false) }()
	time.Sleep(100 * time.Millisecond) // 确保 Update 已进入下载

	start := time.Now()
	st := u.Status()
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("Status 被下载阻塞了 %v", elapsed)
	}
	if len(st.Files) != 1 {
		t.Fatalf("状态文件数错误: %d", len(st.Files))
	}
	if err := <-done; err != nil {
		t.Fatalf("Update 应成功: %v", err)
	}
	if !u.Has("slow.srs") {
		t.Fatal("下载后文件应存在")
	}
}

// 并发 Update 应串行化，不会重复下载。
func TestConcurrentUpdateSerialized(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Write([]byte("x"))
	}))
	defer srv.Close()

	u := &Updater{
		dir:    t.TempDir(),
		assets: []Asset{{Name: "a.srs", URL: srv.URL}},
		client: &http.Client{Timeout: 10 * time.Second},
		set:    Settings{AutoUpdate: true, IntervalHours: 24, Files: map[string]*FileMeta{}},
	}
	u.set.Files["a.srs"] = &FileMeta{Name: "a.srs", URL: srv.URL}

	done := make(chan struct{})
	go func() { _ = u.Update(context.Background(), false); close(done) }()
	if err := u.Update(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	<-done
	if count != 2 {
		t.Fatalf("期望串行下载 2 次，实际 %d", count)
	}
}
