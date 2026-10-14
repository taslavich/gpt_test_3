'use strict';
async function main() {
 const { Engine, Request, RequestType, setConfiguration } = await import('@adguard/tsurlfilter');
 const chunks = []; let bytes = 0;
 for await (const chunk of process.stdin) { bytes += chunk.length; if (bytes > 170 * 1024 * 1024) throw new Error('input limit'); chunks.push(chunk); }
 const { lists, requests } = JSON.parse(Buffer.concat(chunks).toString('utf8'));
 if (!Array.isArray(lists) || !lists.length || lists.length > 8 || !lists.every(s => typeof s === 'string') || !Array.isArray(requests) || requests.length > 20000) throw new Error('invalid input');
 setConfiguration({ engine: 'extension', version: '1.0.0', verbose: false });
 // FilterList conversion inside the factory normalizes ABP/uBO lists to AdGuard syntax.
 const engine = Engine.createSync({ filters: lists.map((content, id) => ({ id, content, ignoreCosmetic: true, ignoreJS: true })) });
 if (!engine.getRulesCount()) throw new Error('no valid network rules');
 const types = { other: RequestType.Other, script: RequestType.Script, xmlhttprequest: RequestType.XmlHttpRequest, main_frame: RequestType.Document, popup: RequestType.Document };
 const result = requests.map(({ url, source_url, type, method, origin, component }) => {
   if (!types[type]) throw new Error('unsupported resource type');
   const result = engine.matchRequest(new Request(url, source_url, types[type], method || (component.endsWith('_preflight') ? 'OPTIONS' : component === 'delivery_probe' ? 'HEAD' : ['request', 'confirm', 'diagnostics'].includes(component) ? 'POST' : 'GET')));
   let rule = result.getBasicResult();
   const popupRule = result.getPopupRule();
   // Popup policy is applied only to an actual new tab, never to scripts/XHR.
   if (type !== 'popup' && rule === popupRule) rule = null;
   if (type === 'popup') rule = result.getDocumentBlockingResult() || rule || popupRule;
   return { origin, component, blocked: !!rule && !rule.isAllowlist() };
 });
 process.stdout.write(JSON.stringify(result));
}
main().catch(e => { console.error(String(e)); process.exitCode = 1; });
