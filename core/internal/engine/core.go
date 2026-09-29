package engine

// 内核名称常量。
const (
	KernelSingBox = "sing-box"
	KernelXray    = "xray"
)

// Core 内核抽象接口。sing-box 与 xray 各自实现，
// 上层（API / UI）只依赖此接口，不感知具体内核差异。
type Core interface {
	// Name 内核名称："sing-box" 或 "xray"。
	Name() string
	// Binary 内核二进制路径。
	Binary() string
	// Version 内核版本输出。
	Version() (string, error)
	// Check 校验配置文件合法性（各内核自带 check 语义）。
	Check(configPath string) error
	// Start 启动数据面（configPath 必须已通过 Check）。
	Start(configPath string) error
	// Stop 优雅停止。
	Stop() error
	// Reload 三段式热加载：check -> stop -> start。
	Reload(configPath string) error
	// Snapshot 状态快照。
	Snapshot() Snapshot
	// Logs 日志流（供事件推送）。
	Logs() <-chan string
}
