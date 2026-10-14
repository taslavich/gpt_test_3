'use strict';
// VM tests are deterministic behavior tests, not browser/extension certification.
const vm = require('node:vm'), fs = require('node:fs'), path = require('node:path'), assert = require('node:assert/strict'), { randomUUID } = require('node:crypto');
const root = path.resolve(__dirname, '../internal/services/pub');
const readable = ['routing.js','adManager.js'].map(n => fs.readFileSync(path.join(root,n),'utf8')).join('\n');
const production = fs.readFileSync(path.join(root,'runtime.prod.js'),'utf8');
const sdkProduction = fs.readFileSync(path.join(root,'sdk.prod.js'),'utf8');
const legacy = fs.readFileSync(path.join(root,'adManager.js'),'utf8');
const paths = { sdk:'/sdk.js',infrastructure:'/infra.json',runtime:'/assets/runtime.js',config:'/x/settings',request:'/x/query',delivery:'/x/open/',confirm:'/x/done/',diagnostics:'/infra/event',health:'/infra/health' };
const a='https://work-a.test',b='https://work-b.test',stable='https://stable.test';
const token = randomUUID()+'.'+'a'.repeat(64);
const flush = async () => { for(let i=0;i<15;i++) await new Promise(setImmediate); };
function rig({source=production, origins=[a,b], enabled=true, trigger=1, runtimeBlocked=[], apiBlocked=[], configStatus={}, lostResponse=false, result='fill', openURL=b+paths.delivery+token, headBlocked=[], headStatus={}, popupBlocked=false, invalidManifest=false, message='', lateRuntime=false}={}) {
 const pins={enabled,fingerprint:enabled?'fixture-generation':'',stable_origin:stable,origins,paths,legacy_runtime:'https://legacy.test/static/adManager.js'};
 let manifest={...pins,runtime_origins:origins,api_origins:origins,delivery_origins:origins};
 const clicks=[],calls=[],children=[],notices=[],requests=[],navigations=[],events={},timers=[],ids=new Set();let auctions=0,opens=0,closed=0;
 const location={href:'https://publisher.test/page'};
 const window={location,top:{location},dispatchEvent(e){events[e.type]=e},open(){opens++;if(popupBlocked)return null;return {closed:false,opener:null,location:{replace(url){navigations.push(url)}},close(){this.closed=true;closed++}}}};
 const doc={currentScript:null,createElement(type){const item={type,dataset:{},setAttribute(){},remove(){this.removed=true}};return item},addEventListener(name,fn){if(name==='click')clicks.push(fn)}};
 const storage=new Map();
 const sandbox={window,document:doc,location,URL,crypto:{randomUUID},AbortSignal,Date,console,CustomEvent:class{constructor(type,props){this.type=type;Object.assign(this,props)}},
  localStorage:{getItem:k=>storage.get(k)||null,setItem:(k,v)=>storage.set(k,v)},history:{pushState(_a,_b,url){location.href=new URL(url,location.href).href},replaceState(_a,_b,url){location.href=new URL(url,location.href).href}},
  addEventListener(name,fn){events[name]=fn},setInterval(){return 1},clearInterval(){},setTimeout(fn,ms){if(ms===8000){timers.push(fn);return timers.length}setImmediate(fn);return 0},clearTimeout(){},
  async fetch(raw,opt={}) {
   const u=new URL(raw);calls.push({url:u.href,opt});assert.equal(opt.credentials,'omit');
   if(u.pathname===paths.infrastructure){return {ok:true,json:async()=>invalidManifest?{...manifest,api_origins:['https://attacker.test']}:manifest}}
   if(u.pathname===paths.diagnostics)return {ok:true,status:204};
   if(u.pathname===paths.config || u.pathname==='/config') {if(apiBlocked.includes(u.origin))throw Error('API blocked');return {ok:!configStatus[u.origin],status:configStatus[u.origin]||200,json:async()=>({active:true,trigger_click:trigger,session_token:'session:'+u.searchParams.get('code')})}}
   if(opt.method==='HEAD'){if(headBlocked.includes(u.origin))throw Error('delivery blocked');const status=headStatus[u.origin]||200;return {ok:status===200,status}}
   if(opt.method==='POST') {
    const payload=JSON.parse(opt.body);requests.push({origin:u.origin,payload});
    if(!ids.has(payload.request_id)){ids.add(payload.request_id);auctions++}
    if(lostResponse && (lostResponse==='always'||requests.length===1))throw Error('response lost AFTER processing');
    return {ok:result!=='refusal',status:result==='refusal'?403:200,json:async()=>({status:result,open_url:openURL})};
   }
   throw Error('unexpected URL '+raw);
  }
 };
 vm.createContext(sandbox);
 function execute(child,text){const before=doc.currentScript;doc.currentScript=child;vm.runInContext(text,sandbox);doc.currentScript=before}
 const append=child=>{if(child.type==='span'){notices.push(child);return};children.push(child);setImmediate(()=>{
  if(runtimeBlocked.includes(new URL(child.src).origin)){child.onerror?.();return}
  if(lateRuntime && children.length===1)return;
  execute(child,child.src.includes('/static/adManager.js')?legacy:source);child.onload?.();
 })};doc.head={appendChild:append};doc.body={appendChild(child){assert.equal(child.type,'span');notices.push(child)}};doc.documentElement=doc.head;
 function boot(code='tb_test',old=false) {const tag={src:old?'https://legacy.test/static/adManager.js':stable+paths.sdk,dataset:{adCodeId:code,antiblockMessage:message},parentNode:doc.head};execute(tag,old?legacy:sdkProduction.replace('__TWINBID_BOOTSTRAP_SETTINGS__',JSON.stringify(pins)))}
 return {boot,window,sandbox,clicks,calls,children,notices,requests,navigations,timers,execute,async click(){for(const fn of clicks)await fn({isTrusted:true,button:0})},get auctions(){return auctions},get opens(){return opens},get closed(){return closed},setManifest(next,invalidate=true){manifest=next;if(invalidate)for(const cfg of window.__twinbidSDKV1.configurations.values())cfg.readAt=0},manifest};
}
async function routingSuite(source) {
 const pool=Array.from({length:15},(_,i)=>`https://pool-${i+1}.test`);
 const callsAt=(r,p)=>r.calls.filter(c=>new URL(c.url).pathname===p);
 for(const size of [5,15]) {
  const origins=pool.slice(0,size);
  let r=rig({source,origins,apiBlocked:origins.slice(0,3),openURL:origins[0]+paths.delivery+token});r.boot();r.boot();await flush();
  assert.deepEqual(callsAt(r,paths.config).map(c=>new URL(c.url).origin),origins.slice(0,4),'config did not reach fourth domain');
  await r.click();assert.equal(r.requests[0].origin,origins[3]);assert.equal(r.auctions,1);assert.equal(r.opens,1);
  await r.window.__twinbidPopV1.codes.get('tb_test').reload();
  assert.equal(new URL(callsAt(r,paths.config).at(-1).url).origin,origins[3],'working config cursor was lost');
  for(const lostResponse of [true,'always']) {
   r=rig({source,origins,apiBlocked:origins.slice(0,3),lostResponse,openURL:origins[0]+paths.delivery+token});r.boot();await flush();await r.click();assert.equal(r.requests.length,lostResponse===true?2:3,'request retries changed with pool size');assert.ok(r.requests.every(q=>JSON.stringify(q.payload)===JSON.stringify(r.requests[0].payload)));assert.equal(r.auctions,1);assert.equal(r.opens,1);assert.equal(r.navigations.length,lostResponse===true?1:0);
  }
  r=rig({source,origins,apiBlocked:origins.slice(0,-1),openURL:origins[0]+paths.delivery+token});r.boot();await flush();
  assert.deepEqual(callsAt(r,paths.config).map(c=>new URL(c.url).origin),origins,'config skipped/repeated a domain');await r.click();assert.equal(r.requests[0].origin,origins.at(-1));
  r=rig({source,origins,apiBlocked:origins});r.boot();await flush();assert.deepEqual(callsAt(r,paths.config).map(c=>new URL(c.url).origin),origins);await r.click();assert.equal(r.auctions,0);assert.equal(r.opens,0);
  r=rig({source,origins,apiBlocked:[origins[0]],configStatus:{[origins[1]]:403}});r.boot();await flush();assert.equal(callsAt(r,paths.config).length,2,'business config refusal retried');await r.click();assert.equal(r.auctions,0);
  r=rig({source,origins,openURL:origins[0]+paths.delivery+token,headBlocked:origins.slice(0,3)});r.boot();await flush();await r.click();
  assert.deepEqual(r.calls.filter(c=>c.opt.method==='HEAD').map(c=>new URL(c.url).origin),origins.slice(0,4));assert.equal(r.navigations[0],origins[3]+paths.delivery+token);
  assert.equal(r.auctions,1);assert.equal(r.opens,1);assert.equal(r.requests.length,1);assert.equal(r.navigations.length,1);assert.equal(r.calls.some(c=>c.url.includes(paths.confirm)),false,'HEAD confirmed a delivery');
  r=rig({source,origins,openURL:origins[0]+paths.delivery+token,headBlocked:origins});r.boot();await flush();await r.click();
  assert.deepEqual(r.calls.filter(c=>c.opt.method==='HEAD').map(c=>new URL(c.url).origin),origins);assert.equal(r.navigations[0],origins[0]+paths.delivery+token,'blocked HEAD cancelled permitted navigation');assert.equal(r.opens,1);assert.equal(r.auctions,1);assert.equal(r.calls.some(c=>c.url.includes(paths.confirm)),false);
  for(const result of ['no_fill','refusal','frequency','session_cap']) {
   r=rig({source,origins,result});r.boot();await flush();await r.click();assert.equal(r.requests.length,1);assert.equal(r.navigations.length,0);assert.equal(r.calls.filter(c=>c.opt.method==='HEAD').length,0);assert.equal(r.closed,1);
  }
  r=rig({source,origins,popupBlocked:true});r.boot();await flush();await r.click();assert.equal(r.requests.length,0);
  for(const openURL of ['https://attacker.test'+paths.delivery+token,origins[0]+paths.delivery+token+'?redirect=evil',origins[0]+paths.delivery+token+'/extra']) {
   r=rig({source,origins,openURL});r.boot();await flush();await r.click();assert.equal(r.navigations.length,0);assert.equal(r.calls.filter(c=>c.opt.method==='HEAD').length,0);assert.equal(r.closed,1);
  }
  for(const status of [409,410]) {
   r=rig({source,origins,openURL:origins[0]+paths.delivery+token,headBlocked:origins.slice(0,3),headStatus:{[origins[3]]:status}});r.boot();await flush();await r.click();
   assert.equal(r.calls.filter(c=>c.opt.method==='HEAD').length,4);assert.equal(r.navigations.length,0,'delivery business refusal retried/fell back');assert.equal(r.auctions,1);assert.equal(r.closed,1);
  }
  // A cached RTB response may name a domain removed from the current available subset.
  r=rig({source,origins,openURL:origins[0]+paths.delivery+token});r.boot();await flush();r.setManifest({...r.manifest,delivery_origins:[origins[3],origins[4]]},false);await r.click();
  assert.equal(r.navigations[0],origins[3]+paths.delivery+token);assert.equal(r.calls.filter(c=>c.opt.method==='HEAD').length,1);
  for(const badToken of ['not-a-token',token.slice(0,-1),token.replace(/\.[a-f0-9]+$/,'.'+'g'.repeat(64)),token.replace(/^[^.]+/,'00000000')]) {
   r=rig({source,origins,openURL:origins[0]+paths.delivery+badToken});r.boot();await flush();await r.click();assert.equal(r.navigations.length,0);assert.equal(r.calls.filter(c=>c.opt.method==='HEAD').length,0);
  }
  // Well-formed, altered HMAC is refused by /delivery; cryptographic verification stays server-side.
  r=rig({source,origins,openURL:origins[0]+paths.delivery+token.slice(0,-1)+'b',headStatus:{[origins[0]]:410}});r.boot();await flush();await r.click();assert.equal(r.navigations.length,0);assert.equal(r.calls.filter(c=>c.opt.method==='HEAD').length,1);assert.equal(r.calls.some(c=>c.url.includes(paths.confirm)),false);
 }
 // Exercise real elapsed deadlines, a wrapped cursor, refresh failure and a stalled JSON body.
 const configCalls=[],requestCalls=[],headCalls=[],clock=Date.now;let mode='working';
 const manifest={enabled:true,fingerprint:'deadline-fixture',origins:pool,api_origins:pool,delivery_origins:pool,paths,legacy_runtime:'https://legacy.test/static/adManager.js'};
 const sandbox={window:{},document:{currentScript:null},URL,AbortSignal,Date,console,async fetch(raw,opt){
  const u=new URL(raw);assert.ok(pool.includes(u.origin));assert.equal(opt.credentials,'omit');assert.equal(opt.redirect,'error');
  if(opt.method==='HEAD'){headCalls.push(u.href);return new Promise(()=>{})}
  if(opt.method==='POST'){requestCalls.push(u.origin);return {ok:true}}
  configCalls.push(u.origin);
  if(mode==='working'){if(u.origin!==pool[3])throw Error('blocked');return {ok:true,json:async()=>({active:true})}}
  if(mode==='wrap'){if(u.origin!==pool[0])throw Error('blocked');return {ok:true,json:async()=>({active:true})}}
  if(mode==='body')return {ok:true,json:()=>new Promise(()=>{})};
  return new Promise(()=>{}); // Deliberately ignores signal: the operation deadline still bounds it.
 }};
 vm.createContext(sandbox);vm.runInContext(source,sandbox);
 const route=sandbox.window.__twinbidCreateRouteV1({manifest,report(){},notify(){},refresh:async()=>manifest});
 await route.config('tb_test','visitor','session');await route.request({request_id:'same-id'});assert.deepEqual(requestCalls,[pool[3]]);
 mode='wrap';configCalls.length=0;await route.config('tb_test','visitor','session');assert.deepEqual(configCalls,[...pool.slice(3),...pool.slice(0,1)],'cursor wrap skipped/repeated domains');
 const guard=setInterval(()=>{},1000);
 try {
  for(mode of ['hung','body']) {
   configCalls.length=0;const start=clock();assert.equal(await route.config('tb_test','visitor','session'),null);assert.ok(clock()-start<7000,'15 config domains exceeded shared deadline');assert.equal(new Set(configCalls).size,configCalls.length);assert.equal(configCalls.length,15);assert.equal(requestCalls.length,1,'config failure auctioned');
  }
  const start=clock(),open=route.delivery(pool[0]+paths.delivery+token);const bridge=await route.bridge(open);assert.ok(clock()-start<3000,'15 HEAD checks exceeded shared deadline');assert.equal(bridge.href,open.href);assert.equal(headCalls.length,15);assert.equal(new Set(headCalls).size,15);assert.equal(requestCalls.length,1);
  // A stuck infrastructure refresh must not extend config's overall six-second budget.
  const stuck=sandbox.window.__twinbidCreateRouteV1({manifest,report(){},notify(){},refresh:()=>new Promise(()=>{})});mode='working';const refreshStart=clock();assert.ok(await stuck.config('tb_test','visitor','session'));assert.ok(clock()-refreshStart<2000);
 } finally {clearInterval(guard)}
 console.log('PASS routing source/production: 5/15 domains, full pool/cursor/no repeats, shared deadlines including refresh/body, business refusals, advisory HEAD/current subset/same token, malformed/altered token, one popup/auction, zero HEAD confirmations. Node VM only.');
}
async function suite(source) {
 let r=rig({source,origins:[a],openURL:a+paths.delivery+token});r.boot();r.boot();await flush();assert.equal(r.clicks.length,1);await r.click();assert.equal(r.auctions,1);assert.equal(r.navigations.length,1);assert.equal(r.requests[0].payload.session_token,'session:tb_test');
 r=rig({source,trigger:2});r.boot();r.boot();await flush();const session=r.window.__twinbidPopV1.session;await r.click();assert.equal(r.auctions,0);await r.click();assert.equal(r.auctions,1);assert.ok(r.navigations[0].startsWith(b));
 r.sandbox.history.pushState({},'', '/spa');await flush();assert.notEqual(r.window.__twinbidPopV1.session,session);await r.click();assert.equal(r.auctions,1);await r.click();assert.equal(r.auctions,2);
 r=rig({source,runtimeBlocked:[a],apiBlocked:[a]});r.boot();await flush();assert.equal(r.children.length,2);assert.equal(r.clicks.length,1);await r.click();assert.equal(r.requests[0].origin,b);
 r=rig({source,lostResponse:true});r.boot();await flush();await r.click();assert.equal(r.requests.length,2);assert.equal(r.requests[0].origin,a);assert.equal(r.requests[1].origin,b);assert.deepEqual(r.requests[0].payload,r.requests[1].payload);assert.equal(r.auctions,1);assert.equal(r.navigations.length,1);
 r=rig({source,headBlocked:[b]});r.boot();await flush();await r.click();assert.ok(r.navigations[0].startsWith(a));assert.equal(r.auctions,1);assert.equal(r.calls.filter(c=>c.url.includes(paths.confirm)).length,0,'runtime itself confirmed');
 for(const result of ['no_fill','refusal']){r=rig({source,result});r.boot();await flush();await r.click();assert.equal(r.requests.length,1);assert.equal(r.navigations.length,0);assert.equal(r.closed,1)}
 r=rig({source,popupBlocked:true});r.boot();await flush();await r.click();assert.equal(r.requests.length,0);
 for(const openURL of ['https://attacker.test'+paths.delivery+token,'http://work-b.test'+paths.delivery+token,'https://work-b.test:8443'+paths.delivery+token,'https://u@work-b.test'+paths.delivery+token,b+paths.delivery+token+'?url=evil',b+'/adm?url=evil',b+paths.delivery+token+'/extra']) {
  r=rig({source,openURL});r.boot();await flush();await r.click();assert.equal(r.navigations.length,0);assert.equal(r.calls.filter(c=>c.opt.method==='HEAD').length,0);assert.equal(r.closed,1);
 }
 r=rig({source,invalidManifest:true,message:'Реклама недоступна.'});r.boot();await flush();assert.equal(r.children.length,0);assert.equal(r.clicks.length,0);assert.equal(r.notices.length,1);assert.equal(r.notices[0].textContent,'Реклама недоступна.');
 r=rig({source,runtimeBlocked:[a,b]});r.boot();await flush();assert.equal(r.clicks.length,0);assert.equal(r.requests.length,0);
 r=rig({source,enabled:false,result:'no_fill'});r.boot();await flush();await r.click();assert.equal(r.children[0].src,'https://legacy.test/static/adManager.js');assert.equal(r.requests[0].origin,'https://legacy.test');assert.equal(r.calls.some(c=>c.url.includes(paths.infrastructure)),false);
 r=rig({source});r.boot('tb_test',true);r.boot();await flush();assert.equal(r.clicks.length,1);assert.equal(r.children.length,0);await r.click();assert.equal(r.requests.length,1);
 r=rig({source});r.boot();await flush();r.boot('tb_test',true);assert.equal(r.clicks.length,1);
 r=rig({source,lateRuntime:true});r.boot();await flush();r.timers[0]();await flush();assert.equal(r.clicks.length,1);r.execute(r.children[0],source);assert.equal(r.clicks.length,1,'late runtime duplicated listener');
 r=rig({source});r.boot('tb_a');r.boot('tb_b');await flush();assert.equal(r.clicks.length,2);const state=r.window.__twinbidPopV1;assert.equal(state.codes.get('tb_a').session,state.codes.get('tb_b').session);await r.click();assert.deepEqual(r.requests.map(q=>q.payload.code),['tb_a','tb_b']);
 r=rig({source,openURL:'https://legacy.test/delivery/'+token});r.boot();await flush();await r.click();assert.ok(r.navigations[0].startsWith('https://legacy.test/delivery/'));
 r=rig({source,result:'no_fill'});r.boot();await flush();r.setManifest({enabled:false,legacy_runtime:'https://legacy.test/static/adManager.js'});await r.window.__twinbidPopV1.codes.get('tb_test').reload();await r.click();assert.equal(r.requests[0].origin,'https://legacy.test');assert.equal(r.requests[0].payload.session_token,'session:tb_test');
 r=rig({source});r.boot();await flush();r.setManifest({enabled:false,legacy_runtime:'https://legacy.test/static/adManager.js'});await r.window.__twinbidPopV1.codes.get('tb_test').reload();await r.click();assert.equal(r.requests[0].origin,'https://legacy.test');assert.ok(r.navigations[0].startsWith(b+paths.delivery),'disabled in-flight cached bridge rejected');
 r=rig({source});r.boot();await flush();r.setManifest({...r.manifest,api_origins:[b,a]});await r.window.__twinbidPopV1.codes.get('tb_test').reload();await r.click();assert.equal(r.requests[0].origin,b);
}
(async()=>{await routingSuite(readable);await routingSuite(production);await suite(readable);await suite(production);console.log('PASS source+production: legacy/off, one/multiple domains, runtime/API failure, uncertain response retry same ID/one auction, trusted cross-domain bridge, malicious URLs, no-fill/refusal/blocked popup, duplicate/late scripts, first/second click, SPA, multiple codes, configuration refresh, optional UX. Node VM only.');})().catch(e=>{console.error(e);process.exitCode=1});
