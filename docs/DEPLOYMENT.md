# Vercel + Railway 演示部署

目标结构：Vercel 托管 Vite 前端；Railway 同一项目内运行 MySQL、Gin API 和后台 worker。MySQL 只走 Railway 私有网络，不开启 Public Networking。数据库使用 `backend/migrations/` 中编号 migration，不手工复制或重建 schema。

## Railway

1. 在 Railway 创建 Project，从 GitHub 导入本仓库。
2. `+ New` → `Database` → `MySQL` 创建数据库服务。保留默认私有网络，等待其健康状态正常。
3. `+ New` → `GitHub Repo` 选择相同仓库，创建 `api` 服务。Settings → Build → Root Directory 设为 `/backend`，Builder 选 Dockerfile（文件为 `backend/Dockerfile`）。
4. API 的 Settings → Deploy → Pre-deploy Command 填 `/migrate-railway.sh`。脚本使用 Railway 注入的 `MYSQL_URL` 自动应用所有未运行的 migration；`up` 可重复运行。Healthcheck Path 填 `/healthz`。
5. API Variables 添加下表变量。数据库项用 Railway 引用变量，点击变量值输入框的引用选择器选择 MySQL 服务，不要粘贴本地 `.env`。`CORS_ORIGIN` 暂时留空，等 Vercel 域名生成后再补。
6. API Settings → Networking → Generate Domain，记录 `https://…up.railway.app`。将这个域名填入 `ZHIHU_OAUTH_REDIRECT_URI`（加上表格中的回调路径），然后部署 API。访问 `<API 域名>/healthz` 应返回 `{"data":{"status":"ok"}}`。
7. 再添加相同仓库为 `worker` 服务，Root Directory 同为 `/backend`。Variables 设置 API 所需的数据库、AI 和知乎变量（不需要 `AUTH_SIGNING_KEY`、`CORS_ORIGIN`）；Settings → Deploy → Custom Start Command 填 `/worker`。worker 不需要公网域名或健康检查。

| 变量 | API | Worker | 值 |
| --- | --- | --- | --- |
| `APP_ENV` | 是 | 是 | `production` |
| `MYSQLHOST` | 是 | 是 | `${{MySQL.MYSQLHOST}}` |
| `MYSQLPORT` | 是 | 是 | `${{MySQL.MYSQLPORT}}` |
| `MYSQLUSER` | 是 | 是 | `${{MySQL.MYSQLUSER}}` |
| `MYSQLPASSWORD` | 是 | 是 | `${{MySQL.MYSQLPASSWORD}}` |
| `MYSQLDATABASE` | 是 | 是 | `${{MySQL.MYSQLDATABASE}}` |
| `MYSQL_URL` | 是 | 是 | `${{MySQL.MYSQL_URL}}`；API 预部署迁移使用 |
| `AUTH_SIGNING_KEY` | 是 | — | 至少 32 字节随机密钥 |
| `CORS_ORIGIN` | Vercel 部署后设置 | — | Vercel 正式域名 Origin，例如 `https://your-project.vercel.app`，不能带路径或尾斜杠 |
| `CORS_ORIGINS` | 可选 | — | 额外允许的精确 Origin，逗号分隔；不要写通配符 |
| `ZHIHU_OAUTH_APP_ID` | 是 | 是 | 知乎开放平台应用 ID |
| `ZHIHU_OAUTH_APP_KEY` | 是 | 是 | 知乎 OAuth 应用密钥 |
| `ZHIHU_ACCESS_SECRET` | 是 | 是 | 知乎开放平台 Access Secret |
| `ZHIHU_TOKEN_ENCRYPTION_KEY` | 是 | 是 | 32 个随机字节的 Base64 编码；两服务必须相同 |
| `ZHIHU_OAUTH_REDIRECT_URI` | 是 | 是 | `https://<API域名>/api/v1/integrations/zhihu/callback` |
| `AI_BASE_URL` | 是 | 是 | 兼容 OpenAI Chat Completions 的 API 根地址，带 `/v1` |
| `AI_API_KEY` | 是 | 是 | 模型服务密钥 |
| `AI_MODEL` | 是 | 是 | 模型名称 |

Railway 的 `PORT` 会自动注入，API 监听 `0.0.0.0:$PORT`，本地缺省仍是 `127.0.0.1:8080`。`MYSQL_DSN` 和 `HTTP_ADDR` 可留空。若 Dashboard 的 MySQL 服务名不是 `MySQL`，在引用选择器中选实际服务；表中名字只是示例。

先添加 MySQL，接着部署 API（它在启动前执行 migration），再部署 worker。迁移完成后不要删除数据库卷。演示上线后在 Railway 配置备份。

## Vercel

1. 在 Vercel Add New → Project 导入相同 GitHub 仓库。
2. Root Directory 选择 `frontend`，Framework Preset 选 Vite。仓库的 `frontend/vercel.json` 已设冻结锁文件安装、构建和 SPA 路由回退，输出目录为 `dist`。
3. 生产请求通过 `vercel.json` 将同域 `/api/*` 转发至 Railway，避免 OAuth 与登录会话被浏览器按第三方 Cookie 拦截。`VITE_API_BASE_URL` 只供本地开发直连 Go 后端使用。
4. Deploy。Vercel 给出 `https://…vercel.app` 后，回到 Railway API Variables，把完整 Origin 设置为 `CORS_ORIGIN`，并把 `ZHIHU_OAUTH_REDIRECT_URI` 设置为 `https://…vercel.app/api/v1/integrations/zhihu/callback`。知乎项目页面登记完全相同的回调地址，然后重新部署 API。
5. 在知乎开放平台将回调地址精确登记为 `ZHIHU_OAUTH_REDIRECT_URI` 的值。Cookie 跨站配置要求正式前端和 API 都使用 HTTPS。

## 前端联调与验证

登录后的页面已调用后端：AI 整理问题和建议目标、用户确认后抛瓶、账号瓶子柜、经历保存和编辑、接收邀请、提交审核回信和双方同意后的匿名聊天。前端不持有模型密钥。未登录的本地开发保留原交互演示；开发者指南保存在当前浏览器。

部署时 API 和 worker 都必须配置 `AI_BASE_URL`、`AI_MODEL`、`AI_API_KEY`。知乎搜索的 `ZHIHU_ACCESS_SECRET` 用于获取公开讨论，不能代替当前模型配置。仅部署前端或 API 不会运行后台审核、匹配任务，必须同时启动 worker。

验证步骤：知乎登录后检查「✓ 知乎已登录」；打开空瓶填写问题，点击「让 AI 帮我整理」；确认页修改目标经历再抛瓶；进入瓶子柜刷新实际审核和寻找状态；在日记中保存本人经历并开放接收。需要另一个真实账号填写符合条件的经历，才能验证接收、回信和聊天。没有匹配人时不会生成 AI 回信。

若显示「内容处理服务尚未配置」，检查 API 的模型地址和模型名；若显示「内容处理暂时不可用」，检查密钥、模型权限和上游网络。自动化浏览器测试模拟业务接口，不证明部署环境中的模型凭证已可用。

## 本地预检

```sh
cd frontend && corepack pnpm install --frozen-lockfile && corepack pnpm build && corepack pnpm test
cd ../backend && go build ./... && go test ./... && go vet ./...
```
