// bigcatvd：BigCat V 跨平台核心守护进程。
//
// 职责：订阅解析 -> 配置生成 -> 内核（sing-box / xray）进程管理 ->
// 本地 REST + SSE 控制 API。UI 层（Tauri / iOS / Android）只调 API。
//
// 启动逻辑已抽取到 internal/bootstrap（与 mobile 绑定共用），
// main 仅负责参数解析与信号等待。
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	bigcatv "bigcatv"
	"bigcatv/internal/bootstrap"
)

var version = "0.1.0" // 发布时由 -ldflags 覆盖

func main() {
	listen := flag.String("listen", "127.0.0.1:17890", "API 监听地址")
	workDir := flag.String("workdir", "", "工作目录（默认 ~/.bigcatv）")
	showVer := flag.Bool("version", false, "打印版本")
	flag.Parse()

	ver := version
	if ver == "0.1.0" {
		if ev := bigcatv.Version(); ev != "" && ev != "0.1.0" {
			ver = ev // 未用 ldflags 覆盖时，回退到嵌入的 core/VERSION
		}
	}
	if *showVer {
		fmt.Println("bigcatvd", ver)
		return
	}

	h, err := bootstrap.Start(ver, *listen, *workDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}

	fmt.Printf("bigcatvd %s listening on http://%s\n", ver, h.Addr())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\n收到退出信号，正在关闭…")
	if err := h.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "关闭异常:", err)
	}
}
