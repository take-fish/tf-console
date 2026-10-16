# tf-console

FishMatrix（摸鱼矩阵）流水线可视化控制台 —— Go(Gin) + Vue 3，前端构建产物内嵌进单个二进制。

- 角色：把「代码提交 → 分支管理 → 审核 → 自动构建 → 发布」抽成一页实时泳道图，轮询 11 个仓库的 GitHub 状态
- 端口：`8301`（可用 `PORT` 环境变量覆盖）
- 健康检查：`GET /healthz`；数据接口：`GET /api/v1/pipeline`

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

## 本地开发

```bash
# 后端（dist/.gitkeep 占位即可编译；真实前端产物见下）
PORT=8301 GITHUB_TOKEN=<token> go run .

# 前端（热更新，/api 自动代理到 8301）
npm install
npm run dev
```

完整构建（前端产物嵌入二进制）：

```bash
npm ci && npm run build-only   # 先生成 dist/
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
