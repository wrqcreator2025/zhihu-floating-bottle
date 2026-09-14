import { test, expect } from '@playwright/test';

test('真实来信接收、回信审核和对方邀请后开启聊天', async ({ page }) => {
  await page.addInitScript(() =>
    localStorage.setItem(
      'episode-island-v1',
      JSON.stringify({
        version: 1,
        experiences: [],
        draft: { body: '', target: '' },
        bottles: [
          {
            id: 'guide',
            sampleId: 'developer-guide-v1',
            kind: 'received',
            body: '指南',
            target: '',
            status: 'saved',
          },
        ],
      }),
    ),
  );
  let accepted = false;
  let replied = false;
  let chatActive = false;
  let messages = [];
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace('/api/v1', '');
    let data;
    if (path === '/auth/session')
      data = { id: 'usr_receiver', provider: 'zhihu' };
    else if (path === '/invitations/next')
      data = {
        id: 'inv_test',
        letter: { episodeTitle: '第一次求职', body: '想听听你的经历。' },
      };
    else if (path === '/invitations/inv_test/decision') {
      expect(request.postDataJSON()).toEqual({ decision: 'accept' });
      accepted = true;
      data = { connectionId: 'con_test' };
    } else if (path === '/connections/con_test') {
      expect(accepted).toBe(true);
      data = {
        id: 'con_test',
        status: replied ? 'replied' : 'awaiting_first_reply',
        letter: { episodeTitle: '第一次求职', body: '想听听你的经历。' },
        messages,
      };
    } else if (path === '/connections/con_test/chat')
      data = {
        invitation: replied
          ? { id: 'chi_test', status: chatActive ? 'accepted' : 'pending' }
          : null,
        session: chatActive ? { status: 'active' } : null,
      };
    else if (path === '/chat-invitations/chi_test/decision') {
      expect(request.postDataJSON()).toEqual({ decision: 'accept' });
      chatActive = true;
      data = {};
    } else if (path.endsWith('/messages')) {
      const isChat = path.includes('/chat/');
      if (isChat) expect(chatActive).toBe(true);
      expect(request.headers()['idempotency-key']).toBeTruthy();
      messages.push({
        id: `msg_${messages.length}`,
        body: request.postDataJSON().body,
        kind: isChat ? 'chat' : 'reply',
        senderRole: 'responder',
        deliveryStatus: 'pending_moderation',
      });
      data = { message: messages.at(-1) };
    } else return route.fulfill({ status: 404 });
    await route.fulfill({ json: { data } });
  });
  await page.goto('/');
  await expect(page.locator('#loading')).not.toBeVisible();
  await expect(page.getByText('✓ 知乎已登录')).toBeVisible();
  await page.locator('#navigation [data-action="receive"]').click();
  await page.locator('#receive-accept').click();
  await page
    .locator('#message-body')
    .fill('我也经历过求职受挫，后来调整了准备方法。');
  await page.locator('#server-message button[type=submit]').click();
  await expect(page.locator('.message-list')).toContainText('审核中');
  await expect(page.locator('#server-message')).toHaveCount(0);
  replied = true;
  messages[0].deliveryStatus = 'delivered';
  await page.locator('#connection-refresh').click();
  await expect(page.locator('.message-list')).toContainText('已送达');
  await page.locator('#accept-server-chat').click();
  await page.locator('#message-body').fill('很高兴继续聊聊。');
  await page.locator('#server-message button[type=submit]').click();
  await expect(page.locator('.message-list')).toContainText('很高兴继续聊聊。');
  expect(messages.at(-1).kind).toBe('chat');
});
