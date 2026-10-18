// Headless-Chrome check: loads the unpacked extension in a throwaway profile,
// generates real history by visiting local pages, then verifies the service
// worker runs and the data layer returns exactly those visits.
// Usage: PLAYWRIGHT_BROWSERS_PATH=<dir> NODE_PATH=<dir>/node_modules node scripts/chrome-check.cjs  (uses playwright's Chromium; branded Chrome ignores --load-extension)
const { chromium } = require('playwright-core');
const http = require('http');
const fs = require('fs');
const os = require('os');
const path = require('path');

const ext = path.resolve(__dirname, '..');
const results = [];
const check = (ok, msg) => { results.push(`${ok ? 'PASS' : 'FAIL'} ${msg}`); };

(async () => {
  const server = http.createServer((req, res) => {
    res.setHeader('content-type', 'text/html');
    res.end(`<title>Local page ${req.url}</title><a id=n href="/next">next</a>`);
  }).listen(0, '127.0.0.1');
  await new Promise((r) => server.once('listening', r));
  const base = `http://127.0.0.1:${server.address().port}`;
  const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'ext-check-'));
  const ctx = await chromium.launchPersistentContext(profile, {
    ...(process.env.CHROME ? { executablePath: process.env.CHROME } : {}),
    headless: false,
    channel: 'chromium',
    ignoreDefaultArgs: ['--disable-extensions'],
    args: ['--headless=new', '--no-sandbox', `--disable-extensions-except=${ext}`, `--load-extension=${ext}`,
      '--disable-features=DisableLoadExtensionCommandLineSwitch'],
  });
  const errors = [];
  try {
    let [sw] = ctx.serviceWorkers();
    if (!sw) sw = await ctx.waitForEvent('serviceworker', { timeout: 15000 });
    check(sw.url().endsWith('/src/background.js'), `service worker running: ${sw.url().replace(/^chrome-extension:\/\/[a-z]+/, '')}`);
    const id = new URL(sw.url()).host;

    const page = await ctx.newPage();
    const expected = ['/a', '/b', '/c', '/c', '/b'];
    for (const p of expected) { await page.goto(base + p); await page.waitForTimeout(150); }
    await page.click('#n');
    expected.push('/next');
    await page.waitForTimeout(500);

    const tl = await ctx.newPage();
    tl.on('console', (m) => { if (['error', 'warning'].includes(m.type())) errors.push(m.text()); });
    tl.on('pageerror', (e) => errors.push('pageerror ' + e.message));
    await tl.goto(`chrome-extension://${id}/timeline.html`);
    await tl.waitForFunction(() => /visits in the last 7 days/.test(document.getElementById('status').textContent), null, { timeout: 15000 });
    const status = await tl.textContent('#status');

    const got = await tl.evaluate(async () => {
      const { loadVisits, groupSessions, topDomains } = await import('./src/history/index.js');
      const vs = await loadVisits(chrome.history, { startTime: 0, endTime: Date.now() + 1 });
      const t = vs[0].time;
      const edge = {
        atEndExcluded: (await loadVisits(chrome.history, { startTime: 0, endTime: t })).every((v) => v.time < t),
        startIncluded: (await loadVisits(chrome.history, { startTime: t, endTime: t + 1 })).some((v) => v.time === t),
        emptyWindow: (await loadVisits(chrome.history, { startTime: t, endTime: t })).length,
      };
      return { edge, vs: vs.map((v) => ({ ...v })), sessions: groupSessions(vs).length, top: topDomains(vs, 3) };
    });
    const local = got.vs.filter((v) => v.url.startsWith(base));
    check(got.edge.atEndExcluded && got.edge.startIncluded && got.edge.emptyWindow === 0, `real Chromium: endTime exclusive, startTime inclusive (${JSON.stringify(got.edge)})`);
    check(local.length === expected.length, `data layer returned ${local.length} visits for ${expected.length} navigations to the local server`);
    check(local.every((v) => v.domain === '127.0.0.1' && v.time > 0 && v.title.startsWith('Local page') && typeof v.isLocal === 'boolean'), 'visit fields populated (domain, time, title, isLocal)');
    check(new Set(local.map((v) => v.transition)).has('typed') || new Set(local.map((v) => v.transition)).has('link'), `transitions: ${[...new Set(local.map((v) => v.transition))].join(', ')}`);
    check(got.sessions >= 1 && got.top[0].domain === '127.0.0.1', `aggregations ok (sessions=${got.sessions}, top=${got.top[0].domain})`);
    check(/^\d+ visits in the last 7 days$/.test(status), `timeline page rendered: "${status}"`);
    check(errors.length === 0, `no console errors/warnings on timeline page${errors.length ? ': ' + errors.join(' | ') : ''}`);
    const manifest = await tl.evaluate(() => chrome.runtime.getManifest());
    check(JSON.stringify(manifest.permissions) === '["history","storage"]' && !manifest.host_permissions, 'only the history and storage permissions, no host permissions');
  } catch (e) {
    check(false, 'exception: ' + e.message);
  } finally {
    await ctx.close();
    server.close();
    fs.rmSync(profile, { recursive: true, force: true });
  }
  console.log(results.join('\n'));
  process.exit(results.some((r) => r.startsWith('FAIL')) ? 1 : 0);
})();
