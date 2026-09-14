# 测试规划

当前仅注释骨架，没有可执行测试。正式实现采用 Go testing；SQL 集成测试使用真实 MySQL。

重点覆盖：并发 launch 名额、同瓶多人并发接住、重复决策与发送幂等、暂停和投递竞争、拒收继续漂流、任务租约恢复、画像缺失降级、聊天须对方接受及不同会话隔离。

知乎 adapter 用 httptest 模拟上游，核对双 Token、秒级时间戳、参数大小写、Code 错误、分页和跨用户缓存隔离；日常测试不消耗真实接口额度。

初始化 go.mod 和实际实现后再启用 go test ./...、go vet ./...；目前不声称这些命令可运行。

## 已实现的数据库集成测试

先按 `../database/README.md` 启动 MySQL 并迁移，然后在 backend 执行：

```sh
python3 -m unittest discover -s tests -p 'test_database.py' -v
# 单个回归用例
python3 -m unittest discover -s tests -p 'test_database.py' -k experience_delete -v
```

只依赖 Python 标准库和 Docker Compose。测试在真实 MySQL 上执行，数据事务回滚，不测试尚未实现的 Go API。


## Go 后端测试

`go test ./...` 跑认证与知乎协议测试；业务测试需要 `TEST_MYSQL_DSN`。推荐运行 `python3 scripts/test_backend.py`，它建立独立测试库并运行 `go test -race -count=1 ./...`。

覆盖：多人接收和会话隔离、并发 3 个主动瓶子及第 4 个拒绝、幂等首封回信、内容审核拒绝/修改/旧版本隔离、聊天同意、关闭与切片、邀请过期、暂停与重试、柜子游标分页、屏蔽/举报、搜索缓存与共享额度、后台最终失败。协议测试使用 HTTP 模拟服务器，不消耗真实知乎接口额度。

`internal/httpapi/routes_test.go` 锁定对外路由清单，防止文档中的入口在重构时遗失。业务集成测试从 HTTP 入口调用，覆盖 handler 校验、service 状态、MySQL 事务、outbox 处理、通知可见性和最终响应。
