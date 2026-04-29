# 爱签网关

爱签网关（ai-sign-in-gateway）是一个单机优先的 API 中转站账号管理后台。后端使用 Go、chi、GORM 和纯 Go SQLite 驱动，前端使用 Vue 3、Vite、TypeScript 和 Ant Design Vue。项目面向 API 中转站、NewAPI/sub2api 类站点和自定义 HTTP 站点，提供站点管理、签到、额度同步、API Key 池化、聚合网关、路由健康观测和请求历史。

默认数据库是 SQLite。直接运行二进制时，数据位于 `~/.ai-sign-in-gateway/ai-sign-in-gateway.db`；使用开发脚本 `run.sh` 时，数据位于项目内 `.run/ai-sign-in-gateway-go.db`；Docker 默认保存到命名卷 `ai-sign-in-gateway-data`。

## 文档导航

- [开发指南](docs/DEVELOPMENT.md)：从 clone、安装工具链、安装依赖、启动开发环境到测试与贡献。
- [部署指南](docs/DEPLOYMENT.md)：裸机/VPS、systemd、Docker Compose、Docker run、离线包、AppImage、Windows 单文件 exe 等部署方式。
- [使用指南](docs/USAGE.md)：登录、站点管理、签到、网关配置、路由策略、请求历史和常见操作。

## 核心功能

- 单管理员登录，支持在设置页修改账号和密码。
- 插件式站点接入：通用 HTTP 站点、仅网关供应商、sub2api 平台、NewAPI 系站点。
- 站点连通测试、余额/套餐同步、API Key 同步、邀请码读取和公开邀请码列表。
- 手动签到、批量签到、定时签到，支持站点级“可签到/禁止签到”控制。
- 分组管理，同一站点可归属多个分组，网关可按分组筛选路由。
- 聚合网关：API Key 池化、多出口回退、路由级健康检查、熔断、并发限制、流式转发。
- 网关策略：Smart 评分、Round Robin、Latency First、Priority，并可设置并发溢出策略。
- 网关中心：24h 概览、近 1 分钟趋势、路由列表、路由请求历史、最近请求日志。

## 快速开始

### 1. 准备工具链

最低要求：

- Go `1.25+`
- Node.js `22+`
- npm `10+`
- Git
- Linux/macOS/WSL 推荐；Windows 原生可自行适配脚本

可选：

- Docker `24+` 和 Docker Compose，用于容器部署
- `ss`，开发脚本用来探测端口

### 2. Clone 并安装依赖

```bash
git clone <repo-url> ai-sign-in-gateway
cd ai-sign-in-gateway

go mod download
cd frontend
npm ci
cd ..
```

国内网络可先配置镜像：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
npm config set registry https://registry.npmmirror.com
```

### 3. 启动开发环境

```bash
./run.sh
```

脚本会构建后端二进制、安装/校验前端依赖，并启动：

- 前端 Vite：默认 `http://127.0.0.1:3721`
- 后端 API：默认从 `http://127.0.0.1:8972` 开始，端口被占用会自动顺延
- 日志：`.run/backend.log`、`.run/frontend.log`

停止：

```bash
./stop.sh
```

默认管理员：

- 用户名：`admin`
- 密码：`admin123`

首次登录后请立即到 `设置 -> 账号与密码` 修改。生产环境也可以在首次启动前设置 `DEFAULT_ADMIN_USERNAME` 和 `DEFAULT_ADMIN_PASSWORD`。

## 生产部署概览

更完整的上线步骤见 [部署指南](docs/DEPLOYMENT.md)。

### 方式一：裸机/VPS 脚本部署

```bash
git clone <repo-url> /opt/ai-sign-in-gateway
cd /opt/ai-sign-in-gateway

cd frontend && npm ci && npm run build && cd ..

export SECRET_KEY="$(openssl rand -base64 32)"
export GATEWAY_API_KEY="$(openssl rand -base64 32)"
export DEFAULT_ADMIN_PASSWORD="<change-me>"
export DATABASE_URL="sqlite:////var/lib/ai-sign-in-gateway/data.db"
export CORS_ORIGINS="https://your-domain.example"

./start-prod.sh
```

停止：

```bash
./stop-prod.sh
```

### 方式二：Docker Compose

先修改 `compose.yaml` 中的 `SECRET_KEY`、`GATEWAY_API_KEY`、`DEFAULT_ADMIN_PASSWORD` 和 `CORS_ORIGINS`，再启动：

```bash
docker compose up -d --build
```

数据保存在 Docker volume `ai-sign-in-gateway-data`。

### 方式三：离线分发包

在构建机生成压缩包：

```bash
./package-release.sh
```

把 `.release/ai-sign-in-gateway-<version>.tar.gz` 上传到服务器解压后运行：

```bash
./start-prod.sh
```

### 方式四：单文件分发包

```bash
# 一次构建服务版和桌面端分发包
./scripts/build-single-release.sh

# 无桌面服务器 Web 单文件，默认输出 Linux amd64
./scripts/build-server-single.sh

# 当前系统桌面自包含二进制
./scripts/build-desktop-single.sh

# Windows 单文件 exe
./scripts/build-windows-exe.sh

# Linux AppImage
./scripts/build-appimage.sh

# 当前系统桌面二进制、Linux AppImage、Windows exe
./scripts/build-desktop-platforms.sh
```

这些脚本都会把 `frontend/dist` 嵌入 Go 二进制。服务版产物默认监听 `0.0.0.0:8972`、不自动打开浏览器，端口被占用时继续沿用自动偏移策略。Windows 产物是单个 GUI `.exe`，Linux 桌面产物是可分发的 `.AppImage`。
桌面产物默认启用原生桌面壳：本地服务在后台启动，完整前端在系统 WebView 窗口中打开，托盘提供网关 24h 简要统计、路由健康、站点连通率检测、同步路由和探测路由等快捷操作，不使用 Electron。

### 方式五：GitHub 自动发布

```bash
# 首次使用前登录 GitHub CLI
gh auth login

# 本地打包后创建/更新 GitHub Release，并同步 release 分支
./scripts/release.sh v1.0.0

# 已经手动打包过时，直接发布 .release/ 里的现有产物
./scripts/release.sh v1.0.0 --skip-build
```

发布脚本会上传 `.release/` 下的服务版单文件、桌面分发包和校验和到 GitHub Release，同时把同一批产物写入 `release` 分支。`release` 分支是纯产物分支，只保留最新发布文件、`SHA256SUMS`、`RELEASE_NOTES.md`、`RELEASE.txt` 和说明文件，不保存源码。

## 环境变量

常用变量：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `APP_NAME` | `爱签网关` | 应用显示名 |
| `AI_SIGN_IN_GATEWAY_HOST` | `127.0.0.1` | 监听地址；生产/容器通常用 `0.0.0.0` |
| `AI_SIGN_IN_GATEWAY_PORT` | `8972` | 服务模式后端/API/网关端口；兼容旧变量 |
| `AI_SIGN_IN_GATEWAY_BACKEND_PORT` | `8972` | 桌面模式后端/API/网关端口，优先级高于 `AI_SIGN_IN_GATEWAY_PORT` |
| `AI_SIGN_IN_GATEWAY_FRONTEND_PORT` | `3721` | 桌面模式前端窗口入口端口 |
| `AI_SIGN_IN_GATEWAY_DESKTOP` | 桌面构建为 `true` | 桌面构建是否启用 WebView 主窗口和托盘；设为 `false` 可退回本地服务模式 |
| `AI_SIGN_IN_GATEWAY_OPEN_BROWSER` | `true` | 启动后是否自动打开浏览器 |
| `DATABASE_URL` | `sqlite:///~/.ai-sign-in-gateway/ai-sign-in-gateway.db` | SQLite 数据库地址 |
| `SECRET_KEY` | 开发默认值 | JWT 签名密钥，生产必须修改 |
| `GATEWAY_API_KEY` | 空 | 初始化聚合网关 Bearer Key；空值表示不校验网关 Bearer，生产必须设置 |
| `DEFAULT_ADMIN_USERNAME` | `admin` | 首次初始化管理员用户名 |
| `DEFAULT_ADMIN_PASSWORD` | `admin123` | 首次初始化管理员密码，生产必须修改 |
| `CORS_ORIGINS` | 本地开发域名 | 允许的前端来源，逗号分隔 |
| `SCHEDULER_TIMEZONE` | `Asia/Shanghai` | 定时签到时区 |

完整说明见 [部署指南：环境变量](docs/DEPLOYMENT.md#环境变量)。

## 聚合网关

推荐入口：

```text
/api/gateway
```

兼容入口：

```text
/api/gateway/v1
```

认证方式：

```http
Authorization: Bearer <GATEWAY_API_KEY>
```

路由筛选：

- `?group=<分组>`：只使用指定分组的路由
- `?type=claude|codex|gemini`：手动指定路由类型
- 不传 `type` 时，会根据请求体中的 `model` 自动推断 Claude/GPT/Gemini 路由

示例：

```bash
curl http://localhost:8972/api/gateway/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}]}'
```

## 仓库结构

```text
.
├── cmd/ai-sign-in-gateway/      # Go 入口和前端静态资源服务
├── internal/                    # 后端 handlers/services/plugins/models/migrations
├── frontend/                    # Vue 3 管理端
├── docs/                        # 开发、部署、使用文档
├── scripts/                     # 自包含二进制、Windows exe、AppImage 构建脚本
├── Dockerfile / compose.yaml    # 容器部署
├── run.sh / stop.sh             # 开发起停脚本
├── start-prod.sh / stop-prod.sh # 生产起停脚本
└── package-release.sh           # 离线发布包
```

## 验证命令

```bash
go test ./...
cd frontend && npm run build
```

## 安全提醒

- 生产环境必须修改 `SECRET_KEY`、`DEFAULT_ADMIN_PASSWORD` 和网关 API Key。
- 不要提交 `.run/`、`backups/`、数据库文件、`.env`、Cookie、API Key、导出的浏览器 localStorage。
- 如果曾经把真实数据库或密钥提交到 Git 历史里，删除文件不足以脱敏，需要轮换密钥并重写仓库历史。
