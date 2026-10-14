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
function rig({source=production, origins=[a,b], enabled=true, trigger=1, runtimeBlocked=[], apiBlocked=[], lostResponse=false, result='fill', openURL=b+paths.delivery+token, headBlocked=[], popupBlocked=false, invalidManifest=false, message='', lateRuntime=false}={}) {
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
   if(u.pathname===paths.config || u.pathname==='/config') {if(apiBlocked.includes(u.origin))throw Error('API blocked');return {ok:true,json:async()=>({active:true,trigger_click:trigger,session_token:'session:'+u.searchParams.get('code')})}}
   if(opt.method==='HEAD'){if(headBlocked.includes(u.origin))throw Error('delivery blocked');return {ok:true,status:200}}
   if(opt.method==='POST') {
    const payload=JSON.parse(opt.body);requests.push({origin:u.origin,payload});
    if(!ids.has(payload.request_id)){ids.add(payload.request_id);auctions++}
    if(lostResponse && requests.length===1)throw Error('response lost AFTER processing');
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
 return {boot,window,sandbox,clicks,calls,children,notices,requests,navigations,timers,execute,async click(){for(const fn of clicks)await fn({isTrusted:true,button:0})},get auctions(){return auctions},get opens(){return opens},get closed(){return closed},setManifest(next){manifest=next;for(const cfg of window.__twinbidSDKV1.configurations.values())cfg.readAt=0},manifest};
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
(async()=>{await suite(readable);await suite(production);console.log('PASS source+production: legacy/off, one/multiple domains, runtime/API failure, uncertain response retry same ID/one auction, trusted cross-domain bridge, malicious URLs, no-fill/refusal/blocked popup, duplicate/late scripts, first/second click, SPA, multiple codes, configuration refresh, optional UX. Node VM only.');})().catch(e=>{console.error(e);process.exitCode=1});
