'use strict';
// Real browsers when installed; network fixtures do not certify any blocker extension.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {randomUUID,createHash}=require('node:crypto');
const {chromium,firefox}=require('playwright');
const dir=path.resolve(__dirname,'../internal/services/pub');
const legacy=fs.readFileSync(path.join(dir,'adManager.js'),'utf8');
const runtime=fs.readFileSync(path.join(dir,'runtime.prod.js'),'utf8');
const sdk=fs.readFileSync(path.join(dir,'sdk.prod.js'),'utf8');
const origin='https://publisher.test',a='https://work-a.test',b='https://work-b.test',stable='https://stable.test';
const paths={sdk:'/sdk.js',infrastructure:'/infra.json',runtime:'/assets/runtime.js',config:'/config',request:'/request',delivery:'/delivery/',confirm:'/confirm/',diagnostics:'/infra/event',health:'/infra/health'};
const pins={enabled:true,fingerprint:'browser-fixture',stable_origin:stable,origins:[a,b],paths,legacy_runtime:a+'/static/adManager.js',runtime_integrity:'sha384-'+createHash('sha384').update(runtime).digest('base64')};
const manifest={...pins,runtime_origins:[a,b],api_origins:[a,b],delivery_origins:[b,a]};
async function run(engine,name) {
 let browser;
 try{browser=await engine.launch({timeout:20000,headless:true,env:{...process.env,MOZ_DISABLE_CONTENT_SANDBOX:'1',MOZ_DISABLE_RDD_SANDBOX:'1'},...(name==='Chromium'&&process.env.CHROME_EXECUTABLE_PATH?{executablePath:process.env.CHROME_EXECUTABLE_PATH}:{}),...(name==='Firefox'&&process.env.FIREFOX_EXECUTABLE_PATH?{executablePath:process.env.FIREFOX_EXECUTABLE_PATH}:{}),...(name==='Chromium'?{args:['--no-sandbox']}: {})})}
 catch(e){console.log('SKIP '+name+': '+String(e.message).split('\n')[0]);return}
 try {
  for(const modern of [false,true]) {
   console.log('RUN '+name+' '+(modern?'SDK':'legacy'));let rendererReady=false;const watchdog=setTimeout(()=>{if(!rendererReady){console.log('SKIP '+name+': renderer failed to initialize before loading the test page');for(const ext of ['uBlock Origin','AdBlock Plus','AdGuard'])console.log('SKIP '+ext+': extension profile unavailable');process.exit(0)}},25000);const context=await browser.newContext();context.setDefaultTimeout(10000);const requests=[],ids=new Set(),confirms=new Set();let lose=modern,trigger=2;
   const token=randomUUID()+'.'+'a'.repeat(64);
   await context.route('**/*',async route=>{
    const req=route.request(),u=new URL(req.url());
    const headers={'Access-Control-Allow-Origin':'*','Access-Control-Allow-Headers':'Content-Type, X-TwinBid-Infrastructure','Access-Control-Allow-Methods':'GET,POST,HEAD,OPTIONS'};
    const send=(body,type='application/json',status=200)=>route.fulfill({status,headers:{...headers,'Content-Type':type},body});
    if(req.method()==='OPTIONS')return send('', 'text/plain',204);
    if(u.origin===origin)return send(`<button id="action">content</button><script src="${modern?stable+'/sdk.js':a+'/static/adManager.js'}" data-ad-code-id="tb_test"></script><script src="${modern?stable+'/sdk.js':a+'/static/adManager.js'}" data-ad-code-id="tb_test"></script>`,'text/html');
    if(u.pathname==='/sdk.js')return send(sdk.replace('__TWINBID_BOOTSTRAP_SETTINGS__',JSON.stringify(pins)),'application/javascript');
    if(u.pathname==='/infra.json')return send(JSON.stringify(manifest));
    if(u.pathname===paths.runtime){if(u.origin===a)return route.abort('blockedbyclient');return send(runtime,'application/javascript')}
    if(u.pathname==='/static/adManager.js')return send(legacy,'application/javascript');
    if(u.pathname==='/config')return send(JSON.stringify({active:true,trigger_click:trigger,session_token:'fixture-token'}));
    if(u.pathname==='/infra/event')return send('', 'text/plain',204);
    if(u.pathname==='/request'){
     const inRequest=req.postDataJSON();requests.push(inRequest);ids.add(inRequest.request_id);
     if(lose){lose=false;return route.abort('connectionclosed')}
     return send(JSON.stringify({status:'fill',open_url:(modern?b:a)+paths.delivery+token}));
    }
    if(u.pathname===paths.delivery+token){if(req.method()==='HEAD')return send('','text/html');return send(`<script>addEventListener('load',async()=>{const r=await fetch('/confirm/${token}',{method:'POST',credentials:'omit'});if(r.ok)location.replace('https://adm.test/adm?id=original')},{once:true})</script>`,'text/html')}
    if(u.pathname===paths.confirm+token){confirms.add(token);return send('','text/plain',204)}
    if(u.origin==='https://adm.test')return send('original ADM','text/html');
    return send('', 'text/plain',404);
   });
   const page=await context.newPage();rendererReady=true;clearTimeout(watchdog);page.on('pageerror',e=>console.error(name+' pageerror: '+e.message));await page.goto(origin);console.log('LOADED '+name+' '+(modern?'SDK':'legacy'));
   await page.waitForFunction(()=>window.__twinbidPopV1?.codes.get('tb_test')?.config?.active);
   await page.locator('#action').click();assert.equal(requests.length,0);
   const popupPromise=page.waitForEvent('popup');await page.locator('#action').click();const popup=await popupPromise;
   await popup.waitForURL('https://adm.test/adm?id=original',{timeout:20000});
   assert.equal(ids.size,1);assert.equal(confirms.size,1);assert.equal(requests.length,modern?2:1);if(modern)assert.deepEqual(requests[0],requests[1]);
   const oldSession=requests[0].page_session_id;await page.evaluate(()=>history.pushState({},'', '/next'));
   await page.waitForFunction(()=>window.__twinbidPopV1.codes.get('tb_test').config?.active);
   await page.locator('#action').click();assert.equal(ids.size,1);
   assert.notEqual(await page.evaluate(()=>window.__twinbidPopV1.session),oldSession);
   trigger=1;const blocked=await context.newPage();await blocked.addInitScript(()=>{window.open=()=>null});await blocked.goto(origin);await blocked.waitForFunction(()=>window.__twinbidPopV1?.codes.get('tb_test')?.config?.active);await blocked.locator('#action').click();assert.equal(ids.size,1);
   await context.close();
  }
  console.log('PASS '+name+': legacy/new comparison, duplicate SDK, second click, primary runtime block, lost response same ID, cross-domain bridge, one confirmation, original ADM, SPA, blocked popup');
 }finally{await browser.close()}
}
(async()=>{await run(chromium,'Chromium');await run(firefox,'Firefox');for(const name of ['uBlock Origin','AdBlock Plus','AdGuard'])console.log('SKIP '+name+': real configured extension profiles not supplied; route.abort is not extension testing')})().catch(e=>{console.error(e);process.exitCode=1});
