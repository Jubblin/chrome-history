import { domainOf } from './domain.js';
import { normalizeTransition } from './transitions.js';

const DEFAULTS = { pageSize: 5000, maxPageSize: 1_000_000, concurrency: 8 };

export function abortError(visits) {
  const e = new Error('Loading cancelled');
  e.name = 'AbortError';
  e.visits = visits;
  return e;
}

// Yield to the event loop so a UI thread is never blocked for long.
const tick = () => new Promise((r) => setTimeout(r, 0));

async function pool(items, limit, signal, fn) {
  let next = 0;
  let failed = false;
  const worker = async () => {
    while (next < items.length && !signal?.aborted && !failed) {
      try {
        await fn(items[next++]);
      } catch (e) {
        failed = true; // stop the other workers; Promise.all rejects with the first error
        throw e;
      }
    }
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker));
}

/**
 * Load every visit in the half-open window [startTime, endTime) (ms since epoch):
 * startTime is inclusive, endTime is exclusive, like chrome.history.search and
 * the Go app's `to`, so adjacent windows [a,b) and [b,c) return every visit
 * exactly once. endTime defaults to Date.now() + 1 (so "now" is included).
 *
 * chrome.history.search returns one item per URL, newest first, so the window
 * is paged by time: each page asks for the newest `pageSize` URLs ending at the
 * oldest lastVisitTime seen so far. A URL's getVisits result already contains
 * all its visits, so a URL is fetched once and visits are de-duplicated by
 * visitId. If a full page makes no time progress (many URLs sharing one
 * timestamp) the page size doubles until it does.
 *
 * Options: startTime, endTime, pageSize, concurrency, signal (AbortSignal),
 * onBatch(visits) called with each newly loaded batch, onProgress({urls,
 * visits, done}). Cancellation rejects with an AbortError whose `.visits` holds
 * what was loaded. Resolves with all visits sorted newest first.
 */
export async function loadVisits(history, opts = {}) {
  const o = { ...DEFAULTS, ...opts };
  const startTime = o.startTime ?? 0;
  const { signal } = o;
  let curEnd = o.endTime ?? Date.now() + 1; // exclusive
  const endTime = curEnd;
  let pageSize = o.pageSize;
  const seenUrls = new Set();
  const seenVisits = new Set();
  const all = [];
  let urls = 0;
  const report = (done) => o.onProgress?.({ urls, visits: all.length, done });
  const finish = () => all.sort((a, b) => b.time - a.time || b.visitId.localeCompare(a.visitId));

  for (;;) {
    if (signal?.aborted) throw abortError(finish());
    const items = await history.search({ text: '', startTime, endTime: curEnd, maxResults: pageSize });
    const fresh = items.filter((it) => it.url && !seenUrls.has(it.url));
    fresh.forEach((it) => seenUrls.add(it.url));
    const batch = [];
    await pool(fresh, o.concurrency, signal, async (it) => {
      const vs = await history.getVisits({ url: it.url });
      urls++;
      for (const v of vs) {
        if (v.visitTime < startTime || v.visitTime >= endTime) continue;
        if (seenVisits.has(v.visitId)) continue;
        seenVisits.add(v.visitId);
        batch.push({
          visitId: String(v.visitId),
          time: v.visitTime,
          url: it.url,
          title: it.title ?? '',
          domain: domainOf(it.url),
          transition: normalizeTransition(v.transition),
          referringVisitId: v.referringVisitId && v.referringVisitId !== '0' ? String(v.referringVisitId) : '',
          isLocal: v.isLocal ?? true,
        });
      }
    });
    all.push(...batch);
    if (batch.length) o.onBatch?.(batch);
    report(false);
    if (signal?.aborted) throw abortError(finish());
    if (items.length < pageSize) break;
    const oldest = Math.min(...items.map((it) => it.lastVisitTime));
    // Next window must still contain `oldest` (unseen ties), hence +1 (exclusive).
    if (oldest + 1 < curEnd) {
      curEnd = oldest + 1;
    } else if (pageSize < o.maxPageSize) {
      pageSize = Math.min(pageSize * 2, o.maxPageSize);
    } else {
      break;
    }
    await tick();
  }
  report(true);
  return finish();
}
