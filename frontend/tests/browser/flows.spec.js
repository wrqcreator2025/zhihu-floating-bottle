import { test, expect } from '@playwright/test';
const saved = (page) =>
  page.evaluate(() => JSON.parse(localStorage.getItem('episode-island-v1')));
async function openBottle(page, kind = 'write') {
  if (kind === 'bottle') {
    const bounds = await page.locator('[data-action="bottle"]').boundingBox();
    await page.mouse.click(
      bounds.x + bounds.width / 2,
      bounds.y + bounds.height / 2,
    );
  } else await page.locator(`[data-action="${kind}"]`).first().click();
  await page.locator('[data-action="uncork"]').click();
  await expect(page.locator('#sheet')).toBeVisible();
}
test.beforeEach(async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#loading')).not.toBeVisible();
});
test('第一次接收开发者指南，保存刷新后可重读', async ({ page }) => {
  await expect(page.locator('#scene canvas')).toBeVisible();
  await expect(page.locator('[data-action="bottle"]')).toHaveCount(0);
  await expect(page.getByText('本地体验', { exact: true })).toHaveCount(0);
  await openBottle(page, 'receive');
  await expect(page.locator('#sheet')).toContainText('来自漂流瓶开发者');
  await page.screenshot({ path: `../开发者指南-${test.info().project.name}-${Date.now()}.png`, animations: 'disabled' });
  await page.locator('[data-action="keep-guide"]').click();
  await page.reload();
  await expect(page.locator('#loading')).not.toBeVisible();
  await page.locator('[data-action="receive"]').first().click();
  await expect(page.locator('#toast')).toContainText('暂时没有新的来信');
  await expect(page.locator('#sheet')).not.toBeVisible();
  await page.locator('#navigation [data-action="island"]').click();
  await page.locator('#navigation [data-action="room"]').click();
  await page.locator('#navigation [data-action="cabinet"]').click();
  await page.locator('[data-action="records"]').click();
  await page.locator('.record').click();
  await expect(page.locator('#sheet')).toContainText('你的第一只漂流瓶');
  await expect(page.locator('#reply-form')).toHaveCount(0);
});
test('知乎登录状态由真实会话响应决定，刷新仍回显', async ({ page }) => {
  await page.route('**/api/v1/auth/session', (route) => route.fulfill({
    json: { data: { id: 'usr_test', provider: 'zhihu' } },
  }));
  await page.reload();
  await expect(page.getByText('✓ 知乎已登录')).toBeVisible();
  await expect(page.locator('[data-action="zhihu-login"]')).toHaveCount(0);
  await page.route('**/api/v1/auth/session', (route) => route.fulfill({ status: 401, json: { error: {} } }));
  await page.reload();
  await expect(page.locator('[data-action="zhihu-login"]')).toBeVisible();
  await expect(page.getByText('✓ 知乎已登录')).toHaveCount(0);
});
test('连续抛出两只瓶子，日记独立保存、编辑和关闭接收', async ({ page }) => {
  for (const body of [
    '第一次找实习，不知道如何开始。',
    '新的选择：要不要读研。',
  ]) {
    await openBottle(page);
    await page.locator('#bottle-body').fill(body);
    await page.locator('#bottle-target').fill('亲自走过这段路的人');
    await page.locator('#write-form button[type=submit]').click();
    await expect(page.locator('#toast')).toHaveClass(/show/, {
      timeout: 12000,
    });
    await expect(page.locator('#navigation')).not.toHaveAttribute('inert', '');
  }
  expect((await saved(page)).bottles).toHaveLength(2);
  for (const bottle of (await saved(page)).bottles) {
    expect(bottle.contacts).toEqual([]);
    expect(bottle.status).toBe('saved');
  }
  await page.locator('#navigation [data-action="island"]').click();
  await page.locator('#navigation [data-action="room"]').click();
  await page.locator('#navigation [data-action="diary"]').click();
  await page.locator('#experience-title').fill('找到了第一份实习');
  await page
    .locator('#experience-body')
    .fill('投递很久，也怀疑过自己，后来学着向别人介绍项目。');
  await page.locator('#confirmed').check();
  await page.locator('#receive-open').check();
  await page.locator('#diary-form button[type=submit]').click();
  await expect(page.locator('#diary-saved')).toContainText('保存');
  await page.locator('#receive-open').uncheck();
  await page.locator('#diary-form button[type=submit]').click();
  await page.reload();
  const state = await saved(page);
  expect(state.experiences).toHaveLength(1);
  expect(state.experiences[0].open).toBe(false);
  expect(state.bottles).toHaveLength(2);
});
