# 开发指南

本页面向贡献者，说明本地开发、构建、测试和提交流程。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go、chi、GORM、纯 Go SQLite 驱动、JWT、bcrypt。 |
| 前端 | Vue 3、Vite、TypeScript、Ant Design Vue、vue-router。 |
| 桌面端 | Go 原生 WebView、系统托盘，不使用 Electron。 |
| 部署 | Docker、systemd、单文件 release、Windows exe、Linux AppImage。 |

## 先决条件

| 工具 | 最低版本 | 备注 |
|---|---|---|
| Go | `1.25+` | 见 `go.mod`。 |
| Node.js | `22+` | Vite 8 要求。 |
| npm | `10+` | 随 Node 22。 |
| Git | 任意 | 拉取代码和提交。 |
| `ss` | 任意 | `run.sh` 用于探测端口。 |
| Docker | `24+` | 仅容器部署需要。 |

平台：Linux、macOS。Windows 建议 WSL2。

## 本地启动

```bash
git clone <repo-url> ai-sign-in-gateway
cd ai-sign-in-gateway

cd frontend && npm ci && cd ..
go mod download
./run.sh
```

默认地址：

| 服务 | 地址 |
|---|---|
| 前端 Vite | `http://127.0.0.1:3721` |
| 后端 API | 从 `http://127.0.0.1:8972` 开始，端口占用会自动顺延 |

停止：

```bash
./stop.sh
```

## 构建

后端二进制：

```bash
go build -trimpath -ldflags "-s -w" -o ./bin/ai-sign-in-gateway ./cmd/ai-sign-in-gateway
```

前端静态资源：

```bash
cd frontend
npm run build
npm run preview
```

自包含单文件产物：

```bash
./scripts/build-single-release.sh
./scripts/build-server-single.sh
./scripts/build-desktop-single.sh
./scripts/build-windows-exe.sh
./scripts/build-appimage.sh
./scripts/build-desktop-platforms.sh
```

## 测试

```bash
go test ./...
go vet ./...
cd frontend && npm run build
```

当前测试覆盖：

- `internal/plugins/*_test.go`：插件登录、状态、邀请、API Key 同步等。
- `internal/handlers/*_test.go`：签到参与、公开邀请码、站点刷新和浏览器存储解析。
- `internal/services/gateway_service_test.go`：网关调度核心。

新增功能优先按影响面补测试：插件逻辑放 `internal/plugins`，HTTP 行为放 `internal/handlers`，网关调度放 `internal/services`。

## 仓库结构

```text
.
├── cmd/ai-sign-in-gateway/      # Go 入口、桌面壳和前端静态资源服务
├── internal/
│   ├── config/                  # 环境变量与默认配置
│   ├── database/                # GORM 初始化
│   ├── handlers/                # HTTP handler，按域分文件
│   ├── httpx/                   # 请求/响应小工具
│   ├── middleware/              # CORS / Auth 中间件
│   ├── migrations/              # 运行时轻量迁移
│   ├── models/                  # GORM 实体 + JSONMap 类型
│   ├── plugins/                 # 站点适配插件
│   ├── schemas/                 # API 请求/响应 DTO
│   ├── security/                # bcrypt / JWT
│   ├── seed/                    # 默认管理员/系统设置初始化
│   └── services/                # 业务核心：网关代理、浏览器 HTTP、TOTP
├── frontend/                    # Vue 3 管理端
├── docs/                        # 开发、部署、使用文档
├── wiki/                        # GitHub Wiki 页面源文件
└── scripts/                     # 打包和发布脚本
```

## 代码约定

Go：

- 路由和 handler 按域分文件。
- 公共 handler 工具放 `internal/handlers/helpers.go`。
- 错误处理：handler 用 `writeError(w, status, detail)`；底层返回 `error`。
- DB 写入优先用 `Updates(map[string]any{})` 限定字段。
- 时间写库统一 `time.Now().UTC()`。
- 插件接口实现 `internal/plugins/base.go::Plugin`，并在 `manager.go` 注册。

TypeScript / Vue：

- 视图层使用 `<script setup lang="ts">` 和 `<style scoped>`。
- API 调用统一走 `frontend/src/api.ts` 的 `request()` 封装。
- 类型写到 `frontend/src/types.ts`，跟后端 DTO 字段名一一对应。
- 表格高度复用 `useTableScrollHeights`。

## 贡献流程

1. Fork 并创建新分支。
2. 本地 `./run.sh` 验证。
3. `go build ./... && go vet ./... && go test ./...` 通过。
4. `cd frontend && npm run build` 通过。
5. 提交 PR，描述动机、影响面和验证结果。

Commit message 建议使用 Conventional Commits：

```text
feat(go): 批量连通测试统一时区与字段写入
fix(frontend): 修复站点表格分页 sticky 失效
chore(infra): 锁定前端端口 3721
```
