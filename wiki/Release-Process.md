# 发布流程

本页说明单文件构建、桌面产物和 GitHub Release 发布。

## 本地构建完整产物

```bash
./scripts/build-single-release.sh
```

按需构建：

```bash
./scripts/build-server-single.sh      # 服务版单文件，默认 Linux amd64
./scripts/build-desktop-single.sh     # 当前系统桌面单文件
./scripts/build-windows-exe.sh        # Windows 单文件 exe
./scripts/build-appimage.sh           # Linux AppImage
./scripts/build-desktop-platforms.sh  # 当前系统桌面、Linux AppImage、Windows exe
```

默认输出：

```text
.release/ai-sign-in-gateway-server-linux-amd64
.release/ai-sign-in-gateway
.release/ai-sign-in-gateway-windows-amd64.exe
.release/ai-sign-in-gateway-x86_64.AppImage
```

## 构建变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `OUTPUT_DIR` | `.release` | 产物输出目录。 |
| `APP_NAME` | `ai-sign-in-gateway` | 产物文件名前缀。 |
| `TARGET_GOOS` | 当前脚本决定 | 目标系统，如 `linux`、`windows`、`darwin`。 |
| `TARGET_GOARCH` / `TARGET_ARCH` | `amd64` 或当前架构 | 目标架构。 |
| `OUTPUT_PATH` | 脚本默认路径 | 自定义输出文件。 |
| `SERVER_HOST` | `0.0.0.0` | 服务版默认监听地址。 |
| `SERVER_OPEN_BROWSER` | `false` | 服务版默认是否打开浏览器。 |
| `DESKTOP_SHELL` | `true` | 构建桌面壳；服务版脚本会设为 `false`。 |
| `WINDOWS_GUI` | `true` | Windows exe 是否使用 GUI 子系统。 |
| `WINDOWS_ICON` | `true` | Windows exe 是否注入图标。 |
| `APPIMAGETOOL` | 自动查找或下载 | 指定 AppImageKit appimagetool。 |

## Windows exe

默认使用 GUI 子系统，不弹控制台窗口。需要调试日志时可构建控制台版：

```bash
WINDOWS_GUI=false ./scripts/build-windows-exe.sh
```

Windows exe 使用系统 WebView2 运行时。

## Linux AppImage

AppImage 构建依赖 AppImageKit 官方 `appimagetool`。如果 PATH 中没有可用工具，脚本会尝试下载；也可以手动指定：

```bash
APPIMAGETOOL=/path/to/appimagetool-x86_64.AppImage ./scripts/build-appimage.sh
```

Linux 桌面壳构建还需要 `pkg-config`、`gtk+-3.0`、`webkit2gtk-4.0/4.1` 开发文件；运行机需要对应运行库。

## 发布到 GitHub Release

前置条件：

- 已安装并登录 GitHub CLI：`gh auth login`。
- 当前仓库已配置 GitHub remote，默认使用 `origin`。
- 构建机具备对应打包依赖。

创建或更新 tag、执行本地打包、上传 GitHub Release、同步 `release` 分支：

```bash
./scripts/release.sh v1.0.0
```

如果已经执行过打包，可直接发布 `.release/` 下已有产物：

```bash
./scripts/release.sh v1.0.0 --skip-build
```

覆盖已有 tag 到当前 HEAD，并覆盖同名 GitHub Release 资产：

```bash
./scripts/release.sh v1.0.0 --retag-current -y
```

## 发布脚本参数

| 参数 | 说明 |
|---|---|
| `version` | 发布版本号，例如 `v1.0.0`；省略时使用最新 git tag。 |
| `--skip-build` | 不重新打包，直接发布 `.release/` 下已有产物。 |
| `--retag-current` | 将已有版本 tag 强制移动到当前 HEAD，再覆盖发布 release。 |
| `-y`, `--yes` | 跳过交互确认。 |
| `-h`, `--help` | 显示帮助。 |

## 发布脚本环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `GIT_REMOTE` | `origin` | Git remote 名称。 |
| `GH_REPO` | 从 remote URL 解析 | GitHub 仓库，格式 `owner/repo`。 |
| `RELEASE_BRANCH` | `release` | 纯产物发布分支。 |
| `BUILD_COMMAND` | `./scripts/build-single-release.sh` | 本地打包命令。 |
| `RELEASE_DIR` | `.release` | 产物目录。 |
| `GH_TOKEN` / `GITHUB_TOKEN` | 空 | 无 `gh` 登录时用于 GitHub API 发布。 |

## 校验下载文件

```bash
sha256sum -c ai-sign-in-gateway-<version>-SHA256SUMS.txt
```

Windows PowerShell：

```powershell
Get-FileHash .\ai-sign-in-gateway-windows-amd64.exe -Algorithm SHA256
```
