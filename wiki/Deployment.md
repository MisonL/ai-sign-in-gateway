# 部署指南

本页覆盖生产上线、长期运行、容器部署和反向代理。

## 部署前准备

生产环境至少准备：

```bash
export SECRET_KEY="$(openssl rand -base64 32)"
export GATEWAY_API_KEY="$(openssl rand -base64 32)"
export DEFAULT_ADMIN_USERNAME="admin"
export DEFAULT_ADMIN_PASSWORD="<change-this-password>"
```

说明：

| 变量 | 用途 |
|---|---|
| `SECRET_KEY` | 后台 JWT 签名密钥，泄露后应立即更换。 |
| `GATEWAY_API_KEY` | `/api/gateway` 聚合网关认证，也可登录后台后在 `网关中心 -> 网关策略` 中维护。 |
| `DEFAULT_ADMIN_*` | 只在数据库首次初始化管理员时生效；管理员创建后再改环境变量不会覆盖数据库。 |

推荐生产数据目录：

```bash
sudo mkdir -p /var/lib/ai-sign-in-gateway
sudo chown -R "$USER":"$USER" /var/lib/ai-sign-in-gateway
export DATABASE_URL="sqlite:////var/lib/ai-sign-in-gateway/data.db"
```

## 单文件服务版

服务版产物已内嵌前端，适合 VPS、NAS 和 1Panel。

```bash
chmod +x ai-sign-in-gateway-server-linux-amd64

DATABASE_URL=sqlite:////var/lib/ai-sign-in-gateway/data.db \
SECRET_KEY="<32+ byte random string>" \
GATEWAY_API_KEY="<gateway bearer token>" \
DEFAULT_ADMIN_PASSWORD="<strong password>" \
./ai-sign-in-gateway-server-linux-amd64
```

服务版默认监听 `0.0.0.0:8972`，不自动打开浏览器。

## systemd 托管

创建 `/etc/systemd/system/ai-sign-in-gateway.service`：

```ini
[Unit]
Description=AI Sign In Gateway
After=network.target

[Service]
Type=simple
User=ai-gateway
WorkingDirectory=/opt/ai-sign-in-gateway
Environment=AI_SIGN_IN_GATEWAY_HOST=127.0.0.1
Environment=AI_SIGN_IN_GATEWAY_PORT=8972
Environment=AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false
Environment=AI_SIGN_IN_GATEWAY_CONFIG_DIR=/var/lib/ai-sign-in-gateway
Environment=DATABASE_URL=sqlite:////var/lib/ai-sign-in-gateway/data.db
Environment=SECRET_KEY=<32+ byte random string>
Environment=GATEWAY_API_KEY=<gateway bearer token>
Environment=DEFAULT_ADMIN_USERNAME=admin
Environment=DEFAULT_ADMIN_PASSWORD=<strong password>
Environment=CORS_ORIGINS=https://your-domain.example
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
sudo systemctl status ai-sign-in-gateway
```

日志：

```bash
journalctl -u ai-sign-in-gateway -f
```

## Docker Compose

启动前修改 `compose.yaml` 中的敏感变量：

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

保留数据时不要删除 volume。

## Docker run

```bash
./docker-build.sh

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

## 反向代理

生产建议只对外暴露 HTTPS 域名，通过 Nginx 或 Caddy 反代到本机端口。

Nginx 示例：

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

        proxy_buffering off;
        proxy_read_timeout 600s;
    }
}
```

Caddy 示例：

```caddyfile
your-domain.example {
    reverse_proxy 127.0.0.1:8972 {
        flush_interval -1
    }
}
```

## 上线检查清单

- 已修改默认管理员密码。
- 已修改 `SECRET_KEY`。
- 已设置网关 API Key。
- `CORS_ORIGINS` 与公网域名一致。
- 数据库目录有持久化和备份策略。
- 反向代理关闭流式请求缓冲。
- 没有把 `.run/`、`backups/`、数据库、`.env` 或真实 Cookie/API Key 提交到仓库。
