(() => {
  'use strict';
  // Replaced by the server with operator-configured pins; never taken from a query string.
  const pins = __TWINBID_BOOTSTRAP_SETTINGS__;
  const tag = document.currentScript;
  const code = tag?.dataset.adCodeId;
  if (!code || code.length > 80 || !tag?.src || !crypto.randomUUID) return;
  const sdk = window.__twinbidSDKV1 ||= { pending: new Map(), routesByTag: new WeakMap(), notices: new Set(), configurations: new Map() };
  if (window.__twinbidPopV1?.codes.has(code) || sdk.pending.has(code)) return;
  const configurations = sdk.configurations ||= new Map();
  const cacheKey = pins.stable_origin + ':' + pins.fingerprint;
  if (!configurations.has(cacheKey)) configurations.set(cacheKey, { manifest: null, reading: null, readAt: 0 });
  const configuration = configurations.get(cacheKey);
  const message = tag.dataset.antiblockMessage || '';
  const notify = (component, reason) => {
    try { window.dispatchEvent(new CustomEvent('twinbid:availability', { detail: { component, reason } })); } catch {}
    if (message && !sdk.notices.has(code)) {
      sdk.notices.add(code);
      const note = document.createElement('span'); note.setAttribute('role', 'status'); note.textContent = message.slice(0, 240);
      const append = () => { const parent = tag.parentNode && tag.parentNode !== document.head ? tag.parentNode : document.body; parent?.appendChild(note); };
      if (document.body) append(); else document.addEventListener('DOMContentLoaded', append, { once: true });
    }
  };
  const publicOrigin = raw => {
    const u = new URL(raw);
    if (u.protocol !== 'https:' || u.username || u.password || u.port || u.pathname !== '/' || u.search || u.hash) throw new Error('origin');
    return u.origin;
  };
  const validate = next => {
    if (next?.enabled === false) {
      if (next.legacy_runtime !== pins.legacy_runtime) throw new Error('untrusted disable target');
      return Object.freeze({ ...next, fingerprint: pins.fingerprint, paths: pins.paths, legacy_runtime: pins.legacy_runtime, origins: Object.freeze([...(pins.origins || [])]), delivery_origins: Object.freeze([...(pins.origins || [])]) });
    }
    if (!next || next.enabled !== pins.enabled || next.fingerprint !== pins.fingerprint || next.legacy_runtime !== pins.legacy_runtime || next.stable_origin !== pins.stable_origin || next.runtime_integrity !== pins.runtime_integrity || JSON.stringify(next.paths) !== JSON.stringify(pins.paths)) throw new Error('configuration generation');
    if (!next.enabled) { if (next.legacy_runtime !== pins.legacy_runtime) throw new Error('legacy runtime'); return next; }
    const trusted = new Set(pins.origins.map(publicOrigin));
    for (const field of ['runtime_origins', 'api_origins', 'delivery_origins']) {
      if (!Array.isArray(next[field]) || !next[field].length || next[field].length > trusted.size) throw new Error('empty routes');
      const seen = new Set();
      for (const origin of next[field]) { if (publicOrigin(origin) !== origin || !trusted.has(origin) || seen.has(origin)) throw new Error('untrusted origin'); seen.add(origin); }
    }
    // Keep the pins separate from the rotating subsets: cached delivery tokens may use an older subset.
    return Object.freeze({ ...next, paths: Object.freeze({ ...pins.paths }), origins: Object.freeze([...(pins.origins || [])]), runtime_origins: Object.freeze([...next.runtime_origins]), api_origins: Object.freeze([...next.api_origins]), delivery_origins: Object.freeze([...next.delivery_origins]), message });
  };
  const readInfrastructure = async force => {
    const now = Date.now();
    if (!force && configuration.manifest && now - configuration.readAt < 60000) return configuration.manifest;
    if (configuration.reading) return configuration.reading;
    configuration.reading = (async () => {
      const res = await fetch(pins.stable_origin + pins.paths.infrastructure, { credentials: 'omit', redirect: 'error', cache: 'no-store', signal: AbortSignal.timeout(8000) });
      if (!res.ok) throw new Error('infrastructure unavailable');
      const next = validate(await res.json()); configuration.manifest = next; configuration.readAt = Date.now(); return next;
    })().finally(() => { configuration.reading = null; });
    return configuration.reading;
  };
  const reported = new Set();
  const report = (manifest, component, reason) => {
    const key = component + ":" + reason; if (reported.has(key)) return; reported.add(key);
    try { void fetch(pins.stable_origin + pins.paths.diagnostics, { method: 'POST', credentials: 'omit', redirect: 'error', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ component, reason }), signal: AbortSignal.timeout(2000) }).catch(() => {}); } catch {}
  };
  const load = (url, manifest) => new Promise((resolve, reject) => {
    const child = document.createElement('script'); child.async = true; child.src = url;
    child.dataset.adCodeId = code;
    if (tag.nonce) child.nonce = tag.nonce;
    child.referrerPolicy = 'no-referrer'; child.crossOrigin = 'anonymous';
    if (manifest?.enabled && pins.runtime_integrity) child.integrity = pins.runtime_integrity;
    if (manifest?.enabled) sdk.routesByTag.set(child, { manifest, refresh: readInfrastructure, notify, report });
    let timer;
    const finish = error => { clearTimeout(timer); child.onload = child.onerror = null; if (error) { child.remove(); reject(error); } else resolve(); };
    child.onload = () => finish(window.__twinbidPopV1?.codes.has(code) ? null : new Error('runtime did not register'));
    child.onerror = () => finish(new Error('runtime network error'));
    timer = setTimeout(() => finish(new Error('runtime timeout')), 8000);
    (document.head || document.documentElement).appendChild(child);
  });
  sdk.pending.set(code, (async () => {
    try {
      // Disabled flag: no new transport, endpoint selection or domain rotation.
      if (!pins.enabled) { await load(pins.legacy_runtime, null); return; }
      report(null, 'bootstrap', 'loaded');
      const manifest = await readInfrastructure(false);
      if (!manifest.enabled) { await load(pins.legacy_runtime, null); return; }
      for (const origin of manifest.runtime_origins) {
        if (window.__twinbidPopV1?.codes.has(code)) return;
        try { await load(origin + manifest.paths.runtime, manifest); report(manifest, 'runtime', 'loaded'); return; }
        catch { report(manifest, 'runtime', 'network'); }
      }
      notify('runtime', 'unavailable');
    } catch { if (pins.enabled) report(null, 'infrastructure', 'unavailable'); notify('infrastructure', 'unavailable'); }
    finally { sdk.pending.delete(code); }
  })());
})();
