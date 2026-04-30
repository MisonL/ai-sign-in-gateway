# 运维与排障

本页记录健康检查、备份恢复、升级和常见问题。

## 健康检查

```bash
curl http://127.0.0.1:8972/api/health
```

期望返回：

```json
{"status":"ok"}
```

## 日志位置

| 运行方式 | 日志 |
|---|---|
| 开发脚本 | `.run/backend.log`、`.run/frontend.log` |
| 生产脚本 | `.run/prod.log` |
| systemd | `journalctl -u ai-sign-in-gateway -f` |
| Docker Compose | `docker compose logs -f app` |
| 默认配置目录 | `~/.ai-sign-in-gateway/logs/shell.log` |

## 备份 SQLite

推荐使用 `sqlite3` 在线备份：

```bash
sqlite3 /var/lib/ai-sign-in-gateway/data.db ".backup '/var/lib/ai-sign-in-gateway/data-$(date +%F).db'"
```

如果没有 `sqlite3` 命令，可在停服务后复制 `.db` 文件。

```bash
cp ~/.ai-sign-in-gateway/ai-sign-in-gateway.db ./ai-sign-in-gateway.db.bak
```

## 恢复

```bash
./stop-prod.sh
cp /path/to/backup.db /var/lib/ai-sign-in-gateway/data.db
./start-prod.sh
```

Docker Compose 可先停容器，再替换 volume 中的数据库文件。

## 升级

1. 备份数据库。
2. 拉取新代码或替换新单文件产物。
3. 如使用源码部署，重建前端和二进制。
4. 重启服务。
5. 查看 `/api/health` 和后台页面。

数据库迁移会在应用启动时自动执行。

## 常见问题

### 登录后提示无权限或状态失效

可能原因：

- `SECRET_KEY` 变更导致旧 token 失效。
- 浏览器保存了旧 token。

处理：重新登录，必要时清理浏览器 localStorage。

### 前端打不开，提示 `frontend/dist` 不存在

生产模式下后端直接服务 `frontend/dist`。先构建前端：

```bash
cd frontend
npm ci
npm run build
cd ..
./start-prod.sh
```

开发模式建议直接使用：

```bash
./run.sh
```

单文件 release 产物已内嵌前端，不需要外部 `frontend/dist`。

### 网关返回 401

检查：

- `Authorization: Bearer <key>` 是否正确。
- 后台 `网关策略` 中的 API Key 是否与客户端一致。
- 环境变量 `GATEWAY_API_KEY` 是否只在首次初始化时生效，后续已被数据库设置覆盖。

### 网关提示没有可用路由

检查：

1. 站点是否启用。
2. 站点是否有 API Key。
3. 路由是否同步。
4. 路由类型是否与请求 `type` 或 `model` 匹配。
5. 分组筛选是否过窄。
6. 路由是否被停用或处于熔断状态。

### 签到状态不显示

如果站点被设置为 `禁止` 签到，列表不会显示签到状态。这是预期行为，表示该站点不参与签到任务。

### API Key 同步不到

检查：

- 登录凭证是否有效。
- 是否先执行过 `测试连接`。
- 插件类型是否正确。
- 通用 HTTP 插件是否配置了正确的状态接口路径和字段路径。

### 流式请求不流畅

如果前面有反向代理，确认关闭响应缓冲：

```nginx
proxy_buffering off;
proxy_read_timeout 600s;
```

## 数据安全

- 不要把 Cookie、access token、refresh token、API Key、数据库、浏览器存储导出内容提交到 Git。
- 公开部署时务必使用 HTTPS。
- 定期备份 SQLite 数据库。
- API Key 泄露后应立即在上游站点和本系统中轮换。
