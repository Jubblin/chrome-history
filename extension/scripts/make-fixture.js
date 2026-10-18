// Deterministic synthetic history generator (no real data).
// Usage: node scripts/make-fixture.js [count] > fixture.json
import { TRANSITIONS } from '../src/history/transitions.js';

export function makeFixtureEntries({ visits = 12000, endTime = Date.UTC(2025, 5, 15, 12), days = 90, seed = 1 } = {}) {
  let s = seed >>> 0;
  const rnd = () => ((s = (Math.imul(s, 1664525) + 1013904223) >>> 0) / 2 ** 32);
  const hosts = Array.from({ length: 40 }, (_, i) => `site${i}.example.test`);
  const byUrl = new Map();
  for (let i = 0; i < visits; i++) {
    const host = hosts[Math.floor(rnd() ** 2 * hosts.length)];
    const url = `https://${host}/page/${Math.floor(rnd() * 400)}`;
    let e = byUrl.get(url);
    if (!e) byUrl.set(url, (e = { url, title: rnd() < 0.1 ? '' : `Page ${byUrl.size} on ${host}`, visits: [] }));
    e.visits.push({
      visitTime: endTime - Math.floor(rnd() * days * 864e5),
      transition: TRANSITIONS[Math.floor(rnd() * TRANSITIONS.length)],
      isLocal: rnd() < 0.9,
    });
  }
  return [...byUrl.values()];
}

if (import.meta.url === `file://${process.argv[1]}`) {
  process.stdout.write(JSON.stringify(makeFixtureEntries({ visits: Number(process.argv[2]) || 12000 })));
}
