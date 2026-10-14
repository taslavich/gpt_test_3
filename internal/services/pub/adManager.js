(() => {
  'use strict';
  const tag = document.currentScript;
  const code = tag?.dataset.adCodeId;
  if (!code || !tag.src || !crypto.randomUUID) return;
  const base = new URL(tag.src).origin;
  const root = window.__twinbidPopV1 ||= { codes: new Map(), session: crypto.randomUUID() };
  if (root.codes.has(code)) return; // Same Code on one page shares one runtime/quota.
  const state = { clicks: 0, busy: false, config: null, session: root.session };
  root.codes.set(code, state);
  let visitor;
  try {
    visitor = localStorage.getItem('twinbid_pop_visitor_v1');
    if (!/^[0-9a-f-]{36}$/i.test(visitor || '')) {
      visitor = crypto.randomUUID(); localStorage.setItem('twinbid_pop_visitor_v1', visitor);
    }
  } catch { visitor = root.visitor ||= crypto.randomUUID(); }
  async function config() {
    try { const session=state.session;const r = await fetch(`${base}/config?code=${encodeURIComponent(code)}&visitor_id=${visitor}&page_session_id=${session}`, { credentials: 'omit' });
      const next=r.ok?await r.json():null;
      if(state.session!==session)return;
      if(next&&state.config?.session_token&&state.configSession===session) next.session_token=state.config.session_token;
      state.config=next;state.configSession=session;
    } catch { state.config = null; }
  }
  state.reload=config;void config(); const timer = setInterval(config, 30000);
  function page() {
    try { return window.top.location.href; } catch { return null; }
  }
  if (!root.navigationHook) {
    root.navigationHook = true;
    const reset = () => { root.session = crypto.randomUUID(); for (const s of root.codes.values()) {
      s.session = root.session; s.clicks = 0; s.config = null; if(s.reload)void s.reload();
    }};
    for (const method of ['pushState', 'replaceState']) {
      const original = history[method]; history[method] = function (...args) {
        const before = location.href; const value = original.apply(this, args);
        if (location.href !== before) reset(); return value;
      };
    }
    addEventListener('popstate', reset);
    addEventListener('pageshow', e => { if (e.persisted) reset(); });
  }
  document.addEventListener('click', async event => {
    if (!event.isTrusted || event.button !== 0) return;
    state.clicks++; // Page clicks are counted before auction/confirmation, even while busy.
    if (state.busy || !state.config?.active) return;
    if (state.clicks < state.config.trigger_click) return;
    const pageURL = page();
    const session = state.session;
    const provisional = pageURL ? window.open('about:blank', '_blank') : null;
    if (pageURL && !provisional) return; // Blocked open never consumes server confirmed quota.
    try { provisional.opener = null; } catch { /* browsers vary */ }
    state.busy = true;
    const payload = { request_id: crypto.randomUUID(), code, page_url: pageURL,
      visitor_id: visitor, page_session_id: session, session_token:state.config.session_token, page_click: state.clicks };
    try {
      // Reuse ONE id on network retries; no_fill/business refusal is not retried.
      for (let attempt = 0; attempt < 3; attempt++) {
        if (state.session !== session || (provisional && provisional.closed)) break;
        let response;
        try { response = await fetch(`${base}/request`, { method: 'POST', credentials: 'omit',
          headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
          signal: AbortSignal.timeout(6000) });
        } catch (e) {
          if (attempt < 2) { await new Promise(resolve => setTimeout(resolve, 5000)); continue; }
          break;
        }
        const result = await response.json();
        if (!response.ok || result.status !== 'fill' || state.session !== session || !provisional || provisional.closed) break;
        const open = new URL(result.open_url);
        if (open.origin !== base || !open.pathname.startsWith('/delivery/')) break;
        provisional.location.replace(open.href); return;
      }
      try { provisional.close(); } catch { /* no false event */ }
    } catch { try { provisional.close(); } catch {} }
    finally { state.busy = false; }
  }, { capture: true });
  addEventListener('pagehide', () => clearInterval(timer), { once: true });
})();
