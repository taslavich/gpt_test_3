(() => {
  'use strict';
  window.__twinbidCreateRouteV1 ||= context => {
    let manifest = context.manifest, cursor = 0;
    const reported = new Set();
    const report = (component, reason) => {
      const key = component + ':' + reason;
      if (reported.has(key)) return; reported.add(key);
      context.report(manifest, component, reason);
      if (reason !== 'loaded' && reason !== 'business_refusal') context.notify(component, reason);
    };
    // Bound the whole operation, including reading a response body. HEAD remains advisory.
    const within = async (milliseconds, work) => {
      const signal = AbortSignal.timeout(Math.max(1, milliseconds));
      let abort;
      const timeout = new Promise((_, reject) => {
        abort = () => reject(signal.reason); signal.addEventListener('abort', abort, { once: true });
      });
      try { return await Promise.race([work(signal), timeout]); }
      finally { signal.removeEventListener('abort', abort); }
    };
    const refresh = async (milliseconds, force = false) => {
      try { const next = await within(milliseconds, () => context.refresh(force)); if (next.fingerprint === manifest.fingerprint) { manifest = next; if (manifest.enabled) cursor %= manifest.api_origins.length; } }
      catch { report('infrastructure', 'network'); } // Last validated route snapshot remains usable.
    };
    const config = async (code, visitor, session) => {
      const deadline = Date.now() + 6000;
      await refresh(1000);
      if (!manifest.enabled) { const base = new URL(manifest.legacy_runtime).origin; const res = await fetch(base + `/config?code=${encodeURIComponent(code)}&visitor_id=${visitor}&page_session_id=${session}`, { credentials: 'omit', redirect: 'error' }); return res.ok ? await res.json() : null; }
      const origins = [...manifest.api_origins], start = cursor;
      for (let n = 0; n < origins.length; n++) {
        const remaining = deadline - Date.now();
        if (remaining <= 0) break;
        const origin = origins[(start + n) % origins.length];
        try {
          return await within(Math.floor(remaining / (origins.length - n)), async signal => {
            const response = await fetch(origin + manifest.paths.config + `?code=${encodeURIComponent(code)}&visitor_id=${visitor}&page_session_id=${session}`, { credentials: 'omit', redirect: 'error', signal });
            if (!response.ok) { report('config', 'business_refusal'); return null; }
            const settings = await response.json();
            if (!signal.aborted) cursor = Math.max(0, manifest.api_origins.indexOf(origin));
            return settings;
          });
        } catch { report('config', 'network'); cursor = (manifest.api_origins.indexOf(origin) + 1) % manifest.api_origins.length; }
      }
      return null;
    };
    const request = async payload => {
      if (!manifest.enabled) return fetch(new URL(manifest.legacy_runtime).origin + '/request', { method: 'POST', credentials: 'omit', redirect: 'error', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload), signal: AbortSignal.timeout(6000) });
      const origin = manifest.api_origins[cursor];
      try {
        return await fetch(origin + manifest.paths.request, { method: 'POST', credentials: 'omit', redirect: 'error', headers: { 'Content-Type': 'application/json', 'X-TwinBid-Infrastructure': manifest.fingerprint }, body: JSON.stringify(payload), signal: AbortSignal.timeout(6000) });
      } catch (error) { report('request', 'network'); cursor = (cursor + 1) % manifest.api_origins.length; throw error; }
    };
    const delivery = raw => {
      const open = new URL(raw);
      const legacy = open.origin === new URL(manifest.legacy_runtime).origin && open.pathname.startsWith('/delivery/');
      const prefix = legacy ? '/delivery/' : manifest.paths.delivery;
      const token = open.pathname.slice(prefix.length);
      if (open.protocol !== 'https:' || open.username || open.password || open.port || open.search || open.hash || (!legacy && !manifest.origins.includes(open.origin)) || !open.pathname.startsWith(prefix) || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.[0-9a-f]{64}$/.test(token)) {
        report('delivery', 'untrusted_delivery'); return null;
      }
      return open;
    };
    const bridge = async open => {
      open = delivery(open.href);
      if (!open) return null;
      if (open.origin === new URL(manifest.legacy_runtime).origin && open.pathname.startsWith('/delivery/')) return open;
      // HEAD only verifies reachability. It NEVER runs /confirm or requests another auction.
      const deadline = Date.now() + 2000;
      await refresh(500, true);
      const candidates = [...manifest.delivery_origins];
      if (!candidates.length) return null;
      if (candidates.includes(open.origin)) candidates.splice(candidates.indexOf(open.origin), 1), candidates.unshift(open.origin);
      for (let n = 0; n < candidates.length; n++) {
        const remaining = deadline - Date.now();
        if (remaining <= 0) break;
        const origin = candidates[n];
        const target = new URL(open.pathname, origin);
        try {
          const response = await within(Math.min(500, Math.floor(remaining / (candidates.length - n))), signal => fetch(target.href, { method: 'HEAD', credentials: 'omit', redirect: 'error', signal }));
          if (response.ok) return target;
          if (response.status === 410 || response.status === 409) { report('delivery', 'business_refusal'); return null; }
        } catch {}
      }
      // Fetch may be blocked while document navigation is allowed. Navigate ONE bridge;
      // its server still verifies the same token and owns confirmation/ADM. No cross-origin retry.
      report('delivery', 'network'); return new URL(open.pathname, candidates[0]);
    };
    return { config, request, delivery, bridge, report };
  };
})();
