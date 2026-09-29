// Package bootstrap bigcatvd 守护进程与移动端绑定的共享启动逻辑。
//
// 抽取自 cmd/bigcatvd/main.go：工作目录准备 -> 内核探测 ->
// Store + API Server 构造 -> 监听并后台服务。
// daemon 与 mobile 包都走这里，保证两端行为一致。
package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"bigcatv/internal/api"
	"bigcatv/internal/engine"
	"bigcatv/internal/geo"
	"bigcatv/internal/routing"
)

// Handle 运行中的核心实例。
type Handle struct {
	Server  *api.Server
	Cores   map[string]engine.Core
	WorkDir string
	Geo     *geo.Updater

	httpSrv *http.Server
	ln      net.Listener
	cancel  context.CancelFunc
}

// resolveWorkDir 解析并创建工作目录（空则默认 ~/.bigcatv）。
func resolveWorkDir(workDir string) (string, error) {
	dir := workDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			dir = "./data"
		} else {
			dir = filepath.Join(home, ".bigcatv")
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建工作目录失败: %w", err)
	}
	return dir, nil
}

// probeCores 探测可用内核：缺失二进制只告警，不致命（API 标记不可用）。
func probeCores() map[string]engine.Core {
	cores := make(map[string]engine.Core)
	if sb, err := engine.New(); err != nil {
		fmt.Fprintln(os.Stderr, "[warn] sing-box 不可用:", err)
	} else {
		cores[engine.KernelSingBox] = sb
		fmt.Println("[ok] sing-box:", sb.Binary())
	}
	if xr, err := engine.NewXray(); err != nil {
		fmt.Fprintln(os.Stderr, "[warn] xray 不可用:", err)
	} else {
		cores[engine.KernelXray] = xr
		fmt.Println("[ok] xray:", xr.Binary())
	}
	return cores
}

// Start 启动控制 API（不阻塞，调用方负责等待退出信号）。
// version 仅用于展示；listenAddr 如 "127.0.0.1:17890"。
func Start(version, listenAddr, workDir string) (*Handle, error) {
	return startWithCores(version, listenAddr, workDir, probeCores())
}

// StartMobile 移动端入口：优先使用进程内引擎（移动端无 fork/exec），
// 其余与 Start 一致。桌面构建下落回进程引擎。
func StartMobile(version, listenAddr, workDir string) (*Handle, error) {
	return startWithCores(version, listenAddr, workDir, probeCoresMobile())
}

func startWithCores(version, listenAddr, workDir string, cores map[string]engine.Core) (*Handle, error) {
	dir, err := resolveWorkDir(workDir)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("监听 %s 失败: %w", listenAddr, err)
	}
	store := api.NewStore(version)
	// 分流规则存储
	routingStore, err := routing.NewStore(dir)
	if err != nil {
		return nil, err
	}
	// geo 资源更新器（xray 通过 XRAY_LOCATION_ASSET 找到 .dat）
	geoDir := filepath.Join(dir, "geo")
	geoUpdater, err := geo.New(geoDir, func(msg string) {
		fmt.Println("[geo]", msg)
	})
	if err != nil {
		return nil, err
	}
	os.Setenv("XRAY_LOCATION_ASSET", geoDir)
	srv := api.NewServer(store, cores, dir, routingStore, geoUpdater)
	ctx, cancel := context.WithCancel(context.Background())
	h := &Handle{Server: srv, Cores: cores, WorkDir: dir, Geo: geoUpdater, ln: ln, cancel: cancel}
	geoUpdater.Start(ctx)
	h.httpSrv = &http.Server{Handler: srv.Handler()}
	go func() {
		_ = h.httpSrv.Serve(ln) // Close 时返回 ErrServerClosed，忽略
	}()
	return h, nil
}

// Addr 返回实际监听地址。
func (h *Handle) Addr() string {
	if h.ln == nil {
		return ""
	}
	return h.ln.Addr().String()
}

// StopEngines 停止所有已启动的内核（幂等）。
func (h *Handle) StopEngines() {
	for _, c := range h.Cores {
		_ = c.Stop()
	}
}

// Close 优雅关闭：先停引擎，再停 geo 更新器与 HTTP 服务。
func (h *Handle) Close() error {
	h.StopEngines()
	if h.cancel != nil {
		h.cancel()
	}
	if h.httpSrv != nil {
		return h.httpSrv.Close()
	}
	return nil
}
