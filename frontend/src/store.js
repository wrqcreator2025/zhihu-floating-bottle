import { checkMessage } from './moderation.js';

export const STORAGE_KEY = 'episode-island-v1';
const blank = () => ({
  version: 1,
  bottles: [],
  experiences: [],
  draft: { body: '', target: '' },
});
const string = (v) => typeof v === 'string';
const normalizeBottle = (b) => {
  if (b.kind === 'sent') {
    b.contacts = (b.contacts ?? []).filter((contact) => contact.id !== `contact-${b.id}`);
    if (!b.contacts.length && b.status === 'replied') b.status = 'saved';
    b.contacts.forEach((contact) => {
      contact.chatStatus ??= 'none';
      contact.chatMessages ??= [];
    });
    if (b.contacts.length && b.status === 'saved') b.status = 'replied';
  } else {
    b.chatInvite ??= null;
    b.chatMessages ??= [];
  }
  return b;
};
export function validState(s) {
  return (
    s?.version === 1 &&
    Array.isArray(s.bottles) &&
    Array.isArray(s.experiences) &&
    s.bottles.every(
      (b) =>
        string(b.id) &&
        ['sent', 'received'].includes(b.kind) &&
        string(b.body) &&
        string(b.target) &&
        ['saved', 'accepted', 'replied', 'declined'].includes(b.status),
    ) &&
    s.experiences.every(
      (e) =>
        string(e.id) &&
        string(e.title) &&
        string(e.body) &&
        e.confirmed === true &&
        typeof e.open === 'boolean',
    ) &&
    string(s.draft?.body) &&
    string(s.draft?.target)
  );
}
export function createStore(storage, onError = () => {}) {
  let state = blank();
  try {
    const raw = storage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (!validState(parsed)) throw Error('invalid');
      state = parsed;
      state.bottles = state.bottles.filter((b) => !b.demo && !b.sampleId?.startsWith('first-internship'));
      state.bottles.forEach(normalizeBottle);
    }
  } catch {
    onError('无法读取本地记录，本次内容将暂存于当前页面。');
  }
  const persist = () => {
    try {
      storage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {
      onError('浏览器未能保存，当前内容仅在此页面保留。');
    }
  };
  const id = () => globalThis.crypto.randomUUID();
  return {
    get: () => structuredClone(state),
    draft(body, target) {
      state.draft = { body, target };
      persist();
    },
    send(body, target) {
      if (!body.trim() || !target.trim())
        throw Error('请写下处境和想听谁的经历。');
      const bottle = {
        id: id(),
        kind: 'sent',
        body: body.trim(),
        target: target.trim(),
        status: 'saved',
        contacts: [],
        createdAt: Date.now(),
      };
      state.bottles.unshift(bottle);
      state.draft = { body: '', target: '' };
      persist();
      return bottle;
    },
    receive(sample, decision) {
      const existing = state.bottles.find((b) => b.sampleId === sample.id);
      if (existing) return existing;
      if (!['accepted', 'declined'].includes(decision))
        throw Error('无效的接收选择');
      const bottle = {
        id: id(),
        sampleId: sample.id,
        kind: 'received',
        body: sample.body,
        target: sample.target,
        status: decision,
        reply: '',
        chatInvite: null,
        chatMessages: [],
        createdAt: Date.now(),
      };
      state.bottles.unshift(bottle);
      persist();
      return bottle;
    },
    reply(id, text, finish = false) {
      const b = state.bottles.find((b) => b.id === id);
      if (!b || b.kind !== 'received' || b.status !== 'accepted')
        throw Error('这封来信当前不可回应');
      if (finish && !text.trim()) throw Error('请先写一点想说的话。');
      b.reply = text;
      if (finish) {
        b.status = 'replied';
        b.chatInvite ??= {
          status: 'pending',
          createdAt: Date.now(),
        };
      }
      persist();
    },
    requestChat(bottleId, contactId) {
      const b = state.bottles.find(
        (item) => item.id === bottleId && item.kind === 'sent',
      );
      const contact = b?.contacts?.find((item) => item.id === contactId);
      if (!contact) throw Error('没有找到这位匿名回信者');
      if (contact.chatStatus === 'none') contact.chatStatus = 'pending';
      persist();
      return structuredClone(contact);
    },
    decideChat(bottleId, decision, contactId = null) {
      if (!['active', 'declined'].includes(decision))
        throw Error('无效的聊天邀请选择');
      const b = state.bottles.find((item) => item.id === bottleId);
      if (!b) throw Error('没有找到联系你们的瓶子');
      if (b.kind === 'sent') {
        const contact = b.contacts?.find((item) => item.id === contactId);
        if (!contact || contact.chatStatus !== 'pending')
          throw Error('当前没有等待处理的聊天邀请');
        contact.chatStatus = decision;
        persist();
        return structuredClone(contact);
      }
      if (b.chatInvite?.status !== 'pending')
        throw Error('当前没有等待处理的聊天邀请');
      b.chatInvite.status = decision;
      persist();
      return structuredClone(b.chatInvite);
    },
    chat(bottleId, text, contactId = null) {
      if (!text.trim()) throw Error('写一点想说的话再寄出。');
	  const moderation = checkMessage(text);
	  if (moderation.action !== 'allow') throw Error(moderation.message);
      const b = state.bottles.find((item) => item.id === bottleId);
      if (!b) throw Error('没有找到联系你们的瓶子');
      const message = {
        id: id(),
        side: 'me',
        body: text.trim(),
        createdAt: Date.now(),
      };
      if (b.kind === 'sent') {
        const contact = b.contacts?.find((item) => item.id === contactId);
        if (!contact || contact.chatStatus !== 'active')
          throw Error('对方还没有接受匿名聊天');
        contact.chatMessages.push(message);
      } else {
        if (b.chatInvite?.status !== 'active')
          throw Error('你还没有接受匿名聊天');
        b.chatMessages.push(message);
      }
      persist();
      return structuredClone(message);
    },
    experience(entry) {
      if (!entry.title.trim() || !entry.body.trim() || !entry.confirmed)
        throw Error('请填写经历并确认是自己的真实经历。');
      const old = state.experiences.find((e) => e.id === entry.id);
      const next = {
        id: old?.id ?? id(),
        title: entry.title.trim(),
        body: entry.body.trim(),
        confirmed: true,
        open: !!entry.open,
        updatedAt: Date.now(),
      };
      if (old) Object.assign(old, next);
      else state.experiences.unshift(next);
      persist();
      return next;
    },
  };
}
