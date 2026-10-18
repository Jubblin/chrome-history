// Timeline page. All history-derived strings (titles, URLs, domains) are untrusted:
// they only enter the DOM via textContent / setAttribute, never innerHTML.
import { loadVisits, TRANSITIONS } from '../src/history/index.js';
import { CATEGORIES, createClassifier, parseUserRules, loadBundledRules } from './classify-import.js';
import { MASK, isFlagged, isMasked, annotate, applyFilters, categoryCounts, chartBins, domainStats, layout, toCsv } from './model.js';

const PAGE = 200;
const HIDE_KEY = 'chrome-history.hideSensitive';
const RULES_KEY = 'userRulesText';
const CAT_ICON = { adult: '18+', gambling: '♠', health: '+', finance: '$', privacy: '◍', news_politics: '§' };
const catLabel = (id) => CATEGORIES.find((c) => c.id === id)?.label ?? id;
const $ = (id) => document.getElementById(id);
const pad = (n) => String(n).padStart(2, '0');
const localTime = (ms) => { const d = new Date(ms); return `${pad(d.getHours())}:${pad(d.getMinutes())}`; };
const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
const fmtDay = (ms) => new Date(ms).toLocaleDateString(undefined, { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric' });
const num = (n) => n.toLocaleString();

const st = { all: [], classifier: null, bundled: null, hide: false, revealed: new Set(), cats: new Set(),
  filtered: [], lay: null, rendered: 0, cur: { day: -1, sess: -1, dayEl: null, sessEl: null }, loading: false, rulesSaved: '' };

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}
const setStatus = (m) => { $('status').textContent = m || ''; };
const hue = (s) => { let h = 0; for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0; return h % 360; };
const safeHref = (u) => { try { const x = new URL(u); return /^https?:$/.test(x.protocol) ? x.href : null; } catch { return null; } };

// ---- storage (chrome.storage.local; in-memory fallback if the permission is absent) ----
const mem = {};
const store = {
  async get(k) { return chrome.storage?.local ? (await chrome.storage.local.get(k))[k] : mem[k]; },
  async set(k, v) { if (chrome.storage?.local) await chrome.storage.local.set({ [k]: v }); else mem[k] = v; },
  async remove(k) { if (chrome.storage?.local) await chrome.storage.local.remove(k); else delete mem[k]; },
};

// ---- filters ----
function selectedTransitions() {
  const on = [...$('f-trans').querySelectorAll('input')].filter((c) => c.checked).map((c) => c.value);
  return on.length === TRANSITIONS.length ? null : on.length ? on : ['\u0000none'];
}
const filters = () => ({ start: $('f-start').value, end: $('f-end').value, text: $('f-q').value, domain: $('f-domain').value,
  transitions: selectedTransitions(), cats: st.cats });

function buildTransitionFilter() {
  for (const t of TRANSITIONS) {
    const l = el('label');
    const c = el('input');
    c.type = 'checkbox'; c.checked = true; c.value = t;
    c.addEventListener('change', () => refresh());
    l.append(c, document.createTextNode(t));
    $('f-trans').appendChild(l);
  }
}

// ---- rendering ----
function catBadge(c) {
  const b = el('span', 'cat cat-' + c);
  const ic = el('i', '', CAT_ICON[c] || '*');
  ic.setAttribute('aria-hidden', 'true');
  b.append(ic, el('span', '', catLabel(c)));
  return b;
}

function renderChart(visits) {
  const { unit, bins } = chartBins(visits);
  const max = Math.max(1, ...bins.map((b) => b.visits));
  const chart = $('chart');
  const focusKey = chart.contains(document.activeElement) ? document.activeElement.dataset.key : null;
  $('chart-unit').textContent = `(visits per ${unit}, ${TZ})`;
  chart.replaceChildren();
  let focused = null;
  bins.forEach((b, i) => {
    const e = el('button', 'bar');
    e.type = 'button'; e.dataset.key = b.key;
    e.tabIndex = i === 0 ? 0 : -1;
    const label = `${b.key}: ${num(b.visits)} visits` + (b.flagged ? `, ${num(b.flagged)} flagged` : '');
    e.title = label; e.setAttribute('aria-label', label);
    const bar = el('i'); bar.style.height = Math.max(2, (b.visits / max) * 100) + '%';
    e.appendChild(bar);
    if (b.flagged) { const f = el('b'); f.style.height = Math.max(2, (b.flagged / max) * 100) + '%'; e.appendChild(f); }
    e.addEventListener('click', () => { $('f-start').value = b.range[0]; $('f-end').value = b.range[1]; refresh(); });
    chart.appendChild(e);
    if (b.key === focusKey) focused = e;
  });
  if (focused) { for (const x of chart.children) x.tabIndex = -1; focused.tabIndex = 0; focused.focus(); }
}

function renderDomains(visits) {
  const ol = $('domains');
  ol.replaceChildren();
  for (const d of domainStats(visits)) {
    const li = el('li');
    if (st.hide && d.categories.length) {
      const row = el('div', 'dom-masked');
      const tags = el('span', 'dom-tags');
      for (const c of d.categories) tags.append(catBadge(c));
      row.append(el('span', '', MASK), tags, el('span', 'n', num(d.visits)));
      li.appendChild(row);
    } else {
      const b = el('button');
      b.type = 'button';
      b.append(el('span', '', d.domain), el('span', 'n', num(d.visits)));
      b.addEventListener('click', () => { $('f-domain').value = d.domain; refresh(); });
      li.appendChild(b);
    }
    ol.appendChild(li);
  }
}

function renderChips(visits) {
  const counts = categoryCounts(visits);
  const box = $('cat-chips');
  const had = box.contains(document.activeElement) ? document.activeElement.dataset.cat : null;
  box.replaceChildren();
  for (const c of CATEGORIES) {
    const n = counts.get(c.id) ?? 0;
    if (!n && !st.cats.has(c.id)) continue;
    const b = el('button', 'chip cat-' + c.id);
    b.type = 'button'; b.dataset.cat = c.id;
    b.setAttribute('aria-pressed', st.cats.has(c.id) ? 'true' : 'false');
    b.setAttribute('aria-label', `${c.label}, ${num(n)} visits`);
    b.append(el('span', '', c.label), el('span', 'n', num(n)));
    b.addEventListener('click', () => { if (st.cats.has(c.id)) st.cats.delete(c.id); else st.cats.add(c.id); refresh(); });
    box.appendChild(b);
  }
  if (had) box.querySelector(`[data-cat="${had}"]`)?.focus();
}

function fillRow(row, v) {
  const masked = isMasked(v, st.hide, st.revealed);
  row.classList.toggle('masked', masked);
  const t = el('time', '', localTime(v.time));
  t.dateTime = new Date(v.time).toISOString();
  const badge = el('span', 'badge', masked ? MASK : (v.domain || '(none)'));
  if (masked) badge.classList.add('domain-hidden'); else badge.style.background = `hsl(${hue(v.domain)} 45% 38%)`;
  const main = el('div', 'vmain');
  let tl, url;
  if (masked) {
    tl = el('span', 'vtitle', 'Sensitive details hidden');
    url = el('span', 'vurl', '••••••••');
  } else {
    const title = v.title || v.url;
    const href = safeHref(v.url);
    if (href) { tl = el('a', 'vtitle', title); tl.href = href; tl.rel = 'noopener noreferrer'; tl.target = '_blank'; }
    else tl = el('span', 'vtitle', title);
    tl.title = title;
    url = el('span', 'vurl', v.url); url.title = v.url;
  }
  main.append(tl, url);
  const meta = el('div', 'vmeta');
  for (const c of v.categories) meta.append(catBadge(c));
  meta.append(el('span', 'tr', v.transition));
  if (st.hide && isFlagged(v)) {
    const rb = el('button', 'reveal', masked ? 'Reveal' : 'Hide');
    rb.type = 'button';
    rb.setAttribute('aria-label', `${masked ? 'Reveal' : 'Hide'} details of the ${localTime(v.time)} visit`);
    rb.addEventListener('click', () => {
      if (masked) st.revealed.add(v.visitId); else st.revealed.delete(v.visitId);
      fillRow(row, v);
      row.querySelector('.reveal')?.focus();
    });
    meta.append(rb);
  }
  row.replaceChildren(t, badge, main, meta);
}

function renderMore(count) {
  const tl = $('timeline');
  const { days, sessions, entries } = st.lay;
  const end = Math.min(entries.length, st.rendered + count);
  const c = st.cur;
  const frag = document.createDocumentFragment();
  for (let i = st.rendered; i < end; i++) {
    const { v, day, sess } = entries[i];
    if (day !== c.day) {
      c.day = day; c.sess = -1;
      c.dayEl = el('section', 'day');
      const d = days[day];
      const h = el('h3', '', fmtDay(d.ms));
      const meta = el('span', '', `${num(d.visits)} visit${d.visits === 1 ? '' : 's'}`);
      h.appendChild(meta);
      if (d.flagged) {
        const f = el('span', 'flagn', `⚑ ${num(d.flagged)} flagged`);
        f.style.marginLeft = '8px';
        h.appendChild(f);
      }
      c.dayEl.appendChild(h);
      frag.appendChild(c.dayEl);
    }
    if (sess !== c.sess) {
      c.sess = sess;
      c.sessEl = el('div', 'session');
      const s = sessions[sess];
      const span = s.first === s.last ? localTime(s.first) : `${localTime(s.last)}–${localTime(s.first)}`;
      c.sessEl.appendChild(el('div', 'session-head', `Session ${span} · ${num(s.visits)} visit${s.visits === 1 ? '' : 's'}`));
      c.dayEl.appendChild(c.sessEl);
    }
    const row = el('div', 'visit');
    fillRow(row, v);
    c.sessEl.appendChild(row);
  }
  tl.appendChild(frag);
  st.rendered = end;
  $('more').hidden = st.rendered >= entries.length;
  updateSummaryLine();
}

function updateSummaryLine() {
  const total = st.filtered.length;
  $('summary-line').textContent = total ? `${num(total)} visits match · showing ${num(Math.min(st.rendered, total))}` : '';
}

function checkSentinel() {
  if (!st.lay || st.rendered >= st.lay.entries.length) return;
  if ($('sentinel').getBoundingClientRect().top < window.innerHeight + 600) { renderMore(PAGE); checkSentinel(); }
}

function updateMaskNote() {
  $('mask-note').hidden = !(st.hide && ($('f-q').value.trim() || $('f-domain').value.trim()));
}

// Recompute everything from st.all. keepCount keeps already-rendered rows when
// background loading adds visits.
function refresh({ keepCount = false } = {}) {
  updateMaskNote();
  const f = filters();
  const prev = keepCount ? st.rendered : 0;
  const inRange = applyFilters(st.all, { ...f, cats: null, transitions: null }, st.hide, { ignoreTextDomain: true });
  const overview = applyFilters(st.all, f, st.hide, { ignoreTextDomain: true, ignoreTransitions: true });
  st.filtered = applyFilters(st.all, f, st.hide).sort((a, b) => b.time - a.time || (b.visitId < a.visitId ? -1 : 1));
  renderChart(overview);
  renderDomains(overview);
  renderChips(inRange);
  st.lay = layout(st.filtered);
  st.rendered = 0;
  st.cur = { day: -1, sess: -1, dayEl: null, sessEl: null };
  $('timeline').replaceChildren();
  renderMore(Math.max(PAGE, prev));
  $('empty').hidden = st.filtered.length > 0 || st.loading;
  checkSentinel();
}

// ---- loading ----
let refreshTimer = 0;
function scheduleRefresh() {
  if (refreshTimer) return;
  refreshTimer = setTimeout(() => { refreshTimer = 0; refresh({ keepCount: true }); }, 400);
}

async function loadAll(history) {
  st.loading = true;
  const prog = $('load-progress');
  prog.hidden = false;
  try {
    await loadVisits(history, {
      startTime: 0, endTime: Date.now() + 1, // endTime is exclusive
      onBatch: (batch) => { annotate(batch, st.classifier); st.all.push(...batch); scheduleRefresh(); },
      onProgress: ({ urls, visits, done }) => { if (!done) setStatus(`Loading… ${num(visits)} visits from ${num(urls)} pages`); },
    });
    setStatus(`${num(st.all.length)} visits loaded`);
  } catch (e) {
    setStatus('Error loading history: ' + (e && e.message ? e.message : 'unknown error'));
  } finally {
    st.loading = false;
    prog.hidden = true;
    clearTimeout(refreshTimer); refreshTimer = 0;
    refresh({ keepCount: true });
  }
}

// ---- settings: user rules ----
function userRulesFromText(text) {
  if (!text.trim()) return { rules: null };
  return parseUserRules(text);
}
function rebuildClassifier(userRules) {
  st.classifier = createClassifier({ bundledRules: st.bundled, userRules });
  for (const v of st.all) v.categories = st.classifier.classify(v.url);
}
function initSettings(savedText) {
  st.rulesSaved = savedText ?? '';
  $('rules-text').value = st.rulesSaved;
  $('settings-btn').addEventListener('click', () => {
    const open = $('settings').hidden;
    $('settings').hidden = !open;
    $('settings-btn').setAttribute('aria-expanded', String(open));
    if (open) $('rules-text').focus();
  });
  $('rules-save').addEventListener('click', async () => {
    const text = $('rules-text').value;
    const r = userRulesFromText(text);
    $('rules-status').textContent = '';
    if (r.error) { $('rules-error').textContent = r.error; $('rules-text').setAttribute('aria-invalid', 'true'); return; }
    $('rules-error').textContent = '';
    $('rules-notice').textContent = '';
    $('rules-text').removeAttribute('aria-invalid');
    try {
      if (text.trim()) await store.set(RULES_KEY, text); else await store.remove(RULES_KEY);
    } catch (e) { $('rules-error').textContent = 'Could not save the rules.'; return; }
    st.rulesSaved = text;
    rebuildClassifier(r.rules);
    $('rules-status').textContent = 'Rules saved and applied.';
    refresh({ keepCount: true });
  });
  $('rules-revert').addEventListener('click', () => {
    $('rules-text').value = st.rulesSaved;
    $('rules-error').textContent = '';
    $('rules-text').removeAttribute('aria-invalid');
    $('rules-status').textContent = 'Changes reverted.';
  });
}

// ---- CSV ----
function exportCsv() {
  const blob = new Blob([toCsv(st.filtered, st.hide)], { type: 'text/csv' });
  const a = el('a');
  a.href = URL.createObjectURL(blob);
  a.download = 'history.csv';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  setStatus(`Exported ${num(st.filtered.length)} rows`);
}

// ---- wiring ----
const debounce = (fn, ms) => { let t; return () => { clearTimeout(t); t = setTimeout(fn, ms); }; };

function wire() {
  buildTransitionFilter();
  $('filters').addEventListener('submit', (e) => e.preventDefault());
  const deb = debounce(() => refresh(), 250);
  $('f-q').addEventListener('input', deb);
  $('f-domain').addEventListener('input', deb);
  $('f-start').addEventListener('change', () => refresh());
  $('f-end').addEventListener('change', () => refresh());
  $('f-reset').addEventListener('click', () => {
    for (const id of ['f-start', 'f-end', 'f-q', 'f-domain']) $(id).value = '';
    st.cats.clear();
    for (const c of $('f-trans').querySelectorAll('input')) c.checked = true;
    refresh();
  });
  $('f-csv').addEventListener('click', exportCsv);
  try { st.hide = localStorage.getItem(HIDE_KEY) === '1'; } catch { /* ignore */ }
  $('f-hide').checked = st.hide;
  $('f-hide').addEventListener('change', (e) => {
    st.hide = e.target.checked;
    st.revealed.clear();
    try { localStorage.setItem(HIDE_KEY, st.hide ? '1' : '0'); } catch { /* ignore */ }
    refresh({ keepCount: true });
  });
  $('more').addEventListener('click', () => renderMore(PAGE));
  window.addEventListener('scroll', checkSentinel, { passive: true });
  window.addEventListener('resize', checkSentinel);
  // Roving tabindex on the chart.
  const chart = $('chart');
  chart.addEventListener('keydown', (e) => {
    const bars = [...chart.querySelectorAll('.bar')];
    const i = bars.indexOf(document.activeElement);
    if (i < 0) return;
    const j = { ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: bars.length - 1 }[e.key];
    if (j == null) return;
    e.preventDefault();
    const t = bars[Math.max(0, Math.min(bars.length - 1, j))];
    bars[i].tabIndex = -1; t.tabIndex = 0; t.focus();
  });
  chart.addEventListener('focusin', (e) => {
    const t = e.target.closest?.('.bar');
    if (t) for (const b of chart.querySelectorAll('.bar')) b.tabIndex = b === t ? 0 : -1;
  });
}

// `history` is the chrome.history API, passed in by ui/main.js. This module never selects the source itself.
export async function start(history) {
  wire();
  try {
    st.bundled = await loadBundledRules();
    const text = (await store.get(RULES_KEY)) ?? '';
    const r = userRulesFromText(typeof text === 'string' ? text : '');
    st.classifier = createClassifier({ bundledRules: st.bundled, userRules: r.rules ?? null });
    initSettings(typeof text === 'string' ? text : '');
    if (r.error) {
      const msg = 'Saved rules are invalid and were ignored (built-in rules only): ' + r.error;
      $('rules-error').textContent = msg;
      $('rules-notice').textContent = msg + ' Open Settings to fix them; the saved text is kept until you save a valid replacement.';
    }
  } catch (e) { setStatus('Error starting: ' + (e && e.message ? e.message : 'unknown error')); return; }
  refresh();
  await loadAll(history);
}
