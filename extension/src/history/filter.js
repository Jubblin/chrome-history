// Filters: startTime/endTime (inclusive ms), text (case-insensitive substring
// of url or title), domain (host or parent domain), transitions (array of
// transition types; empty/absent means any).
export function matchesDomain(visitDomain, wanted) {
  const d = wanted.trim().toLowerCase();
  if (!d) return true;
  return visitDomain === d || visitDomain.endsWith('.' + d);
}

export function filterVisits(visits, f = {}) {
  const text = (f.text ?? '').toLowerCase();
  const types = f.transitions?.length ? new Set(f.transitions) : null;
  return visits.filter((v) => {
    if (f.startTime != null && v.time < f.startTime) return false;
    if (f.endTime != null && v.time > f.endTime) return false;
    if (types && !types.has(v.transition)) return false;
    if (f.domain && !matchesDomain(v.domain, f.domain)) return false;
    if (text && !v.url.toLowerCase().includes(text) && !v.title.toLowerCase().includes(text)) return false;
    return true;
  });
}
