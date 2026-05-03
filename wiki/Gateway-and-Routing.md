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

## 策略

主策略在 `网关中心 -> 网关策略` 配置。

| 策略 | 适用场景 |
|---|---|
| Smart 评分 | 综合延迟、并发、失败次数、优先级做选择，适合多数生产场景。 |
| Round Robin | 均匀轮询，适合路由质量接近时分摊请求。 |
| Latency First | 优先低延迟路由，适合追求响应速度。 |
| Priority | 严格按 `priority` 数值升序选择路由，适合主备或成本分层。 |

`Priority` 策略下，数值越小优先级越高。只要高优先级路由未达到单路由并发上限，网关会继续使用该路由；延迟、权重、当前并发均衡和近期失败降权不会把流量提前分配给低优先级路由。路由被停用、缺少 API Key、熔断打开或达到并发上限后，才会转到下一条可用路由。

## 并发控制

网关策略中可配置单路由并发上限、并发转移策略和并发溢出优先级。

| 配置 | 说明 |
|---|---|
| 单路由并发上限 | 例如填 `5`，单条路由当前并发达到 5 后，新请求会转到其它未满路由。填 `0` 表示不限制。 |
| 并发达上限转移 | 保持当前主策略排序，只有当前候选路由达到并发上限才转移。 |
| 并发均衡转移 | 非 `Priority` 主策略下，在未满上限的候选路由中优先选择当前并发更低的路由。 |

并发溢出优先级只在候选路由都达到并发上限后参与排序：

| 策略 | 说明 |
|---|---|
| `Latency First` | 优先低延迟路由。 |
| `Sequential` | 按当前主策略顺序继续尝试。`Priority` 主策略下仍按优先级升序。 |

## 网关监控

网关中心提供 24h 概览、实时调用和时间段消耗查询。

时间段消耗支持按开始/结束时间查询，并按路由列出请求数、成功率、Token 使用量和费用。官方费用按以下单价估算：

| 类型 | 单价 |
|---|---|
| Input | `$5 / 1M tokens` |
| Cached Input | `$0.5 / 1M tokens` |
| Output | `$30 / 1M tokens` |

请求日志会记录 `prompt_tokens`、`cached_input_tokens`、`completion_tokens`、`total_tokens` 和上游返回的 `usage_cost`。如果上游响应不返回 usage 字段，对应 Token 和费用会显示为 0 或空值。

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
