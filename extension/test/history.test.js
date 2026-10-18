import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  loadVisits, createMockHistory, visitsPerDay, topDomains, groupSessions, filterVisits, dayKey, normalizeTransition, domainOf,
} from '../src/history/index.js';
import { makeFixtureEntries } from '../scripts/make-fixture.js';

const T0 = Date.UTC(2025, 0, 10, 12);
const MIN = 60_000;
const e = (url, times, extra = {}) => ({ url, title: extra.title ?? 't', visits: times.map((t) => ({ visitTime: t, ...extra.v })) });

test('loads visits with decoded fields', async () => {
  const h = createMockHistory([
    { url: 'https://A.example.test:8080/x', title: 'X', visits: [{ visitTime: T0, transition: 'typed', isLocal: false }, { visitTime: T0 - MIN, referringVisitId: 7 }] },
  ]);
  const vs = await loadVisits(h, { startTime: 0, endTime: T0 + 1 });
  assert.equal(vs.length, 2);
  assert.deepEqual(vs[0], { visitId: vs[0].visitId, time: T0, url: 'https://A.example.test:8080/x', title: 'X', domain: 'a.example.test', transition: 'typed', referringVisitId: '', isLocal: false });
  assert.equal(vs[1].referringVisitId, '7');
  assert.equal(vs[1].isLocal, true);
  assert.equal(vs[1].transition, 'link');
});

test('unknown transitions fall back to link; bad urls give empty domain', () => {
  assert.equal(normalizeTransition('weird'), 'link');
  assert.equal(domainOf('not a url'), '');
});

test('empty titles are preserved as empty strings', async () => {
  const h = createMockHistory([{ url: 'https://a.test/', visits: [{ visitTime: T0 }] }]);
  const [v] = await loadVisits(h, { endTime: T0 + 1 });
  assert.equal(v.title, '');
});

test('empty history', async () => {
  assert.deepEqual(await loadVisits(createMockHistory([]), { endTime: T0 }), []);
});

test('paging by time returns every visit across many pages', async () => {
  const entries = makeFixtureEntries({ visits: 3000, seed: 3 });
  const total = entries.reduce((n, x) => n + x.visits.length, 0);
  const h = createMockHistory(entries);
  const vs = await loadVisits(h, { startTime: 0, endTime: Date.UTC(2026, 0, 1), pageSize: 50, concurrency: 4 });
  assert.equal(vs.length, total);
  assert.equal(new Set(vs.map((v) => v.visitId)).size, total);
  assert.ok(h.calls.search > 5);
  assert.equal(h.calls.getVisits, entries.length, 'each URL fetched once');
  for (let i = 1; i < vs.length; i++) assert.ok(vs[i - 1].time >= vs[i].time);
});

test('many URLs sharing one timestamp do not stall paging', async () => {
  const entries = Array.from({ length: 250 }, (_, i) => e(`https://s.test/${i}`, [T0]));
  const vs = await loadVisits(createMockHistory(entries), { endTime: T0 + 1, pageSize: 10 });
  assert.equal(vs.length, 250);
});

test('duplicate URL visits are all kept, outside-window visits dropped', async () => {
  const h = createMockHistory([e('https://d.test/', [T0, T0 - 5 * MIN, T0 - 10 * MIN, T0 - 99 * MIN])]);
  const vs = await loadVisits(h, { startTime: T0 - 20 * MIN, endTime: T0 + 1 });
  assert.equal(vs.length, 3);
});

test('window boundaries: start inclusive, end inclusive of endTime', async () => {
  const h = createMockHistory([e('https://b.test/', [T0 - MIN, T0, T0 + MIN])]);
  const vs = await loadVisits(h, { startTime: T0 - MIN, endTime: T0 });
  assert.deepEqual(vs.map((v) => v.time), [T0, T0 - MIN]);
});

test('huge counts (12k visits) load and report progress', async () => {
  const entries = makeFixtureEntries({ visits: 12000 });
  const total = entries.reduce((n, x) => n + x.visits.length, 0);
  const progress = [];
  let batched = 0;
  const vs = await loadVisits(createMockHistory(entries), {
    startTime: 0, endTime: Date.UTC(2026, 0, 1), pageSize: 1000,
    onProgress: (p) => progress.push(p), onBatch: (b) => { batched += b.length; },
  });
  assert.ok(total > 10000);
  assert.equal(vs.length, total);
  assert.equal(batched, total);
  assert.equal(progress.at(-1).done, true);
  assert.equal(progress.at(-1).visits, total);
  assert.ok(progress.length > 3);
});

test('cancellation rejects with AbortError carrying partial visits', async () => {
  const entries = makeFixtureEntries({ visits: 5000, seed: 9 });
  const ac = new AbortController();
  const h = createMockHistory(entries);
  let pages = 0;
  const p = loadVisits(h, { startTime: 0, endTime: Date.UTC(2026, 0, 1), pageSize: 100, signal: ac.signal, onBatch: () => { if (++pages === 2) ac.abort(); } });
  await assert.rejects(p, (err) => err.name === 'AbortError' && Array.isArray(err.visits) && err.visits.length > 0);
  assert.ok(h.calls.search < 20);
});

test('already-aborted signal does no work', async () => {
  const h = createMockHistory([e('https://a.test/', [T0])]);
  const ac = new AbortController();
  ac.abort();
  await assert.rejects(loadVisits(h, { endTime: T0 + 1, signal: ac.signal }), { name: 'AbortError' });
  assert.equal(h.calls.search, 0);
});

test('concurrency is bounded', async () => {
  const h = createMockHistory(Array.from({ length: 30 }, (_, i) => e(`https://c.test/${i}`, [T0 - i])));
  let cur = 0, max = 0;
  const orig = h.getVisits.bind(h);
  h.getVisits = async (a) => { cur++; max = Math.max(max, cur); await new Promise((r) => setTimeout(r, 2)); const r = await orig(a); cur--; return r; };
  await loadVisits(h, { endTime: T0 + 1, concurrency: 3 });
  assert.ok(max <= 3 && max > 1, `max=${max}`);
});

test('day boundaries use local time zone', () => {
  const d = (h, m) => new Date(2025, 2, 9, h, m).getTime();
  const vs = [d(23, 59), d(0, 1), d(0, 0), d(12, 0)].map((time) => ({ time, domain: 'a' }));
  assert.deepEqual(visitsPerDay(vs), [{ date: '2025-03-09', visits: 4 }]);
  const next = new Date(2025, 2, 10, 0, 0).getTime();
  assert.equal(dayKey(next), '2025-03-10');
  assert.equal(dayKey(next - 1), '2025-03-09');
  assert.deepEqual(visitsPerDay([{ time: next }, { time: next - 1 }]).map((x) => x.date), ['2025-03-09', '2025-03-10']);
});

test('top domains order by count then name, skip empty, limit n', () => {
  const vs = ['b', 'a', 'b', 'c', 'a', '', 'b'].map((domain) => ({ domain }));
  assert.deepEqual(topDomains(vs), [{ domain: 'b', visits: 3 }, { domain: 'a', visits: 2 }, { domain: 'c', visits: 1 }]);
  assert.equal(topDomains(vs, 2).length, 2);
});

test('sessions: default 30 min gap inclusive, configurable, order independent', () => {
  const vs = [0, 10, 40, 41, 100].map((m) => ({ time: T0 + m * MIN }));
  const shuffled = [vs[3], vs[0], vs[4], vs[2], vs[1]];
  const s = groupSessions(shuffled);
  assert.deepEqual(s.map((x) => x.visits.length), [4, 1]);
  assert.equal(s[0].start, T0);
  assert.equal(s[0].end, T0 + 41 * MIN);
  assert.equal(groupSessions(vs, 5 * MIN).length, 4);
  assert.equal(groupSessions(vs, 0).length, 2);
  assert.deepEqual(groupSessions([]), []);
  assert.equal(groupSessions([{ time: 0 }, { time: 30 * MIN }]).length, 1);
  assert.equal(groupSessions([{ time: 0 }, { time: 30 * MIN + 1 }]).length, 2);
});

test('filters: range, text, domain, transition', () => {
  const vs = [
    { time: 1, url: 'https://www.Example.test/A', title: 'Hello', domain: 'www.example.test', transition: 'typed' },
    { time: 2, url: 'https://other.test/', title: 'World', domain: 'other.test', transition: 'link' },
    { time: 3, url: 'https://notexample.test/', title: '', domain: 'notexample.test', transition: 'link' },
  ];
  assert.equal(filterVisits(vs, {}).length, 3);
  assert.equal(filterVisits(vs, { startTime: 2, endTime: 2 }).length, 1);
  assert.equal(filterVisits(vs, { text: 'HELLO' }).length, 1);
  assert.equal(filterVisits(vs, { text: '/a' }).length, 1);
  assert.deepEqual(filterVisits(vs, { domain: 'example.test' }).map((v) => v.time), [1]);
  assert.equal(filterVisits(vs, { domain: 'www.example.test' }).length, 1);
  assert.equal(filterVisits(vs, { transitions: ['link'] }).length, 2);
  assert.equal(filterVisits(vs, { transitions: [] }).length, 3);
  assert.equal(filterVisits(vs, { transitions: ['link'], text: 'world' }).length, 1);
});
