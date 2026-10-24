'use strict';
const { spawnSync } = require('node:child_process'), assert = require('node:assert/strict'), path = require('node:path');
function match(rules, url, type = 'script', source = 'https://publisher.test/', method = 'GET') {
 const input = { lists: ['[Adblock Plus 2.0]\n' + rules], requests: [{ url, source_url: source, type, method, origin: new URL(url).origin, component: 'test' }] };
 const p = spawnSync(process.execPath, [path.join(__dirname, 'filter.cjs')], { input: JSON.stringify(input), encoding: 'utf8' });
 assert.equal(p.status, 0, p.stderr); return JSON.parse(p.stdout)[0].blocked;
}
assert.equal(match('||work.test^', 'https://work.test/a.js'), true);
assert.equal(match('||work.test^', 'https://work.test.attacker.test/a.js'), false);
assert.equal(match('||work.test/assets/*$script,third-party', 'https://work.test/assets/runtime.js'), true);
assert.equal(match('||work.test/assets/*$script,third-party', 'https://work.test/assets/runtime.js', 'xmlhttprequest'), false);
assert.equal(match('||work.test^$third-party', 'https://work.test/a.js', 'script', 'https://work.test/page'), false);
assert.equal(match('||work.test^$script,domain=publisher.test', 'https://work.test/a.js'), true);
assert.equal(match('||work.test^$script,domain=other.test', 'https://work.test/a.js'), false);
assert.equal(match('||work.test^\n@@||work.test/a.js$script', 'https://work.test/a.js'), false);
assert.equal(match('||work.test^$important\n@@||work.test/a.js$script', 'https://work.test/a.js'), true);
assert.equal(match('||work.test^$script\n||work.test^$script,badfilter', 'https://work.test/a.js'), false);
assert.equal(match('||work.test^$popup', 'https://work.test/delivery/tok', 'popup'), true);
assert.equal(match('||work.test^$popup', 'https://work.test/a.js', 'script'), false);
assert.equal(match('||work.test^$popup\n@@||work.test^$popup', 'https://work.test/delivery/tok', 'popup'), false);
assert.equal(match('||work.test/request$xmlhttprequest,method=POST', 'https://work.test/request', 'xmlhttprequest', 'https://publisher.test/', 'POST'), true);
assert.equal(match('/https:\\/\\/work\\.test\\/assets\\/.*\\.js/$script', 'https://work.test/assets/runtime.js'), true);
assert.equal(match('||work.test/request$method=OPTIONS', 'https://work.test/request', 'other', 'https://publisher.test/', 'OPTIONS'), true);
console.log('PASS: actual filter engine domain boundaries, paths, script/XHR/document/popup, first/third party, source domain, exceptions/important/badfilter, POST method, regex');
