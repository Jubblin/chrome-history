export const DEFAULT_SESSION_GAP_MS = 30 * 60 * 1000;

// Day keys (YYYY-MM-DD) use the browser's LOCAL time zone, so a visit at
// 23:59 local and one at 00:01 local fall on different days.
export function dayKey(ms) {
  const d = new Date(ms);
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// [{date, visits}] ascending by date.
export function visitsPerDay(visits) {
  const m = new Map();
  for (const v of visits) {
    const k = dayKey(v.time);
    m.set(k, (m.get(k) ?? 0) + 1);
  }
  return [...m].map(([date, n]) => ({ date, visits: n })).sort((a, b) => (a.date < b.date ? -1 : 1));
}

// [{domain, visits}] by visits desc then domain asc; n <= 0 means all.
export function topDomains(visits, n = 0) {
  const m = new Map();
  for (const v of visits) if (v.domain) m.set(v.domain, (m.get(v.domain) ?? 0) + 1);
  const out = [...m].map(([domain, count]) => ({ domain, visits: count }));
  out.sort((a, b) => b.visits - a.visits || (a.domain < b.domain ? -1 : 1));
  return n > 0 ? out.slice(0, n) : out;
}

// Runs of visits whose consecutive gaps are <= gapMs (gapMs <= 0 uses the
// 30 minute default), ascending in time: [{start, end, visits}].
export function groupSessions(visits, gapMs = DEFAULT_SESSION_GAP_MS) {
  const gap = gapMs > 0 ? gapMs : DEFAULT_SESSION_GAP_MS;
  const vs = [...visits].sort((a, b) => a.time - b.time);
  const out = [];
  for (const v of vs) {
    const s = out[out.length - 1];
    if (s && v.time - s.end <= gap) {
      s.end = v.time;
      s.visits.push(v);
    } else {
      out.push({ start: v.time, end: v.time, visits: [v] });
    }
  }
  return out;
}
