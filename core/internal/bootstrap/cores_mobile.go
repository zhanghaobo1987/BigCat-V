//go:build mobile || android || ios

package bootstrap

import (
	"fmt"
	"os"

	"bigcatv/internal/engine"
)

// probeCoresMobile 移动端：优先使用进程内引擎（iOS/Android 禁止 fork/exec）。
// 进程内不可用时回退到进程引擎（桌面端以 mobile tag 调试时仍可工作）。
func probeCoresMobile() map[string]engine.Core {
	cores := make(map[string]engine.Core)
	if sb, err := engine.NewInProcessSingBox(); err != nil {
		fmt.Fprintln(os.Stderr, "[warn] 进程内 sing-box 不可用:", err)
		if pb, err := engine.New(); err == nil {
			cores[engine.KernelSingBox] = pb
			fmt.Println("[ok] sing-box(进程):", pb.Binary())
		}
	} else {
		cores[engine.KernelSingBox] = sb
		fmt.Println("[ok] sing-box(进程内):", sb.Binary())
	}
	if xr, err := engine.NewInProcessXray(); err != nil {
		fmt.Fprintln(os.Stderr, "[warn] 进程内 xray 不可用:", err)
		if px, err := engine.NewXray(); err == nil {
			cores[engine.KernelXray] = px
			fmt.Println("[ok] xray(进程):", px.Binary())
		}
	} else {
		cores[engine.KernelXray] = xr
		fmt.Println("[ok] xray(进程内):", xr.Binary())
	}
	return cores
}
