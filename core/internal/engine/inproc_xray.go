//go:build mobile || android || ios

package engine

import (
	"bytes"
	"fmt"
	"os"
	"sync"

	xcore "github.com/xtls/xray-core/core"
	_ "github.com/xtls/xray-core/main/distro/all" // 注册全部入站/出站/传输组件
	_ "github.com/xtls/xray-core/main/json"       // 注册 JSON 配置加载器
)

// InProcessXray Xray 内核的进程内实现（移动端用：iOS/Android 禁止 fork/exec）。
//
// 与进程版 Xray 的差异：
//   - 不依赖外部二进制，直接以 Go library 方式运行（core.New/Start/Close）；
//   - Binary() 返回 "in-process"；
//   - 无崩溃退避重启：移动端由 App 层管理生命周期，异常退出只记录状态；
//   - Check 通过 LoadConfig 做等价校验（与 `xray run -test` 同源解析逻辑）。
type InProcessXray struct {
	mu      sync.Mutex
	inst    *xcore.Instance
	status  Status
	lastErr string
	logCh   chan string
}

// NewInProcessXray 创建进程内 Xray 内核。
func NewInProcessXray() (*InProcessXray, error) {
	return &InProcessXray{status: StatusStopped, logCh: make(chan string, 512)}, nil
}

// Name 内核名称（实现 Core 接口）。
func (x *InProcessXray) Name() string { return KernelXray }

// Binary 进程内实现无二进制路径。
func (x *InProcessXray) Binary() string { return "in-process" }

// Version 返回 Xray-core 库版本。
func (x *InProcessXray) Version() (string, error) { return xcore.Version(), nil }

// loadConfig 读取并解析 JSON 配置（Check 与 Start 共用）。
func loadConfig(configPath string) (*xcore.Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	cfg, err := xcore.LoadConfig("json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("xray 配置解析失败: %w", err)
	}
	return cfg, nil
}

// Check 校验配置文件合法性。
func (x *InProcessXray) Check(configPath string) error {
	_, err := loadConfig(configPath)
	return err
}

// Start 启动数据面（configPath 必须已通过 Check）。
func (x *InProcessXray) Start(configPath string) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.status == StatusRunning {
		return fmt.Errorf("引擎已在运行")
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		x.status = StatusError
		x.lastErr = err.Error()
		return err
	}
	inst, err := xcore.New(cfg)
	if err != nil {
		x.status = StatusError
		x.lastErr = err.Error()
		return fmt.Errorf("xray 实例创建失败: %w", err)
	}
	if err := inst.Start(); err != nil {
		x.status = StatusError
		x.lastErr = err.Error()
		_ = inst.Close()
		return fmt.Errorf("xray 启动失败: %w", err)
	}
	x.inst = inst
	x.status = StatusRunning
	x.lastErr = ""
	x.emit("[bigcatv] xray 进程内实例已启动")
	return nil
}

// Stop 优雅停止。
func (x *InProcessXray) Stop() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.status != StatusRunning || x.inst == nil {
		x.status = StatusStopped
		return nil
	}
	if err := x.inst.Close(); err != nil {
		x.lastErr = err.Error()
	}
	x.inst = nil
	x.status = StatusStopped
	x.emit("[bigcatv] xray 进程内实例已停止")
	return nil
}

// Reload 三段式热加载：check -> stop -> start。
func (x *InProcessXray) Reload(configPath string) error {
	if err := x.Check(configPath); err != nil {
		return err
	}
	_ = x.Stop()
	return x.Start(configPath)
}

func (x *InProcessXray) emit(s string) {
	select {
	case x.logCh <- s:
	default:
	}
}

// Snapshot 状态快照。
func (x *InProcessXray) Snapshot() Snapshot {
	x.mu.Lock()
	defer x.mu.Unlock()
	return Snapshot{Status: x.status, Binary: x.Binary(), LastErr: x.lastErr}
}

// Logs 返回日志通道。
func (x *InProcessXray) Logs() <-chan string { return x.logCh }
