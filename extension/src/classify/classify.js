// JavaScript port of internal/classify (Go). Behaviour must stay identical;
// test/vectors.json (exported from the Go classifier) is the shared contract.
// Pure: no network, no logging, no URLs retained.

export const CATEGORIES = ['adult', 'finance', 'gambling', 'health', 'news_politics', 'privacy'];
export const MAX_RULES_BYTES = 1 << 20;
const MIN_KEYWORD_LEN = 3;

const BIT = new Map(CATEGORIES.map((c, i) => [c, 1 << i]));
const enc = new TextEncoder();

/** Quotes a user-supplied value for an error message, truncated so messages stay short. */
function show(s) {
  const str = String(s);
  return JSON.stringify(str.length > 40 ? `${str.slice(0, 40)}...` : str);
}

function isObject(v) {
  return v !== null && typeof v === 'object' && !Array.isArray(v);
}

function normDomain(d) {
  let n = d.trim().toLowerCase();
  if (n.startsWith('*.')) n = n.slice(2);
  n = n.replace(/^\.+|\.+$/g, '');
  if (n === '' || /[ \t/:?#@*]/.test(n) || !n.includes('.')) {
    throw new Error(`invalid domain ${show(d)} (use a bare domain such as example.com)`);
  }
  return n;
}

function addRules(cat, r, doms, kws, bundled) {
  const m = BIT.get(cat);
  if (!m) throw new Error(`unknown category ${show(cat)} (valid: ${CATEGORIES.join(', ')})`);
  if (r === null) return;
  for (const d of r.domains ?? []) {
    const n = normDomain(d);
    doms.set(n, (doms.get(n) ?? 0) | m);
  }
  for (const raw of r.keywords ?? []) {
    const k = raw.trim().toLowerCase();
    if (enc.encode(k).length < MIN_KEYWORD_LEN) {
      throw new Error(`category ${JSON.stringify(cat)}: keyword ${show(k)} is shorter than ${MIN_KEYWORD_LEN} characters`);
    }
    if (/[ \t/?#]/.test(k)) {
      throw new Error(`category ${JSON.stringify(cat)}: keyword ${show(k)} must not contain spaces or any of / ? #`);
    }
    kws.push({ kw: k, mask: m, bundled });
  }
}

/** Extracts lower-cased host and escaped path of an http(s)/ftp URL, or null. */
function hostPath(raw) {
  let u;
  try {
    u = new URL(raw);
  } catch {
    return null;
  }
  const p = u.protocol;
  if (p !== 'http:' && p !== 'https:' && p !== 'ftp:') return null;
  let h = u.hostname.toLowerCase();
  if (h.startsWith('[') && h.endsWith(']')) h = h.slice(1, -1);
  if (h.endsWith('.')) h = h.slice(0, -1);
  if (h === '') return null;
  return { host: h, path: u.pathname.toLowerCase() };
}

/** kw is a whole host token (delimited by '.' or '-') or exactly one path segment. */
function tokenMatch(host, path, kw) {
  for (let i = 0; i + kw.length <= host.length;) {
    const j = host.indexOf(kw, i);
    if (j < 0) break;
    const b = j + kw.length;
    const before = j === 0 || host[j - 1] === '.' || host[j - 1] === '-';
    const after = b === host.length || host[b] === '.' || host[b] === '-';
    if (before && after) return true;
    i = j + 1;
  }
  let start = 0;
  for (;;) {
    const e = path.indexOf('/', start);
    const seg = e < 0 ? path.slice(start) : path.slice(start, e);
    if (seg === kw) return true;
    if (e < 0) return false;
    start = e + 1;
  }
}

export class Classifier {
  /**
   * @param {object} bundled parsed rules.json
   * @param {object|null} user validated user rules (see parseUserRules)
   */
  constructor(bundled, user = null) {
    this.bundledDomains = new Map();
    this.bundledAllow = new Map();
    this.userDomains = new Map();
    this.allowDomains = new Map();
    this.keywords = [];
    this.allowKeywords = [];
    for (const [cat, r] of Object.entries(bundled.categories)) {
      addRules(cat, r, this.bundledDomains, this.keywords, true);
    }
    for (const [cat, r] of Object.entries(bundled.allow ?? {})) {
      if ((r.keywords ?? []).length > 0) throw new Error(`bundled rules: allow ${JSON.stringify(cat)}: keywords are not supported`);
      addRules(cat, r, this.bundledAllow, null, true);
    }
    if (user) {
      for (const [cat, r] of Object.entries(user.add ?? {})) addRules(cat, r, this.userDomains, this.keywords, false);
      for (const [cat, r] of Object.entries(user.allow ?? {})) addRules(cat, r, this.allowDomains, this.allowKeywords, false);
    }
  }

  /** Sorted categories of rawURL; [] if none or not a web URL. */
  classify(rawURL) {
    const hp = hostPath(rawURL);
    if (!hp) return [];
    const { host, path } = hp;
    let gotB = 0;
    let gotU = 0;
    let bundledOK = 0;
    let allowed = 0;
    for (let h = host; ;) {
      gotB |= this.bundledDomains.get(h) ?? 0;
      gotU |= this.userDomains.get(h) ?? 0;
      bundledOK |= this.bundledAllow.get(h) ?? 0;
      allowed |= this.allowDomains.get(h) ?? 0;
      const i = h.indexOf('.');
      if (i < 0) break;
      h = h.slice(i + 1);
    }
    const joined = host + path;
    for (const k of this.keywords) {
      if (k.bundled) {
        if ((gotB & k.mask) !== k.mask && tokenMatch(host, path, k.kw)) gotB |= k.mask;
      } else if ((gotU & k.mask) !== k.mask && joined.includes(k.kw)) {
        gotU |= k.mask;
      }
    }
    if (this.allowKeywords.length > 0 && ((gotB & ~bundledOK) | gotU) !== 0) {
      for (const k of this.allowKeywords) if (joined.includes(k.kw)) allowed |= k.mask;
    }
    const got = ((gotB & ~bundledOK) | gotU) & ~allowed;
    if (got === 0) return [];
    const out = [];
    for (let i = 0; i < CATEGORIES.length; i++) if (got & (1 << i)) out.push(CATEGORIES[i]);
    return out;
  }
}

function checkCategoryMap(where, v) {
  if (v === undefined || v === null) return;
  if (!isObject(v)) throw new Error(`${where} must be an object of categories`);
  for (const [cat, r] of Object.entries(v)) {
    if (r === null) continue; // Go decodes null as an empty rule set
    if (!isObject(r)) throw new Error(`${where}.${cat} must be an object with "domains" and/or "keywords"`);
    for (const [f, list] of Object.entries(r)) {
      if (f !== 'domains' && f !== 'keywords') throw new Error(`${where}.${cat}: unknown field ${show(f)}`);
      if (list === null) continue;
      if (!Array.isArray(list) || list.some((s) => typeof s !== 'string')) {
        throw new Error(`${where}.${cat}.${f} must be an array of strings`);
      }
    }
  }
}

/**
 * Parses and validates user rules JSON text (same schema as the Go app:
 * {"add": {...}, "allow": {...}}). Throws Error with a clear message.
 * Returns the plain rules object.
 */
export function parseUserRules(text, bundled) {
  if (typeof text !== 'string') throw new Error('rules must be JSON text');
  if (enc.encode(text).length > MAX_RULES_BYTES) {
    throw new Error(`rules are larger than the ${MAX_RULES_BYTES >> 20} MiB limit`);
  }
  let v;
  try {
    v = JSON.parse(text);
  } catch (e) {
    throw new Error(`invalid JSON: ${e.message}`);
  }
  if (v === null) v = {}; // Go accepts a bare null as empty rules
  if (!isObject(v)) throw new Error('rules must be a JSON object');
  for (const k of Object.keys(v)) {
    if (k !== 'add' && k !== 'allow') throw new Error(`unknown field ${show(k)} (valid: add, allow)`);
  }
  checkCategoryMap('add', v.add);
  checkCategoryMap('allow', v.allow);
  const user = { add: v.add ?? {}, allow: v.allow ?? {} };
  new Classifier(bundled ?? { categories: {} }, user);
  return user;
}
