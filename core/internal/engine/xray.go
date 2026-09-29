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

// Xray Xray 内核的 Core 实现。
//
// 与 sing-box 的关键差异：
//   - 二进制查找目标为 xray；
//   - 校验命令为 `xray run -test -c`；
//   - 不支持 hysteria2 / tuic 协议（由 generator.XraySupported 表达，
//     启动前 API 层过滤并提示）。
type Xray struct {
	mu       sync.Mutex
	bin      string
	cmd      *exec.Cmd
	status   Status
	lastErr  string
	logCh    chan string
	stopCh   chan struct{}
	restarts int
}

// NewXray 创建 Xray 内核实例，自动查找二进制。
func NewXray() (*Xray, error) {
	bin, err := findXrayBinary()
	if err != nil {
		return nil, err
	}
	return &Xray{bin: bin, status: StatusStopped, logCh: make(chan string, 512)}, nil
}

func findXrayBinary() (string, error) {
	if exe, err := os.Executable(); err == nil {
		for _, name := range []string{"xray", "xray.exe"} {
			cand := filepath.Join(filepath.Dir(exe), "bin", name)
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return cand, nil
			}
		}
	}
	for _, cand := range []string{"./bin/xray", "bin/xray", "./bin/xray.exe"} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(cand)
			return abs, nil
		}
	}
	if p, err := exec.LookPath("xray"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("未找到 xray 二进制（查找 ./bin/xray 与 PATH）")
}

// Name 内核名称（实现 Core 接口）。
func (x *Xray) Name() string { return KernelXray }

// Binary 返回找到的二进制路径。
func (x *Xray) Binary() string { return x.bin }

// Version 返回 xray 版本。
func (x *Xray) Version() (string, error) {
	out, err := exec.Command(x.bin, "version").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Check 校验配置文件：xray run -test -c。
func (x *Xray) Check(configPath string) error {
	out, err := exec.Command(x.bin, "run", "-test", "-c", configPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xray config 测试失败: %w\n%s", err, out)
	}
	return nil
}

// Start 启动 xray。
func (x *Xray) Start(configPath string) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.status == StatusRunning {
		return fmt.Errorf("引擎已在运行")
	}
	if err := x.Check(configPath); err != nil {
		x.status = StatusError
		x.lastErr = err.Error()
		return err
	}
	cmd := exec.Command(x.bin, "run", "-c", configPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		x.status = StatusError
		x.lastErr = err.Error()
		return fmt.Errorf("启动失败: %w", err)
	}
	x.cmd = cmd
	x.status = StatusRunning
	x.stopCh = make(chan struct{})
	x.lastErr = ""
	go x.pumpLogs(stdout, stderr)
	go x.watchdog(configPath)
	return nil
}

// Stop 优雅停止。
func (x *Xray) Stop() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.status != StatusRunning || x.cmd == nil {
		x.status = StatusStopped
		return nil
	}
	close(x.stopCh)
	if x.cmd.Process != nil {
		_ = x.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { x.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = x.cmd.Process.Kill()
		}
	}
	x.cmd = nil
	x.status = StatusStopped
	return nil
}

// Reload 三段式热加载。
func (x *Xray) Reload(configPath string) error {
	if err := x.Check(configPath); err != nil {
		return err
	}
	_ = x.Stop()
	return x.Start(configPath)
}

func (x *Xray) pumpLogs(stdout, stderr interface{ Read([]byte) (int, error) }) {
	for _, r := range []interface{ Read([]byte) (int, error) }{stdout, stderr} {
		go func(r interface{ Read([]byte) (int, error) }) {
			sc := bufio.NewScanner(r)
			sc.Buffer(make([]byte, 64*1024), 64*1024)
			for sc.Scan() {
				select {
				case x.logCh <- sc.Text():
				default:
				}
			}
		}(r)
	}
}

func (x *Xray) watchdog(configPath string) {
	err := x.cmd.Wait()
	x.mu.Lock()
	defer x.mu.Unlock()
	select {
	case <-x.stopCh:
		return
	default:
	}
	x.restarts++
	x.status = StatusError
	x.lastErr = fmt.Sprintf("xray 异常退出: %v", err)
	x.emit(fmt.Sprintf("[bigcatvd] xray 异常退出，10s 后尝试重启（第 %d 次）", x.restarts))
	go func(n int) {
		time.Sleep(time.Duration(n) * 10 * time.Second)
		x.mu.Lock()
		stopped := x.status != StatusError
		x.mu.Unlock()
		if stopped {
			return
		}
		if err := x.Start(configPath); err != nil {
			x.emit("[bigcatvd] 重启失败: " + err.Error())
		}
	}(x.restarts)
}

func (x *Xray) emit(s string) {
	select {
	case x.logCh <- s:
	default:
	}
}

// Snapshot 状态快照。
func (x *Xray) Snapshot() Snapshot {
	x.mu.Lock()
	defer x.mu.Unlock()
	return Snapshot{Status: x.status, Binary: x.bin, Restarts: x.restarts, LastErr: x.lastErr}
}

// Logs 返回日志通道。
func (x *Xray) Logs() <-chan string { return x.logCh }
