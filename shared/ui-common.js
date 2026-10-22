// Pure UI helpers shared by the web UI and the Chrome extension UI.
// Source of truth: shared/ui-common.js. Copies in web/static/ and extension/ui/ are generated
// by scripts/sync-ui-common.mjs (make ui-sync / make ui-check); do not edit them.
// All history-derived strings are untrusted: only ever put them in the DOM via textContent / setAttribute.

export const MASK = '[hidden]';
export const CAT_ICON = { adult: '18+', gambling: '♠', health: '+', finance: '$', privacy: '◍', news_politics: '§' };

export function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}
export const hue = (s) => { let h = 0; for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0; return h % 360; };
export const safeHref = (u) => { try { const x = new URL(u); return /^https?:$/.test(x.protocol) ? x.href : null; } catch { return null; } };
export const pad = (n) => String(n).padStart(2, '0');
export const localTime = (ms) => { const d = new Date(ms); return `${pad(d.getHours())}:${pad(d.getMinutes())}`; };
export const fmtDay = (ms) => new Date(ms).toLocaleDateString(undefined, { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric' });
export const num = (n) => n.toLocaleString();
export const debounce = (fn, ms) => { let t; return () => { clearTimeout(t); t = setTimeout(fn, ms); }; };

// `label` is the already-resolved display name (each front end owns its category labels).
export function catBadge(c, label) {
  const b = el('span', 'cat cat-' + c);
  const ic = el('i', '', CAT_ICON[c] || '*');
  ic.setAttribute('aria-hidden', 'true');
  b.append(ic, el('span', '', label));
  return b;
}

// Appends one checked checkbox per transition to `box`; onChange(transition, checked) on toggle.
export function buildTransitionFilter(box, transitions, onChange) {
  for (const t of transitions) {
    const l = el('label');
    const c = el('input');
    c.type = 'checkbox'; c.checked = true; c.value = t;
    c.addEventListener('change', () => onChange(t, c.checked));
    l.append(c, document.createTextNode(t));
    box.appendChild(l);
  }
}

export function csvCell(v) {
  let s = String(v ?? '');
  if (/^[=+\-@\t\r]/.test(s)) s = "'" + s; // neutralise spreadsheet formulas
  return /[",\n\r]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
}

export function downloadBlob(blob, filename) {
  const a = el('a');
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}
