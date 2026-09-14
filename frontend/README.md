# 海岛漂流瓶前端

以 `../海岛漂流瓶-设计展示-03.png` 为视觉基准的交互前端。入口为 `index.html`，三维场景位于 `src/scene.js`，界面流程位于 `src/main.js`，本地数据逻辑位于 `src/store.js`。

当前实现包括：海面与小岛、镜头推进、进入小屋、瓶子柜、拖动瓶塞，以及知乎登录后的真实业务操作。`src/api.js` 负责带会话 Cookie 的请求和版本化草稿；`src/online.js` 负责 AI 整理、确认抛瓶、账号瓶子柜、经历、来信、审核回信和匿名聊天。登录成功后显示「✓ 知乎已登录」。

抛瓶先创建服务端草稿，再请求问题摘要和目标经历建议；用户确认或修改后才提交审核与匹配。AI 失败会保留原稿并显示服务错误，重试复用同一草稿。点击整理前的输入保留在当前页面内存；点击后原稿存入账号，浏览器仅保存按账号隔离的草稿 ID。刷新可恢复已保存草稿。生产环境抛瓶要求登录；未登录的本地开发仍可运行原有本地交互演示。

首次点击「接收一个瓶子」会收到署名开发者的使用指南。选择「收好指南」后存入瓶子柜，刷新不会重复投递，可在已接收记录中重读；它不提供回信或聊天功能。首次领取以当前浏览器记录为准。

## 安装与运行

需要 Node.js 20 或更高版本，以及 Corepack。

```sh
corepack pnpm install
corepack pnpm dev
```

本地联调知乎 OAuth 时设置 `VITE_API_BASE_URL=http://127.0.0.1:8080`。Vercel 生产环境通过同域 `/api` 重写转发至 Railway，使 OAuth 和登录会话 Cookie 保持第一方；知乎项目和 Railway 的 OAuth 回调地址均登记为 `https://zhihu-floating-bottle.vercel.app/api/v1/integrations/zhihu/callback`。详细步骤见 [部署说明](../docs/DEPLOYMENT.md)。

打开终端显示的本地地址。不要直接双击 `index.html`，Vite 负责加载 ES 模块和着色器代码。

## 构建与验证

```sh
corepack pnpm build
corepack pnpm test
corepack pnpm test:e2e
```

浏览器测试需要 Playwright Chromium。首次运行若缺少浏览器，执行 `corepack pnpm exec playwright install chromium`。测试覆盖桌面和手机尺寸下的指南重读、登录回显、本地连续抛瓶、AI 失败重试和确认发送、账号记录与经历、回信审核和聊天。业务接口在浏览器测试中模拟；部署环境仍需使用真实知乎登录和模型配置验证。单独运行接口数据测试：`node --test tests/api.test.js`。

## 数据边界

- 存储键：`episode-island-v1`。
- 清除站点数据会删除本地演示记录和指南，不会删除服务端账号数据；草稿恢复入口依赖本地保存的 ID。
- 不再自动生成示例来信或回信；旧版本示例记录会在读取时移除，本人创建的瓶子和日记保留。
- 连续创建表示保留独立瓶子记录，不声称多个瓶子正在并行匹配。
