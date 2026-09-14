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

## 当前 Demo 的边界

现有 Vite 交互页面的瓶子、经历、收件箱和聊天演示数据仍保存在浏览器 `localStorage`；`VITE_API_BASE_URL` 当前用于知乎登录入口，页面业务操作尚未改为调用后端 API。因此此部署能让评委公网打开并体验现有前端 Demo，也会启动带 MySQL migration 的后端和 worker，但前端 Demo 数据不会跨设备保存到 Railway MySQL。登录、AI 和真实知乎数据需要填好上面相应凭证；缺少密钥时相应功能不能完成线上联调。

## 本地预检

```sh
cd frontend && corepack pnpm install --frozen-lockfile && corepack pnpm build && corepack pnpm test
cd ../backend && go build ./... && go test ./... && go vet ./...
```
