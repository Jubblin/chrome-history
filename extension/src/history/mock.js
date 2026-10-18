// In-memory stand-in for chrome.history (search + getVisits) that mimics
// Chrome: one item per URL, newest first, startTime inclusive / endTime
// exclusive, lastVisitTime = newest visit inside the window. Used by tests and
// for offline/fixture mode in the UI.
// entries: [{url, title, visits: [{visitTime, transition?, referringVisitId?, isLocal?}]}]
export function createMockHistory(entries) {
  let nextId = 1;
  const data = entries.map((e) => ({
    url: e.url,
    title: e.title ?? '',
    visits: e.visits.map((v) => ({
      visitId: String(v.visitId ?? nextId++),
      visitTime: v.visitTime,
      referringVisitId: String(v.referringVisitId ?? 0),
      transition: v.transition ?? 'link',
      ...(v.isLocal === undefined ? {} : { isLocal: v.isLocal }),
    })),
  }));
  return {
    calls: { search: 0, getVisits: 0 },
    async search({ text = '', startTime = 0, endTime = Date.now(), maxResults = 100 } = {}) {
      this.calls.search++;
      const t = text.toLowerCase();
      const items = [];
      for (const e of data) {
        if (t && !e.url.toLowerCase().includes(t) && !e.title.toLowerCase().includes(t)) continue;
        const inWin = e.visits.filter((v) => v.visitTime >= startTime && v.visitTime < endTime);
        if (!inWin.length) continue;
        items.push({
          id: String(data.indexOf(e) + 1), url: e.url, title: e.title,
          lastVisitTime: Math.max(...inWin.map((v) => v.visitTime)),
          visitCount: e.visits.length, typedCount: 0,
        });
      }
      items.sort((a, b) => b.lastVisitTime - a.lastVisitTime);
      return items.slice(0, maxResults);
    },
    async getVisits({ url }) {
      this.calls.getVisits++;
      const e = data.find((x) => x.url === url);
      return e ? e.visits.map((v) => ({ id: String(data.indexOf(e) + 1), ...v })) : [];
    },
  };
}
