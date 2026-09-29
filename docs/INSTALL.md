# BigCat V 安装说明（v0.5.0）

BigCat V 的核心是一个单文件程序 `bigcatvd`：内置完整 Web 控制界面，
浏览器打开即可使用，无需安装。

## 1. 下载

到 [Releases](https://github.com/zhanghaobo1987/BigCat-V/releases) 下载对应系统的压缩包：

| 文件 | 系统 |
|------|------|
| bigcatv-*-windows-amd64.zip | Windows 10/11（64 位） |
| bigcatv-*-darwin-arm64.zip | macOS（M1/M2/M3/M4） |
| bigcatv-*-darwin-amd64.zip | macOS（Intel） |
| bigcatv-*-linux-amd64.zip | Linux（64 位） |
| bigcatv-*-linux-arm64.zip | Linux（ARM64，如树莓派） |

## 2. 运行

1. 解压到任意目录
2. 运行 `bigcatvd`（Windows 下是 `bigcatvd.exe`，双击即可）
3. 浏览器打开 http://127.0.0.1:17890

Linux/macOS 首次运行需要加执行权限：

```bash
chmod +x bigcatvd && ./bigcatvd
```

常用参数：

```bash
./bigcatvd --listen 127.0.0.1:17890   # 监听地址
./bigcatvd --workdir ~/.bigcatv        # 数据目录（节点/订阅/规则/geo 库）
./bigcatvd --version                   # 查看版本
```

## 3. 安装代理内核（二选一或都装）

实际代理流量需要 sing-box 或 xray 内核（二进制），二选一即可：

**自动下载**（需 curl/unzip/tar）：

```bash
./scripts/fetch-kernels.sh   # 从源码仓库运行时
```

或手动下载官方 Release，解压后把 `sing-box` / `xray`
（Windows 为 `.exe`）放进 `bigcatvd` 所在目录的 `bin/` 文件夹：

```
<解压目录>/
├── bigcatvd(.exe)
└── bin/
    ├── sing-box(.exe)   <- https://github.com/SagerNet/sing-box/releases
    └── xray(.exe)       <- https://github.com/XTLS/Xray-core/releases
```

放好后重启 bigcatvd，界面「设置」页的内核状态会显示可用。

## 4. 说明与限制

- 界面通过浏览器使用；桌面原生安装包（.msi/.dmg）、Android APK、
  iOS 应用尚未提供，需要对应平台开发机打包。
- TUN 模式需要管理员/root 权限运行。
- 首次启动会自动下载 GeoIP/GeoSite 规则库（约 20MB），
  需要能访问 github.com。
- macOS 首次运行若提示「无法验证开发者」，到
  系统设置 → 隐私与安全性中允许运行。
- Windows Defender 若误报，请将 bigcatvd.exe 加入排除项
  （Go 编译的单文件程序常被误报）。
