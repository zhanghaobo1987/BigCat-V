#!/bin/bash
# BigCat V 发布构建：内嵌 Web UI -> 五平台交叉编译 -> 打包 zip。
# 用法: ./scripts/build-release.sh
# 产物: dist/bigcatv-<version>-<os>-<arch>.zip（含 bigcatvd + 说明）
set -e
cd "$(dirname "$0")/.."
ROOT="$PWD"
VERSION="$(cat core/VERSION)"
WEBUI="core/internal/api/webui"
DIST="$ROOT/dist"

echo "==> 拷贝 Web UI 到内嵌目录"
cp desktop/src/index.html desktop/src/app.js desktop/src/styles.css "$WEBUI/"

cleanup() {
  echo "==> 清理内嵌拷贝"
  rm -f "$WEBUI/index.html" "$WEBUI/app.js" "$WEBUI/styles.css"
}
trap cleanup EXIT

export CGO_ENABLED=0
GO_BIN="${GO_BIN:-$(command -v go 2>/dev/null || echo ~/go/bin/go)}"
LDFLAGS="-s -w -X main.version=$VERSION"
mkdir -p "$DIST"

build() { # $1=goos $2=goarch
  local goos=$1 goarch=$2 out="bigcatvd"
  [ "$goos" = "windows" ] && out="bigcatvd.exe"
  local name="bigcatv-$VERSION-$goos-$goarch"
  echo "==> 编译 $name"
  (cd core && GOOS=$goos GOARCH=$goarch "$GO_BIN" build -trimpath \
    -ldflags "$LDFLAGS" -o "$DIST/$name/$out" ./cmd/bigcatvd)
  cp docs/INSTALL.md "$DIST/$name/安装说明.md"
  (cd "$DIST" && rm -f "$name.zip" && zip -qr "$name.zip" "$name" -x "*/.*")
  rm -rf "$DIST/$name"
  echo "    -> dist/$name.zip"
}

build windows amd64
build darwin amd64
build darwin arm64
build linux amd64
build linux arm64

echo "完成：$(ls "$DIST"/*.zip | wc -l) 个安装包"
