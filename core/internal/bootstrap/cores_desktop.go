//go:build !(mobile || android || ios)

package bootstrap

import "bigcatv/internal/engine"

// probeCoresMobile 桌面/默认构建：与进程引擎探测一致。
func probeCoresMobile() map[string]engine.Core {
	return probeCores()
}
