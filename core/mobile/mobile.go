// Package mobile gomobile 绑定：把 BigCat V 核心编译进移动端应用。
//
// 构建：
//  1. 安装 gomobile：go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
//  2. Android：gomobile bind -o bigcatv.aar -target android bigcatv/mobile
//  3. iOS：gomobile bind -o Bigcatv.xcframework -target ios bigcatv/mobile
//
// 约束（gomobile）：只导出基础类型（string/int/bool），方法集保持最小。
// 运行时：Start 在进程内启动控制 API（127.0.0.1:17890），
// App 侧用 HTTP 客户端调同一套 REST 接口；引擎走进程内实现（见 engine 包 mobile 构建）。
package mobile

import (
	"sync"
	"sync/atomic"

	bigcatv "bigcatv"
	"bigcatv/internal/bootstrap"
)

var (
	mu     sync.Mutex
	handle *bootstrap.Handle
	level  atomic.Int32 // 0=debug 1=info 2=warn 3=error，默认 info
)

func init() { level.Store(1) }

// Start 在进程内启动控制 API。listenAddr 如 "127.0.0.1:17890"，
// workDir 为空则使用 App 私有目录（调用方应传入确定路径）。
// 成功返回 ""，失败返回错误文本。
func Start(listenAddr, workDir string) string {
	mu.Lock()
	defer mu.Unlock()
	if handle != nil {
		return "bigcatv 已在运行"
	}
	if listenAddr == "" {
		return "listenAddr 不能为空"
	}
	h, err := bootstrap.StartMobile(bigcatv.Version(), listenAddr, workDir)
	if err != nil {
		return err.Error()
	}
	handle = h
	return ""
}

// Stop 停止引擎与 API 并释放资源。成功返回 ""，失败返回错误文本。
func Stop() string {
	mu.Lock()
	defer mu.Unlock()
	if handle == nil {
		return ""
	}
	err := handle.Close()
	handle = nil
	if err != nil {
		return err.Error()
	}
	return ""
}

// Running 报告核心是否在运行。
func Running() bool {
	mu.Lock()
	defer mu.Unlock()
	return handle != nil
}

// Version 返回核心版本号（core/VERSION，编译时嵌入）。
func Version() string {
	return bigcatv.Version()
}

// SetLogLevel 设置日志级别：debug/info/warn/error。
// 成功返回 ""；非法值返回错误文本。
// 注：级别当前仅存储，供后续日志系统消费（v0.3 框架阶段）。
func SetLogLevel(lv string) string {
	switch lv {
	case "debug":
		level.Store(0)
	case "info":
		level.Store(1)
	case "warn":
		level.Store(2)
	case "error":
		level.Store(3)
	default:
		return "非法日志级别: " + lv + "（可选 debug/info/warn/error）"
	}
	return ""
}

// LogLevel 返回当前日志级别。
func LogLevel() string {
	switch level.Load() {
	case 0:
		return "debug"
	case 2:
		return "warn"
	case 3:
		return "error"
	default:
		return "info"
	}
}
