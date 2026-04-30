# 站点与插件

站点插件决定登录、签到、余额、API Key 同步和网关出口的处理方式。

## 插件类型

| 插件 | 用途 |
|---|---|
| `http-relay-station` | 通用 HTTP 站点，适合自定义登录、状态、签到接口。 |
| `api-supplier` | 仅作为网关供应商记录，不登录、不签到。 |
| `sub2api-platform` | 适配 sub2api 类站点，支持 token 或账号密码登录、签到、API Key 同步。 |
| `yellowpeach-newapi` | 适配 NewAPI 类站点，支持 cookie、token、账号密码、签到、API Key 同步。 |

## 常用凭证字段

| 字段 | 说明 |
|---|---|
| `api_key` | 单个模型请求 Key。 |
| `api_keys` | 同步得到的多 Key 列表，网关会展开成多条路由。 |
| `access_token` / `refresh_token` / `cookie` | 用于站内登录、状态、签到或同步接口。 |
| `username` / `email` / `password` | 用于自动登录。 |
| `totp_secret` / `totp_otpauth_url` | 二次验证，系统会自动生成当前验证码。 |
| `gateway_request_url` / `gateway_request_urls` | 上游模型请求地址，支持多个出口按顺序回退。 |
| `api_request_urls` | 插件或网关可用的 API 请求地址列表。 |
| `gateway_priority` / `gateway_weight` | 网关优先级和权重。 |

不要把这些凭证提交到 Git。

## 添加站点流程

1. 进入 `站点中心`。
2. 点击 `新增站点`。
3. 填写名称、站点地址、插件类型和分组。
4. 按插件提示填写凭证字段。
5. 点击 `测试连接`。
6. 保存站点。
7. 如果插件支持，执行 `余额` 和 `API Key 同步`。
8. 到 `网关中心` 执行 `同步路由`。

## 浏览器存储导入

入口：

```text
新增/编辑站点 -> 浏览器存储导入
```

支持：

- 粘贴 JSON 对象。
- 粘贴被字符串包裹的 JSON。
- 从 localStorage 中提取 token、refresh token、用户邮箱、User-Agent 等。
- 自动建议插件类型、站点名称和站点地址。

注意：

- 只在本地可信环境使用。
- 导出的 token、cookie 等同账号凭证，不要提交到仓库或分享给他人。

## TOTP

支持两种输入：

| 字段 | 说明 |
|---|---|
| `totp_secret` | Base32 TOTP Secret。 |
| `totp_otpauth_url` | 标准 `otpauth://` URL。 |

系统会在插件执行登录或验证时生成当前验证码。

## 插件选择建议

| 目标站点 | 推荐插件 |
|---|---|
| NewAPI 类中转站 | `yellowpeach-newapi` |
| sub2api 类平台 | `sub2api-platform` |
| 只有模型 API Key，不需要登录签到 | `api-supplier` |
| 自定义 HTTP 登录、状态或签到接口 | `http-relay-station` |
