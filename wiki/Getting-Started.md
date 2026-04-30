# 快速开始

本页用于完成第一次启动、登录、添加站点和调用聚合网关。

## 选择运行方式

| 产物 | 适合场景 | 启动方式 |
|---|---|---|
| `ai-sign-in-gateway-server-linux-amd64` | Linux VPS、NAS、服务器后台运行 | 作为 Web 服务监听 `8972` |
| `ai-sign-in-gateway-server-<goos>-<goarch>` | 其他系统或架构服务版 | 作为 Web 服务运行 |
| `ai-sign-in-gateway-windows-amd64.exe` | Windows 桌面 | 双击运行 GUI，或命令行运行 |
| `ai-sign-in-gateway-x86_64.AppImage` | Linux 桌面 | 授权后双击或命令行运行 |
| `ai-sign-in-gateway` | 当前构建机系统的桌面二进制 | 本机桌面调试或分发 |

推荐优先使用单文件 release 产物。单文件已经把 `frontend/dist` 嵌入 Go 二进制，不需要 Node.js、npm、Nginx 或额外静态资源目录。

## Linux 服务版

```bash
chmod +x ai-sign-in-gateway-server-linux-amd64

export SECRET_KEY="$(openssl rand -base64 32)"
export GATEWAY_API_KEY="$(openssl rand -base64 32)"
export DEFAULT_ADMIN_PASSWORD="<change-this-password>"
export AI_SIGN_IN_GATEWAY_HOST="0.0.0.0"
export AI_SIGN_IN_GATEWAY_PORT="8972"
export AI_SIGN_IN_GATEWAY_OPEN_BROWSER="false"
export CORS_ORIGINS="http://127.0.0.1:8972,http://localhost:8972"

./ai-sign-in-gateway-server-linux-amd64
```

打开：

```text
http://<server-ip>:8972
```

## Windows 桌面版

1. 下载 `ai-sign-in-gateway-windows-amd64.exe`。
2. 双击运行。
3. 程序会启动本地服务，并在 WebView 窗口中打开管理端。
4. 如系统提示未知发布者，确认文件来源和 SHA256 后再放行。

命令行指定配置目录：

```powershell
$env:AI_SIGN_IN_GATEWAY_CONFIG_DIR="D:\ai-sign-in-gateway-data"
$env:GATEWAY_API_KEY="change-this-gateway-key"
.\ai-sign-in-gateway-windows-amd64.exe
```

## Linux 桌面 AppImage

```bash
chmod +x ai-sign-in-gateway-x86_64.AppImage
./ai-sign-in-gateway-x86_64.AppImage
```

桌面模式会启动本地后端、前端窗口和系统托盘。托盘提供网关 24h 概览、路由健康、站点连通率检测、同步路由和探测路由等快捷入口。

## 首次登录

默认管理员只在首次初始化数据库时写入：

| 字段 | 默认值 |
|---|---|
| 用户名 | `admin` |
| 密码 | `admin123` |

首次登录后立即完成：

1. 进入 `设置 -> 账号与密码`，修改管理员账号和密码。
2. 进入 `网关中心 -> 网关策略`，设置或确认网关 API Key。
3. 进入 `站点中心` 添加站点，选择插件类型并填写凭证。
4. 对站点执行 `测试连接`、`余额`、`API Key 同步`。
5. 按需要创建分组，把站点加入不同分组。
6. 在 `设置` 中开启定时签到，并配置运行时间、并发、重试和超时。

## 发起一次网关请求

推荐入口：

```text
http://<host>:8972/api/gateway
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

客户端 Base URL 示例：

```text
http://127.0.0.1:8972/api/gateway
```

## 本地源码运行

```bash
go mod download
cd frontend && npm ci && cd ..
./run.sh
```

开发脚本默认启动：

| 服务 | 默认地址 |
|---|---|
| 前端 Vite | `http://127.0.0.1:3721` |
| 后端 API | 从 `http://127.0.0.1:8972` 开始，端口占用会自动顺延 |

停止：

```bash
./stop.sh
```
