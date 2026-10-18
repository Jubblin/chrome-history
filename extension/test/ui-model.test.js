import test from 'node:test';
import assert from 'node:assert/strict';
import { applyFilters, toCsv, csvCell, layout, chartBins, domainStats, annotate, isMasked, MASK } from '../ui/model.js';
import { createClassifier, parseUserRules, loadBundledRules } from '../ui/test-support/stub-classifier.js';

const T0 = new Date(2025, 5, 10, 12, 0).getTime();
const mk = (id, min, url, title, transition = 'link') => ({ visitId: String(id), time: T0 + min * 60000, url, title,
  domain: new URL(url).hostname, transition, referringVisitId: '', isLocal: true });
async function data() {
  const clf = createClassifier({ bundledRules: await loadBundledRules(), userRules: null });
  return annotate([
    mk(1, 0, 'https://casino.example.test/slots', 'Secret slots'),
    mk(2, 5, 'https://a.example.test/x', 'Secret garden'),
    mk(3, 100, 'https://a.example.test/y', '=cmd|calc', 'typed'),
    mk(4, 24 * 60, 'https://b.example.test/z', 'Next day'),
  ], clf);
}

test('masked search does not match flagged values', async () => {
  const vs = await data();
  const f = { text: 'secret', domain: '', cats: new Set() };
  assert.deepEqual(applyFilters(vs, f, false).map((v) => v.visitId), ['1', '2']);
  assert.deepEqual(applyFilters(vs, f, true).map((v) => v.visitId), ['2']);
  assert.deepEqual(applyFilters(vs, { text: '', domain: 'casino.example.test', cats: new Set() }, true), []);
  assert.equal(applyFilters(vs, { text: '', domain: 'casino.example.test', cats: new Set() }, false).length, 1);
});

test('category and transition filters', async () => {
  const vs = await data();
  assert.deepEqual(applyFilters(vs, { cats: new Set(['gambling']) }, false).map((v) => v.visitId), ['1']);
  assert.deepEqual(applyFilters(vs, { transitions: ['typed'] }, false).map((v) => v.visitId), ['3']);
});

test('csv masks flagged rows, guards formulas, adds categories', async () => {
  const vs = await data();
  const lines = toCsv(vs, true).trim().split('\r\n');
  assert.equal(lines[0], 'time,url,title,domain,transition,categories');
  assert.ok(lines[1].includes(`${MASK},${MASK},${MASK}`) && lines[1].endsWith('link,gambling'));
  assert.ok(!toCsv(vs, true).includes('Secret slots'));
  assert.ok(toCsv(vs, false).includes('Secret slots'));
  assert.ok(lines[3].includes("'=cmd|calc"));
  assert.equal(csvCell('+1'), "'+1");
  assert.equal(csvCell('a,"b"'), '"a,""b"""');
});

test('layout groups by local day and 30 minute sessions, counts flags', async () => {
  const vs = (await data()).sort((a, b) => b.time - a.time);
  const l = layout(vs);
  assert.equal(l.days.length, 2);
  assert.deepEqual(l.days.map((d) => [d.visits, d.flagged]), [[1, 0], [3, 1]]);
  assert.deepEqual(l.sessions.map((s) => s.visits), [1, 1, 2]);
});

test('chart bins, domain stats and masking helpers', async () => {
  const vs = await data();
  const { unit, bins } = chartBins(vs);
  assert.equal(unit, 'day');
  assert.deepEqual(bins.map((b) => [b.visits, b.flagged]), [[3, 1], [1, 0]]);
  const d = domainStats(vs);
  assert.equal(d[0].domain, 'a.example.test');
  assert.deepEqual(d.find((x) => x.domain === 'casino.example.test').categories, ['gambling']);
  assert.equal(isMasked(vs[0], true, new Set()), true);
  assert.equal(isMasked(vs[0], true, new Set(['1'])), false);
  assert.equal(isMasked(vs[1], true, new Set()), false);
});

test('week and month bins for long ranges', () => {
  const vs = [];
  for (let i = 0; i < 150; i++) vs.push({ visitId: String(i), time: new Date(2025, 0, 1 + i, 12).getTime(), categories: [] });
  const w = chartBins(vs);
  assert.equal(w.unit, 'week');
  assert.ok(w.bins.every((b) => new Date(b.key + 'T12:00').getDay() === 1));
  assert.equal(w.bins.reduce((n, b) => n + b.visits, 0), 150);
});

test('stub parseUserRules sanity', () => {
  assert.ok(parseUserRules('{"add":{"finance":{"domains":["x.test"]}}}').rules);
  assert.ok(parseUserRules('{').error);
  assert.ok(!parseUserRules('{"add":{"nope":{}}}').rules);
});

test('date range is half-open at the next local midnight', async () => {
  const vs = await data();
  const day2 = '2025-06-11';
  const mid = new Date(2025, 5, 11).getTime();
  const edge = [{ ...vs[0], visitId: 'e1', time: mid - 1 }, { ...vs[0], visitId: 'e2', time: mid }];
  assert.deepEqual(applyFilters(edge, { start: '2025-06-10', end: '2025-06-10' }, false).map((v) => v.visitId), ['e1']);
  assert.equal(applyFilters(edge, { start: day2, end: day2 }, false).map((v) => v.visitId)[0], 'e2');
});
