// Package engine 管理 sing-box 数据面进程的生命周期：
// 查找二进制 -> check 校验 -> run 启动 -> 日志采集 -> 优雅停止 / 崩溃重启。
package engine

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Status 引擎状态。
type Status string

const (
	StatusStopped Status = "stopped"
	StatusRunning Status = "running"
	StatusError   Status = "error"
)

// Engine sing-box 进程管理器。
type Engine struct {
	mu       sync.Mutex
	bin      string
	cmd      *exec.Cmd
	status   Status
	lastErr  string
	logCh    chan string
	stopCh   chan struct{}
	restarts int
}

// New 创建 Engine，自动查找 sing-box 二进制。
func New() (*Engine, error) {
	bin, err := findBinary()
	if err != nil {
		return nil, err
	}
	return &Engine{bin: bin, status: StatusStopped, logCh: make(chan string, 512)}, nil
}

// findBinary 查找顺序：./bin/sing-box -> $PATH。
func findBinary() (string, error) {
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "bin", "sing-box")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	for _, cand := range []string{"./bin/sing-box", "bin/sing-box"} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(cand)
			return abs, nil
		}
	}
	if p, err := exec.LookPath("sing-box"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("未找到 sing-box 二进制（查找 ./bin/sing-box 与 PATH）")
}

// Binary 返回找到的二进制路径。
func (e *Engine) Binary() string { return e.bin }

// Name 内核名称（实现 Core 接口）。
func (e *Engine) Name() string { return KernelSingBox }

// Version 返回 sing-box 版本。
func (e *Engine) Version() (string, error) {
	out, err := exec.Command(e.bin, "version").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Check 校验配置文件合法性。
func (e *Engine) Check(configPath string) error {
	out, err := exec.Command(e.bin, "check", "-c", configPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sing-box check 失败: %w\n%s", err, out)
	}
	return nil
}

// Start 启动 sing-box（configPath 必须已通过 Check）。
func (e *Engine) Start(configPath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.status == StatusRunning {
		return fmt.Errorf("引擎已在运行")
	}
	if err := e.Check(configPath); err != nil {
		e.status = StatusError
		e.lastErr = err.Error()
		return err
	}
	cmd := exec.Command(e.bin, "run", "-c", configPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		e.status = StatusError
		e.lastErr = err.Error()
		return fmt.Errorf("启动失败: %w", err)
	}
	e.cmd = cmd
	e.status = StatusRunning
	e.stopCh = make(chan struct{})
	e.lastErr = ""
	go e.pumpLogs(stdout, stderr)
	go e.watchdog(configPath)
	return nil
}

// Stop 优雅停止。
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.status != StatusRunning || e.cmd == nil {
		e.status = StatusStopped
		return nil
	}
	close(e.stopCh)
	if e.cmd.Process != nil {
		_ = e.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { e.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = e.cmd.Process.Kill()
		}
	}
	e.cmd = nil
	e.status = StatusStopped
	return nil
}

// Reload 三段式热加载：check -> stop -> start。
func (e *Engine) Reload(configPath string) error {
	if err := e.Check(configPath); err != nil {
		return err
	}
	_ = e.Stop()
	return e.Start(configPath)
}

func (e *Engine) pumpLogs(stdout, stderr interface{ Read([]byte) (int, error) }) {
	// 分别泵 stdout / stderr；简化实现共用 reader 接口
	for _, r := range []interface{ Read([]byte) (int, error) }{stdout, stderr} {
		go func(r interface{ Read([]byte) (int, error) }) {
			sc := bufio.NewScanner(r)
			sc.Buffer(make([]byte, 64*1024), 64*1024)
			for sc.Scan() {
				select {
				case e.logCh <- sc.Text():
				default:
				}
			}
		}(r)
	}
}

// watchdog 崩溃后退避重启。
func (e *Engine) watchdog(configPath string) {
	err := e.cmd.Wait()
	e.mu.Lock()
	defer e.mu.Unlock()
	select {
	case <-e.stopCh:
		return // 主动停止
	default:
	}
	e.restarts++
	e.status = StatusError
	e.lastErr = fmt.Sprintf("sing-box 异常退出: %v", err)
	e.emit(fmt.Sprintf("[bigcatvd] sing-box 异常退出，10s 后尝试重启（第 %d 次）", e.restarts))
	go func(n int) {
		time.Sleep(time.Duration(n) * 10 * time.Second)
		e.mu.Lock()
		stopped := e.status != StatusError
		e.mu.Unlock()
		if stopped {
			return
		}
		if err := e.Start(configPath); err != nil {
			e.emit("[bigcatvd] 重启失败: " + err.Error())
		}
	}(e.restarts)
}

func (e *Engine) emit(s string) {
	select {
	case e.logCh <- s:
	default:
	}
}

// Snapshot 状态快照。
type Snapshot struct {
	Status   Status `json:"status"`
	Binary   string `json:"binary"`
	Restarts int    `json:"restarts"`
	LastErr  string `json:"last_error,omitempty"`
}

func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return Snapshot{Status: e.status, Binary: e.bin, Restarts: e.restarts, LastErr: e.lastErr}
}

// Logs 返回日志通道（供 WS 推送）。
func (e *Engine) Logs() <-chan string { return e.logCh }
