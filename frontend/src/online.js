import { createBottleDraft } from './api.js';

const $ = (s) => document.querySelector(s);
const lines = (text) =>
  text
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean);
const statusText = (status) =>
  ({
    draft: '草稿',
    searching: '已发出，寻找中',
    paused: '已暂停',
    match_failed: '暂未找到合适的人',
    search_error: '寻找暂时失败，可重试',
    pending_moderation: '审核中',
    moderation_failed: '审核服务暂不可用',
    rejected: '未通过审核',
    delivered: '已送达',
    awaiting_first_reply: '等待回信',
    replied: '已有回信',
    closed: '已结束',
  })[status] || status;

export function createOnline({
  api,
  storage,
  userId,
  openSheet,
  closeButton,
  btn,
  escape,
  toast,
  launch,
  setView,
  showGuide,
  hasGuide,
}) {
  const draft = createBottleDraft(api, storage, userId);
  let busy = false;
  let savedInput = null;
  function errorText(error) {
    if (
      $('#online-write') &&
      ['AI_PROTOCOL_ERROR', 'AI_UNAVAILABLE'].includes(error.code)
    )
      return '暂时无法整理，原稿已保留。你可以直接点击“发出瓶子”。';
    if (error.code === 'NOT_EXPERIENCE_MATCHING')
      return `${error.message}。可以修改问题后再试。`;
    return error.message;
  }
  async function run(form, work) {
    if (busy) return;
    busy = true;
    const controls = [
      ...document.querySelectorAll(
        '#sheet button, #sheet input, #sheet textarea',
      ),
    ];
    controls.forEach((el) => {
      el.disabled = true;
    });
    const status = form?.querySelector('[role="status"]');
    if (status) status.textContent = '正在处理，请稍候……';
    try {
      await work();
    } catch (error) {
      if (status?.isConnected) status.textContent = errorText(error);
      toast(errorText(error));
    } finally {
      busy = false;
      controls.forEach((el) => {
        el.disabled = false;
      });
    }
  }
  async function load(title, work) {
    const token = crypto.randomUUID();
    openSheet(
      `${closeButton()}<h2>${title}</h2><p data-loading="${token}" role="status">正在读取……</p>`,
    );
    const current = () =>
      !!document.querySelector(`[data-loading="${token}"]`) && $('#sheet').open;
    try {
      const render = await work();
      if (current()) render();
    } catch (error) {
      if (current()) $('[data-loading]').textContent = errorText(error);
    }
  }
  function writeForm(body = '', hint = '', note = '') {
    openSheet(
      `${closeButton()}<div class="eyebrow">写给走过这段路的人</div><h2>最近，你在演哪一集？</h2><p class="subtext">写好就可以发出。提交后会根据你的描述，寻找有相似经历、愿意回应的人。</p><form id="online-write"><label for="bottle-body">你正在经历什么？</label><textarea id="bottle-body" required maxlength="4000" rows="5">${escape(body)}</textarea><label for="bottle-target">想听谁说说？（可不填）</label><textarea id="bottle-target" maxlength="4000" rows="2" placeholder="不填时，寻找经历过相似处境的人">${escape(hint)}</textarea><p role="status" class="save-status">${escape(note)}</p><button class="primary" type="submit" id="send-bottle">发出瓶子 ↗</button><button class="text-button" type="submit" id="suggest-bottle">先让 AI 帮我整理（可选）</button></form>`,
    );
    const form = $('#online-write');
    form.addEventListener('input', () => {
      savedInput = {
        body: $('#bottle-body').value,
        hint: $('#bottle-target').value,
      };
    });
    form.addEventListener('submit', (event) => {
      event.preventDefault();
      const suggest = event.submitter?.id === 'suggest-bottle';
      savedInput = {
        body: $('#bottle-body').value.trim(),
        hint: $('#bottle-target').value.trim(),
      };
      if (!savedInput.body) return;
      void run(form, async () => {
        await draft.prepare(savedInput.body, savedInput.hint);
        if (!suggest) {
          await draft.confirm(
            Array.from(savedInput.body).slice(0, 40).join(''),
            {
              requiredExperiences: [
                savedInput.hint || '亲身经历过与这段描述相似的处境',
              ],
              preferredExperiences: [],
              viewpointPreferences: [],
            },
          );
          draft.clear();
          savedInput = null;
          launch('瓶子已发出，正在寻找有相似经历的人。可在瓶子柜查看进度。');
          return;
        }
        const suggestion = await draft.suggest();
        if (
          suggestion.episode.needsClarification ||
          suggestion.target.needsClarification
        ) {
          writeForm(
            savedInput.body,
            savedInput.hint,
            suggestion.episode.question ||
              suggestion.target.question ||
              '请补充你正在经历的具体事情，再试一次。',
          );
        } else confirmForm(suggestion);
      });
    });
  }
  function confirmForm({ episode, target }) {
    openSheet(
      `${closeButton()}<div class="eyebrow">AI 整理 · 等你确认</div><h2>这样表达，合你的心意吗？</h2><p class="letter-copy">${escape(episode.summary)}</p><form id="online-confirm"><label for="ai-title">给这一集起个名字</label><input id="ai-title" required maxlength="100" value="${escape(episode.title)}"><label for="ai-required">希望对方亲自经历过（每行一项）</label><textarea id="ai-required" required rows="3">${escape((target.requiredExperiences || []).join('\n'))}</textarea><label for="ai-preferred">如果还经历过这些就更好（可选）</label><textarea id="ai-preferred" rows="2">${escape((target.preferredExperiences || []).join('\n'))}</textarea><label for="ai-viewpoints">想听到的不同看法（可选）</label><textarea id="ai-viewpoints" rows="2">${escape((target.viewpointPreferences || []).join('\n'))}</textarea><p class="fine-print">${target.activityUsed ? '建议参考了可用的知乎活动主题。' : '建议基于你填写的问题；暂未使用知乎活动主题。'} 原文保持不变。提交后根据你的描述寻找愿意回应的人。</p><p role="status" class="save-status"></p><div class="receive-actions"><button class="primary" type="submit">确认，抛向海面 ↗</button><button type="button" id="ai-back" class="text-button">修改原文</button></div></form>`,
    );
    $('#ai-back').onclick = () => writeForm(savedInput.body, savedInput.hint);
    $('#ai-title').maxLength = 80;
    const form = $('#online-confirm');
    form.addEventListener('submit', (event) => {
      event.preventDefault();
      const required = lines($('#ai-required').value);
      if (!required.length) {
        form.querySelector('[role=status]').textContent =
          '请填写至少一项希望对方有的经历。';
        return;
      }
      const title = $('#ai-title').value.trim();
      const confirmedTarget = {
        requiredExperiences: required,
        preferredExperiences: lines($('#ai-preferred').value),
        viewpointPreferences: lines($('#ai-viewpoints').value),
      };
      void run(form, async () => {
        await draft.confirm(title, confirmedTarget);
        draft.clear();
        savedInput = null;
        launch('瓶子已发出，正在寻找有相似经历的人。可在瓶子柜查看进度。');
      });
    });
  }
  async function write() {
    if (savedInput) return writeForm(savedInput.body, savedInput.hint);
    await load('取回你的草稿', async () => {
      const saved = await draft.restore();
      return () => {
        if (saved?.status === 'draft') {
          savedInput = {
            body: saved.episode.rawText,
            hint: saved.target.hintText,
          };
          writeForm(savedInput.body, savedInput.hint);
        } else {
          draft.clear();
          writeForm();
        }
      };
    });
  }
  function records(direction = 'sent', cursor = '') {
    setView('cabinet');
    return load('来过的信，都收在这里。', async () => {
      const result = await api(
        `/cabinet?direction=${direction}&limit=20${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`,
        { envelope: true },
      );
      return () => {
        openSheet(
          `${closeButton()}<h2>来过的信，都收在这里。</h2><div class="tabs"><button id="server-sent" aria-selected="${direction === 'sent'}">已发出</button><button id="server-received" aria-selected="${direction === 'received'}">已接收</button></div><div class="record-list">${direction === 'received' && hasGuide() ? '<button class="record" id="server-guide">开发者使用指南 ↗</button>' : ''}${result.data.map((b) => `<button class="record" data-server-record="${escape(b.id)}"><span><strong>${escape(b.title || b.preview || '一只漂流瓶')}</strong><small>${escape(statusText(b.bottleStatus))}</small></span>↗</button>`).join('') || '<p class="subtext">还没有记录。</p>'}</div><div class="receive-actions"><button id="server-refresh" class="text-button">刷新记录</button>${result.nextCursor ? '<button id="server-next" class="text-button">下一页 →</button>' : ''}</div>`,
          'cabinet-sheet',
        );
        $('#server-sent').onclick = () => records('sent');
        $('#server-received').onclick = () => records('received');
        $('#server-refresh').onclick = () => records(direction);
        if ($('#server-guide'))
          $('#server-guide').onclick = () => showGuide(true);
        if ($('#server-next'))
          $('#server-next').onclick = () =>
            records(direction, result.nextCursor);
        document.querySelectorAll('[data-server-record]').forEach((el) => {
          el.onclick = () => record(el.dataset.serverRecord, direction);
        });
      };
    });
  }
  function record(id, direction) {
    return load('打开这封信', async () => {
      const value = await api(`/cabinet/${encodeURIComponent(id)}`);
      return () => {
        if (direction === 'received') return connection(value.id, 'responder');
        const b = value.bottle;
        openSheet(
          `${closeButton()}<h2>${escape(b.episode.title || '写给走过这段路的人')}</h2><p class="subtext">${escape(statusText(b.status))}${b.failureReason ? ` · ${escape(b.failureReason)}` : ''}</p><p class="letter-copy">${escape(b.episode.rawText)}</p><div class="wanted"><small>想听谁的经历</small><p>${escape(b.target.requiredExperiences.join('；'))}</p></div><div class="contact-list">${value.connections.map((c, i) => `<button class="record" data-connection="${escape(c.id)}">匿名来信 ${i + 1} · ${escape(statusText(c.status))} ↗</button>`).join('') || '<p>还没有人接住，稍后可以刷新查看。</p>'}</div>${['match_failed', 'search_error', 'paused', 'searching'].includes(b.status) ? `<button class="primary" id="bottle-control">${b.status === 'searching' ? '暂停寻找' : '继续寻找'}</button>` : ''}<button class="text-button" id="record-refresh">刷新状态</button>${btn('records', '← 返回瓶子柜')}`,
        );
        document.querySelectorAll('[data-connection]').forEach((el) => {
          el.onclick = () => connection(el.dataset.connection, 'sender');
        });
        $('#record-refresh').onclick = () => record(id, direction);
        if ($('#bottle-control'))
          $('#bottle-control').onclick = () =>
            run(null, async () => {
              await api(
                `/bottles/${b.id}/${b.status === 'searching' ? 'pause' : b.status === 'paused' ? 'resume' : 'retry'}`,
                { method: 'POST' },
              );
              await record(id, direction);
            });
      };
    });
  }
  function diary(id = null, notice = '') {
    setView('diary');
    return load('我的经历', async () => {
      const entries = await api('/experiences');
      const e = entries.find((x) => x.id === id) || {};
      return () => {
        openSheet(
          `${closeButton()}<div class="book-spread"><section class="book-left"><div class="eyebrow">MY CHAPTERS</div><h2>我走过的经历</h2><p class="subtext">没有成功的那一段，也算。</p><div class="diary-entries">${entries.map((x, i) => `<button class="diary-entry ${x.id === id ? 'selected' : ''}" data-server-experience="${escape(x.id)}"><small>${String(i + 1).padStart(2, '0')}</small><strong>${escape(x.title)}</strong><span>${x.receiveOpen ? '愿意接收相关来信' : '暂不接收来信'}</span></button>`).join('') || '<p class="diary-empty">翻开新的一页。<br>记下你真正走过的一段路。</p>'}</div><button id="new-server-experience" class="new-entry">＋ 写下一段经历</button><p class="book-note">经历会一直保留，<br>与瓶子分开保存。</p></section><section class="book-right"><div class="eyebrow">${id ? 'REVISIT A CHAPTER' : 'A NEW CHAPTER'}</div><h3>${id ? '回头看看这一段' : '写下一段经历'}</h3><form id="server-diary"><label for="experience-title">给这一段起个名字</label><input id="experience-title" required maxlength="80" value="${escape(e.title || '')}"><label for="experience-body">你走过了什么？</label><textarea id="experience-body" class="ruled" required maxlength="8000" rows="6">${escape(e.body || '')}</textarea><label class="check"><input id="confirmed" type="checkbox" required ${e.confirmedByUser ? 'checked' : ''}>这段经历确实发生在我身上</label><label class="check switch-label"><input id="receive-open" type="checkbox" role="switch" ${e.receiveOpen ? 'checked' : ''}>愿意接收相关来信</label><p class="fine-print">经历独立保存在你的账号下，可随时修改接收意愿。</p><button class="primary full" type="submit">保存经历 ↗</button><p role="status" id="diary-saved" class="save-status">${escape(notice)}</p></form></section></div>`,
          'diary-sheet',
        );
        document.querySelectorAll('[data-server-experience]').forEach((el) => {
          el.onclick = () => diary(el.dataset.serverExperience);
        });
        $('#new-server-experience').onclick = () => diary();
        $('#experience-title').maxLength = 80;
        const form = $('#server-diary');
        form.onsubmit = (event) => {
          event.preventDefault();
          const body = {
            title: $('#experience-title').value.trim(),
            body: $('#experience-body').value.trim(),
            confirmedByUser: $('#confirmed').checked,
            receiveOpen: $('#receive-open').checked,
          };
          void run(form, async () => {
            const saved = await api(
              id ? `/experiences/${id}` : '/experiences',
              { method: id ? 'PATCH' : 'POST', body },
            );
            await diary(saved.id, '这一页，已经保存到账号。');
          });
        };
      };
    });
  }
  async function receive() {
    try {
      const incoming = await api('/invitations/next');
      if (!incoming) {
        toast('暂时没有新的来信。');
        return;
      }
      openSheet(
        `${closeButton()}<div class="eyebrow">来自海上的一封信</div><h2>${escape(incoming.letter.episodeTitle)}</h2><p class="letter-copy">${escape(incoming.letter.body)}</p><div class="receive-actions"><button class="primary" id="receive-accept">我演过，接住这封信</button><button class="text-button" id="receive-decline">这次不聊</button></div>`,
      );
      const decide = (decision) =>
        run(null, async () => {
          const result = await api(`/invitations/${incoming.id}/decision`, {
            method: 'POST',
            body: { decision },
          });
          if (result.connectionId)
            await connection(result.connectionId, 'responder');
          else {
            await records('received');
            toast('这封信继续漂流。');
          }
        });
      $('#receive-accept').onclick = () => decide('accept');
      $('#receive-decline').onclick = () => decide('not_now');
    } catch (error) {
      toast(errorText(error));
    }
  }
  function connection(id, role) {
    return load('由这只瓶子联系起来', async () => {
      const results = await Promise.allSettled([
        api(`/connections/${id}`),
        api(`/connections/${id}/chat`),
      ]);
      const failure = results.find((r) => r.status === 'rejected');
      if (failure) throw failure.reason;
      const [c, chat] = results.map((r) => r.value);
      return () => {
        const active =
          chat.session?.status === 'active' && c.status !== 'closed';
        const canReply =
          role === 'responder' &&
          c.status === 'awaiting_first_reply' &&
          !c.messages.some((m) => m.kind === 'reply');
        openSheet(
          `${closeButton()}<h2>${escape(c.letter.episodeTitle)}</h2><p class="letter-copy">${escape(c.letter.body)}</p><section class="message-list">${c.messages.map((m) => `<div class="message ${m.senderRole === role ? 'me' : ''}"><p>${escape(m.body)}</p><small>${escape(statusText(m.deliveryStatus))}</small></div>`).join('')}</section>${canReply || active ? '<form id="server-message"><label for="message-body">写一点想说的话</label><textarea id="message-body" required maxlength="8000" rows="4"></textarea><p role="status" class="save-status">发送后先审核，通过后对方才会收到。</p><button class="primary" type="submit">提交回信 ↗</button></form>' : ''}${role === 'sender' && c.status === 'replied' && !chat.invitation ? '<button id="invite-chat" class="primary">邀请匿名聊天</button>' : ''}${role === 'responder' && chat.invitation?.status === 'pending' ? '<button id="accept-server-chat" class="primary">接受匿名聊天</button><button id="decline-server-chat" class="text-button">暂时不聊</button>' : ''}${chat.invitation?.status === 'pending' && role === 'sender' ? '<p>等待对方接受聊天邀请。</p>' : ''}<button id="connection-refresh" class="text-button">刷新回信</button>${btn('records', '← 返回瓶子柜')}`,
        );
        $('#connection-refresh').onclick = () => connection(id, role);
        if ($('#server-message')) {
          const form = $('#server-message');
          const keys = new Map();
          form.onsubmit = (event) => {
            event.preventDefault();
            const body = $('#message-body').value.trim();
            if (!body) return;
            if (!keys.has(body)) keys.set(body, crypto.randomUUID());
            void run(form, async () => {
              await api(
                `/connections/${id}/${active ? 'chat/messages' : 'messages'}`,
                {
                  method: 'POST',
                  body: { body },
                  headers: { 'Idempotency-Key': keys.get(body) },
                },
              );
              await connection(id, role);
              toast('已提交审核。');
            });
          };
        }
        if ($('#invite-chat'))
          $('#invite-chat').onclick = () =>
            run(null, async () => {
              await api(`/connections/${id}/chat-invitations`, {
                method: 'POST',
              });
              await connection(id, role);
            });
        for (const decision of ['accept', 'decline']) {
          const button = $(`#${decision}-server-chat`);
          if (button)
            button.onclick = () =>
              run(null, async () => {
                await api(`/chat-invitations/${chat.invitation.id}/decision`, {
                  method: 'POST',
                  body: { decision },
                });
                await connection(id, role);
              });
        }
      };
    });
  }
  return {
    write,
    records,
    diary,
    receive,
    get busy() {
      return busy;
    },
  };
}
