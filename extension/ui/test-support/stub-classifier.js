// TEST STAND-IN ONLY. Not the real classifier and not shipped; implements just the
// interface contract of extension/src/classify/index.js so the UI can be built and tested.
export const CATEGORIES = [
  { id: 'adult', label: 'Adult' }, { id: 'gambling', label: 'Gambling' }, { id: 'health', label: 'Health' },
  { id: 'finance', label: 'Finance' }, { id: 'privacy', label: 'Privacy tools' }, { id: 'news_politics', label: 'News & politics' },
];
const IDS = new Set(CATEGORIES.map((c) => c.id));
const BUNDLED = {
  version: 1,
  categories: {
    gambling: { domains: ['casino.example.test'] },
    health: { domains: ['clinic.example.test'], keywords: ['symptom'] },
    finance: { domains: ['bank.example.test'] },
    news_politics: { domains: ['news.example.test'] },
    adult: { domains: ['adult.example.test'] },
    privacy: { domains: ['vpn.example.test'] },
  },
};
export async function loadBundledRules() { return BUNDLED; }

export function parseUserRules(text) {
  if (text.length > 1 << 20) return { error: 'Rules are larger than the 1 MiB limit.' };
  let o;
  try { o = JSON.parse(text); } catch { return { error: 'Not valid JSON.' }; }
  if (!o || typeof o !== 'object' || Array.isArray(o)) return { error: 'Rules must be a JSON object.' };
  for (const k of Object.keys(o)) if (k !== 'add' && k !== 'allow') return { error: `Unknown top-level key "${k.slice(0, 40)}".` };
  for (const sec of ['add', 'allow']) {
    for (const [cat, r] of Object.entries(o[sec] ?? {})) {
      if (!IDS.has(cat)) return { error: `Unknown category "${cat.slice(0, 40)}" in ${sec}.` };
      for (const [k, list] of Object.entries(r ?? {})) {
        if ((k !== 'domains' && k !== 'keywords') || !Array.isArray(list) || list.some((x) => typeof x !== 'string' || !x)) return { error: `Invalid ${sec}.${cat}.${k}.` };
      }
    }
  }
  return { rules: o };
}

export function createClassifier({ bundledRules, userRules }) {
  const sets = (src, field) => Object.entries(src ?? {}).flatMap(([c, r]) => (r?.[field] ?? []).map((x) => [c, x.toLowerCase()]));
  const doms = [...sets(bundledRules?.categories, 'domains'), ...sets(userRules?.add, 'domains')];
  const kws = [...sets(bundledRules?.categories, 'keywords'), ...sets(userRules?.add, 'keywords')];
  const aDoms = sets(userRules?.allow, 'domains');
  const aKws = sets(userRules?.allow, 'keywords');
  const hit = (host, d) => host === d || host.endsWith('.' + d);
  return {
    classify(url) {
      let u;
      try { u = new URL(url); } catch { return []; }
      const host = u.hostname.toLowerCase();
      const low = String(url).toLowerCase();
      const got = new Set();
      for (const [c, d] of doms) if (hit(host, d)) got.add(c);
      for (const [c, k] of kws) if (low.includes(k)) got.add(c);
      for (const [c, d] of aDoms) if (hit(host, d)) got.delete(c);
      for (const [c, k] of aKws) if (low.includes(k)) got.delete(c);
      return [...got].sort();
    },
  };
}
