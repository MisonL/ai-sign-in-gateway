# 安全指南

本页记录生产安全、密钥管理和敏感数据处理建议。

## 必改项

生产环境必须修改：

| 配置 | 原因 |
|---|---|
| `SECRET_KEY` | 用于后台 JWT 签名，泄露后可伪造或延长登录状态。 |
| `DEFAULT_ADMIN_PASSWORD` | 默认密码只适合本地首次启动。 |
| `GATEWAY_API_KEY` | 控制 `/api/gateway` 调用权限，泄露后可能产生上游费用或额度消耗。 |

建议在第一次启动前设置这些变量。`DEFAULT_ADMIN_*` 只在数据库首次初始化时生效。

## 不应提交到仓库的内容

- `.run/`
- `backups/`
- SQLite 数据库文件。
- `.env`。
- Cookie。
- API Key。
- access token、refresh token。
- 浏览器 localStorage/sessionStorage 导出内容。
- 含真实域名、账号或密钥的日志。

## 公网部署

公网部署建议：

1. 使用 HTTPS。
2. 反向代理只转发到本机 `127.0.0.1:8972`。
3. 设置 `CORS_ORIGINS=https://your-domain.example`。
4. 使用强管理员密码。
5. 使用随机 `SECRET_KEY` 和 `GATEWAY_API_KEY`。
6. 限制服务器 SSH 登录和防火墙入站端口。
7. 定期备份并保护数据库文件。

## 凭证泄露处理

如果怀疑 `GATEWAY_API_KEY` 泄露：

1. 立即在后台 `网关中心 -> 网关策略` 更换 Key。
2. 更新所有客户端配置。
3. 检查请求历史是否有异常调用。

如果怀疑上游 API Key 泄露：

1. 在上游站点撤销或轮换 Key。
2. 在本系统中同步或替换 API Key。
3. 执行 `同步路由`。
4. 检查相关路由的请求历史。

如果怀疑 `SECRET_KEY` 泄露：

1. 修改 `SECRET_KEY`。
2. 重启服务。
3. 让管理员重新登录。

如果真实数据库曾被提交到 Git 历史：

1. 轮换所有管理员密码、网关 Key 和上游 API Key。
2. 删除当前文件不足以脱敏，需要清理 Git 历史。
3. 视情况重建仓库或使用专门工具清理历史。

## 浏览器存储导入风险

浏览器存储导入可能包含 token、refresh token、cookie、邮箱和 User-Agent。只在本地可信环境使用，不要把导出内容放入 issue、PR、日志或聊天记录。

## Issue 和 PR

提交 issue 或 PR 时：

- 使用脱敏 URL，例如 `https://example.com`。
- 使用假 Key，例如 `sk-***`。
- 不附带真实数据库、日志或截图中的明文凭证。
- 报错堆栈只保留必要上下文。
