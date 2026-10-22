import test from 'node:test';
import assert from 'node:assert/strict';

// Minimal DOM stand-in (no jsdom): enough for el()/fillRow().
class Node_ {
  constructor(tag) { this.tag = tag; this.children = []; this.attrs = {}; this.dataset = {}; this.listeners = {}; this.style = {}; this._text = ''; this.className = '';
    const cls = new Set(); this.classList = { add: (c) => cls.add(c), toggle: (c, on) => (on ? cls.add(c) : cls.delete(c)), has: (c) => cls.has(c) }; }
  set textContent(v) { this._text = String(v); } get textContent() { return this._text; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  append(...n) { this.children.push(...n); } appendChild(n) { this.children.push(n); }
  replaceChildren(...n) { this.children = n; }
  addEventListener(t, f) { this.listeners[t] = f; }
  walk(f) { f(this); for (const c of this.children) c.walk(f); }
  find(pred) { let r = null; this.walk((n) => { if (!r && pred(n)) r = n; }); return r; }
  querySelector(sel) { const c = sel.slice(1); return this.find((n) => n !== this && n.className === c); }
  focus() { Node_.focused = this; }
  dump() { const out = []; this.walk((n) => { out.push(n._text, n.href, n.title, n.dateTime, JSON.stringify(n.attrs), JSON.stringify(n.dataset), JSON.stringify(n.style)); }); return out.filter(Boolean).join('\n'); }
}
globalThis.document = { createElement: (t) => new Node_(t), createTextNode: (t) => { const n = new Node_('#text'); n._text = t; return n; } };
const { fillRow } = await import('../../shared/ui-common.js');

const SECRET_URL = 'https://casino.example.test/slots?token=SEKRIT';
const SECRET_TITLE = 'Secret slots page';
const secrets = ['casino.example.test', 'SEKRIT', SECRET_TITLE, SECRET_URL];
const cb = (c) => { const n = document.createElement('span'); n.textContent = c; return n; };

function setup(hide) {
  const revealed = new Set();
  const v = { id: 'v1', categories: ['gambling'] };
  const makeView = () => ({ id: 'v1', timeMs: 1750000000000, iso: new Date(1750000000000).toISOString(), url: SECRET_URL, title: SECRET_TITLE,
    domain: 'casino.example.test', transition: 'link', categories: v.categories, durationMs: 0,
    masked: hide && v.categories.length > 0 && !revealed.has('v1'), hide,
    onToggleReveal: () => { if (revealed.has('v1')) revealed.delete('v1'); else revealed.add('v1'); } });
  const row = document.createElement('div');
  fillRow(row, makeView, cb);
  return { row, makeView };
}

test('masked row contains neither real title nor URL anywhere', () => {
  const { row } = setup(true);
  const d = row.dump();
  for (const s of secrets) assert.ok(!d.includes(s), `leaked ${s}`);
  assert.ok(row.classList.has('masked'));
  assert.equal(row.find((n) => n.tag === 'a'), null);
});

test('reveal shows details, hide masks again', () => {
  const { row } = setup(true);
  row.querySelector('.reveal').listeners.click();
  const d = row.dump();
  assert.ok(d.includes(SECRET_TITLE) && d.includes(SECRET_URL));
  assert.ok(!row.classList.has('masked'));
  assert.equal(Node_.focused, row.querySelector('.reveal'));
  row.querySelector('.reveal').listeners.click();
  for (const s of secrets) assert.ok(!row.dump().includes(s), `leaked ${s}`);
});

test('without hide mode the row is unmasked and has no reveal button', () => {
  const { row } = setup(false);
  assert.ok(row.dump().includes(SECRET_TITLE));
  assert.equal(row.querySelector('.reveal'), null);
});

test('non-http URLs are not linked', () => {
  const row = document.createElement('div');
  fillRow(row, () => ({ id: 'x', timeMs: 0, iso: '1970-01-01T00:00:00.000Z', url: 'javascript:alert(1)', title: 't', domain: '', transition: 'link', categories: [], durationMs: 0, masked: false, hide: false, onToggleReveal() {} }), cb);
  assert.equal(row.find((n) => n.tag === 'a'), null);
});
