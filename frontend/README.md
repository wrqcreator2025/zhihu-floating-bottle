# 海岛漂流瓶前端

以 `../海岛漂流瓶-设计展示-03.png` 为视觉基准的交互前端。入口为 `index.html`，三维场景位于 `src/scene.js`，界面流程位于 `src/main.js`，本地数据逻辑位于 `src/store.js`。

当前实现包括：海面与小岛、镜头推进、进入小屋、瓶子柜、拖动瓶塞、创建多个独立瓶子、保存回应，以及独立编辑和关闭接收的经历日记。数据保存在浏览器 `localStorage` 中，不会联网或真实发送。此目录是前端体验实现，不代表完整 PRD 的账户、AI、真人匹配和知乎集成功能已经完成。

## 安装与运行

需要 Node.js 20 或更高版本，以及 Corepack。

```sh
corepack pnpm install
corepack pnpm dev
```

本地联调知乎 OAuth 时设置 `VITE_API_BASE_URL=http://127.0.0.1:8080`。部署到 Vercel 时在 Project Environment Variables 中设置 `VITE_API_BASE_URL=https://<Railway API 域名>`，再触发一次生产部署。详细步骤见 [部署说明](../docs/DEPLOYMENT.md)。

打开终端显示的本地地址。不要直接双击 `index.html`，Vite 负责加载 ES 模块和着色器代码。

## 构建与验证

```sh
corepack pnpm build
corepack pnpm test
corepack pnpm test:e2e
```

浏览器测试需要 Playwright Chromium。首次运行若缺少浏览器，执行 `corepack pnpm exec playwright install chromium`。测试覆盖桌面和手机尺寸下的拒收、连续抛瓶、回应归档、经历独立保存和接收开关。

## 数据边界

- 存储键：`episode-island-v1`。
- 清除站点数据会删除本地记录，记录不能跨设备同步。
- 不再自动生成示例来信或回信；旧版本示例记录会在读取时移除，本人创建的瓶子和日记保留。
- 连续创建表示保留独立瓶子记录，不声称多个瓶子正在并行匹配。
