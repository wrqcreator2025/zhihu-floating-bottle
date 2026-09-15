export function createAPI(base = '', fetcher = fetch) {
  return async (
    path,
    { method = 'GET', body, headers = {}, envelope = false } = {},
  ) => {
    let response;
    try {
      response = await fetcher(`${base}/api/v1${path}`, {
        method,
        credentials: 'include',
        headers: {
          Accept: 'application/json',
          ...(body ? { 'Content-Type': 'application/json' } : {}),
          ...headers,
        },
        ...(body ? { body: JSON.stringify(body) } : {}),
        signal: AbortSignal.timeout(60000),
      });
    } catch {
      throw Error('连接暂时中断，原稿仍保留，请稍后重试。');
    }
    if (response.status === 204) return null;
    const result = await response.json().catch(() => null);
    if (!response.ok) {
      const error = Error(
        response.status === 401
          ? '登录已失效，请重新使用知乎账号登录。'
          : result?.error?.message || '服务暂时不可用，请稍后重试。',
      );
      error.code = result?.error?.code;
      error.status = response.status;
      error.details = result?.error?.details;
      throw error;
    }
    if (!result || !Object.hasOwn(result, 'data'))
      throw Error('服务返回了无效结果，请稍后重试。');
    return envelope ? result : result.data;
  };
}

// Keep server drafts separate for each authenticated account. Never store credentials.
export function createBottleDraft(api, storage, userId) {
  const key = `drift-server-draft:${userId}`;
  let bottle = null;
  const remember = (value) => {
    bottle = value;
    try {
      storage.setItem(key, value.id);
    } catch {
      /* Current page still retains the draft. */
    }
    return value;
  };
  return {
    async restore() {
      let id;
      try {
        id = storage.getItem(key);
      } catch {
        return null;
      }
      if (!id) return null;
      const detail = await api(`/bottles/${encodeURIComponent(id)}`);
      bottle = detail.bottle;
      return bottle;
    },
    async prepare(body, hint) {
      // A launch may have succeeded even when its response was lost.
      if (bottle) {
        const latest = await api(`/bottles/${bottle.id}`);
        remember(latest.bottle);
        if (bottle.status === 'searching') {
          if (
            bottle.episode.rawText === body &&
            bottle.target.hintText === hint
          )
            return bottle;
          throw Error('上一只瓶子已发出，请重新打开发瓶页面后写新瓶子。');
        }
      }
      if (!bottle || bottle.status !== 'draft') {
        return remember(
          await api('/bottles', {
            method: 'POST',
            body: { episodeText: body, targetHint: hint },
          }),
        );
      }
      if (bottle.episode.rawText === body && bottle.target.hintText === hint)
        return bottle;
      return remember(
        await api(`/bottles/${bottle.id}`, {
          method: 'PATCH',
          body: {
            sourceContentVersion: bottle.contentVersion,
            episode: { rawText: body, confirmed: false },
            target: { hintText: hint },
          },
        }),
      );
    },
    async suggest() {
      const input = {
        bottleId: bottle.id,
        contentVersion: bottle.contentVersion,
      };
      const results = await Promise.allSettled([
        api('/ai/episode-drafts', { method: 'POST', body: input }),
        api('/ai/target-drafts', { method: 'POST', body: input }),
      ]);
      const failed = results.find((r) => r.status === 'rejected');
      if (failed) throw failed.reason;
      const [episode, target] = results.map((r) => r.value);
      if (
        [episode, target].some(
          (v) =>
            v.bottleId !== bottle.id ||
            v.sourceContentVersion !== bottle.contentVersion,
        )
      ) {
        throw Error('内容已更新，请重新整理后确认。');
      }
      return { episode, target };
    },
    async confirm(title, target) {
      // Reconcile a previous launch whose response may have been lost.
      const latest = await api(`/bottles/${bottle.id}`);
      if (latest.bottle.status === 'searching') return latest.bottle;
      if (latest.bottle.contentVersion !== bottle.contentVersion)
        throw Error('内容已在其他页面更新，请重新打开草稿。');
      remember(
        await api(`/bottles/${bottle.id}`, {
          method: 'PATCH',
          body: {
            sourceContentVersion: bottle.contentVersion,
            episode: { title, confirmed: true },
            target,
          },
        }),
      );
      return api(`/bottles/${bottle.id}/launch`, { method: 'POST' });
    },
    clear() {
      bottle = null;
      try {
        storage.setItem(key, '');
      } catch {
        /* No credentials or server data are removed. */
      }
    },
  };
}
