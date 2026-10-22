// Pure view-model logic for the timeline (no DOM, no chrome.* APIs) so it can be unit tested.
import { dayKey, filterVisits, DEFAULT_SESSION_GAP_MS } from '../src/history/index.js';

import { MASK, csvCell, binOf, binRange } from './ui-common.js';
export { MASK, csvCell };
export const plural = (n, one, many = one + 's') => `${n.toLocaleString()} ${n === 1 ? one : many}`;
export const EMPTY_HISTORY = 'The browser has no browsing history for this profile (it may be new, or its history was cleared). Visit some pages, then reload this tab.';
export const EMPTY_FILTERED = 'No visits match these filters.';
const pad = (n) => String(n).padStart(2, '0');

export function isFlagged(v) { return v.categories.length > 0; }
export function isMasked(v, hide, revealed) { return hide && isFlagged(v) && !(revealed && revealed.has(v.visitId)); }

// Local-time bounds from <input type=date> values ('' = open).
export function dayStart(s) { if (!s) return null; const [y, m, d] = s.split('-').map(Number); return new Date(y, m - 1, d).getTime(); }
// Exclusive upper bound: local midnight after the given day (half-open ranges).
export function dayEnd(s) { if (!s) return null; const [y, m, d] = s.split('-').map(Number); return new Date(y, m - 1, d + 1).getTime(); }

export function annotate(visits, classifier) {
  for (const v of visits) v.categories = classifier.classify(v.url);
  return visits;
}

// f: {start, end ('YYYY-MM-DD'), text, domain, transitions (array|null), cats (Set)}.
// With `hide` on, flagged visits never match text/domain filters, so a search cannot
// reveal what a hidden title, URL or domain contains.
export function applyFilters(visits, f, hide, { ignoreTextDomain = false, ignoreTransitions = false } = {}) {
  let out = filterVisits(visits, {
    startTime: dayStart(f.start) ?? undefined,
    transitions: ignoreTransitions ? undefined : f.transitions ?? undefined,
  });
  const endEx = dayEnd(f.end);
  if (endEx != null) out = out.filter((v) => v.time < endEx);
  if (f.cats && f.cats.size) out = out.filter((v) => v.categories.some((c) => f.cats.has(c)));
  const text = (f.text ?? '').trim(), domain = (f.domain ?? '').trim();
  if (!ignoreTextDomain && (text || domain)) {
    if (hide) out = out.filter((v) => !isFlagged(v));
    out = filterVisits(out, { text, domain });
  }
  return out;
}

export function categoryCounts(visits) {
  const m = new Map();
  for (const v of visits) for (const c of v.categories) m.set(c, (m.get(c) ?? 0) + 1);
  return m;
}

// ---- chart bins (local days; weeks start Monday) ----
export function chartBins(visits) {
  const days = new Map();
  for (const v of visits) {
    const k = dayKey(v.time);
    const e = days.get(k) ?? { visits: 0, flagged: 0 };
    e.visits++; if (isFlagged(v)) e.flagged++;
    days.set(k, e);
  }
  const unit = days.size > 400 ? 'month' : days.size > 120 ? 'week' : 'day';
  const bins = new Map();
  for (const [k, e] of days) {
    const b = binOf(k, unit);
    const x = bins.get(b) ?? { key: b, visits: 0, flagged: 0 };
    x.visits += e.visits; x.flagged += e.flagged;
    bins.set(b, x);
  }
  const list = [...bins.values()].sort((a, b) => (a.key < b.key ? -1 : 1));
  for (const b of list) b.range = binRange(b.key, unit);
  return { unit, bins: list };
}

// [{domain, visits, categories:[...]}] top n; categories = union over the domain's visits.
export function domainStats(visits, n = 12) {
  const m = new Map();
  for (const v of visits) {
    if (!v.domain) continue;
    const e = m.get(v.domain) ?? { domain: v.domain, visits: 0, cats: new Set() };
    e.visits++; for (const c of v.categories) e.cats.add(c);
    m.set(v.domain, e);
  }
  return [...m.values()].sort((a, b) => b.visits - a.visits || (a.domain < b.domain ? -1 : 1))
    .slice(0, n).map((e) => ({ domain: e.domain, visits: e.visits, categories: [...e.cats].sort() }));
}

// Lay visits (newest first) out into days and sessions. entries[i] = {v, day, sess}.
export function layout(visits, gapMs = DEFAULT_SESSION_GAP_MS) {
  const days = [], sessions = [], entries = [];
  let lastMs = null;
  for (const v of visits) {
    const k = dayKey(v.time);
    let d = days[days.length - 1];
    if (!d || d.key !== k) { d = { key: k, ms: v.time, visits: 0, flagged: 0 }; days.push(d); lastMs = null; }
    let s = sessions[sessions.length - 1];
    if (!s || lastMs === null || lastMs - v.time > gapMs) { s = { first: v.time, last: v.time, visits: 0 }; sessions.push(s); }
    s.last = v.time; s.visits++; d.visits++; if (isFlagged(v)) d.flagged++;
    lastMs = v.time;
    entries.push({ v, day: days.length - 1, sess: sessions.length - 1 });
  }
  return { days, sessions, entries };
}

// ---- CSV ----
// Masks every flagged visit regardless of per-row reveals when `hide` is on.
export function toCsv(visits, hide) {
  const rows = [['time', 'url', 'title', 'domain', 'transition', 'categories'].join(',')];
  for (const v of visits) {
    const m = hide && isFlagged(v);
    rows.push([new Date(v.time).toISOString(), m ? MASK : v.url, m ? MASK : v.title, m ? MASK : v.domain,
      v.transition, v.categories.join(';')].map(csvCell).join(','));
  }
  return rows.join('\r\n') + '\r\n';
}
