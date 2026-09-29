#!/bin/bash
# 下载官方 sing-box / xray 内核二进制到 ./bin/（bigcatvd 按 ./bin 优先查找）。
# 用法: ./scripts/fetch-kernels.sh [windows-amd64|darwin-amd64|darwin-arm64|linux-amd64|linux-arm64]
# 缺省按本机平台。需要 curl / unzip / tar。
set -e
TARGET="${1:-}"
if [ -z "$TARGET" ]; then
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"
  case "$ARCH" in x86_64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; esac
  TARGET="$OS-$ARCH"
fi
case "$TARGET" in
  windows-amd64) SB=windows-amd64;   XR=windows-64;      EXE=.exe;;
  darwin-amd64)  SB=darwin-amd64;     XR=macos-64;        EXE=;;
  darwin-arm64)  SB=darwin-arm64;     XR=macos-arm64;      EXE=;;
  linux-amd64)   SB=linux-amd64;      XR=linux-64;        EXE=;;
  linux-arm64)   SB=linux-arm64;      XR=linux-arm64-v8a; EXE=;;
  *) echo "不支持的平台: $TARGET"; exit 1;;
esac

SB_VER="$(curl -fsSL -o /dev/null -w '%{redirect_url}' https://github.com/SagerNet/sing-box/releases/latest | sed 's#.*/tag/v##')"
echo "sing-box v$SB_VER / xray latest ($TARGET)"
mkdir -p bin && cd bin
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

echo "==> 下载 sing-box"
if [ "$SB" = "windows-amd64" ]; then
  curl -fL -o "$TMP/sb.zip" "https://github.com/SagerNet/sing-box/releases/latest/download/sing-box-$SB_VER-$SB.zip"
  unzip -o -j -q "$TMP/sb.zip" "*/sing-box$EXE" -d .
else
  curl -fL -o "$TMP/sb.tgz" "https://github.com/SagerNet/sing-box/releases/latest/download/sing-box-$SB_VER-$SB.tar.gz"
  tar xzf "$TMP/sb.tgz" -C "$TMP"
  cp "$TMP/sing-box-$SB_VER-$SB/sing-box$EXE" .
fi

echo "==> 下载 xray"
curl -fL -o "$TMP/xr.zip" "https://github.com/XTLS/Xray-core/releases/latest/download/Xray-$XR.zip"
unzip -o -j -q "$TMP/xr.zip" "xray$EXE" -d .

chmod +x "sing-box$EXE" "xray$EXE"
echo "完成：$(pwd)/sing-box$EXE $(pwd)/xray$EXE"
"./sing-box$EXE" version 2>/dev/null | head -1 || true
"./xray$EXE" version 2>/dev/null | head -1 || true
