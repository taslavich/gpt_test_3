// Deterministic JS behavior tests without a browser. Real browser coverage is separate.
const vm = require('node:vm'), fs = require('node:fs'), assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const source = fs.readFileSync(require('node:path').join(__dirname,'../internal/services/pub/pageHelper.js'),'utf8');
async function main(attributes, dataset, shouldRun, separateHost=false) {
 const requests=[], clicks=[], events={}, storage=new Map();
 let trigger=2, opens=0;
 const api='https://pub.example.test', asset=separateHost?'https://script.example.test':api, fetchURLs=[];
 const script=separateHost?source.replace('/* publisher-api-origin */ tag.src',JSON.stringify(api)):source;
 const location={href:'https://site.example.test/'};
 const window={location,top:{location},open(){opens++;return {closed:false,opener:null,location:{replace(){}},close(){this.closed=true}}}};
 const sandbox={window,location,URL,crypto:{randomUUID},AbortSignal,console,
  document:{currentScript:{dataset,getAttribute(name){return attributes[name]??null},src:asset+'/static/pageHelper.js'},addEventListener(name,fn){if(name==='click')clicks.push(fn)}},
  localStorage:{getItem:key=>storage.get(key)||null,setItem:(key,val)=>storage.set(key,val)},
  history:{pushState(a,b,url){location.href=new URL(url,location.href).href},replaceState(a,b,url){location.href=new URL(url,location.href).href}},
  addEventListener(name,fn){events[name]=fn},setInterval(){return 1},clearInterval(){},setTimeout,
  async fetch(url,opt){fetchURLs.push(new URL(url));if(url.includes('/config'))return {ok:true,json:async()=>({active:true,trigger_click:trigger,session_token:'token'})};requests.push(JSON.parse(opt.body));return {ok:true,json:async()=>({status:'no_fill',error:false})}}
 };
 vm.createContext(sandbox);vm.runInContext(script,sandbox);vm.runInContext(script,sandbox);
 await new Promise(setImmediate);
 if(!shouldRun){assert.equal(clicks.length,0);assert.equal(requests.length,0);assert.equal(window.__twinbidPopV1,undefined);return}
 assert.equal(clicks.length,1,'duplicate CODE listeners');
 const click=()=>clicks[0]({isTrusted:true,button:0});
 await click();assert.equal(requests.length,0);await click();assert.equal(requests.length,1);assert.equal(requests[0].page_click,2);assert.equal(requests[0].code,'tb_test');
 const state=window.__twinbidPopV1.codes.get('tb_test'),session=state.session;
 sandbox.history.pushState({},'', '/next');await new Promise(setImmediate);
 assert.notEqual(state.session,session);await click();assert.equal(requests.length,1);await click();assert.equal(requests.length,2);
 // busy clicks are page clicks, independent of confirmation/server result.
 state.busy=true;const count=state.clicks;await click();assert.equal(state.clicks,count+1);assert.equal(requests.length,2);state.busy=false;
 window.open=()=>null;await click();assert.equal(requests.length,2,'blocked opening started an auction');
 const before=opens;Object.defineProperty(window,'top',{get(){throw new Error('cross origin')}});
 await click();assert.equal(requests.length,3,'missing page was silently lost');assert.equal(requests[2].page_url,null);assert.equal(opens,before);
 assert.equal(state.busy,false);
 assert.ok(fetchURLs.some(url=>url.pathname==='/config'));
 assert.ok(fetchURLs.some(url=>url.pathname==='/request'));
 assert.ok(fetchURLs.every(url=>url.origin===api));
 const beforeUntrusted=state.clicks;await clicks[0]({isTrusted:false,button:0});assert.equal(state.clicks,beforeUntrusted);
 console.log('PASS: second page click, duplicate CODE, SPA session, busy click counting, blocked open, missing/null page request, untrusted click');
}
main({food:'tb_test'}, {}, true)
 .then(()=>main({food:'tb_test'}, {}, true, true))
 .then(()=>main({}, {adCodeId:'tb_test'}, false))
 .then(()=>main({}, {food:'tb_test'}, false)).catch(e=>{console.error(e);process.exitCode=1});
