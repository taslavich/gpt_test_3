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
    const refresh = async () => {
      try { const next = await context.refresh(false); if (next.fingerprint === manifest.fingerprint) { manifest = next; if (manifest.enabled) cursor %= manifest.api_origins.length; } }
      catch { report('infrastructure', 'network'); } // Last validated route snapshot remains usable.
    };
    const config = async (code, visitor, session) => {
      await refresh();
      if (!manifest.enabled) { const base = new URL(manifest.legacy_runtime).origin; const res = await fetch(base + `/config?code=${encodeURIComponent(code)}&visitor_id=${visitor}&page_session_id=${session}`, { credentials: 'omit', redirect: 'error' }); return res.ok ? await res.json() : null; }
      for (let n = 0; n < Math.min(3, manifest.api_origins.length); n++) {
        const origin = manifest.api_origins[cursor];
        try {
          const response = await fetch(origin + manifest.paths.config + `?code=${encodeURIComponent(code)}&visitor_id=${visitor}&page_session_id=${session}`, { credentials: 'omit', redirect: 'error', signal: AbortSignal.timeout(6000) });
          if (!response.ok) { report('config', 'business_refusal'); return null; }
          return await response.json();
        } catch { report('config', 'network'); cursor = (cursor + 1) % manifest.api_origins.length; }
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
      if (open.origin === new URL(manifest.legacy_runtime).origin && open.pathname.startsWith('/delivery/')) return open;
      // HEAD only verifies reachability. It NEVER runs /confirm or requests another auction.
      const candidates = [...manifest.delivery_origins];
      if (candidates.includes(open.origin)) candidates.splice(candidates.indexOf(open.origin), 1), candidates.unshift(open.origin);
      for (const origin of candidates.slice(0, 3)) {
        const target = new URL(open.pathname, origin);
        try {
          const response = await fetch(target.href, { method: 'HEAD', credentials: 'omit', redirect: 'error', signal: AbortSignal.timeout(3000) });
          if (response.ok) return target;
          if (response.status === 410 || response.status === 409) { report('delivery', 'business_refusal'); return null; }
        } catch {}
      }
      report('delivery', 'network'); return null;
    };
    return { config, request, delivery, bridge, report };
  };
})();
