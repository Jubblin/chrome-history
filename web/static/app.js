(() => {
'use strict';
// All history-derived strings (titles, URLs, domains) are untrusted: they are
// only ever placed into the DOM via textContent / setAttribute, never innerHTML.

const PAGE = 200;
const TRANSITIONS = ['link','typed','auto_bookmark','auto_subframe','manual_subframe','generated',
  'start_page','form_submit','reload','keyword','keyword_generated','unknown'];
const $ = id => document.getElementById(id);
const st = { profile: '', gapMs: 30 * 60000, offset: 0, total: 0, loading: false, done: false,
  gen: 0, hidden: new Set(), day: null, dayEl: null, sessEl: null, sessCount: 0, lastMs: null };

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}
const pad = n => String(n).padStart(2, '0');
const utcDate = ms => { const d = new Date(ms); return `${d.getUTCFullYear()}-${pad(d.getUTCMonth()+1)}-${pad(d.getUTCDate())}`; };
const utcTime = ms => { const d = new Date(ms); return `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`; };
const fmtDay = ms => new Date(ms).toLocaleDateString(undefined, { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric', timeZone: 'UTC' });
const fmtDur = ms => {
  if (!ms || ms < 1000) return '';
  const s = Math.round(ms / 1000);
  if (s < 60) return s + 's';
  const m = Math.floor(s / 60);
  return m < 60 ? `${m}m ${s % 60}s` : `${Math.floor(m / 60)}h ${m % 60}m`;
};
const num = n => n.toLocaleString();

async function api(path, params) {
  const q = new URLSearchParams();
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
    q: $('f-q').value.trim(), domain: $('f-domain').value.trim() };
}
function setStatus(msg) { $('status').textContent = msg || ''; }

// ---------- profiles ----------
async function loadProfiles() {
  const { profiles } = await api('profiles');
  const sel = $('profile');
  for (const p of profiles) {
    const o = el('option', '', [p.browser + (p.channel ? ' ' + p.channel : ''), p.name, p.email,
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
  return utcDate(t - dow * 86400000);
}
function binRange(key, unit) {
  if (unit === 'day') return [key, key];
  if (unit === 'month') {
    const [y, m] = key.split('-').map(Number);
    return [`${key}-01`, utcDate(Date.UTC(y, m, 0))];
  }
  const [y, m, d] = key.split('-').map(Number);
  return [key, utcDate(Date.UTC(y, m - 1, d) + 6 * 86400000)];
}

async function loadSummary() {
  const f = filters();
  const s = await api('summary', { profile: f.profile, start: f.start, end: f.end });
  st.gapMs = (s.sessionGapMinutes || 30) * 60000;
  const days = s.perDay || [];
  const unit = days.length > 400 ? 'month' : days.length > 120 ? 'week' : 'day';
  const bins = new Map();
  for (const d of days) bins.set(binOf(d.date, unit), (bins.get(binOf(d.date, unit)) || 0) + d.visits);
  const keys = [...bins.keys()].sort();
  const max = Math.max(1, ...bins.values());
  $('chart-unit').textContent = `(visits per ${unit}, UTC)`;
  const chart = $('chart');
  chart.replaceChildren();
  for (const k of keys) {
    const n = bins.get(k);
    const b = el('button', 'bar');
    b.type = 'button';
    b.tabIndex = chart.childElementCount === 0 ? 0 : -1;
    b.title = `${k}: ${num(n)} visits`;
    b.setAttribute('aria-label', `${k}: ${num(n)} visits`);
    const i = el('i');
    i.style.height = Math.max(2, (n / max) * 100) + '%';
    b.appendChild(i);
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
  const dl = $('domains');
  dl.replaceChildren();
  for (const d of s.topDomains || []) {
    const li = el('li');
    const b = el('button');
    b.type = 'button';
    b.append(el('span', '', d.domain), el('span', 'n', num(d.visits)));
    b.addEventListener('click', () => { $('f-domain').value = d.domain; refresh(); });
    li.appendChild(b);
    dl.appendChild(li);
  }
  return s;
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
  const span = first === last ? utcTime(first) : `${utcTime(last)}–${utcTime(first)}`;
  sessionHead(sess).textContent = `Session ${span} UTC · ${vs} visit${vs === 1 ? '' : 's'} loaded`;
}

function renderVisit(v) {
  const row = el('div', 'visit');
  row.dataset.tr = v.transition;
  if (st.hidden.has(v.transition)) row.hidden = true;
  const t = el('time', '', utcTime(v.timeMs));
  t.dateTime = v.time;
  const badge = el('span', 'badge', v.domain || '(none)');
  badge.style.background = `hsl(${hue(v.domain || '')} 45% 38%)`;
  const main = el('div', 'vmain');
  const title = v.title || v.url;
  const href = safeHref(v.url);
  let tl;
  if (href) { tl = el('a', 'vtitle', title); tl.href = href; tl.rel = 'noopener noreferrer'; tl.target = '_blank'; }
  else tl = el('span', 'vtitle', title);
  tl.title = title;
  const url = el('span', 'vurl', v.url);
  url.title = v.url;
  main.append(tl, url);
  const meta = el('div', 'vmeta');
  meta.append(el('span', 'tr', v.transition));
  const d = fmtDur(v.durationMs);
  if (d) meta.append(document.createTextNode(d));
  row.append(t, badge, main, meta);
  return row;
}

function appendVisits(visits) {
  const tl = $('timeline');
  let touched = null;
  for (const v of visits) {
    const day = utcDate(v.timeMs);
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

async function refresh() {
  resetTimeline();
  setStatus('Loading…');
  try {
    await Promise.all([loadSummary(), loadMore()]);
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
    const rows = [['time','url','title','domain','transition','duration_ms'].join(',')];
    let offset = 0, total = Infinity;
    while (offset < total) {
      setStatus(`Exporting… ${num(offset)}`);
      const r = await api('timeline', { ...f, limit: 1000, offset });
      total = r.total;
      if (!r.visits.length) break;
      for (const v of r.visits) {
        if (st.hidden.has(v.transition)) continue;
        rows.push([v.time, v.url, v.title, v.domain, v.transition, v.durationMs].map(csvCell).join(','));
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
    for (const c of $('f-trans').querySelectorAll('input')) c.checked = true;
    refresh();
  });
  $('f-csv').addEventListener('click', exportCsv);
  $('more').addEventListener('click', loadMore);
  window.addEventListener('scroll', checkSentinel, { passive: true });
  window.addEventListener('resize', checkSentinel);
  refresh();
}
init();
})();
