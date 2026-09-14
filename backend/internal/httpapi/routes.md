# 路由职责清单

路由已注册和实现；最新版请求/响应见 docs/API.md 第 8、14、18 节。HTTP 层仅解析、调用用例、映射响应。

- `GET /api/v1/home`
- `POST /api/v1/bottles`
- `PATCH /api/v1/bottles/{bottleId}`
- `POST /api/v1/bottles/{bottleId}/launch`
- `POST /api/v1/bottles/{bottleId}/pause`
- `POST /api/v1/bottles/{bottleId}/resume`
- `POST /api/v1/bottles/{bottleId}/retry`
- `GET /api/v1/bottles/{bottleId}`
- `POST /api/v1/ai/episode-drafts`
- `POST /api/v1/ai/target-drafts`
- `GET /api/v1/invitations/next`
- `POST /api/v1/invitations/{invitationId}/decision`
- `GET /api/v1/connections/{connectionId}`
- `POST /api/v1/connections/{connectionId}/messages`
- `POST /api/v1/connections/{connectionId}/close`
- `POST /api/v1/connections/{connectionId}/feedback`
- `GET /api/v1/cabinet`
- `GET /api/v1/cabinet/{recordId}`
- `GET /api/v1/experiences`
- `POST /api/v1/experiences`
- `PATCH /api/v1/experiences/{experienceId}`
- `DELETE /api/v1/experiences/{experienceId}`
- `POST /api/v1/ai/experience-suggestions`
- `GET /api/v1/ai/experience-suggestions/{jobId}`
- `GET /api/v1/bottles/{bottleId}/search-status`
- `GET /api/v1/notifications`
- `POST /api/v1/notifications/{notificationId}/read`
- `POST /api/v1/connections/{connectionId}/slice-drafts`
- `GET /api/v1/slice-drafts/{sliceId}`
- `PATCH /api/v1/slice-drafts/{sliceId}`
- `POST /api/v1/slice-drafts/{sliceId}/publish`
- `GET /api/v1/integrations/zhihu/status`

归属：bottles → 瓶子用例；invitations → 接收与决策；connections → 回信与反馈；cabinet/home/notifications → 查询；experiences → 长期经历；ai → AI 整理或异步建议；slice-drafts → 切片；integrations → 授权及画像状态。

补充实现：连接消息分页、聊天邀请/决策/状态/发送、消息修改/重试/复核、瓶子复核、举报/屏蔽和知乎授权发起/回调/断开。详见 `routes.go` 与 `docs/API.md` 第 18 节。
