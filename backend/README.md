# 漂流瓶后端

可运行的 Gin + MySQL 后端，包含 HTTP API、持久任务 worker、知乎 HTTP 适配器和真实数据库集成测试。依赖锁定在 `go.mod/go.sum`：Go 1.25.0、Gin 1.12.0、MySQL driver 1.10.0。使用已有数据库结构，不修改已执行的 migration。

## 本地运行

需要 Go 1.25.0、Python 3、Docker Compose。以下命令在 `backend/` 执行：

```sh
go mod download
python3 scripts/setup_db.py
docker compose up -d --wait mysql
docker compose run --rm migrate
```

为 API 配置 `AUTH_SIGNING_KEY`（至少 32 字节随机值），通过终端环境或部署 Secret 注入。`.env.example` 记录配置名；`scripts/run.py` 只补充 `.env` 中未被环境覆盖的配置，不执行其中的 Shell 内容。

```sh
# 两个终端分别运行
python3 scripts/run.py api
python3 scripts/run.py worker
```

API 默认监听 `127.0.0.1:8080`，健康检查为 `GET /healthz`，业务前缀为 `/api/v1`。本地开发可用 `python3 scripts/run.py dev-token alice` 为一个测试主体生成一天有效的 Token；不要将 Token 输出提交到仓库。生产没有按任意 userId 登录的接口：由可信宿主签发 HS256 JWT，包含 `sub`（稳定主体）、`iss=AUTH_ISSUER`、`aud=AUTH_AUDIENCE` 和 `exp`。业务请求使用 `Authorization: Bearer <token>`。

需要一组可重复使用的本地测试数据时，运行 `python3 scripts/run.py seed-demo`。命令会幂等创建 4 个本站用户、3 个正在寻找的瓶子、可接收经历、待处理邀请和通知，并输出每个用户的 24 小时开发 Token。

## 已实现流程

- 独立草稿、版本确认、每人默认 3 个同时寻找瓶子、暂停/恢复/重试、寻找状态和柜子分页。`ACTIVE_BOTTLE_LIMIT` 可调高至 20，不能低于 3。
- 本人确认经历的增删改查、接收开关和披露快照。
- 直接匹配、有限多人投递、邀请接住/放行/72 小时过期、每个接收者独立连接。
- 回信持久化、内容版本审核、幂等发送、审核拒绝后修改/重试/复核。消息先经本地分层规则：明确联系方式、站外引流和违法话术直接拒绝，普通内容直接通过，只有带语境歧义的风险词句调用 AI。
- 发信者收到首封回信后发出匿名聊天邀请，对方接受才允许继续发送；结束、举报、屏蔽、反馈。
- 通知与单条已读、作者主动同意后生成脱敏切片、编辑及最终发布。
- AI 问题整理、目标建议、候选语义判断和排序、活动主题提取；外部请求在数据库事务外执行。

最新接口差异与新增路径见 [API.md](../docs/API.md) 第 18 节。首封回信和聊天写入返回 `202`，消息未审核通过时只对作者可见。查询消息取得 `deliveryStatus`；通过后首封回信才使连接进入 `replied`，前端此时播放回海动画。原“一次追问”流程已由双方同意的匿名聊天替代。

## AI 接入

默认使用知乎直答：只需设置 `ZHIHU_ACCESS_SECRET`，后端自动采用 `https://developer.zhihu.com/v1`、`zhida-fast-1p5`，并发送 Bearer 鉴权和 `X-Request-Timestamp`。也可同时显式设置 `AI_BASE_URL`、`AI_MODEL`、`AI_API_KEY` 切换到其他兼容服务。适配器调用 `POST <AI_BASE_URL>/chat/completions`，使用 `model/messages/stream=false` 并解析 `choices[0].message.content` 的 JSON。任务提示和结构集中在 `internal/ai/ai.go`。AI 只用于整理、审核和匹配，不生成代替真人的回信。

未配置模型时，草稿保存和已有记录查询可用；AI 整理返回 `503 AI_UNAVAILABLE`。瓶子不进行内容审核，原文仅用于推荐匹配；匿名交流消息保留审核。任务失败按重试机制保留为 `search_error` 或 `moderation_failed`。测试模型只存在于测试文件，生产没有自动放行的 mock 模式。

AI 适配器兼容 JSON 外的 Markdown、说明文字、前置思考块和文本内容块；格式异常时在请求期限内重试一次。截断响应、多个冲突对象和字段类型错误仍返回 `AI_PROTOCOL_ERROR`。日志仅记录任务和失败原因（如 `invalid_envelope`、`invalid_json_object`、`invalid_field_type`），不记录用户原文、模型全文或密钥。前端支持直接发瓶，整理为可选步骤；瓶子匹配与匿名交流消息审核仍由 worker 执行。

## 知乎接入

严格依据 `../zhihu/references/{hackathon-oauth,user-api,http-api}.md`，直接使用 HTTP，不需要安装或部署知乎 CLI：

- OAuth 表单为 `app_id/app_key/grant_type=authorization_code/redirect_uri/code`；支持响应业务码 `20000`，按 `access_token` 和 `expires_in` 判断有效结果。
- 用户创作与关注列表同时带 Bearer Access Secret、用户 `X-OAuth-Token`、秒级时间戳。缺少用户 Token 不发送请求，不退回开发者账号。
- 搜索使用 `Query`、`Count=10`；搜索和额度查询不带用户 Token。
- 搜索缓存 24 小时；实际调用前原子占用同一 Access Secret 的共享日预算，缓存命中不计费。用户用量另行记录；上海自然日重置。适配器每次只发一个 HTTP 请求，后台失败重试仍经预算层。
- 每次活动刷新只读取创作与关注各一页，立即提取非敏感主题；原始关注与创作不持久化。主题 6 小时有效，过期由 worker 刷新，失败保留旧主题。匹配不现场为候选人抓取知乎数据。
- 没有“关注内容流”路径，状态明确返回 `followFeedAvailable=false`。搜索没有可靠本人作者过滤，所以经历建议创建接口按契约返回 `503 ZHIHU_AUTHOR_FILTER_UNAVAILABLE`，用户可直接手动填写经历。

使用环境注入 `ZHIHU_ACCESS_SECRET`、`ZHIHU_OAUTH_APP_KEY`、`ZHIHU_TOKEN_ENCRYPTION_KEY`，不要放进 `.env` 或源码。加密密钥为 32 随机字节的 base64。另配置公开的 `ZHIHU_OAUTH_APP_ID` 和登记的 `ZHIHU_OAUTH_REDIRECT_URI`。

`GET /auth/zhihu` 是插件的主登录入口：授权回调换取 Token，通过知乎 `/user` 读取稳定主体、昵称和头像，映射到内部 `users.id`，并签发 30 天 HttpOnly 会话 Cookie。`GET /auth/session` 返回当前用户，`POST /auth/logout` 退出。`POST /integrations/zhihu/authorize` 仅作为已有会话重新授权的兼容入口。昵称和头像只展示，唯一映射使用稳定主体。

回调支持 `authorization_code` 和 `code`，要求有效的随机 state 与同浏览器 Cookie；回调不返回 state 时返回 `OAUTH_STATE_INVALID`，需要平台确认其支持。临时 OAuth 事务只在 API 进程内保存，重启后需重新发起；当前部署为单 API 实例。

授权 Token 用 AES-GCM 加密保存并绑定本地用户，worker 可解密刷新活动；过期、鉴权失败或用户 `DELETE /integrations/zhihu` 后清除 Token。没有虚构刷新 Token、撤销端点或 scope。

## 代码结构与校验

- `cmd/api`、`cmd/worker`：入口、日志、退出处理；`internal/app` 装配配置和依赖。
- `internal/httpapi`：Gin 路由、JSON 入参、一次性文本/枚举校验、响应映射和认证。请求正文最多 1 MiB。
- `internal/service`：资源权限、状态转换和事务边界；不重复执行 HTTP 格式校验。
- `internal/repository/mysql`：连接池、SQL、行映射、通知和 outbox 落库。SQL 集中在 `queries.go`，service 在用例事务中调用 `database/sql`，避免给每条 SQL 再套一层转发方法。
- `internal/jobs`：`SKIP LOCKED` 认领、5 分钟租约、带抖动退避、最多 5 次尝试、失败状态和恢复扫描。
- `internal/ai`、`internal/zhihu`：有超时、限长响应和明确结果类型的外部适配器。

匹配按 200 条经历分页计算，找到足够合格的不同用户或扫描结束才停止；写入邀请前重新检查经历仍开放、内容版本和瓶子状态。AI 不可用视为技术失败，不解释为“没有人愿意回应”。批次内按语义分数排序，首版不做全站用户排名。

格式化使用 `gofmt -w cmd internal tests`，静态检查使用 `go vet ./...`。日志只记录请求 ID、方法、路由模板、状态和耗时；任务错误只记稳定代码，不记录正文、Token、SQL 参数或回调查询串。

## 构建与测试

```sh
go build ./...
go test ./...
go vet ./...
python3 scripts/test_backend.py
python3 -m unittest discover -s tests -p 'test_database.py' -v
```

普通 `go test` 跑认证与上游协议测试；未设置 `TEST_MYSQL_DSN` 时跳过业务数据库集成测试。`scripts/test_backend.py` 在当前 MySQL 容器建立独立 `drift_backend_test` 数据库，以初始 migration 初始化，执行 `go test -race -count=1 ./...`，随后针对同一个独立测试库运行已有 Python 约束测试，不修改 `drift_bottle`。测试保留记录以便诊断，并在结束时关闭测试经历的接收开关。已有 Python 数据库约束测试继续针对项目数据库、使用事务回滚。

单项测试：`TEST_MYSQL_DSN='...' go test ./tests -run TestBackendFlow -v`。不要将带真实密码的命令保存到提交中。

## 容器部署

先执行 migration，再提供环境配置：

```sh
docker compose -f compose.yaml -f compose.app.yaml --profile app up -d --build
```

`Dockerfile` 构建 api 和 worker，使用非 root 运行时镜像；仅 API 端口映射到宿主机回环地址，公网入口由 HTTPS 反向代理提供。生产 Cookie 使用 Secure；本地 HTTP 联调设 `APP_ENV=development`。API 启动不会自动迁移。

仓库已补齐 `000002_multiple_active_bottles` 成对迁移，与现有 version 2 数据库一致。API/worker 启动时仅检查一次多瓶寻找表结构；不兼容时返回 `DATABASE_SCHEMA_INCOMPATIBLE`。同时数量上限由 service 在用户行锁下原子检查，不依赖单列唯一键。

真实知乎授权与真实模型的端到端验收需要对应平台凭证和登记回调；自动测试使用 HTTP 模拟上游验证协议和真实 MySQL 验证业务，不声称已完成平台账号联调。
