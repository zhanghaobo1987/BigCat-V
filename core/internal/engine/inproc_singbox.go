//go:build mobile || android || ios

package engine

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

// InProcessSingBox sing-box 内核的进程内实现（移动端用：iOS/Android 禁止 fork/exec）。
//
// 实现要点（与 `sing-box run/check` 同源逻辑）：
//   - 解析：include.Context 注入全部入站/出站/端点注册表，再
//     UnmarshalExtendedContext[option.Options]（CLI cmd_run.go 相同路径）；
//   - 校验：等价于 `sing-box check`（New + Close，不 Start）；
//   - 运行：box.New(box.Options{Context, Options}) + Start，停止时 Close + cancel。
//
// 与进程版 Engine 的差异：
//   - 不依赖外部二进制；Binary() 返回 "in-process"；
//   - 无崩溃退避重启（移动端由 App 层管理生命周期）；
//   - 日志走 sing-box 默认输出（stderr），暂未接入 logCh（框架阶段，见文档）。
type InProcessSingBox struct {
	mu      sync.Mutex
	inst    *box.Box
	cancel  context.CancelFunc
	status  Status
	lastErr string
	logCh   chan string
}

// singBoxLibVersion 引入的 sing-box 库版本（与 go.mod 一致）。
const singBoxLibVersion = "v1.12.16"

// NewInProcessSingBox 创建进程内 sing-box 内核。
func NewInProcessSingBox() (*InProcessSingBox, error) {
	return &InProcessSingBox{status: StatusStopped, logCh: make(chan string, 512)}, nil
}

// Name 内核名称（实现 Core 接口）。
func (s *InProcessSingBox) Name() string { return KernelSingBox }

// Binary 进程内实现无二进制路径。
func (s *InProcessSingBox) Binary() string { return "in-process" }

// Version 返回 sing-box 库版本。
func (s *InProcessSingBox) Version() (string, error) {
	return "sing-box " + singBoxLibVersion + " (in-process library)", nil
}

// parseOptions 读取并解析 sing-box JSON 配置。
func parseOptions(configPath string) (option.Options, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return option.Options{}, fmt.Errorf("读取配置失败: %w", err)
	}
	ctx := include.Context(context.Background())
	opts, err := singjson.UnmarshalExtendedContext[option.Options](ctx, data)
	if err != nil {
		return option.Options{}, fmt.Errorf("sing-box 配置解析失败: %w", err)
	}
	return opts, nil
}

// Check 校验配置文件合法性（等价 `sing-box check`）。
func (s *InProcessSingBox) Check(configPath string) error {
	opts, err := parseOptions(configPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(include.Context(context.Background()))
	defer cancel()
	inst, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		return fmt.Errorf("sing-box check 失败: %w", err)
	}
	inst.Close()
	return nil
}

// Start 启动数据面（configPath 必须已通过 Check）。
func (s *InProcessSingBox) Start(configPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == StatusRunning {
		return fmt.Errorf("引擎已在运行")
	}
	opts, err := parseOptions(configPath)
	if err != nil {
		s.status = StatusError
		s.lastErr = err.Error()
		return err
	}
	ctx, cancel := context.WithCancel(include.Context(context.Background()))
	inst, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		cancel()
		s.status = StatusError
		s.lastErr = err.Error()
		return fmt.Errorf("sing-box 实例创建失败: %w", err)
	}
	if err := inst.Start(); err != nil {
		cancel()
		inst.Close()
		s.status = StatusError
		s.lastErr = err.Error()
		return fmt.Errorf("sing-box 启动失败: %w", err)
	}
	s.inst = inst
	s.cancel = cancel
	s.status = StatusRunning
	s.lastErr = ""
	s.emit("[bigcatv] sing-box 进程内实例已启动")
	return nil
}

// Stop 优雅停止。
func (s *InProcessSingBox) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != StatusRunning || s.inst == nil {
		s.status = StatusStopped
		return nil
	}
	s.inst.Close()
	if s.cancel != nil {
		s.cancel()
	}
	s.inst = nil
	s.cancel = nil
	s.status = StatusStopped
	s.emit("[bigcatv] sing-box 进程内实例已停止")
	return nil
}

// Reload 三段式热加载：check -> stop -> start。
func (s *InProcessSingBox) Reload(configPath string) error {
	if err := s.Check(configPath); err != nil {
		return err
	}
	_ = s.Stop()
	return s.Start(configPath)
}

func (s *InProcessSingBox) emit(line string) {
	select {
	case s.logCh <- line:
	default:
	}
}

// Snapshot 状态快照。
func (s *InProcessSingBox) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{Status: s.status, Binary: s.Binary(), LastErr: s.lastErr}
}

// Logs 返回日志通道。
func (s *InProcessSingBox) Logs() <-chan string { return s.logCh }
