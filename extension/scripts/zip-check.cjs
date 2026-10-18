// Loads the BUILT ZIP (unzipped to a temp dir) in Playwright's full Chromium and proves:
// the shipped timeline works, and fixture mode / test-support are unreachable.
// Usage: node scripts/zip-check.cjs path/to/extension.zip   (PLAYWRIGHT_BROWSERS_PATH, NODE_PATH as for ui-check)
const { chromium } = require('playwright-core');
const { execFileSync } = require('child_process');
const fs = require('fs'), os = require('os'), path = require('path');

const zip = path.resolve(process.argv[2] || '');
if (!fs.existsSync(zip)) { console.error('usage: zip-check.cjs <zip>'); process.exit(2); }
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'zip-check-'));
const ext = path.join(tmp, 'ext');
fs.mkdirSync(ext);
execFileSync('unzip', ['-q', zip, '-d', ext]);
const results = [];
const check = (ok, msg) => results.push(`${ok ? 'PASS' : 'FAIL'} ${msg}`);

(async () => {
  const ctx = await chromium.launchPersistentContext(fs.mkdtempSync(path.join(tmp, 'p-')), {
    headless: false, channel: 'chromium', viewport: { width: 1280, height: 800 },
    ignoreDefaultArgs: ['--disable-extensions'],
    args: ['--headless=new', '--no-sandbox', `--disable-extensions-except=${ext}`, `--load-extension=${ext}`,
      '--disable-features=DisableLoadExtensionCommandLineSwitch'],
  });
  try {
    let [sw] = ctx.serviceWorkers();
    if (!sw) sw = await ctx.waitForEvent('serviceworker', { timeout: 15000 });
    const base = `chrome-extension://${new URL(sw.url()).host}`;
    const errors = [];
    const page = await ctx.newPage();
    page.on('console', (m) => { if (['error', 'warning'].includes(m.type())) errors.push(m.text()); });
    page.on('pageerror', (e) => errors.push('pageerror ' + e.message));
    await page.goto(`${base}/timeline.html`);
    await page.waitForFunction(() => /visits loaded|No visits|0 visits/i.test(document.getElementById('status').textContent), null, { timeout: 20000 }).catch(() => {});
    check(await page.locator('#timeline').count() === 1, 'shipped timeline page renders from the zip');
    check(errors.length === 0, `no console errors/CSP violations (${errors.join(' | ')})`);
    // ?fixture=1 is ignored: still the real (here: empty) profile history, never synthetic data.
    await page.goto(`${base}/timeline.html?fixture=1`);
    await page.waitForTimeout(1500);
    const nv = await page.locator('#timeline .visit').count();
    const hrefs = await page.locator('#timeline .visit a[href]').evaluateAll((as) => as.map((a) => a.href));
    check(nv < 50 && hrefs.every((h) => h.startsWith('chrome-extension://')), `?fixture=1 shows no synthetic data (${nv} rows, only the profile's own extension-page visits)`);
    for (const p of ['ui/test-support/timeline-fixture.html', 'ui/test-support/fixture.js', 'ui/test-support/fixture-main.js', 'scripts/make-fixture.js']) {
      const r = await page.goto(`${base}/${p}`).catch(() => null);
      const body = r ? await page.evaluate(() => document.body.innerText).catch(() => '') : '';
      check(!r || !r.ok() || /ERR_FILE_NOT_FOUND|not found|blocked/i.test(body) || page.url().startsWith('chrome-error'), `${p} is not reachable`);
    }
  } finally { await ctx.close(); }
  console.log(results.join('\n'));
  fs.rmSync(tmp, { recursive: true, force: true });
  process.exit(results.some((r) => r.startsWith('FAIL')) ? 1 : 0);
})().catch((e) => { console.error(e); process.exit(1); });
