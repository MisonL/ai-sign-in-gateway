# UI 设计语言重构方案

日期: 2026-05-21

## 背景事实

- 当前仓库前端为 Vue 3、Vite、TypeScript、Ant Design Vue，依赖记录在 `frontend/package.json`。
- 当前全局样式入口是 `frontend/src/style.css`，已经定义 IBM Plex Sans 字体、浅色页面背景、Ant Design Vue token 覆盖和大量业务页面样式。
- 当前壳层是 `frontend/src/components/ShellLayout.vue`，包含左侧导航、顶部 KPI、分组管理、用户入口和退出登录。
- 当前运行中的 Docker 服务暴露 `8972`，登录页和总览页可访问。
- 参考站截图的核心效果是白底运营后台: 蓝色主导航、窄边框卡片、紧凑指标面板、浅灰页面底色、清晰图标按钮和低装饰密度。
- 参考站技术栈为 React、shadcn/ui、Tailwind CSS、Radix UI 和 Lucide，但本项目无需为了视觉目标先迁移框架。

## 设计目标

本次重构目标不是复制参考站源码或技术栈，而是在当前 Vue/Ant Design Vue 架构内重建同类设计语言:

- 白底、浅灰页面、蓝色主操作色。
- 侧边导航固定、扁平、可扫描，弱化当前插画和玻璃渐变。
- 卡片圆角收敛到 8px，边框优先于重阴影。
- 页面密度提升，指标、表格、按钮和筛选区更接近运营后台。
- 图标使用现有 Ant Design icons 先统一尺寸和线性风格，不优先新增 Lucide 依赖。
- 保留现有业务入口和路由，不改变 API、鉴权和数据流。

## 视觉语言

### 色彩

- 页面背景: `#f8fafc` 或 `#f6f8fb`。
- 主要面板: `#ffffff`。
- 主色: `#2563eb`，用于 active nav、主按钮、关键数字、焦点边框。
- 主色浅底: `#eff6ff`，用于选中态和信息卡片。
- 文本主色: `#0f172a`。
- 文本弱色: `#64748b`。
- 边框: `#e2e8f0`。
- 成功、警告、危险色保持语义明确，减少大面积渐变。

### 形状与空间

- 卡片、按钮、输入框、菜单项统一 8px 圆角。
- 页边距桌面端 24px，窄屏 16px。
- 卡片内边距默认 24px，数据表和紧凑工具条可降到 16px。
- 顶部栏高度约 64px，左侧导航宽度约 248px 到 280px。
- 页面标题区独立于卡片，不把整块页面包进大卡片。

### 组件语言

- 导航: 白底、左侧品牌、蓝色选中态、线性图标、底部折叠按钮。
- 顶部栏: 当前页面标题、必要操作按钮、用户和账户状态，不放过多彩色 KPI。
- 指标卡: 小标题、主数字、副说明、可选轻量图标，第一关键指标可用浅蓝底强调。
- 表格: 白底、细边框、浅灰表头、紧凑行高、操作按钮图标化。
- 表单和弹窗: 分组清晰、字段密度适中、底部固定操作区。
- 空状态: 使用简短文本和主要操作，不使用大面积插画。

## 页面改造边界

- `ShellLayout.vue`: 重构侧边栏、顶部栏、导航选中态和全局壳层密度。
- `style.css`: 收敛全局 token、Ant Design Vue 组件覆盖、通用卡片和表格样式。
- `OverviewView.vue`: 对齐参考站总览页结构，重做指标卡和两列内容区。
- `GatewayView.vue`: 优先拆出网关总览、路由表格、监控面板的视觉结构，降低单文件样式耦合。
- `SitesView.vue`: 优先统一筛选、表格、站点编辑弹窗和批量操作区。
- `ChatTestView.vue`: 保持工作台功能，但将底部输入区、会话列表和消息区纳入统一白底面板语言。
- `LoginView.vue`: 降低营销页感，改为参考站同类的简洁登录卡和轻量公告/联系入口。

## 技术策略

推荐方案: 保留 Vue 3 + Ant Design Vue，建立本项目自己的轻量 design token 和页面结构规范。

原因:

- 当前业务页面和复杂表格大量依赖 Ant Design Vue，直接迁移到 React/shadcn 或 Tailwind 会扩大风险。
- 参考站的可感知效果主要来自布局、token、圆角、阴影、密度和图标语言，不依赖 React 本身。
- 当前代码已经有全局 token 覆盖，适合先用 CSS 变量和 Ant theme 完成第一轮视觉统一。

暂不做:

- 不迁移到 React。
- 不引入 shadcn/ui。
- 不把所有页面重写为 Tailwind。
- 不改变后端 API 和数据模型。
- 不在当前规划阶段改业务代码。

可选后续:

- 如果 Ant icons 与目标视觉差异明显，再评估 `lucide-vue-next`。
- 如果现有 CSS 继续膨胀，再把 `style.css` 拆为 `tokens.css`、`layout.css`、`components.css`、`pages.css`。

## 验收标准

- `npm run build` 通过。
- 登录页、总览页、站点中心、路由管理、网关监控、对话页、设置页在 1440px 和 390px 宽度下无明显重叠和横向溢出。
- 所有主要卡片圆角不超过 8px，除非 Ant Design Vue 组件内部限制无法稳定覆盖。
- 侧边栏、顶部栏、按钮、表格和表单使用同一套颜色、圆角、边框和字体 token。
- 保留现有路由、功能入口、API 调用和错误提示行为。
- 不新增静默降级、mock 成功路径或隐藏错误。

## 风险与控制

- 风险: `frontend/src/style.css` 已超过 2000 行，继续追加会加重维护负担。
  控制: 第一轮只收敛 token 和关键组件，必要时同步拆分 CSS 文件。
- 风险: `GatewayView.vue` 和 `SitesView.vue` 文件过大，视觉改造容易误伤业务逻辑。
  控制: 优先抽取纯展示组件或样式 class，不改数据请求和核心动作。
- 风险: 当前 Docker 运行态可能加载旧 `frontend/dist`。
  控制: 实现阶段必须执行 `npm run build`，如需运行态验证再重建 Docker。
- 风险: 参考站是 shadcn/Tailwind，本项目是 Ant Design Vue。
  控制: 以视觉验收为准，不以技术栈一致为准。

## 任务 1 验证记录

日期: 2026-05-21

- 范围: `frontend/src/App.vue`、`frontend/src/style.css`、`ui-design-language-refactor.md`。
- 改动: 将 Ant Design Vue theme 和全局 CSS token 收敛到白底浅灰、蓝色主色、8px 组件圆角、细边框和轻阴影基线；清理旧青绿色硬编码和主要壳层大圆角。
- `npm run build`: 通过。包含 `vue-tsc -b` 和 Vite production build。仍有既有大 chunk 与 plugin timing 警告。
- `npm audit --audit-level=high`: 通过，0 个漏洞。
- `git diff --check`: 通过。

## 任务 2 验证记录

日期: 2026-05-21

- 范围: `frontend/src/components/ShellLayout.vue`、`frontend/src/style.css`、`ui-design-language-refactor.md`。
- 改动: 侧边栏改为白底细边框、蓝色选中态和底部折叠按钮；顶部栏新增当前页面上下文、紧凑运行状态组、用户标签和图标化退出按钮；移除侧栏插画依赖和玻璃渐变装饰。
- Debug-First: 将网关概览刷新失败从静默吞错改为 `console.warn`，避免隐藏真实失败。
- `npm run build`: 通过。包含 `vue-tsc -b` 和 Vite production build。仍有既有大 chunk 与 plugin timing 警告。
- `npm audit --audit-level=high`: 通过，0 个漏洞。
- `git diff --check`: 通过。
- 浏览器验证: Vite dev server `http://127.0.0.1:5174/overview` 登录后验证。1440px 桌面、桌面折叠侧栏、390px 移动模拟均无文档级横向溢出，顶部操作区未出视口；DevTools console 无 error/warn。

## 任务 3 验证记录

日期: 2026-05-21

- 范围: `frontend/src/views/OverviewView.vue`、`frontend/src/styles/overview.css`、`frontend/src/styles/overview-feed.css`、`ui-design-language-refactor.md`。
- 改动: 总览页改为参考站式运营后台结构，新增页面头、4 张紧凑指标卡、最近任务主面板、运行计划面板和待处理站点面板；将概览页样式拆到专用 CSS 文件，避免继续扩大单文件组件。
- `npm run build`: 通过。包含 `vue-tsc -b` 和 Vite production build。仍有既有大 chunk 与 plugin timing 警告。
- `npm audit --audit-level=high`: 通过，0 个漏洞。
- `git diff --check`: 通过。
- 浏览器验证: Vite dev server `http://127.0.0.1:5174/overview` 登录后验证。真实空数据在 1440px、1024px、390px 下均无文档级横向溢出，4 个指标卡和 3 个面板均可见。
- 浏览器验证: 临时只拦截 `/api/overview` 的 visual fixture 有数据场景。1440px 下 5 条 feed 行可见且无横向溢出；390px 移动模拟下 5 条 feed 行可见，最大行右边界 359px，小于 390px 视口，文档无横向溢出。
- DevTools console: error/warn 为空。

## 任务 4 验证记录

日期: 2026-05-21

- 范围: `frontend/src/views/GatewayView.vue`、`frontend/src/views/SitesView.vue`、`frontend/src/styles/management-surfaces.css`、`ui-design-language-refactor.md`。
- 改动: 新增管理页共享样式，统一路由管理、网关监控和站点中心的表格、筛选区、弹窗、抽屉、触控按钮尺寸；移除站点编辑弹窗的装饰插画和装饰符号；将相关分隔符改为 ASCII 文本。
- `npm run build`: 通过。包含 `vue-tsc -b` 和 Vite production build。仍有既有大 chunk 与 plugin timing 警告。
- `npm audit --audit-level=high`: 通过，0 个漏洞。
- `git diff --check`: 通过。
- 纯文本约束扫描: 装饰符号扫描和 tab 扫描均已覆盖任务 4 代码文件，结果无命中。
- 浏览器验证: Vite dev server `http://127.0.0.1:5174/` 代理当前 Docker 后端 `8972`，`/api/health` 返回 200。
- 浏览器验证: Playwright 登录后检查 `/gateway/routes`、`/gateway/monitor`、`/sites`，覆盖 1440px、1024px、390px；同时打开网关策略弹窗、最近请求抽屉、站点编辑弹窗。18 个页面状态均无文档级横向溢出、无关键表面越界、无按钮文本溢出、无触控尺寸 warning。
- DevTools console: error/warn 为空。

## 任务 5 验证记录

日期: 2026-05-21

- 范围: `frontend/src/views/ChatTestView.vue`、`frontend/src/views/SettingsView.vue`、`frontend/src/views/DesktopServiceView.vue`、`frontend/src/views/LoginView.vue`、`frontend/src/styles/workspace-surfaces.css`、`ui-design-language-refactor.md`。
- 改动: 新增工作台共享样式，统一对话页、设置页、桌面服务页和登录页的白底面板、细边框、8px 圆角、输入控件和按钮语言；降低登录页营销插画感；移除对话页空态大图和装饰性分隔符；保留原有 API 调用、鉴权、会话、设置表单和登录流程。
- `npm run build`: 通过。包含 `vue-tsc -b` 和 Vite production build。仍有既有大 chunk 警告。
- `npm audit --audit-level=high`: 通过，0 个漏洞。
- `node --test frontend/tests/*.test.ts`: 通过，14 个前端状态辅助测试全部通过。
- `go test ./...`: 通过，后端现有测试全部通过。
- `git diff --check`: 通过。
- 纯文本约束扫描: 装饰符号扫描和 tab 扫描均已覆盖任务 5 代码文件，结果无命中。
- 浏览器验证: Vite dev server `http://127.0.0.1:5174/` 代理当前 Docker 后端 `8972`，`/api/health` 返回 200。
- 浏览器验证: Playwright 登录后检查 `/chat-test`、`/settings`、`/desktop`，并以未登录上下文检查 `/login`，覆盖 1440px、1024px、390px；桌面视口额外执行登录提交并跳转 `/overview`。13 个页面状态均无文档级横向溢出、无关键表面越界、无按钮或输入文本溢出、无触控尺寸 warning。
- 补充复验: 修正对话页输入区主列宽度后，使用 `agent-browser` 复查 `/login`、`/chat-test`、`/settings`、`/desktop` 的 1440px、1024px、390px 视口和登录提交跳转，12 个页面状态和登录提交均通过。
- DevTools console: error/warn 为空。

## 任务 6 验证记录

日期: 2026-05-21

- 范围: `frontend/src/style.css`、`frontend/src/styles/management-surfaces.css`、`frontend/src/views/GatewayView.vue`、`frontend/src/views/ChatTestView.vue`、`ui-design-language-refactor.md`。
- 改动: 网关监控指标卡由横向滚动改为自适应换行网格；管理页搜索输入实际点击高度稳定到 32px；时间段消耗输入框补充可访问名称；对话页添加参考图按钮补充 `title` 和 `aria-label`。
- 首轮浏览器审计: `agent-browser` 检查 `/login`、`/overview`、`/gateway/routes`、`/gateway/monitor`、`/sites`、`/chat-test`、`/settings`、`/desktop`，覆盖 1440px、1024px、390px，发现网关监控指标卡中窄屏横向滚动、两个时间输入框缺少可访问名称、对话页添加参考图按钮缺少可访问名称。
- 修复后浏览器复验: 同一路由和视口共 24 个页面状态，文档级横向溢出、关键表面越界、按钮或输入文本溢出、不可点击控件、缺少可访问名称控件、图片 alt 缺失、重复 id、console error/warn 均为 0。
- `npm run build`: 通过。包含 `vue-tsc -b` 和 Vite production build。仍有既有大 chunk 和 plugin timing 警告。
- `npm audit --audit-level=high`: 通过，0 个漏洞。
- `git diff --check`: 通过。
- 纯文本约束扫描: 装饰符号扫描和 tab 扫描均已覆盖任务 6 代码文件，结果无命中。

## 任务 7 验证记录

日期: 2026-05-21

- 范围: `compose.yaml` 运行态、Dockerfile 镜像构建、`http://127.0.0.1:8972` 管理端主要页面、`ui-design-language-refactor.md`。
- `npm run build`: 已在任务 6 后再次执行通过；Docker 镜像内 `RUN npm run build` 也通过。两者仍有既有大 chunk warning。
- `docker compose up -d --build`: 通过。镜像 `ai-sign-in-gateway-app:latest` 构建完成，容器 `ai-sign-in-gateway` 重新创建并启动。
- `docker compose ps`: `ai-sign-in-gateway` 状态为 `Up`，端口映射为 `0.0.0.0:8972->8972/tcp`。
- `docker compose logs --tail=80 app`: 服务模式启动成功，监听 `http://127.0.0.1:8972`，网关地址为 `http://127.0.0.1:8972/api/gateway`，数据库路径为 `/app/data/ai-sign-in-gateway.db`。
- `curl http://127.0.0.1:8972/api/health`: HTTP 200，响应 `status` 为 `ok`。
- `curl http://127.0.0.1:8972/`: HTTP 200，返回 2048 字节 HTML。
- `POST /api/auth/login`: 当前管理员密码 `1111` 返回 HTTP 200；旧默认密码 `admin123` 返回 HTTP 401。
- 运行态浏览器验证: `agent-browser` 直接访问 `http://127.0.0.1:8972`，检查 `/login`、`/overview`、`/gateway/routes`、`/gateway/monitor`、`/sites`、`/chat-test`、`/settings`、`/desktop`，覆盖 1440px、1024px、390px，共 24 个页面状态。文档级横向溢出、关键表面越界、按钮或输入文本溢出、不可点击控件、缺少可访问名称控件、图片 alt 缺失、重复 id、console error/warn 均为 0。

## 外部复核后补充清理

日期: 2026-05-22

- 范围: 清理已无引用的旧 UI 插画资源。
- 删除文件: `frontend/src/assets/design/session-pic.png`、`frontend/src/assets/design/sidebar-gateway.png`、`frontend/src/assets/design/sidebar-skyline.png`、`frontend/src/assets/site-editor-account.png`、`frontend/src/assets/site-editor-cloud.png`、`frontend/src/assets/site-editor-gateway.png`、`frontend/src/assets/hero.png`、`frontend/src/assets/vite.svg`、`frontend/src/assets/vue.svg`。
- 引用检查: `rg -n 'site-editor-(account|cloud|gateway)\.png|session-pic\.png|sidebar-(gateway|skyline)\.png|hero\.png|vite\.svg|vue\.svg' frontend/src` 无命中。
- `npm run build`: 通过。包含 `vue-tsc -b` 和 Vite production build。仍有既有大 chunk 和 plugin timing 警告。
- `node --test frontend/tests/*.test.ts`: 通过，14 个前端状态辅助测试全部通过。
- `go test ./...`: 通过，后端现有测试全部通过。
- `docker compose up -d --build`: 通过。镜像 `ai-sign-in-gateway-app:latest` 重新构建，容器 `ai-sign-in-gateway` 重新创建并启动。
- `docker compose ps`: `ai-sign-in-gateway` 状态为 `Up`，端口映射为 `0.0.0.0:8972->8972/tcp`。
- `curl http://127.0.0.1:8972/api/health`: HTTP 200，响应 `status` 为 `ok`。
- `POST /api/auth/login`: 当前管理员密码 `1111` 返回 HTTP 200，响应包含 `access_token` 和 `token_type`。

## 移动端复核后补充优化

日期: 2026-05-22

- 范围: `frontend/src/views/GatewayView.vue`、`frontend/src/views/ChatTestView.vue`、`frontend/src/views/SettingsView.vue`、`frontend/src/style.css`。
- 改动: 路由管理移动端工具栏改为两列操作网格，筛选控件改为全宽；对话页移动端主工作区优先、会话历史下置，并统一历史区图标按钮为 36px；设置页移动端 tabs 改为横向滚动并隐藏 Ant overflow 操作按钮，数字输入步进控件补足触控命中区；移动端折叠侧栏收窄为 72px。
- `git diff --check`: 通过。
- `node --test frontend/tests/*.test.ts`: 通过，14 个前端状态辅助测试全部通过。
- `npm run build`: 通过。包含 `vue-tsc -b` 和 Vite production build。仍有既有大 chunk 和 plugin timing 警告。
- `go test ./...`: 通过，后端现有测试全部通过。
- `npm audit --audit-level=high`: 通过，0 个漏洞。
- `docker compose up -d --build`: 通过。镜像内重新执行前端 production build，容器 `ai-sign-in-gateway` 重新创建并启动。
- `curl http://127.0.0.1:8972/api/health`: HTTP 200，响应 `status` 为 `ok`。
- 运行态浏览器验证: 直接访问 `http://127.0.0.1:8972`，复查 `/gateway/routes`、`/chat-test`、`/settings`。390px 下文档级横向溢出为 0，对话页输入区在会话历史之前，设置页 tabs 为横向滚动；1440px 下对话页和路由管理无关键表面越界、无按钮文本溢出、无小于 32px 的可见可点击控件。
- DevTools console: error/warn 为空。

## 代码结构复核后补充拆分

日期: 2026-05-22

- 范围: `frontend/src/style.css`、`frontend/src/views/GatewayView.vue`、`frontend/src/views/SitesView.vue` 及其拆分出的配置、工具与 CSS 模块。
- 改动: 将全局样式、网关页样式、站点页样式拆为按职责聚合的 CSS 模块；将网关页与站点页的常量配置移出大型 Vue 文件；抽取共享视图工具函数；改用 Ant Design Vue 子模块导入以减少无关入口耦合。
- 文件长度检查: `wc -l frontend/src/styles/*.css frontend/src/*Config.ts frontend/src/viewUtils.ts | sort -nr | head -12`，最长文件为 `frontend/src/styles/workspace-surfaces.css` 299 行，新增拆分文件均不超过 300 行。
- `git diff --check`: 通过。
- `node --test frontend/tests/*.test.ts`: 通过，14 个前端状态辅助测试全部通过。
- `npm run build`: 通过。包含 `vue-tsc -b` 和 Vite production build。仍有既有大 chunk 警告。
- `npm audit --audit-level=high`: 通过，0 个漏洞。
- `go test ./...`: 通过，后端现有测试全部通过。
- 运行态浏览器验证: `npm run preview -- --host 127.0.0.1 --port 4173` 后访问 `http://127.0.0.1:4173/login`，页面正常加载，DevTools console error/warn 为空。
