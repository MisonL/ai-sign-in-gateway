# 部署指南

本文覆盖爱签网关（ai-sign-in-gateway）从服务器准备、构建到上线运行的多种部署方式。开发环境请看 [开发指南](DEVELOPMENT.md)，功能操作请看 [使用指南](USAGE.md)。

## 部署前准备

### 必备安全变量

生产环境至少准备这些值：

```bash
export SECRET_KEY="$(openssl rand -base64 32)"
export GATEWAY_API_KEY="$(openssl rand -base64 32)"
export DEFAULT_ADMIN_USERNAME="admin"
export DEFAULT_ADMIN_PASSWORD="<change-this-password>"
```

说明：

- `SECRET_KEY` 用于后台 JWT 签名，泄露后应立即更换。
- `GATEWAY_API_KEY` 用于 `/api/gateway` 聚合网关认证，也可登录后台后在 `网关中心 -> 网关策略` 中维护。
- `DEFAULT_ADMIN_*` 只在数据库首次初始化管理员时生效；管理员创建后再改环境变量不会覆盖数据库里的账号。

### 数据目录

推荐生产数据目录：

```bash
sudo mkdir -p /var/lib/ai-sign-in-gateway
sudo chown -R "$USER":"$USER" /var/lib/ai-sign-in-gateway
export DATABASE_URL="sqlite:////var/lib/ai-sign-in-gateway/data.db"
```

`DATABASE_URL` 使用 SQLite URL：

- 相对路径：`sqlite:///data/app.db`
- 绝对路径：`sqlite:////var/lib/ai-sign-in-gateway/data.db`

### 反向代理

生产建议只对外暴露 HTTPS 域名，通过 Nginx/Caddy 反代到本机端口，例如 `127.0.0.1:8972`。对应设置：

```bash
export AI_SIGN_IN_GATEWAY_HOST=127.0.0.1
export AI_SIGN_IN_GATEWAY_PORT=8972
export AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false
export CORS_ORIGINS=https://your-domain.example
```

如果直接暴露端口，可设置 `AI_SIGN_IN_GATEWAY_HOST=0.0.0.0`，并自行配置防火墙和 HTTPS。

## 方式一：裸机/VPS 脚本部署

适合有 Go 和 Node.js 工具链的服务器。

### 1. 安装工具链

需要：

- Go `1.25+`
- Node.js `22+`
- npm `10+`
- Git
- `setsid`

Ubuntu/Debian 示例：

```bash
sudo apt-get update
sudo apt-get install -y git curl ca-certificates

GO_VERSION=1.25.0
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
  | sudo tar -C /usr/local -xz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc

curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
sudo apt-get install -y nodejs
```

国内网络可选：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
npm config set registry https://registry.npmmirror.com
```

### 2. 拉代码并构建前端

```bash
git clone <repo-url> /opt/ai-sign-in-gateway
cd /opt/ai-sign-in-gateway

go mod download
cd frontend
npm ci
npm run build
cd ..
```

后端二进制会由 `start-prod.sh` 构建到 `.run/bin/ai-sign-in-gateway`。

### 3. 启动

```bash
export AI_SIGN_IN_GATEWAY_HOST=127.0.0.1
export AI_SIGN_IN_GATEWAY_PORT=8972
export AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false
export SECRET_KEY="$(openssl rand -base64 32)"
export GATEWAY_API_KEY="$(openssl rand -base64 32)"
export DEFAULT_ADMIN_PASSWORD="<change-this-password>"
export DATABASE_URL="sqlite:////var/lib/ai-sign-in-gateway/data.db"
export CORS_ORIGINS="https://your-domain.example"

./start-prod.sh
```

验证：

```bash
curl http://127.0.0.1:8972/api/health
```

停止：

```bash
./stop-prod.sh
```

查看日志：

```bash
tail -f .run/prod.log
```

### 4. 更新版本

```bash
cd /opt/ai-sign-in-gateway
git pull
cd frontend && npm ci && npm run build && cd ..
./stop-prod.sh
./start-prod.sh
```

数据库迁移会在应用启动时自动执行。

## 方式二：systemd 托管

适合长期运行的 VPS。先按裸机方式构建一次前端和二进制：

```bash
cd /opt/ai-sign-in-gateway
cd frontend && npm ci && npm run build && cd ..
go build -trimpath -ldflags "-s -w" -o .run/bin/ai-sign-in-gateway ./cmd/ai-sign-in-gateway
```

创建 `/etc/systemd/system/ai-sign-in-gateway.service`：

```ini
[Unit]
Description=爱签网关
After=network.target

[Service]
Type=simple
User=app
WorkingDirectory=/opt/ai-sign-in-gateway
ExecStart=/opt/ai-sign-in-gateway/.run/bin/ai-sign-in-gateway
Environment=AI_SIGN_IN_GATEWAY_HOST=127.0.0.1
Environment=AI_SIGN_IN_GATEWAY_PORT=8972
Environment=AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false
Environment=DATABASE_URL=sqlite:////var/lib/ai-sign-in-gateway/data.db
Environment=SECRET_KEY=<32+ byte random string>
Environment=GATEWAY_API_KEY=<gateway bearer token>
Environment=DEFAULT_ADMIN_USERNAME=admin
Environment=DEFAULT_ADMIN_PASSWORD=<strong password>
Environment=CORS_ORIGINS=https://your-domain.example
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ai-sign-in-gateway
sudo systemctl status ai-sign-in-gateway
```

日志：

```bash
journalctl -u ai-sign-in-gateway -f
```

更新时重新构建二进制和前端后：

```bash
sudo systemctl restart ai-sign-in-gateway
```

## 方式三：Docker Compose

适合容器部署。先修改 `compose.yaml` 中的敏感变量：

```yaml
environment:
  AI_SIGN_IN_GATEWAY_CONFIG_DIR: /app/data
  DATABASE_URL: sqlite:////app/data/ai-sign-in-gateway.db
  SECRET_KEY: <32+ byte random string>
  GATEWAY_API_KEY: <gateway bearer token>
  DEFAULT_ADMIN_USERNAME: admin
  DEFAULT_ADMIN_PASSWORD: <strong password>
  CORS_ORIGINS: https://your-domain.example
```

当前 Compose 默认把容器内 `8972` 映射到宿主机 `8972`：

```yaml
ports:
  - "8972:8972"
```

如果在 1Panel 使用「容器 -> 编排」部署，编排目录选择项目根目录，Compose 文件使用 `compose.yaml`。反向代理站点指向：

```text
http://127.0.0.1:8972
```

1Panel 的目录挂载要注意：冒号右侧是容器内路径，必须写绝对路径。不要写成 `.ai-sign-in-gateway`：

```yaml
volumes:
  - /opt/1panel/www/sites/ai-sign-in-gateway/.ai-sign-in-gateway:/app/data
```

对应环境变量保持：

```yaml
environment:
  AI_SIGN_IN_GATEWAY_CONFIG_DIR: /app/data
  DATABASE_URL: sqlite:////app/data/ai-sign-in-gateway.db
```

如果启用自动备份数据库，建议额外挂载备份目录，例如：

```yaml
volumes:
  - ai-sign-in-gateway-data:/app/data
  - /opt/ai-sign-in-gateway/backups:/app/backups
```

后台设置页的备份目录填写容器内路径 `/app/backups`。

启动：

```bash
docker compose up -d --build
```

查看状态和日志：

```bash
docker compose ps
docker compose logs -f app
```

停止：

```bash
docker compose down
```

保留数据时不要删除 volume。备份数据：

```bash
docker compose exec app sh -c 'cp /app/data/ai-sign-in-gateway.db /app/data/backup.db'
```

## 方式四：Docker run

构建镜像：

```bash
./docker-build.sh
```

运行：

```bash
docker run -d \
  --name ai-sign-in-gateway \
  --restart unless-stopped \
  -p 8972:8972 \
  -e AI_SIGN_IN_GATEWAY_HOST=0.0.0.0 \
  -e AI_SIGN_IN_GATEWAY_PORT=8972 \
  -e AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false \
  -e AI_SIGN_IN_GATEWAY_CONFIG_DIR=/app/data \
  -e DATABASE_URL=sqlite:////app/data/ai-sign-in-gateway.db \
  -e SECRET_KEY="$(openssl rand -base64 32)" \
  -e GATEWAY_API_KEY="<gateway bearer token>" \
  -e DEFAULT_ADMIN_PASSWORD="<strong password>" \
  -e CORS_ORIGINS="https://your-domain.example" \
  -v ai-sign-in-gateway-data:/app/data \
  ai-sign-in-gateway:local
```

## 方式五：离线分发包

适合在构建机联网、生产机不安装 Go/Node 的场景。

构建机：

```bash
git clone <repo-url> ai-sign-in-gateway
cd ai-sign-in-gateway
./package-release.sh
```

生成：

```text
.release/ai-sign-in-gateway-<timestamp>.tar.gz
```

压缩包包含二进制、`frontend/dist`、`docs/`、`README.md`、`start-prod.sh` 和 `stop-prod.sh`。

生产机：

```bash
tar -xzf ai-sign-in-gateway-<timestamp>.tar.gz
cd ai-sign-in-gateway

export AI_SIGN_IN_GATEWAY_HOST=127.0.0.1
export AI_SIGN_IN_GATEWAY_PORT=8972
export AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false
export DATABASE_URL=sqlite:////var/lib/ai-sign-in-gateway/data.db
export SECRET_KEY="<32+ byte random string>"
export GATEWAY_API_KEY="<gateway bearer token>"
export DEFAULT_ADMIN_PASSWORD="<strong password>"
export CORS_ORIGINS="https://your-domain.example"

./start-prod.sh
```

## 方式六：单文件分发包

适合 VPS/1Panel 这类无桌面服务器，也适合本地自用、桌面分发或无需安装服务的单机部署。

```bash
git clone <repo-url> ai-sign-in-gateway
cd ai-sign-in-gateway

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

所有单文件脚本都会先构建 `frontend/dist`，再把静态资源嵌入 Go 二进制。服务版产物默认监听 `0.0.0.0:8972`、不自动打开浏览器，端口被占用时继续沿用自动偏移策略，不需要额外复制前端目录。桌面产物不依赖外部 `frontend/dist`，默认启用原生桌面壳：

- 主进程启动本地 Go 服务。
- 桌面窗口使用系统 WebView 加载完整前端，不使用 Electron。
- 托盘提供网关 24h 简要统计、路由健康、当前并发、站点连通率检测、同步路由、探测全部网关路由和打开关键页面。

默认输出：

```text
.release/ai-sign-in-gateway-server-linux-amd64
.release/ai-sign-in-gateway
.release/ai-sign-in-gateway-windows-amd64.exe
.release/ai-sign-in-gateway-x86_64.AppImage
```

Windows exe 默认使用 GUI 子系统，不弹控制台窗口。需要调试日志时可构建控制台版：

```bash
WINDOWS_GUI=false ./scripts/build-windows-exe.sh
```

AppImage 构建依赖 AppImageKit 官方 `appimagetool`。如果 PATH 中没有可用工具，脚本会尝试下载；也可以手动指定：

```bash
APPIMAGETOOL=/path/to/appimagetool-x86_64.AppImage ./scripts/build-appimage.sh
```

直接运行桌面产物时默认启动两个本地端口：桌面窗口入口 `127.0.0.1:3721`，后端/API/网关 `127.0.0.1:8972`。可通过 `AI_SIGN_IN_GATEWAY_FRONTEND_PORT` 和 `AI_SIGN_IN_GATEWAY_BACKEND_PORT` 覆盖；如果只想作为本地服务运行，可设置 `AI_SIGN_IN_GATEWAY_DESKTOP=false`。

Linux AppImage 使用系统 GTK/WebKitGTK 运行桌面窗口。构建机需要 `pkg-config`、`gtk+-3.0`、`webkit2gtk-4.0/4.1` 开发文件；运行机需要对应运行库。Windows exe 使用系统 WebView2 运行时。

## 方式七：GitHub Release 自动发布

适合在本机完成打包后，把同一批产物发布到 GitHub Release，并同步到 `release` 分支供服务器或 1Panel 下载。

前置条件：

- 已安装并登录 GitHub CLI：`gh auth login`
- 当前仓库已配置 GitHub remote，默认使用 `origin`
- 构建机具备对应打包依赖：Go、npm、AppImageKit、Windows 交叉编译工具链等

```bash
# 创建/更新 tag，执行本地打包，上传 GitHub Release，并同步 release 分支
./scripts/release.sh v1.0.0

# 如果已经执行过 ./scripts/build-single-release.sh，可直接发布现有产物
./scripts/release.sh v1.0.0 --skip-build
```

常用覆盖项：

```bash
GH_REPO=owner/repo ./scripts/release.sh v1.0.0
GIT_REMOTE=github ./scripts/release.sh v1.0.0
RELEASE_BRANCH=release ./scripts/release.sh v1.0.0
```

`release` 分支是纯产物分支。脚本会在临时目录中重写该分支，分支根目录只保留最新发布文件、`SHA256SUMS`、`RELEASE_NOTES.md`、`RELEASE.txt` 和说明文件，不保留源码内容。历史版本以 GitHub Release 为准。

服务版生产运行示例。监听地址、端口、是否打开浏览器已经写入服务版产物，不需要再手动设置 `AI_SIGN_IN_GATEWAY_HOST`、`AI_SIGN_IN_GATEWAY_PORT`、`AI_SIGN_IN_GATEWAY_OPEN_BROWSER` 或 `AI_SIGN_IN_GATEWAY_DESKTOP`：

```bash
DATABASE_URL=sqlite:////var/lib/ai-sign-in-gateway/data.db \
SECRET_KEY="<32+ byte random string>" \
GATEWAY_API_KEY="<gateway bearer token>" \
./ai-sign-in-gateway-server-linux-amd64
```

## Nginx 反向代理示例

```nginx
server {
    listen 80;
    server_name your-domain.example;

    location / {
        proxy_pass http://127.0.0.1:8972;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 流式网关需要关闭缓冲。
        proxy_buffering off;
        proxy_read_timeout 600s;
    }
}
```

使用 Caddy：

```caddyfile
your-domain.example {
    reverse_proxy 127.0.0.1:8972 {
        flush_interval -1
    }
}
```

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `APP_NAME` | `爱签网关` | 应用显示名 |
| `AI_SIGN_IN_GATEWAY_HOST` | `127.0.0.1` | 监听地址 |
| `AI_SIGN_IN_GATEWAY_PORT` | `8972` | 服务模式后端/API/网关端口；兼容旧变量 |
| `AI_SIGN_IN_GATEWAY_BACKEND_PORT` | `8972` | 桌面模式后端/API/网关端口，优先级高于 `AI_SIGN_IN_GATEWAY_PORT` |
| `AI_SIGN_IN_GATEWAY_FRONTEND_PORT` | `3721` | 桌面模式前端窗口入口端口 |
| `AI_SIGN_IN_GATEWAY_OPEN_BROWSER` | `true` | 是否启动后自动打开浏览器 |
| `DATABASE_URL` | `sqlite:///~/.ai-sign-in-gateway/ai-sign-in-gateway.db` | 数据库地址 |
| `SECRET_KEY` | `change-me-in-production-at-least-32-bytes` | JWT 签名密钥，生产必须修改 |
| `GATEWAY_API_KEY` | 空 | 初始化系统设置里的网关 API Key；空值表示不校验网关 Bearer，生产必须设置 |
| `ALGORITHM` | `HS256` | JWT 算法 |
| `ACCESS_TOKEN_EXPIRE_MINUTES` | `1440` | 后台登录 token 有效分钟数 |
| `DEFAULT_ADMIN_USERNAME` | `admin` | 首次初始化管理员用户名 |
| `DEFAULT_ADMIN_PASSWORD` | `admin123` | 首次初始化管理员密码 |
| `SCHEDULER_TIMEZONE` | `Asia/Shanghai` | 定时签到默认时区 |
| `CORS_ORIGINS` | `http://localhost:3721,http://127.0.0.1:3721` | 允许跨域来源，逗号分隔 |
| `MANAGED_BROWSER_PROFILE_ROOT` | `~/.ai-sign-in-gateway/browser-profiles` | 浏览器资料目录，占位能力 |
| `MANAGED_BROWSER_HEADLESS` | `false` | 浏览器自动化是否无头，占位能力 |
| `MANAGED_BROWSER_TIMEOUT_SECONDS` | `20` | 浏览器自动化超时，占位能力 |
| `MANAGED_BROWSER_SETTLE_MS` | `1200` | 浏览器自动化页面稳定等待，占位能力 |

## 运维操作

健康检查：

```bash
curl http://127.0.0.1:8972/api/health
```

备份 SQLite：

```bash
sqlite3 /var/lib/ai-sign-in-gateway/data.db ".backup '/var/lib/ai-sign-in-gateway/data-$(date +%F).db'"
```

如果没有 `sqlite3` 命令，可在停服务后复制 `.db` 文件。

恢复：

```bash
./stop-prod.sh
cp /path/to/backup.db /var/lib/ai-sign-in-gateway/data.db
./start-prod.sh
```

升级：

1. 备份数据库。
2. 拉取新代码或替换新包。
3. 重建前端和二进制，或重新构建 Docker 镜像。
4. 重启服务。
5. 查看 `/api/health` 和后台页面。

## 上线检查清单

- 已修改默认管理员密码。
- 已修改 `SECRET_KEY`。
- 已设置网关 API Key。
- `CORS_ORIGINS` 与公网域名一致。
- 数据库目录有持久化和备份策略。
- 反向代理关闭流式请求缓冲。
- 没有把 `.run/`、`backups/`、数据库、`.env` 或真实 Cookie/API Key 提交到仓库。
