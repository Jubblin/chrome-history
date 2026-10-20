(() => {
'use strict';
// All history-derived strings (titles, URLs, domains) are untrusted: they are
// only ever placed into the DOM via textContent / setAttribute, never innerHTML.

const PAGE = 200;
const rowData = new WeakMap();
const TRANSITIONS = ['link','typed','auto_bookmark','auto_subframe','manual_subframe','generated',
  'start_page','form_submit','reload','keyword','keyword_generated','unknown'];
const CAT_ICON = { adult: '18+', gambling: '\u2660', health: '+', finance: '$', privacy: '\u25cd', news_politics: '\u00a7' };
const CAT_LABEL = { adult: 'Adult', gambling: 'Gambling', health: 'Health', finance: 'Finance',
  privacy: 'Privacy tools', news_politics: 'News & politics' };
const catLabel = c => CAT_LABEL[c] || c;
const HIDE_KEY = 'history-timeline.hideSensitive';
const MASK = '[hidden]';
const $ = id => document.getElementById(id);
const st = { profile: '', gapMs: 30 * 60000, offset: 0, total: 0, loading: false, done: false,
  topDomains: [], gen: 0, hidden: new Set(), cats: new Set(), hide: false, revealed: new Set(), day: null, dayEl: null, sessEl: null, sessCount: 0, lastMs: null };

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}
const pad = n => String(n).padStart(2, '0');
const calDate = ms => { const d = new Date(ms); return `${d.getUTCFullYear()}-${pad(d.getUTCMonth()+1)}-${pad(d.getUTCDate())}`; };
const localDate = ms => { const d = new Date(ms); return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}`; };
const localTime = ms => { const d = new Date(ms); return `${pad(d.getHours())}:${pad(d.getMinutes())}`; };
const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
const fmtDay = ms => new Date(ms).toLocaleDateString(undefined, { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric' });
const fmtDur = ms => {
  if (!ms || ms < 1000) return '';
  const s = Math.round(ms / 1000);
  if (s < 60) return s + 's';
  const m = Math.floor(s / 60);
  return m < 60 ? `${m}m ${s % 60}s` : `${Math.floor(m / 60)}h ${m % 60}m`;
};
const num = n => n.toLocaleString();

async function api(path, params) {
  const q = new URLSearchParams({ tz: TZ });
  for (const [k, v] of Object.entries(params || {})) if (v !== '' && v != null) q.set(k, v);
  const r = await fetch(`/api/${path}?${q}`);
  if (!r.ok) {
    let msg = r.statusText;
    try { msg = (await r.json()).error || msg; } catch (_) {}
    throw new Error(msg);
  }
  return r.json();
}

function filters() {
  return { profile: st.profile, start: $('f-start').value, end: $('f-end').value,
    q: $('f-q').value.trim(), domain: $('f-domain').value.trim(), category: [...st.cats].sort().join(','),
    maskFlagged: st.hide ? 'true' : '' };
}
function setStatus(msg) { $('status').textContent = msg || ''; }

// ---------- profiles ----------
async function loadProfiles() {
  const { profiles } = await api('profiles');
  const sel = $('profile');
  for (const p of profiles) {
    const o = el('option', '', [p.browser + (p.channel ? ' ' + p.channel : ''), p.name,
      typeof p.visitCount === 'number' ? num(p.visitCount) + ' visits' : ''].filter(Boolean).join(' · '));
    o.value = p.id;
    sel.appendChild(o);
  }
  if (!profiles.length) throw new Error('No profiles found');
  st.profile = sel.value;
}

// ---------- overview ----------
function binOf(dateStr, unit) {
  if (unit === 'day') return dateStr;
  const [y, m, d] = dateStr.split('-').map(Number);
  if (unit === 'month') return `${y}-${pad(m)}`;
  const t = Date.UTC(y, m - 1, d);
  const dow = (new Date(t).getUTCDay() + 6) % 7; // Monday start
  return calDate(t - dow * 86400000);
}
function binRange(key, unit) {
  if (unit === 'day') return [key, key];
  if (unit === 'month') {
    const [y, m] = key.split('-').map(Number);
    return [`${key}-01`, calDate(Date.UTC(y, m, 0))];
  }
  const [y, m, d] = key.split('-').map(Number);
  return [key, calDate(Date.UTC(y, m - 1, d) + 6 * 86400000)];
}

async function loadSummary() {
  const f = filters();
  const s = await api('summary', { profile: f.profile, start: f.start, end: f.end, category: f.category, maskFlagged: f.maskFlagged });
  st.gapMs = (s.sessionGapMinutes || 30) * 60000;
  const days = s.perDay || [];
  const unit = days.length > 400 ? 'month' : days.length > 120 ? 'week' : 'day';
  const bins = new Map();
  const flaggedBins = new Map();
  for (const d of days) {
    const k = binOf(d.date, unit);
    bins.set(k, (bins.get(k) || 0) + d.visits);
    flaggedBins.set(k, (flaggedBins.get(k) || 0) + (d.flaggedVisits || 0));
  }
  const keys = [...bins.keys()].sort();
  const max = Math.max(1, ...bins.values());
  $('chart-unit').textContent = `(visits per ${unit}, ${TZ})`;
  const chart = $('chart');
  chart.replaceChildren();
  for (const k of keys) {
    const n = bins.get(k);
    const fl = flaggedBins.get(k) || 0;
    const b = el('button', 'bar');
    b.type = 'button';
    b.tabIndex = chart.childElementCount === 0 ? 0 : -1;
    const label = `${k}: ${num(n)} visits` + (fl ? `, ${num(fl)} flagged` : '');
    b.title = label;
    b.setAttribute('aria-label', label);
    const i = el('i');
    i.style.height = Math.max(2, (n / max) * 100) + '%';
    b.appendChild(i);
    if (fl) {
      const m = el('b');
      m.style.height = Math.max(2, (fl / max) * 100) + '%';
      b.appendChild(m);
    }
    b.addEventListener('click', () => {
      const [a, z] = binRange(k, unit);
      $('f-start').value = a; $('f-end').value = z;
      refresh();
    });
    chart.appendChild(b);
  }
  if (!chart.dataset.keys) {
    chart.dataset.keys = '1';
    chart.addEventListener('keydown', e => {
      const bars = [...chart.querySelectorAll('.bar')];
      const i = bars.indexOf(document.activeElement);
      if (i < 0) return;
      const j = { ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: bars.length - 1 }[e.key];
      if (j == null) return;
      e.preventDefault();
      const t = bars[Math.max(0, Math.min(bars.length - 1, j))];
      bars[i].tabIndex = -1; t.tabIndex = 0; t.focus();
    });
    chart.addEventListener('focusin', e => {
      const t = e.target.closest && e.target.closest('.bar');
      if (!t) return;
      for (const b of chart.querySelectorAll('.bar')) b.tabIndex = b === t ? 0 : -1;
    });
  }
  st.topDomains = s.topDomains || [];
  renderDomains();
  return s;
}

function renderDomains() {
  const dl = $('domains');
  dl.replaceChildren();
  for (const d of st.topDomains) {
    const li = el('li');
    const cats = d.categories || [];
    if (st.hide && cats.length) {
      // Flagged domains are not named (or clickable) while hiding is on.
      const row = el('div', 'dom-masked');
      const name = el('span', '', MASK);
      const tags = el('span', 'dom-tags');
      for (const c of cats) tags.append(catBadge(c));
      row.append(name, tags, el('span', 'n', num(d.visits)));
      li.appendChild(row);
    } else {
      const b = el('button');
      b.type = 'button';
      b.append(el('span', '', d.domain), el('span', 'n', num(d.visits)));
      b.addEventListener('click', () => { $('f-domain').value = d.domain; refresh(); });
      li.appendChild(b);
    }
    dl.appendChild(li);
  }
}

// ---------- categories ----------
async function loadCategories() {
  const f = filters();
  const r = await api('categories', { profile: f.profile, start: f.start, end: f.end, maskFlagged: f.maskFlagged });
  const box = $('cat-chips');
  const hadFocus = box.contains(document.activeElement) ? document.activeElement.dataset.cat : null;
  box.replaceChildren();
  for (const c of r.categories || []) {
    CAT_LABEL[c.id] = CAT_LABEL[c.id] || c.label;
    const b = el('button', 'chip cat-' + c.id);
    b.type = 'button';
    b.dataset.cat = c.id;
    b.setAttribute('aria-pressed', st.cats.has(c.id) ? 'true' : 'false');
    b.append(el('span', '', catLabel(c.id)), el('span', 'n', num(c.visits)));
    b.setAttribute('aria-label', `${catLabel(c.id)}, ${num(c.visits)} visits`);
    b.addEventListener('click', () => {
      if (st.cats.has(c.id)) st.cats.delete(c.id); else st.cats.add(c.id);
      b.setAttribute('aria-pressed', st.cats.has(c.id) ? 'true' : 'false');
      refresh();
    });
    box.appendChild(b);
  }
  if (hadFocus) { const b = box.querySelector(`[data-cat="${hadFocus}"]`); if (b) b.focus(); }
}

// ---------- timeline ----------
function hue(s) { let h = 0; for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0; return h % 360; }
function safeHref(u) { try { const x = new URL(u); return /^https?:$/.test(x.protocol) ? x.href : null; } catch (_) { return null; } }

function resetTimeline() {
  st.gen++;
  st.offset = 0; st.total = 0; st.done = false; st.loading = false;
  st.day = st.dayEl = st.sessEl = null; st.sessCount = 0; st.lastMs = null;
  $('timeline').replaceChildren();
  $('empty').hidden = true;
}

function sessionHead(sess) {
  return sess.firstChild;
}
function updateSessionHead(sess) {
  const vs = sess.querySelectorAll('.visit').length;
  const first = Number(sess.dataset.first), last = Number(sess.dataset.last);
  const span = first === last ? localTime(first) : `${localTime(last)}–${localTime(first)}`;
  sessionHead(sess).textContent = `Session ${span} · ${vs} visit${vs === 1 ? '' : 's'} loaded`;
}

function catBadge(c) {
  const b = el('span', 'cat cat-' + c);
  const ic = el('i', '', CAT_ICON[c] || '*');
  ic.setAttribute('aria-hidden', 'true');
  b.append(ic, el('span', '', catLabel(c)));
  return b;
}

function isMasked(v) { return st.hide && v.categories.length > 0 && !st.revealed.has(v.id); }

// Fills (or refills) a visit row. Masked rows never put the real title, URL or
// domain into the DOM.
function fillRow(row, v) {
  const masked = isMasked(v);
  row.classList.toggle('masked', masked);
  const t = el('time', '', localTime(v.timeMs));
  t.dateTime = v.time;
  const badge = el('span', 'badge', masked ? MASK : (v.domain || '(none)'));
  if (masked) badge.classList.add('domain-hidden');
  else badge.style.background = `hsl(${hue(v.domain || '')} 45% 38%)`;
  const main = el('div', 'vmain');
  let tl, url;
  if (masked) {
    tl = el('span', 'vtitle', 'Sensitive details hidden');
    url = el('span', 'vurl', '\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022');
  } else {
    const title = v.title || v.url;
    const href = safeHref(v.url);
    if (href) { tl = el('a', 'vtitle', title); tl.href = href; tl.rel = 'noopener noreferrer'; tl.target = '_blank'; }
    else tl = el('span', 'vtitle', title);
    tl.title = title;
    url = el('span', 'vurl', v.url);
    url.title = v.url;
  }
  main.append(tl, url);
  const meta = el('div', 'vmeta');
  for (const c of v.categories) meta.append(catBadge(c));
  meta.append(el('span', 'tr', v.transition));
  const d = fmtDur(v.durationMs);
  if (d) meta.append(document.createTextNode(d));
  if (st.hide && v.categories.length) {
    const rb = el('button', 'reveal', masked ? 'Reveal' : 'Hide');
    rb.type = 'button';
    rb.setAttribute('aria-label', `${masked ? 'Reveal' : 'Hide'} details of the ${localTime(v.timeMs)} visit`);
    rb.addEventListener('click', () => {
      if (masked) st.revealed.add(v.id); else st.revealed.delete(v.id);
      fillRow(row, v);
      const nb = row.querySelector('.reveal');
      if (nb) nb.focus();
    });
    meta.append(rb);
  }
  row.replaceChildren(t, badge, main, meta);
}

function renderVisit(v) {
  v.categories = v.categories || [];
  const row = el('div', 'visit');
  row.dataset.tr = v.transition;
  if (st.hidden.has(v.transition)) row.hidden = true;
  rowData.set(row, v);
  fillRow(row, v);
  return row;
}

function applyHide() {
  st.revealed.clear();
  renderDomains();
  for (const row of document.querySelectorAll('.visit')) {
    const v = rowData.get(row);
    if (v && v.categories.length) fillRow(row, v);
  }
}

function appendVisits(visits) {
  const tl = $('timeline');
  let touched = null;
  for (const v of visits) {
    const day = localDate(v.timeMs);
    if (day !== st.day) {
      st.day = day;
      st.dayEl = el('section', 'day');
      const h = el('h3', '', fmtDay(v.timeMs));
      st.dayEl.appendChild(h);
      tl.appendChild(st.dayEl);
      st.sessEl = null;
    }
    if (!st.sessEl || st.lastMs - v.timeMs > st.gapMs) {
      if (touched) updateSessionHead(touched);
      st.sessEl = el('div', 'session');
      st.sessEl.appendChild(el('div', 'session-head'));
      st.sessEl.dataset.first = v.timeMs;
      st.dayEl.appendChild(st.sessEl);
    }
    st.sessEl.dataset.last = v.timeMs;
    st.sessEl.appendChild(renderVisit(v));
    touched = st.sessEl;
    st.lastMs = v.timeMs;
  }
  if (touched) updateSessionHead(touched);
}

async function loadMore() {
  if (st.loading || st.done) return;
  st.loading = true;
  const gen = st.gen;
  try {
    const f = filters();
    const r = await api('timeline', { ...f, limit: PAGE, offset: st.offset });
    if (gen !== st.gen) return;
    st.total = r.total;
    appendVisits(r.visits);
    st.offset += r.visits.length;
    st.done = r.visits.length === 0 || st.offset >= r.total;
    updateSummaryLine();
    $('empty').hidden = r.total !== 0;
    $('more').hidden = st.done;
    setStatus('');
  } catch (e) {
    if (gen === st.gen) { setStatus('Error: ' + e.message); st.done = true; }
  } finally {
    if (gen === st.gen) st.loading = false;
  }
  if (gen === st.gen) checkSentinel();
}

function updateSummaryLine() {
  const hiddenN = document.querySelectorAll('.visit[hidden]').length;
  $('summary-line').textContent = st.total
    ? `${num(st.total)} visits match · showing ${num(st.offset)}${hiddenN ? ` (${num(hiddenN)} hidden by transition filter)` : ''}`
    : '';
}

function checkSentinel() {
  const r = $('sentinel').getBoundingClientRect();
  if (!st.done && !st.loading && r.top < window.innerHeight + 600) loadMore();
}

function updateMaskNote() {
  $('mask-note').hidden = !(st.hide && ($('f-q').value.trim() || $('f-domain').value.trim()));
}

async function refresh() {
  updateMaskNote();
  resetTimeline();
  setStatus('Loading…');
  try {
    await Promise.all([loadSummary(), loadMore(), loadCategories()]);
  } catch (e) { setStatus('Error: ' + e.message); }
}

// ---------- transition filter ----------
function buildTransitionFilter() {
  const box = $('f-trans');
  for (const t of TRANSITIONS) {
    const l = el('label');
    const c = el('input');
    c.type = 'checkbox'; c.checked = true; c.value = t;
    c.addEventListener('change', () => {
      if (c.checked) st.hidden.delete(t); else st.hidden.add(t);
      for (const row of document.querySelectorAll('.visit')) row.hidden = st.hidden.has(row.dataset.tr);
      updateSummaryLine();
      checkSentinel();
    });
    l.append(c, document.createTextNode(t));
    box.appendChild(l);
  }
}

// ---------- CSV ----------
function csvCell(v) {
  let s = String(v ?? '');
  if (/^[=+\-@\t\r]/.test(s)) s = "'" + s; // neutralise spreadsheet formulas
  return /[",\n\r]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
}
async function exportCsv() {
  const btn = $('f-csv');
  btn.disabled = true;
  try {
    const f = filters();
    const rows = [['time','url','title','domain','transition','duration_ms','categories'].join(',')];
    let offset = 0, total = Infinity;
    while (offset < total) {
      setStatus(`Exporting… ${num(offset)}`);
      const r = await api('timeline', { ...f, limit: 1000, offset });
      total = r.total;
      if (!r.visits.length) break;
      for (const v of r.visits) {
        if (st.hidden.has(v.transition)) continue;
        const cats = v.categories || [];
        const mask = st.hide && cats.length > 0; // masks regardless of per-row reveals
        rows.push([v.time, mask ? MASK : v.url, mask ? MASK : v.title, mask ? MASK : v.domain,
          v.transition, v.durationMs, cats.join(';')].map(csvCell).join(','));
      }
      offset += r.visits.length;
    }
    const blob = new Blob([rows.join('\r\n') + '\r\n'], { type: 'text/csv' });
    const a = el('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'history.csv';
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
    setStatus(`Exported ${num(rows.length - 1)} rows`);
  } catch (e) { setStatus('Export failed: ' + e.message); }
  finally { btn.disabled = false; }
}

// ---------- wiring ----------
function debounce(fn, ms) { let t; return () => { clearTimeout(t); t = setTimeout(fn, ms); }; }

async function init() {
  buildTransitionFilter();
  try { await loadProfiles(); } catch (e) { setStatus('Error: ' + e.message); return; }
  $('profile').addEventListener('change', e => { st.profile = e.target.value; refresh(); });
  $('filters').addEventListener('submit', e => e.preventDefault());
  const deb = debounce(refresh, 300);
  $('f-q').addEventListener('input', deb);
  $('f-domain').addEventListener('input', deb);
  $('f-start').addEventListener('change', refresh);
  $('f-end').addEventListener('change', refresh);
  $('f-reset').addEventListener('click', () => {
    for (const id of ['f-start','f-end','f-q','f-domain']) $(id).value = '';
    st.hidden.clear();
    st.cats.clear();
    for (const c of $('f-trans').querySelectorAll('input')) c.checked = true;
    refresh();
  });
  $('f-csv').addEventListener('click', exportCsv);
  try { st.hide = localStorage.getItem(HIDE_KEY) === '1'; } catch (_) {}
  $('f-hide').checked = st.hide;
  $('f-hide').addEventListener('change', e => {
    st.hide = e.target.checked;
    try { localStorage.setItem(HIDE_KEY, st.hide ? '1' : '0'); } catch (_) {}
    applyHide();
    // The server decides what Search/Domain and the domain list may match.
    refresh();
  });
  $('more').addEventListener('click', loadMore);
  window.addEventListener('scroll', checkSentinel, { passive: true });
  window.addEventListener('resize', checkSentinel);
  refresh();
}
init();
})();
