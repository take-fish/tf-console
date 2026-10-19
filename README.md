# tf-console（FishMatrix-流水线中台）

FishMatrix-流水线中台 —— Go(Gin) + Vue 3，前端构建产物内嵌进单个二进制。

- 一级页：11 仓实时泳道（提交 → test → PR 审核 → 构建 → 发布）
- 二级页：`/repos/:repo`，按仓库查看分支/提交、合并申请（只读）、构建与发版进度
- 端口：`8301`（可用 `PORT` 环境变量覆盖）
- 健康检查：`GET /healthz`

> 当前为 **A 方案：只读中台**。建分支、合 test、发起/审核/合并 master PR 等写操作，等登录与身份隔离落地后开放；详见项目进度记录。

## API

| 方法/路径 | 说明 |
|---|---|
| `GET /api/v1/pipeline` | 11 仓五阶段快照 + 事件流 + 配额 |
| `GET /api/v1/repos/{repo}/branches` | 仓库分支列表（name / short_sha / protected） |
| `GET /api/v1/repos/{repo}/commits?branch=master&limit=30` | 指定分支最近提交 |
| `GET /healthz` | 健康检查 |

分支/提交接口只允许 `TF_CONSOLE_REPOS` 配置的仓库，按需请求并复用 ETag 缓存。

## 页面

| 路由 | 内容 |
|---|---|
| `/` | 11 仓泳道总览（保留原首页） |
| `/repos/:repo` | 分支与提交 / 合并申请（只读）/ 构建与发版 |

- 合并申请使用快照中的开放 PR 与审核状态；当前只读，审核/合并跳转 GitHub。
- 构建与发版展示 test CI、master CI step 进度、最新 Release；集中机容器部署状态暂不包含。
- 自建分支仅展示提交记录；代码仍由本地 Git 推送。

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `PORT` | `8301` | 监听端口 |
| `GITHUB_TOKEN` | 空 | GitHub token（**只读权限即可**）。不配置则降级：匿名配额仅 60 次/小时，轮询自动降频到 30 分钟一轮并在页面提示 |
| `GITHUB_OWNER` | `take-fish` | 组织名 |
| `GITHUB_API_BASE` | `https://api.github.com` | 测试时可覆盖 |
| `TF_CONSOLE_REPOS` | 11 个仓 | 逗号分隔覆盖展示列表 |
| `TF_CONSOLE_TEST_CI` | `1` | 设为 `0` 关闭 test 分支 CI 状态抓取 |
| `POLL_INTERVAL` | `30s` | 基础轮询间隔（实际按配额自适应，见下） |

## 工作原理（配额友好）

- 每轮全量约 70 个请求，全部走 **ETag 条件请求**：未变化时返回 304、不计配额，因此 30s 一轮也远低于 5000/h 限额
- 自适应间隔：配额 >2000 → 30s；500~2000 → 60s；≤500 → 5 分钟；耗尽时退避到重置时刻
- ETag 与 Authorization 头绑定：**更换 token 后第一轮全量刷新**属正常现象
- org 事件流接口在 GitHub 侧有 30 秒 ~ 数小时延迟，页面已标注「事件有延迟」
- 分支/提交为二级页按需请求，同样走 ETag 缓存

## 本地开发

```bash
# 后端（先构建前端生成 dist/；前端路由需要 SPA fallback）
npm ci && npm run build-only
PORT=8301 GITHUB_TOKEN=<token> go run .

# 前端（热更新，/api 自动代理到 8301；/repos/* 由 Vite history fallback 处理）
npm run dev
```

完整构建（前端产物嵌入二进制）：

```bash
npm ci && npm run build-only
go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o bin/tf-console .
```

## Docker 构建

```bash
docker build -t tf-console .
```

## 集中机部署

由 tf-deploy 的 docker-compose 编排（服务 `console`，容器 `fm-console`），在 `.env` 配置只读 `GITHUB_TOKEN`。

## 分支模型

- `master`：保护分支，只接受 PR（CI 必须通过，squash 合并），合并后自动发版
- `test`：联调分支，开发分支直接合入（不走 PR），可直接推送
- 开发流：`feature/*` 开发 → 直接合入 `test` 联调 → 验证 OK 后从开发分支开 PR 合 `master` → 自动发版
