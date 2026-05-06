# 爱签网关

爱签网关（`ai-sign-in-gateway`）是一个单机优先的 API 中转站账号管理与聚合网关工具。它面向 NewAPI/sub2api 类站点、自定义 HTTP 站点和只作为模型出口的供应商站点，集中处理站点管理、自动签到、余额同步、API Key 池化、分组路由、健康探测、熔断回退和请求历史。

技术栈：

- 后端：Go、chi、GORM、纯 Go SQLite 驱动。
- 前端：Vue 3、Vite、TypeScript、Ant Design Vue。
- 桌面端：Go 原生 WebView/系统托盘，不使用 Electron。
- 数据存储：默认 SQLite，适合单机、VPS、NAS、桌面长期运行。
- 分发方式：推荐使用内嵌前端的单文件 release 产物，也支持 Docker、源码开发和离线包。

## 为什么优先使用单文件

release 单文件已经把前端 `frontend/dist` 嵌入 Go 二进制，直接运行即可得到完整 Web 管理端、后端 API 和聚合网关。

主要优势：

- 对应平台的二进制不需要在运行机器安装 Go 环境；Go 只在源码构建时需要。
- `linux-amd64` 是 Linux x86_64 目标产物，不是 Windows/macOS/Linux 通用跨平台文件；其他系统或架构需要下载或构建对应产物。
- 无需安装 Node.js、npm、Nginx 或额外静态资源目录。
- 一个文件即可复制、备份、迁移、升级，适合 VPS、NAS、Windows 桌面和 Linux 桌面。
- 默认使用 SQLite，数据与程序分离，升级程序时不用移动数据库。
- 服务版可直接部署到服务器，桌面版可作为本地 GUI 应用运行。
- 端口被占用时会在默认端口后自动顺延，降低本机多实例和旧版本残留的启动失败概率。
- 网关地址稳定为 `/api/gateway`，可被 OpenAI 风格客户端、Codex/Claude/Gemini 类工具或自定义脚本调用。

## Release 产物选择

从 GitHub Release 或 `release` 分支下载对应文件。发布脚本会同时生成校验和文件 `ai-sign-in-gateway-<version>-SHA256SUMS.txt`、发布说明和最新产物分支。

| 产物 | 适用场景 | 启动方式 |
|---|---|---|
| `ai-sign-in-gateway-server-linux-amd64` | Linux VPS、NAS、服务器后台运行 | 作为 Web 服务监听 `8972` |
| `ai-sign-in-gateway-server-<goos>-<goarch>` | 其他系统/架构服务版 | 作为 Web 服务运行 |
| `ai-sign-in-gateway-windows-amd64.exe` | Windows 桌面 | 双击运行 GUI，或命令行运行 |
| `ai-sign-in-gateway-x86_64.AppImage` | Linux 桌面 | 授权后双击或命令行运行 |
| `ai-sign-in-gateway` | 当前构建机系统的桌面二进制 | 本机桌面调试或分发 |

校验下载文件：

```bash
sha256sum -c ai-sign-in-gateway-<version>-SHA256SUMS.txt
```

Windows 可使用 PowerShell：

```powershell
Get-FileHash .\ai-sign-in-gateway-windows-amd64.exe -Algorithm SHA256
```

## 快速运行

### Linux 服务版

```bash
chmod +x ai-sign-in-gateway-server-linux-amd64

export SECRET_KEY="$(openssl rand -base64 32)"
export GATEWAY_API_KEY="$(openssl rand -base64 32)"
export DEFAULT_ADMIN_PASSWORD="<change-this-password>"
export CORS_ORIGINS="http://127.0.0.1:8972,http://localhost:8972"

./ai-sign-in-gateway-server-linux-amd64 --host 0.0.0.0 --port 8972 --no-browser
```

访问：

```text
http://<server-ip>:8972
```

默认管理员只在首次初始化数据库时写入：

- 用户名：`admin`
- 密码：`admin123`

生产环境请在第一次启动前设置 `DEFAULT_ADMIN_PASSWORD`，登录后进入 `设置 -> 账号与密码` 修改账号和密码。

### Windows 桌面版

1. 下载 `ai-sign-in-gateway-windows-amd64.exe`。
2. 双击运行。
3. 程序会启动本地服务，并在 WebView 窗口中打开管理端。
4. 如系统提示未知发布者，确认文件来源和 SHA256 后再放行。

如需命令行指定配置目录：

```powershell
$env:GATEWAY_API_KEY="change-this-gateway-key"
.\ai-sign-in-gateway-windows-amd64.exe --config-dir "D:\ai-sign-in-gateway-data"
```

如需快速换端口，服务版直接用 `--port`，桌面版可分别设置窗口入口和后端网关端口：

```bash
./ai-sign-in-gateway-server-linux-amd64 --port 9000
./ai-sign-in-gateway --frontend-port 3722 --backend-port 8973
```

### Linux 桌面 AppImage

```bash
chmod +x ai-sign-in-gateway-x86_64.AppImage
./ai-sign-in-gateway-x86_64.AppImage
```

桌面模式会启动本地后端、前端窗口和系统托盘。托盘提供网关 24h 概览、路由健康、站点连通率检测、同步路由和探测路由等快捷入口。

### 后台长期运行

最小 systemd 示例：

```ini
[Unit]
Description=AI Sign In Gateway
After=network.target

[Service]
Type=simple
User=ai-gateway
WorkingDirectory=/opt/ai-sign-in-gateway
Environment=AI_SIGN_IN_GATEWAY_HOST=0.0.0.0
Environment=AI_SIGN_IN_GATEWAY_PORT=8972
Environment=AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false
Environment=AI_SIGN_IN_GATEWAY_CONFIG_DIR=/var/lib/ai-sign-in-gateway
Environment=DATABASE_URL=sqlite:////var/lib/ai-sign-in-gateway/ai-sign-in-gateway.db
Environment=SECRET_KEY=change-this-secret-key
Environment=GATEWAY_API_KEY=change-this-gateway-key
Environment=DEFAULT_ADMIN_PASSWORD=change-this-admin-password
Environment=CORS_ORIGINS=https://your-domain.example,http://127.0.0.1:8972
ExecStart=/opt/ai-sign-in-gateway/ai-sign-in-gateway-server-linux-amd64
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ai-sign-in-gateway
sudo journalctl -u ai-sign-in-gateway -f
```

## 首次使用流程

1. 启动程序并打开管理端。
2. 使用默认管理员或环境变量初始化的管理员登录。
3. 进入 `设置 -> 账号与密码` 修改管理员账号密码。
4. 进入 `网关中心 -> 网关策略` 设置或确认网关 API Key。
5. 进入 `站点中心` 添加站点，选择插件类型并填写凭证。
6. 对站点执行 `测试连接`、`余额`、`API Key 同步`。
7. 按需要创建分组，把站点加入不同分组。
8. 在 `设置` 中开启定时签到，并配置运行时间、并发、重试和超时。
9. 进入 `验证与对话`，选择站点后自动读取该站点 API Key 和请求 API URL 下的模型列表，用真实模型执行对话或图片生成验证。
10. 使用 `/api/gateway` 作为统一模型请求入口。

## 数据、日志与备份

默认数据目录：

```text
~/.ai-sign-in-gateway/
```

常见文件：

```text
~/.ai-sign-in-gateway/ai-sign-in-gateway.db       # SQLite 数据库
~/.ai-sign-in-gateway/logs/shell.log              # 启动和运行日志
~/.ai-sign-in-gateway/browser-profiles/           # 托管浏览器配置目录
```

开发脚本 `./run.sh` 使用项目内 `.run/ai-sign-in-gateway-go.db`。Docker 默认使用 `/app/data/ai-sign-in-gateway.db`，并映射到 Docker volume `ai-sign-in-gateway-data`。

建议备份：

```bash
cp ~/.ai-sign-in-gateway/ai-sign-in-gateway.db ./ai-sign-in-gateway.db.bak
```

迁移到新机器时，复制单文件程序和数据库目录即可。升级版本时通常只替换二进制文件，保留数据目录不变。

## 启动参数与配置变量

单文件二进制支持少量启动参数，用于快速覆盖本次运行的端口和目录：

| 参数 | 说明 |
|---|---|
| `--port` / `-p` | 快速设置服务/API/网关端口 |
| `--backend-port` | 桌面模式后端/API/网关端口，优先级高于 `--port` |
| `--frontend-port` | 桌面模式前端窗口入口端口 |
| `--host` | 监听地址，例如 `127.0.0.1` 或 `0.0.0.0` |
| `--config-dir` | 用户配置、日志和默认 SQLite 数据目录 |
| `--browser` / `--no-browser` | 启动后是否打开浏览器或桌面窗口 |
| `--desktop` / `--no-desktop` | 是否启用桌面 WebView/托盘 |

更完整的配置仍通过环境变量提供。

| 变量 | 默认值 | 说明 |
|---|---|---|
| `APP_NAME` | `爱签网关` | 应用显示名 |
| `AI_SIGN_IN_GATEWAY_HOST` | `127.0.0.1`；服务版构建默认为 `0.0.0.0` | 监听地址 |
| `AI_SIGN_IN_GATEWAY_PORT` | `8972` | 服务模式后端/API/网关端口 |
| `AI_SIGN_IN_GATEWAY_BACKEND_PORT` | `8972` | 桌面模式后端/API/网关端口，优先级高于 `AI_SIGN_IN_GATEWAY_PORT` |
| `AI_SIGN_IN_GATEWAY_FRONTEND_PORT` | `3721` | 桌面模式前端窗口入口端口 |
| `AI_SIGN_IN_GATEWAY_OPEN_BROWSER` | `true`；服务版构建默认为 `false` | 启动后是否自动打开浏览器 |
| `AI_SIGN_IN_GATEWAY_DESKTOP` | 桌面构建为 `true` | 是否启用 WebView 主窗口和托盘；设为 `false` 可退回本地 Web 服务模式 |
| `AI_SIGN_IN_GATEWAY_DESKTOP_DEBUG` | `false` | 桌面 WebView 调试开关 |
| `AI_SIGN_IN_GATEWAY_CONFIG_DIR` | `~/.ai-sign-in-gateway` | 用户配置、日志、数据库默认目录 |
| `DATABASE_URL` | `sqlite:///<配置目录>/ai-sign-in-gateway.db` | SQLite 数据库地址 |
| `SECRET_KEY` | 开发默认值 | JWT 签名密钥，生产必须修改 |
| `GATEWAY_API_KEY` | 空 | 初始化聚合网关 Bearer Key；空值表示不校验网关 Bearer，生产必须设置 |
| `DEFAULT_ADMIN_USERNAME` | `admin` | 首次初始化管理员用户名 |
| `DEFAULT_ADMIN_PASSWORD` | `admin123` | 首次初始化管理员密码，生产必须修改 |
| `ALGORITHM` | `HS256` | JWT 签名算法 |
| `ACCESS_TOKEN_EXPIRE_MINUTES` | `1440` | 管理端登录 token 有效期，单位分钟 |
| `CORS_ORIGINS` | 本地开发域名 | 允许的前端来源，多个值用英文逗号分隔 |
| `SCHEDULER_TIMEZONE` | `Asia/Shanghai` | 定时签到时区 |
| `MANAGED_BROWSER_PROFILE_ROOT` | `<配置目录>/browser-profiles` | 托管浏览器 profile 目录 |
| `MANAGED_BROWSER_HEADLESS` | `false` | 托管浏览器是否无头运行 |
| `MANAGED_BROWSER_TIMEOUT_SECONDS` | `20` | 托管浏览器操作超时 |
| `MANAGED_BROWSER_SETTLE_MS` | `1200` | 浏览器页面稳定等待时间，单位毫秒 |

SQLite 路径格式示例：

```bash
export DATABASE_URL="sqlite:////var/lib/ai-sign-in-gateway/ai-sign-in-gateway.db"
```

Windows 路径建议优先使用 `AI_SIGN_IN_GATEWAY_CONFIG_DIR` 指定数据目录，让程序自动生成默认数据库路径。

## 聚合网关使用

推荐入口：

```text
http://<host>:8972/api/gateway
```

兼容入口：

```text
http://<host>:8972/api/gateway/v1
```

认证方式：

```http
Authorization: Bearer <GATEWAY_API_KEY>
```

OpenAI 风格请求：

```bash
curl http://127.0.0.1:8972/api/gateway/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "ping"}]
  }'
```

路由筛选：

- `?group=<分组>`：只使用指定分组的路由。
- `?type=claude|codex|gemini`：手动指定路由类型。
- 不传 `type` 时，会根据请求体中的 `model` 自动推断 Claude/GPT/Gemini 路由。

客户端 Base URL 示例：

```text
http://127.0.0.1:8972/api/gateway
```

## 验证与对话

`验证与对话` 是统一的模型连通性验证入口，原独立“模型连通性检测”页面已合并到这里。

使用方式：

1. 在底部输入区上方选择已保存站点。
2. 系统使用该站点保存的 API Key、请求 API URL 和路由类型自动请求模型列表。
3. 在同一控制行选择模型。
4. 输入消息并发送。

模型决定请求方式：

- 普通对话模型走 OpenAI 兼容 `/chat/completions`。
- `gpt-image-*`、`dall-e*`、`imagen` 等图片模型走图片生成/编辑接口。
- 图片模型支持 `1:1`、`3:4`、`4:3`、`16:9`、`9:16` 快捷比例和自定义宽高；选择比例会设置最小 100 基准尺寸并锁定比例，解锁后手动输入会自动识别常见比例但不会重新锁定。
- 只有图片模型可以添加参考图；普通对话模型会阻止参考图输入，避免把图片发给不支持图片输入的上游。

如果模型加载提示 404，通常表示当前站点的请求 API URL 不支持模型列表接口，或后台进程尚未更新到包含 `/api/tools/models` 的版本。先确认站点的 `api_request_urls` / `gateway_request_urls` 是否指向模型请求根地址，并重启最新二进制或后端进程。

路由模型探测如果从 Codex/OpenAI 兼容 `/models` 成功读取到空列表，会默认写入 `gpt-5.4,gpt-5.5` 作为支持模型，避免“探测无模型但编辑仍保留旧模型”的状态不一致。

## 余额与套餐余量

余额探测优先使用站点和供应商的真实余额接口，普通 OpenAI 兼容 `/v1/usage` 只作为 fallback：

- `sub2api-platform`：登录态读取 `/api/v1/subscriptions/progress`，失败时回退 `/api/v1/subscriptions/summary` 或 `/api/v1/keys` 的额度字段。
- `yellowpeach-newapi`：登录态读取 `/api/subscription/self` 的 `amount_total/amount_used`，失败时使用 API Key 调 `/api/usage/token/` 的 `total_available/total_used/total_granted`。
- 官方供应商余额探测支持 DeepSeek、StepFun、SiliconFlow、OpenRouter、Novita AI 的公开余额接口。

套餐余量会写入 `package_remaining/package_total/package_used/package_unit/package_display`，路由列表余额仍保留钱包余额或 API Key 余额。

## 站点与插件

进入 `站点中心 -> 新增站点`，按目标站点选择插件。

| 插件 | 用途 |
|---|---|
| `http-relay-station` | 通用 HTTP 站点，适合自定义登录/状态/签到接口 |
| `api-supplier` | 仅作为网关供应商记录，不登录、不签到 |
| `sub2api-platform` | 适配 sub2api 类站点，支持 token 或账号密码登录、签到、API Key 同步 |
| `yellowpeach-newapi` | 适配 NewAPI 类站点，支持 cookie/token/账号密码、签到、API Key 同步 |

常用凭证字段：

- `api_key` / `api_keys`：模型请求 Key，网关会把多 Key 展开成多条路由。
- `access_token` / `refresh_token` / `cookie`：站内登录、状态、签到或同步接口凭证。
- `username` / `email` / `password`：自动登录凭证。
- `totp_secret` / `totp_otpauth_url`：二次验证，系统会自动生成当前验证码。
- `gateway_request_url(s)` / `api_request_urls`：上游模型请求地址，支持多个出口回退。
- `gateway_priority` / `gateway_weight`：网关优先级和权重。

## 签到、分组与路由

- 单站点签到：在 `站点中心` 点击站点操作列的 `签到`。
- 批量签到：勾选可签到站点后点击 `签到选中`。
- 定时签到：在 `设置` 中开启计划任务，配置时间、时区、并发、重试和超时。
- 分组管理：站点可归属多个分组，网关请求可通过 `?group=<分组>` 限定路由池。
- 路由策略：在 `网关中心 -> 网关策略` 选择 Smart、Round Robin、Latency First、Priority，并配置并发溢出策略。

## 构建 Release

本地构建完整单文件产物：

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

常用构建变量：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `OUTPUT_DIR` | `.release` | 产物输出目录 |
| `APP_NAME` | `ai-sign-in-gateway` | 产物文件名前缀 |
| `TARGET_GOOS` | 当前脚本决定 | 目标系统，如 `linux`、`windows`、`darwin` |
| `TARGET_GOARCH` / `TARGET_ARCH` | `amd64` 或当前架构 | 目标架构 |
| `OUTPUT_PATH` | 脚本默认路径 | 自定义输出文件 |
| `SERVER_HOST` | `0.0.0.0` | 服务版默认监听地址 |
| `SERVER_OPEN_BROWSER` | `false` | 服务版默认是否打开浏览器 |
| `DESKTOP_SHELL` | `true` | 构建桌面壳；服务版脚本会设为 `false` |
| `WINDOWS_GUI` | `true` | Windows exe 是否使用 GUI 子系统 |
| `WINDOWS_ICON` | `true` | Windows exe 是否注入图标 |
| `APPIMAGETOOL` | 自动查找/下载 | 指定 AppImageKit appimagetool |

发布到 GitHub Release 并同步 `release` 分支：

```bash
gh auth login
./scripts/release.sh v1.0.0
```

发布脚本参数：

| 参数 | 说明 |
|---|---|
| `version` | 发布版本号，例如 `v1.0.0`；省略时使用最新 git tag |
| `--skip-build` | 不重新打包，直接发布 `.release/` 下已有产物 |
| `--retag-current` | 将已有版本 tag 强制移动到当前 HEAD，再覆盖发布 release |
| `-y`, `--yes` | 跳过交互确认 |
| `-h`, `--help` | 显示帮助 |

发布脚本环境变量：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `GIT_REMOTE` | `origin` | Git remote 名称 |
| `GH_REPO` | 从 remote URL 解析 | GitHub 仓库，格式 `owner/repo` |
| `RELEASE_BRANCH` | `release` | 纯产物发布分支 |
| `BUILD_COMMAND` | `./scripts/build-single-release.sh` | 本地打包命令 |
| `RELEASE_DIR` | `.release` | 产物目录 |
| `GH_TOKEN` / `GITHUB_TOKEN` | 空 | 无 `gh` 登录时用于 GitHub API 发布 |

## Docker 与源码运行

Docker Compose：

```bash
docker compose up -d --build
```

启动前请修改 `compose.yaml` 中的 `SECRET_KEY`、`GATEWAY_API_KEY`、`DEFAULT_ADMIN_PASSWORD` 和 `CORS_ORIGINS`。

源码开发：

```bash
go mod download
cd frontend && npm ci && cd ..
./run.sh
```

开发脚本默认启动：

- 前端 Vite：`http://127.0.0.1:3721`
- 后端 API：从 `http://127.0.0.1:8972` 开始，端口占用会自动顺延
- 日志：`.run/backend.log`、`.run/frontend.log`

停止：

```bash
./stop.sh
```

## 验证命令

```bash
go test ./...
cd frontend && npm run build
```

## 文档导航

- [使用指南](docs/USAGE.md)：登录、站点管理、签到、网关配置、路由策略、请求历史和常见操作。
- [部署指南](docs/DEPLOYMENT.md)：裸机/VPS、systemd、Docker Compose、Docker run、离线包、AppImage、Windows 单文件 exe 等部署方式。
- [开发指南](docs/DEVELOPMENT.md)：安装工具链、启动开发环境、测试和贡献流程。

## 仓库结构

```text
.
├── cmd/ai-sign-in-gateway/      # Go 入口、桌面壳和前端静态资源服务
├── internal/                    # 后端 handlers/services/plugins/models/migrations
├── frontend/                    # Vue 3 管理端
├── docs/                        # 开发、部署、使用文档
├── scripts/                     # 单文件、Windows exe、AppImage、GitHub Release 脚本
├── Dockerfile / compose.yaml    # 容器部署
├── run.sh / stop.sh             # 开发起停脚本
├── start-prod.sh / stop-prod.sh # 源码生产起停脚本
└── package-release.sh           # 传统离线发布包
```

## 安全提醒

- 生产环境必须修改 `SECRET_KEY`、`DEFAULT_ADMIN_PASSWORD` 和 `GATEWAY_API_KEY`。
- 不要提交 `.run/`、`backups/`、数据库文件、`.env`、Cookie、API Key、导出的浏览器 localStorage。
- 如果曾经把真实数据库或密钥提交到 Git 历史里，删除文件不足以脱敏，需要轮换密钥并重写仓库历史。
