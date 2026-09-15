import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createAPI, createBottleDraft } from '../src/api.js';

const memory = () => {
  const values = new Map();
  return {
    getItem: (key) => values.get(key),
    setItem: (key, value) => values.set(key, value),
  };
};
test('API carries session cookies, handles empty inbox and preserves backend errors', async () => {
  let request;
  const api = createAPI('', async (url, options) => {
    request = { url, ...options };
    return new Response(JSON.stringify({ data: { id: 'btl_test' } }), {
      status: 201,
    });
  });
  await api('/bottles', { method: 'POST', body: { episodeText: '原文' } });
  assert.equal(request.credentials, 'include');
  assert.equal(JSON.parse(request.body).episodeText, '原文');
  assert.equal(
    await createAPI(
      '',
      async () => new Response(null, { status: 204 }),
    )('/invitations/next'),
    null,
  );
  const unavailable = createAPI(
    '',
    async () =>
      new Response(
        JSON.stringify({
          error: { code: 'AI_UNAVAILABLE', message: '内容处理服务尚未配置' },
        }),
        { status: 503 },
      ),
  );
  await assert.rejects(unavailable('/ai/episode-drafts'), {
    code: 'AI_UNAVAILABLE',
  });
});

function server() {
  let bottle;
  const calls = [];
  const api = async (path, options = {}) => {
    calls.push({ path, ...options });
    if (path === '/bottles')
      bottle = {
        id: 'btl_test',
        status: 'draft',
        contentVersion: 1,
        episode: { rawText: options.body.episodeText },
        target: { hintText: options.body.targetHint },
      };
    else if (path.startsWith('/ai/'))
      return {
        bottleId: bottle.id,
        sourceContentVersion: bottle.contentVersion,
        title: '标题',
        summary: '摘要',
        requiredExperiences: ['经历'],
      };
    else if (path.endsWith('/launch')) {
      bottle.status = 'searching';
      return { bottleId: bottle.id, status: bottle.status };
    } else if (options.method === 'PATCH') {
      assert.equal(options.body.sourceContentVersion, bottle.contentVersion);
      bottle.contentVersion++;
      Object.assign(bottle.episode, options.body.episode);
      Object.assign(bottle.target, options.body.target);
    } else return { bottle: structuredClone(bottle) };
    return structuredClone(bottle);
  };
  return { api, calls };
}
test('AI suggestions never launch before confirmation; edits reuse the draft and current version', async () => {
  const { api, calls } = server();
  const storage = memory();
  const draft = createBottleDraft(api, storage, 'alice');
  await draft.prepare('原文', '提示');
  await draft.suggest();
  assert.equal(
    calls.some((c) => c.path.endsWith('/launch')),
    false,
  );
  await draft.prepare('修改后的原文', '');
  await draft.suggest();
  assert.equal(calls.filter((c) => c.path === '/bottles').length, 1);
  assert.equal(calls.at(-1).body.contentVersion, 2);
  await draft.confirm('用户修改的标题', {
    requiredExperiences: ['用户确认的经历'],
  });
  assert.equal(calls.at(-1).path, '/bottles/btl_test/launch');
  assert.equal(calls.at(-2).body.episode.confirmed, true);
  assert.equal(
    (await createBottleDraft(api, storage, 'alice').restore()).status,
    'searching',
  );
  assert.equal(await createBottleDraft(api, storage, 'bob').restore(), null);
});
test('stale AI responses are rejected and an uncertain launch is reconciled without resending', async () => {
  const { api, calls } = server();
  const draft = createBottleDraft(
    async (path, options) =>
      path.startsWith('/ai/')
        ? { bottleId: 'btl_test', sourceContentVersion: 999 }
        : api(path, options),
    memory(),
    'alice',
  );
  await draft.prepare('原文', '');
  await assert.rejects(draft.suggest(), /内容已更新/);
  await draft.confirm('标题', { requiredExperiences: ['经历'] });
  await draft.confirm('标题', { requiredExperiences: ['经历'] });
  assert.equal(calls.filter((c) => c.path.endsWith('/launch')).length, 1);
});

test('retry after a lost launch response reuses the sent bottle', async () => {
  const { api, calls } = server();
  let loseResponse = true;
  const draft = createBottleDraft(
    async (path, options) => {
      const result = await api(path, options);
      if (path.endsWith('/launch') && loseResponse) {
        loseResponse = false;
        throw Error('connection lost');
      }
      return result;
    },
    memory(),
    'alice',
  );
  const target = { requiredExperiences: ['相似处境'] };
  await draft.prepare('原文', '');
  await assert.rejects(draft.confirm('标题', target), /connection lost/);
  await draft.prepare('原文', '');
  await draft.confirm('标题', target);
  assert.equal(calls.filter((c) => c.path === '/bottles').length, 1);
  assert.equal(calls.filter((c) => c.path.endsWith('/launch')).length, 1);
});
