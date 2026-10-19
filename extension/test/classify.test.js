import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { Classifier, parseUserRules, CATEGORIES, MAX_RULES_BYTES } from '../src/classify/classify.js';
import bundledJs from '../src/classify/rules.js';
import { saveUserRules, loadClassifier, loadUserRulesText, clearUserRules } from '../src/classify/storage.js';

const bundled = JSON.parse(readFileSync(new URL('../src/classify/rules.json', import.meta.url), 'utf8'));
const vectors = JSON.parse(readFileSync(new URL('vectors.json', import.meta.url), 'utf8'));

test('generated rules.js matches rules.json', () => {
  assert.deepEqual(bundledJs, bundled);
});

test('six categories', () => {
  assert.deepEqual(CATEGORIES, ['adult', 'finance', 'gambling', 'health', 'news_politics', 'privacy']);
});

test('bundled vectors match the Go classifier 100%', () => {
  const c = new Classifier(bundled);
  const bad = vectors.bundled.filter((v) => JSON.stringify(c.classify(v.url)) !== JSON.stringify(v.want));
  assert.equal(bad.length, 0, JSON.stringify(bad.slice(0, 5).map((v) => [v.url, v.want, c.classify(v.url)])));
  const seen = new Set(vectors.bundled.flatMap((v) => v.want));
  for (const cat of CATEGORIES) assert.ok(seen.has(cat), `vectors cover ${cat}`);
  assert.ok(vectors.bundled.some((v) => v.want.length > 1), 'multi-category vector');
  assert.ok(vectors.bundled.length > 1000);
});

test('user-rule vectors match the Go classifier 100%', () => {
  assert.ok(vectors.user.length >= 4);
  for (const set of vectors.user) {
    const c = new Classifier(bundled, parseUserRules(JSON.stringify(set.rules), bundled));
    for (const v of set.cases) {
      assert.deepEqual(c.classify(v.url), v.want, `${set.name}: ${v.url}`);
    }
  }
});

test('user rules are invalid when malformed', () => {
  const bad = {
    'not json': '{', empty: '', array: '[]', 'unknown field': '{"adds":{}}',
    'unknown category': '{"add":{"weather":{"domains":["x.example"]}}}',
    'unknown cat allow': '{"allow":{"Gambling":{"domains":["x.example"]}}}',
    'bad domain url': '{"add":{"adult":{"domains":["https://x.example/"]}}}',
    'bad domain space': '{"add":{"adult":{"domains":["x y.example"]}}}',
    'no dot': '{"add":{"adult":{"domains":["localhostish"]}}}',
    'empty domain': '{"add":{"adult":{"domains":[""]}}}',
    'short keyword': '{"add":{"adult":{"keywords":["ab"]}}}',
    'empty keyword': '{"add":{"adult":{"keywords":[" "]}}}',
    'slash keyword': '{"add":{"adult":{"keywords":["a/b/c"]}}}',
    'wrong type': '{"add":{"adult":{"domains":"x.example"}}}',
    'non-string item': '{"add":{"adult":{"domains":[1]}}}',
    'unknown key': '{"add":{"adult":{"hosts":["x.example"]}}}',
    'trailing data': '{} {}',
  };
  for (const [name, js] of Object.entries(bad)) {
    assert.throws(() => parseUserRules(js, bundled), Error, name);
  }
  assert.throws(() => parseUserRules('{"adds":{}}', bundled), /unknown field "adds"/);
  assert.throws(() => parseUserRules('{', bundled), /invalid JSON/);
});

test('null is accepted like the Go loader; error messages stay short', () => {
  assert.deepEqual(parseUserRules('null', bundled), { add: {}, allow: {} });
  assert.deepEqual(parseUserRules('{"add":{"adult":null}}', bundled), { add: { adult: null }, allow: {} });
  const long = 'x'.repeat(5000);
  for (const js of [`{"add":{"adult":{"keywords":["${long}/"]}}}`, `{"add":{"adult":{"domains":["${long}"]}}}`, `{"${long}":1}`]) {
    assert.throws(() => parseUserRules(js, bundled), (e) => e.message.length < 200);
  }
});

test('valid user rules and the 1 MiB limit', () => {
  assert.deepEqual(parseUserRules('{}', bundled), { add: {}, allow: {} });
  const big = `{"add":{"adult":{"keywords":["${'a'.repeat(MAX_RULES_BYTES)}"]}}}`;
  assert.throws(() => parseUserRules(big, bundled), /1 MiB/);
});

test('classifier is a pure function: no network, no logging', () => {
  const origFetch = globalThis.fetch;
  const origLog = console.log;
  globalThis.fetch = () => { throw new Error('network'); };
  console.log = () => { throw new Error('log'); };
  try {
    new Classifier(bundled).classify('https://www.bet365.com/');
  } finally {
    globalThis.fetch = origFetch;
    console.log = origLog;
  }
  for (const f of ['classify.js', 'index.js', 'storage.js']) {
    const src = readFileSync(new URL(`../src/classify/${f}`, import.meta.url), 'utf8');
    assert.ok(!/\b(fetch|XMLHttpRequest|WebSocket|console|eval|importScripts|RegExp)\b|new Function|\bimport\s*\(/.test(src), f);
  }
});

test('performance: 100k URLs', () => {
  const c = new Classifier(bundled);
  const urls = [];
  for (let i = 0; i < 100000; i++) {
    urls.push(i % 5 === 0 ? `https://sub${i}.bet365.com/x/${i}` : `https://site${i}.example.com/path/${i}?q=${i}`);
  }
  const t0 = performance.now();
  let hits = 0;
  for (const u of urls) if (c.classify(u).length) hits++;
  const ms = performance.now() - t0;
  assert.equal(hits, 20000);
  assert.ok(ms < 1500, `100k in ${ms.toFixed(0)}ms`);
});

function fakeArea() {
  const m = new Map();
  return {
    async get(k) { return m.has(k) ? { [k]: m.get(k) } : {}; },
    async set(o) { for (const [k, v] of Object.entries(o)) m.set(k, v); },
    async remove(k) { m.delete(k); },
  };
}

test('storage round trip, rejects invalid rules without saving', async () => {
  const area = fakeArea();
  assert.equal(await loadUserRulesText(area), '');
  assert.deepEqual((await loadClassifier(area)).classify('https://lucky.example/'), []);
  await assert.rejects(saveUserRules(area, '{"bad":1}'), /unknown field/);
  assert.equal(await loadUserRulesText(area), '');
  await saveUserRules(area, '{"add":{"gambling":{"domains":["lucky.example"]}}}');
  assert.deepEqual((await loadClassifier(area)).classify('https://www.lucky.example/'), ['gambling']);
  await clearUserRules(area);
  assert.deepEqual((await loadClassifier(area)).classify('https://lucky.example/'), []);
});

test('index.js interface contract', async () => {
  const m = await import('../src/classify/index.js');
  assert.deepEqual(m.CATEGORIES.map((c) => c.id), ['adult', 'gambling', 'health', 'finance', 'privacy', 'news_politics']);
  assert.equal(m.CATEGORIES[4].label, 'Privacy tools');
  assert.equal(m.CATEGORIES[5].label, 'News & politics');
  const b = await m.loadBundledRules();
  const r = m.parseUserRules('{"add":{"gambling":{"domains":["lucky.example"]}}}');
  assert.ok(r.rules && !r.error);
  const c = m.createClassifier({ bundledRules: b, userRules: r.rules });
  assert.deepEqual(c.classify('https://lucky.example/'), ['gambling']);
  assert.deepEqual(c.classify('::not a url'), []);
  assert.deepEqual(m.createClassifier({ bundledRules: b, userRules: null }).classify('https://www.bet365.com/'), ['gambling']);
  const e = m.parseUserRules('{"add":{"x":1}}');
  assert.equal(typeof e.error, 'string');
  assert.ok(!e.rules);
  assert.ok(m.parseUserRules('x'.repeat(2 << 20)).error.includes('1 MiB'));
});
