# 配置参考

程序主要通过环境变量配置。未设置时会使用默认值。

## 通用变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `APP_NAME` | `爱签网关` | 应用显示名。 |
| `AI_SIGN_IN_GATEWAY_HOST` | `127.0.0.1`；服务版构建默认为 `0.0.0.0` | 监听地址。 |
| `AI_SIGN_IN_GATEWAY_PORT` | `8972` | 服务模式后端、API、网关端口。 |
| `AI_SIGN_IN_GATEWAY_BACKEND_PORT` | `8972` | 桌面模式后端、API、网关端口，优先级高于 `AI_SIGN_IN_GATEWAY_PORT`。 |
| `AI_SIGN_IN_GATEWAY_FRONTEND_PORT` | `3721` | 桌面模式前端窗口入口端口。 |
| `AI_SIGN_IN_GATEWAY_OPEN_BROWSER` | `true`；服务版构建默认为 `false` | 启动后是否自动打开浏览器。 |
| `AI_SIGN_IN_GATEWAY_DESKTOP` | 桌面构建为 `true` | 是否启用 WebView 主窗口和托盘；设为 `false` 可退回本地 Web 服务模式。 |
| `AI_SIGN_IN_GATEWAY_DESKTOP_DEBUG` | `false` | 桌面 WebView 调试开关。 |
| `AI_SIGN_IN_GATEWAY_CONFIG_DIR` | `~/.ai-sign-in-gateway` | 用户配置、日志、数据库默认目录。 |
| `SCHEDULER_TIMEZONE` | `Asia/Shanghai` | 定时签到时区。 |
| `CORS_ORIGINS` | 本地开发域名 | 允许的前端来源，多个值用英文逗号分隔。 |

## 安全变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SECRET_KEY` | 开发默认值 | JWT 签名密钥，生产必须修改。 |
| `GATEWAY_API_KEY` | 空 | 初始化聚合网关 Bearer Key；空值表示不校验网关 Bearer，生产必须设置。 |
| `DEFAULT_ADMIN_USERNAME` | `admin` | 首次初始化管理员用户名。 |
| `DEFAULT_ADMIN_PASSWORD` | `admin123` | 首次初始化管理员密码，生产必须修改。 |
| `ALGORITHM` | `HS256` | JWT 签名算法。 |
| `ACCESS_TOKEN_EXPIRE_MINUTES` | `1440` | 管理端登录 token 有效期，单位分钟。 |

`DEFAULT_ADMIN_*` 只在首次初始化数据库时生效。管理员创建后再修改环境变量不会覆盖数据库里的账号。

## 数据库变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `DATABASE_URL` | `sqlite:///<配置目录>/ai-sign-in-gateway.db` | SQLite 数据库地址。 |

SQLite 路径格式：

```bash
export DATABASE_URL="sqlite:///relative/path.db"
export DATABASE_URL="sqlite:////var/lib/ai-sign-in-gateway/data.db"
```

Windows 路径建议优先使用 `AI_SIGN_IN_GATEWAY_CONFIG_DIR` 指定数据目录，让程序自动生成默认数据库路径。

## 托管浏览器变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `MANAGED_BROWSER_PROFILE_ROOT` | `<配置目录>/browser-profiles` | 托管浏览器 profile 目录。 |
| `MANAGED_BROWSER_HEADLESS` | `false` | 托管浏览器是否无头运行。 |
| `MANAGED_BROWSER_TIMEOUT_SECONDS` | `20` | 托管浏览器操作超时。 |
| `MANAGED_BROWSER_SETTLE_MS` | `1200` | 浏览器页面稳定等待时间，单位毫秒。 |

## 默认数据目录

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
