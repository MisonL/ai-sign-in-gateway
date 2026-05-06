# 网关与路由

聚合网关把多个上游站点和 API Key 汇聚为统一模型请求入口。

## 网关地址

推荐入口：

```text
http://<host>:8972/api/gateway
```

兼容入口：

```text
http://<host>:8972/api/gateway/v1
```

无需在客户端里强行追加 `/v1`。系统会把 `/api/gateway` 后面的路径透明转发到上游。

## 认证

所有网关请求使用 Bearer Key：

```http
Authorization: Bearer <GATEWAY_API_KEY>
```

Key 可通过环境变量初始化，也可在后台 `网关中心 -> 网关策略` 中修改。

## OpenAI 风格请求

```bash
curl http://127.0.0.1:8972/api/gateway/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "ping"}]
  }'
```

按分组路由：

```bash
curl "http://127.0.0.1:8972/api/gateway/chat/completions?group=main" \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}]}'
```

## 路由筛选

| 参数 | 说明 |
|---|---|
| `?group=<分组>` | 只使用指定分组的路由。 |
| `?type=claude` | 手动指定 Claude 路由。 |
| `?type=codex` | 手动指定 GPT/OpenAI 兼容路由。 |
| `?type=gemini` | 手动指定 Gemini 路由。 |

不传 `type` 时，系统会根据请求体中的 `model` 自动推断 Claude、GPT/Codex 或 Gemini 路由。

## 支持模型与探测

路由可声明 `supported_models`，用于请求体带 `model` 时做精确匹配。模型探测会调用上游 OpenAI 兼容 `/models` 接口并把结果写回路由支持模型。

如果 Codex/OpenAI 兼容路由的 `/models` 请求成功但返回空列表，系统会默认写入 `gpt-5.4,gpt-5.5`，避免路由列表显示无模型但编辑弹窗仍保留旧模型的状态不一致。

## 策略

主策略在 `网关中心 -> 网关策略` 配置。

| 策略 | 适用场景 |
|---|---|
| Smart 评分 | 综合延迟、并发、失败次数、优先级做选择，适合多数生产场景。 |
| Round Robin | 均匀轮询，适合路由质量接近时分摊请求。 |
| Latency First | 优先低延迟路由，适合追求响应速度。 |
| Priority | 优先使用优先级更高的路由，适合主备或成本分层。 |

## 并发溢出

当某条路由达到并发限制时，系统会按配置选择其它路由：

| 策略 | 说明 |
|---|---|
| `Latency First` | 优先低延迟路由。 |
| `Priority` | 优先高优先级路由。 |
| `Smart` | 继续使用综合评分。 |

## 熔断

路由连续失败达到阈值后会进入 `open` 状态，在冷却时间内不参与调度。冷却结束后进入 `half_open`，允许一次探测；成功恢复，失败继续熔断。

可在路由列表中执行 `熔断重置`。

## 流式请求

如果前面有反向代理，需要关闭响应缓冲。

Nginx：

```nginx
proxy_buffering off;
proxy_read_timeout 600s;
```

Caddy：

```caddyfile
reverse_proxy 127.0.0.1:8972 {
    flush_interval -1
}
```

## 路由不可用排查

如果网关提示没有可用路由，检查：

1. 站点是否启用。
2. 站点是否有 API Key。
3. 路由是否同步。
4. 路由类型是否与请求 `type` 或 `model` 匹配。
5. 分组筛选是否过窄。
6. 路由是否被停用或处于熔断状态。
