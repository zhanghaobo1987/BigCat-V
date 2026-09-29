// Package bigcatv BigCat V 核心库（模块根包，仅放版本信息）。
package bigcatv

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var embeddedVersion string

// Version 返回 core/VERSION 的内容（编译时嵌入，不依赖运行时文件）。
func Version() string {
	return strings.TrimSpace(embeddedVersion)
}
