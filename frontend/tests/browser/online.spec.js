import { test, expect } from '@playwright/test';

test('AI 失败保留原稿，重试复用草稿，用户确认后才抛出并读取云端记录', async ({
  page,
}) => {
  let bottle;
  let creates = 0;
  let launches = 0;
  let aiUnavailable = true;
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace('/api/v1', '');
    const input = request.postDataJSON();
    let data;
    if (path === '/auth/session') data = { id: 'usr_alice', provider: 'zhihu' };
    else if (path === '/bottles') {
      creates++;
      bottle = {
        id: 'btl_test',
        status: 'draft',
        contentVersion: 1,
        episode: { rawText: input.episodeText, title: '' },
        target: { hintText: input.targetHint, requiredExperiences: [] },
      };
      data = bottle;
    } else if (path.startsWith('/ai/')) {
      if (aiUnavailable)
        return route.fulfill({
          status: 503,
          json: {
            error: { code: 'AI_UNAVAILABLE', message: '内容处理服务尚未配置' },
          },
        });
      expect(input.contentVersion).toBe(bottle.contentVersion);
      data = {
        bottleId: bottle.id,
        sourceContentVersion: bottle.contentVersion,
        title: '第一次找实习',
        summary: '希望听听走过求职受挫的人。',
        requiredExperiences: ['经历过求职受挫'],
        preferredExperiences: [],
        viewpointPreferences: [],
        activityUsed: false,
      };
    } else if (path === '/bottles/btl_test/launch') {
      launches++;
      expect(bottle.episode.confirmed).toBe(true);
      expect(bottle.target.requiredExperiences).toEqual([
        '用户改为：转行后找到工作',
      ]);
      bottle.status = 'searching';
      data = { bottleId: bottle.id, status: bottle.status };
    } else if (path === '/bottles/btl_test') {
      if (request.method() === 'PATCH') {
        expect(input.sourceContentVersion).toBe(bottle.contentVersion);
        bottle.contentVersion++;
        Object.assign(bottle.episode, input.episode);
        Object.assign(bottle.target, input.target);
        data = bottle;
      } else data = { bottle, connections: [] };
    } else if (path === '/cabinet')
      data = launches
        ? [
            {
              id: 'cab_sent_test',
              title: bottle.episode.title,
              bottleStatus: bottle.status,
            },
          ]
        : [];
    else if (path === '/cabinet/cab_sent_test')
      data = { bottle, connections: [] };
    else
      return route.fulfill({
        status: 404,
        json: { error: { message: `Unexpected ${path}` } },
      });
    await route.fulfill({ json: { data } });
  });
  await page.goto('/');
  await expect(page.getByText('✓ 知乎已登录')).toBeVisible();
  await expect(page.locator('#loading')).not.toBeVisible();
  await page.locator('#navigation [data-action="write"]').click();
  await page.locator('[data-action="uncork"]').click();
  await page.locator('#bottle-body').fill('第一次找实习，不知道如何面对失败。');
  await page.locator('#online-write button[type=submit]').click();
  await expect(page.locator('#online-write [role=status]')).toContainText(
    '尚未配置',
  );
  await expect(page.locator('#bottle-body')).toHaveValue(
    '第一次找实习，不知道如何面对失败。',
  );
  expect(launches).toBe(0);
  aiUnavailable = false;
  await page.locator('#online-write button[type=submit]').click();
  await expect(page.locator('#online-confirm')).toBeVisible();
  expect(creates).toBe(1);
  expect(launches).toBe(0);
  await page.locator('#ai-required').fill('用户改为：转行后找到工作');
  await page.screenshot({
    path: `../AI确认-${test.info().project.name}-${Date.now()}.png`,
    animations: 'disabled',
  });
  await page.locator('#online-confirm button[type=submit]').click();
  await expect(page.locator('#toast')).toContainText('瓶子已提交', {
    timeout: 15000,
  });
  expect(launches).toBe(1);
  await page.reload();
  await expect(page.locator('#loading')).not.toBeVisible();
  await page.locator('#navigation [data-action="island"]').click();
  await page.locator('#navigation [data-action="room"]').click();
  await page.locator('#navigation [data-action="cabinet"]').click();
  await page.locator('[data-action="records"]').click();
  await page.locator('#server-sent').click();
  await expect(page.locator('[data-server-record]')).toContainText(
    '第一次找实习',
  );
  await page.locator('[data-server-record]').click();
  await expect(page.locator('#sheet')).toContainText(
    '用户改为：转行后找到工作',
  );
});

test('登录后的经历保存到后端，刷新可读取并关闭接收', async ({ page }) => {
  let entries = [];
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let data;
    if (path.endsWith('/auth/session'))
      data = { id: 'usr_alice', provider: 'zhihu' };
    else if (path.endsWith('/experiences') && request.method() === 'GET')
      data = entries;
    else if (path.includes('/experiences')) {
      data = { ...request.postDataJSON(), id: 'exp_test' };
      entries = [data];
    } else return route.fulfill({ status: 404 });
    await route.fulfill({ json: { data } });
  });
  const openDiary = async () => {
    await expect(page.locator('#loading')).not.toBeVisible();
    await expect(page.getByText('✓ 知乎已登录')).toBeVisible();
    await page.locator('#navigation [data-action="island"]').click();
    await page.locator('#navigation [data-action="room"]').click();
    await page.locator('#navigation [data-action="diary"]').click();
  };
  await page.goto('/');
  await openDiary();
  await page.locator('#experience-title').fill('第一份实习');
  await page
    .locator('#experience-body')
    .fill('在求职受挫后，调整简历找到了实习。');
  await page.locator('#confirmed').check();
  await page.locator('#receive-open').check();
  await page.locator('#server-diary button[type=submit]').click();
  await expect(page.locator('#diary-saved')).toContainText('保存到账号');
  await page.reload();
  await openDiary();
  await page.locator('[data-server-experience]').click();
  await expect(page.locator('#receive-open')).toBeChecked();
  await page.locator('#receive-open').uncheck();
  await page.locator('#server-diary button[type=submit]').click();
  await expect(page.locator('#diary-saved')).toContainText('保存到账号');
  expect(entries[0].receiveOpen).toBe(false);
});
