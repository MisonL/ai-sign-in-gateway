# 爱签网关 Wiki

爱签网关（`ai-sign-in-gateway`）是一个单机优先的 API 中转站账号管理与聚合网关工具。它面向 NewAPI、sub2api、自定义 HTTP 站点和仅作为模型出口的供应商站点，集中处理站点管理、自动签到、余额同步、API Key 池化、分组路由、健康探测、熔断回退和请求历史。

## 核心能力

| 能力 | 说明 |
|---|---|
| 单文件运行 | 推荐使用内嵌前端的 release 产物，复制一个二进制即可运行完整 Web 管理端、后端 API 和聚合网关。 |
| 站点管理 | 支持多站点、多插件、多分组，集中维护登录凭证、API Key、邀请信息和状态。 |
| 自动签到 | 支持单站点、批量和定时签到，可配置并发、重试、超时和时区。 |
| 聚合网关 | 统一入口为 `/api/gateway`，兼容 OpenAI 风格客户端，并支持 Claude、GPT/Codex、Gemini 路由类型。 |
| 路由策略 | 支持 Smart、Round Robin、Latency First、Priority，并带并发溢出和熔断恢复。 |
| 桌面壳 | Windows exe 和 Linux AppImage 使用系统 WebView/托盘，不使用 Electron。 |
| 默认 SQLite | 数据默认保存在本机目录，适合 VPS、NAS、桌面长期运行和迁移。 |

## 推荐阅读顺序

| 页面 | 适合场景 |
|---|---|
| [快速开始](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Getting-Started) | 第一次下载、启动、登录和发起网关请求。 |
| [部署指南](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Deployment) | 服务器、Docker、systemd、反向代理和长期运行。 |
| [使用指南](https://github.com/vikingleo/ai-sign-in-gateway/wiki/User-Guide) | 登录、站点、签到、分组、API Key、请求历史等日常操作。 |
| [网关与路由](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Gateway-and-Routing) | `/api/gateway`、认证、路由选择、熔断和流式代理。 |
| [站点与插件](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Site-Plugins) | 插件类型、凭证字段、浏览器存储导入和 TOTP。 |
| [配置参考](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Configuration) | 环境变量、数据目录、端口和安全配置。 |
| [运维与排障](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Operations-and-Troubleshooting) | 健康检查、日志、备份恢复、升级和常见问题。 |
| [开发指南](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Development) | 本地开发、测试、构建和贡献约定。 |
| [发布流程](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Release-Process) | 单文件、Windows exe、AppImage 和 GitHub Release 发布。 |
| [安全指南](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Security) | 生产安全、密钥管理、凭证泄露处理和提交前检查。 |
| [Wiki 发布说明](https://github.com/vikingleo/ai-sign-in-gateway/wiki/Wiki-Publishing) | 如何把本目录同步到 GitHub Wiki 仓库。 |

## 最短启动路径

Linux 服务版示例：

```bash
chmod +x ai-sign-in-gateway-server-linux-amd64

export SECRET_KEY="$(openssl rand -base64 32)"
export GATEWAY_API_KEY="$(openssl rand -base64 32)"
export DEFAULT_ADMIN_PASSWORD="<change-this-password>"
export AI_SIGN_IN_GATEWAY_HOST="0.0.0.0"
export AI_SIGN_IN_GATEWAY_PORT="8972"
export AI_SIGN_IN_GATEWAY_OPEN_BROWSER="false"

./ai-sign-in-gateway-server-linux-amd64
```

访问：

```text
http://<server-ip>:8972
```

首次数据库初始化时的默认管理员：

| 字段 | 默认值 |
|---|---|
| 用户名 | `admin` |
| 密码 | `admin123` |

生产环境必须在第一次启动前设置 `DEFAULT_ADMIN_PASSWORD`，登录后再进入 `设置 -> 账号与密码` 修改管理员账号和密码。

## 项目结构

```text
.
├── cmd/ai-sign-in-gateway/      # Go 入口、桌面壳和前端静态资源服务
├── internal/                    # 后端 handlers/services/plugins/models/migrations
├── frontend/                    # Vue 3 管理端
├── docs/                        # 开发、部署、使用文档
├── wiki/                        # GitHub Wiki 页面源文件
├── scripts/                     # 单文件、Windows exe、AppImage、GitHub Release 脚本
├── Dockerfile / compose.yaml    # 容器部署
├── run.sh / stop.sh             # 开发起停脚本
├── start-prod.sh / stop-prod.sh # 源码生产起停脚本
└── package-release.sh           # 传统离线发布包
```

## 生产安全底线

- 修改 `SECRET_KEY`、`DEFAULT_ADMIN_PASSWORD` 和 `GATEWAY_API_KEY`。
- 不要提交 `.run/`、`backups/`、数据库文件、`.env`、Cookie、API Key 或浏览器存储导出内容。
- 公网部署时使用 HTTPS，并将 `CORS_ORIGINS` 设置为真实域名。
- 定期备份 SQLite 数据库。
