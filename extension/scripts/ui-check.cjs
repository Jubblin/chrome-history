// Headless-Chromium check of the timeline UI (fixture mode, then real history from local pages).
// Works on a temp COPY of the extension with the "storage" permission added (the classifier change
// adds it to the real manifest). Screenshots (synthetic data only) go to $SHOTS or a temp dir.
// Usage: PLAYWRIGHT_BROWSERS_PATH=<dir> NODE_PATH=<dir>/node_modules node scripts/ui-check.cjs
const { chromium } = require('playwright-core');
const http = require('http');
const fs = require('fs');
const os = require('os');
const path = require('path');

const src = path.resolve(__dirname, '..');
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'ui-check-'));
const ext = path.join(tmp, 'ext');
fs.cpSync(src, ext, { recursive: true, filter: (p) => !p.includes('node_modules') });
const mf = JSON.parse(fs.readFileSync(path.join(ext, 'manifest.json'), 'utf8'));
if (!mf.permissions.includes('storage')) mf.permissions.push('storage');
fs.writeFileSync(path.join(ext, 'manifest.json'), JSON.stringify(mf));
const shots = process.env.SHOTS || path.join(tmp, 'shots');
fs.mkdirSync(shots, { recursive: true });

const results = [];
const check = (ok, msg) => { results.push(`${ok ? 'PASS' : 'FAIL'} ${msg}`); };

async function launch(profile, colorScheme) {
  return chromium.launchPersistentContext(profile, {
    headless: false, channel: 'chromium', colorScheme, acceptDownloads: true,
    viewport: { width: 1366, height: 768 },
    ignoreDefaultArgs: ['--disable-extensions'],
    args: ['--headless=new', '--no-sandbox', `--disable-extensions-except=${ext}`, `--load-extension=${ext}`,
      '--disable-features=DisableLoadExtensionCommandLineSwitch'],
  });
}
async function extId(ctx) {
  let [sw] = ctx.serviceWorkers();
  if (!sw) sw = await ctx.waitForEvent('serviceworker', { timeout: 15000 });
  return new URL(sw.url()).host;
}
function watch(page, errors, requests) {
  page.on('console', (m) => { if (['error', 'warning'].includes(m.type())) errors.push(m.text()); });
  page.on('pageerror', (e) => errors.push('pageerror ' + e.message));
  page.on('request', (r) => { if (!/^(chrome-extension|data|blob):/.test(r.url())) requests.push(r.url()); });
}
const loaded = (page) => page.waitForFunction(() => /visits? loaded/.test(document.getElementById('status').textContent), null, { timeout: 60000 });
const rows = (page) => page.locator('#timeline .visit').count();

(async () => {
  const errors = [], requests = [];
  const profile = fs.mkdtempSync(path.join(tmp, 'p1-'));
  let ctx = await launch(profile, 'light');
  try {
    const id = await extId(ctx);
    const base = `chrome-extension://${id}/timeline.html`;
    const page = await ctx.newPage();
    watch(page, errors, requests);
    const t0 = Date.now();
    await page.goto(base.replace('timeline.html','ui/test-support/timeline-fixture.html'));
    await page.waitForSelector('#timeline .visit', { timeout: 30000 }).catch(async (e) => { console.log('DEBUG', errors, requests, await page.textContent('#status'), await page.evaluate(() => document.body.innerText.slice(0,300))); throw e; });
    const firstPaint = Date.now() - t0;
    await loaded(page);
    const status = await page.textContent('#status');
    const total = Number((await page.textContent('#summary-line')).match(/^([\d,]+) visits? match(es)?/)[1].replace(/,/g, ''));
    check(total >= 10000, `fixture loaded ${total} visits (${status}); first rows after ${firstPaint} ms`);
    check(await rows(page) === 200, `progressive rendering: ${await rows(page)} rows in DOM initially`);
    await page.screenshot({ path: path.join(shots, 'fixture-light.png') });

    // structure
    check(await page.locator('#timeline .day > h3').count() > 0 && await page.locator('#timeline .session-head').count() > 0, 'day headers and sessions rendered');
    check(await page.locator('.day > h3').first().evaluate((e) => getComputedStyle(e).position) === 'sticky', 'day headers are sticky');
    check(await page.locator('.flagn').count() > 0, 'per-day flag counts shown');
    check(await page.locator('#timeline .cat').count() > 0, 'sensitive badges (icon + text) shown');
    check(await page.locator('#timeline .cat i[aria-hidden=true]').count() > 0, 'badge icon present and aria-hidden');
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await page.waitForFunction(() => document.querySelectorAll('#timeline .visit').length > 200);
    check(true, `scrolling loads more rows (${await rows(page)})`);
    await page.evaluate(() => window.scrollTo(0, 0));

    // XSS
    const xss = await page.evaluate(() => ({ imgs: document.querySelectorAll('#timeline img').length, title: document.title, anchors: [...document.querySelectorAll('a.vtitle')].filter((a) => /^javascript:/i.test(a.href)).length }));
    await page.fill('#f-q', 'XSS title');
    await page.waitForFunction(() => /^[\d,]+ visits? match(es)?/.test(document.getElementById('summary-line').textContent) && document.querySelectorAll('#timeline .visit').length <= 2);
    const shown = await page.locator('#timeline .vtitle').first().textContent();
    check(xss.imgs === 0 && xss.title === 'History timeline' && xss.anchors === 0 && shown.includes('<img'), 'XSS-style titles rendered as text only; javascript: URLs not linked');
    await page.fill('#f-q', '');

    // chart zoom + roving tabindex
    const bars = page.locator('#chart .bar');
    const nBars = await bars.count();
    await bars.first().focus();
    await page.keyboard.press('ArrowRight');
    const tabs = await bars.evaluateAll((bs) => bs.map((b) => b.tabIndex));
    check(nBars > 10 && tabs.filter((t) => t === 0).length === 1 && tabs[1] === 0, `chart roving tabindex (${nBars} bars, ArrowRight moves the tab stop)`);
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => document.getElementById('f-start').value !== '');
    check(await page.inputValue('#f-start') === await page.inputValue('#f-end'), `chart click/Enter zooms to a day (${await page.inputValue('#f-start')})`);
    await page.click('#f-reset');
    await page.waitForFunction(() => document.getElementById('f-start').value === '');

    // top domain filter
    const dom = await page.locator('#domains button span').first().textContent();
    await page.locator('#domains button').first().click();
    await page.waitForFunction((d) => document.getElementById('f-domain').value === d, dom);
    const doms = await page.locator('#timeline .badge').evaluateAll((bs) => [...new Set(bs.map((b) => b.textContent))]);
    check(doms.length >= 1 && doms.every((d) => d === dom || d.endsWith('.' + dom)), `top domain click filters timeline to ${dom}`);
    await page.click('#f-reset');

    // transition filter
    await page.click('.transitions summary');
    await page.locator('#f-trans input[value=typed]').uncheck();
    await page.waitForTimeout(300);
    const totalNoTyped = Number((await page.textContent('#summary-line')).match(/^([\d,]+)/)[1].replace(/,/g, ''));
    check(totalNoTyped < total, `transition filter reduces results (${total} -> ${totalNoTyped})`);
    await page.click('#f-reset');
    await page.click('.transitions summary');

    // categories
    const chips = await page.locator('#cat-chips .chip').count();
    await page.locator('#cat-chips .chip.cat-gambling').click();
    await page.waitForFunction(() => document.querySelectorAll('#cat-chips .chip[aria-pressed=true]').length === 1);
    const onlyGamb = await page.locator('#timeline .visit').evaluateAll((rs) => rs.every((r) => r.querySelector('.cat-gambling')));
    check(chips >= 5 && onlyGamb, `category chips (${chips}) filter to flagged visits`);
    await page.click('#f-reset');

    // masking
    await page.check('#f-hide');
    await page.waitForTimeout(300);
    let txt = await page.locator('main').innerText();
    check(!/bet365\.com|Secret casino|aetna\.com|Bank accounts|Election story|Symptom checker/.test(txt), 'hide on: no flagged title/URL/domain text anywhere (rows, top domains)');
    check(await page.locator('#domains .dom-masked').count() > 0, 'hide on: flagged top domains masked with category tags');
    await page.fill('#f-q', 'casino');
    await page.waitForTimeout(500);
    check(await rows(page) === 0 && await page.isVisible('#mask-note'), 'hide on: search for hidden value matches nothing and shows the note');
    await page.fill('#f-q', '');
    await page.fill('#f-domain', 'bet365.com');
    await page.waitForTimeout(500);
    check(await rows(page) === 0 && await page.isVisible('#mask-note'), 'hide on: domain filter does not match masked domain');
    check(await page.locator('#mask-live').getAttribute('role') === 'status', 'mask note lives in a role=status region');
    await page.fill('#f-domain', '');
    await page.waitForTimeout(400);
    await page.screenshot({ path: path.join(shots, 'fixture-hidden-light.png') });
    // reveal button
    await page.locator('#timeline .visit.masked .reveal').first().click();
    check(await page.locator('#timeline .visit:not(.masked) .reveal').count() === 1, 'per-row Reveal works while hidden');

    // CSV (masked)
    let [dl] = await Promise.all([page.waitForEvent('download'), page.click('#f-csv')]);
    let csv = fs.readFileSync(await dl.path(), 'utf8');
    check(/^time,url,title,domain,transition,categories\r\n/.test(csv) && !/bet365\.com|Secret casino|aetna\.com|Bank accounts/.test(csv) && csv.includes('[hidden]') && /,gambling\r\n/.test(csv), 'CSV with hide on: flagged values masked, categories column kept');
    check(csv.split('\r\n').length - 2 === total && !/\r\n[=+@-]/.test(csv), `CSV exports all ${total} filtered rows`);
    await page.uncheck('#f-hide');
    await page.waitForTimeout(300);
    [dl] = await Promise.all([page.waitForEvent('download'), page.click('#f-csv')]);
    csv = fs.readFileSync(await dl.path(), 'utf8');
    check(csv.includes('bet365.com') && csv.includes("'=HYPERLINK") && csv.includes("'@SUM"), 'CSV with hide off: real values; formula-injection guard on "=" and "@" titles');

    // settings
    await page.click('#settings-btn');
    await page.fill('#rules-text', '{not json');
    await page.click('#rules-save');
    const err = await page.textContent('#rules-error');
    check(err.length > 0 && !/http|example\.test/.test(err) && await page.locator('#rules-error').getAttribute('role') === 'alert', `invalid rules rejected with message: "${err}"`);
    await page.fill('#rules-text', JSON.stringify({ allow: { gambling: { domains: ['bet365.com'] } }, add: { finance: { domains: ['site1.example.test'] } } }));
    await page.click('#rules-save');
    await page.waitForFunction(() => document.getElementById('rules-status').textContent.includes('saved'));
    await page.fill('#f-domain', 'bet365.com');
    await page.waitForTimeout(400);
    const casinoFlags = await page.locator('#timeline .visit .cat').count();
    await page.fill('#f-domain', 'site1.example.test');
    await page.waitForTimeout(400);
    const s1 = await page.locator('#timeline .visit .cat-finance').count();
    check(casinoFlags === 0 && s1 > 0, `user rules applied (allow removes casino flag; add flags site1: ${s1} rows)`);
    await page.screenshot({ path: path.join(shots, 'fixture-settings-light.png') });
    await page.close();

    // persistence + dark + narrow
    const p2 = await ctx.newPage();
    watch(p2, errors, requests);
    await p2.goto(base.replace('timeline.html','ui/test-support/timeline-fixture.html'));
    await loaded(p2);
    check((await p2.inputValue('#rules-text')).includes('bet365.com'), 'user rules persisted in chrome.storage.local');
    check(await p2.isChecked('#f-hide') === false, 'hide toggle state restored from storage');
    await p2.setViewportSize({ width: 420, height: 800 });
    await p2.waitForTimeout(300);
    check(await p2.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), 'no horizontal overflow at 420px width');
    await p2.screenshot({ path: path.join(shots, 'fixture-narrow-light.png') });
    await p2.setViewportSize({ width: 1024, height: 700 });
    await p2.screenshot({ path: path.join(shots, 'fixture-1024-light.png') });
    // keyboard: skip link
    await p2.setViewportSize({ width: 1366, height: 768 });
    await p2.evaluate(() => document.activeElement.blur());
    await p2.keyboard.press('Tab');
    check(await p2.evaluate(() => document.activeElement.className === 'skip' && document.activeElement.getAttribute('href') === '#timeline'), 'first Tab stop is the "Skip to timeline" link');
    // filter perf
    const perf = await p2.evaluate(async () => {
      const t = performance.now();
      const q = document.getElementById('f-q');
      const before = document.getElementById('summary-line').textContent;
      q.value = 'page'; q.dispatchEvent(new Event('input'));
      while (document.getElementById('summary-line').textContent === before) await new Promise((r) => setTimeout(r, 5));
      return performance.now() - t - 250;
    });
    check(perf < 800, `search over 10k+ visits settles quickly (~${Math.round(perf)} ms beyond the 250 ms debounce)`);
    await p2.close();
    check(requests.length === 0, `no network requests${requests.length ? ': ' + requests.slice(0, 3).join(' ') : ''}`);
    await ctx.close();

    // dark mode
    ctx = await launch(fs.mkdtempSync(path.join(tmp, 'p2-')), 'dark');
    const id2 = await extId(ctx);
    const pd = await ctx.newPage();
    watch(pd, errors, requests);
    await pd.goto(`chrome-extension://${id2}/ui/test-support/timeline-fixture.html`);
    await loaded(pd);
    await pd.check('#f-hide');
    await pd.waitForTimeout(300);
    await pd.screenshot({ path: path.join(shots, 'fixture-hidden-dark.png') });
    await pd.uncheck('#f-hide');
    await pd.waitForTimeout(300);
    await pd.screenshot({ path: path.join(shots, 'fixture-dark.png') });
    check(await pd.evaluate(() => getComputedStyle(document.body).backgroundColor) !== 'rgb(246, 247, 249)', 'dark colour scheme applied');
    await ctx.close();

    // real history
    const server = http.createServer((req, res) => { res.setHeader('content-type', 'text/html'); res.end(`<title>Local page ${req.url}</title><a id=n href="/next">next</a>`); }).listen(0, '127.0.0.1');
    await new Promise((r) => server.once('listening', r));
    const sbase = `http://127.0.0.1:${server.address().port}`;
    ctx = await launch(fs.mkdtempSync(path.join(tmp, 'p3-')), 'light');
    const id3 = await extId(ctx);
    const pg = await ctx.newPage();
    for (const p of ['/a', '/b', '/c', '/c', '/b']) { await pg.goto(sbase + p); await pg.waitForTimeout(120); }
    await pg.click('#n');
    await pg.waitForTimeout(400);
    const rt = await ctx.newPage();
    watch(rt, errors, requests);
    await rt.goto(`chrome-extension://${id3}/timeline.html`);
    await loaded(rt);
    const localRows = await rt.locator('#timeline .badge', { hasText: '127.0.0.1' }).count();
    check(localRows === 6, `real history: ${localRows} rows on 127.0.0.1 for 6 navigations (${await rows(rt)} rows in total)`);
    check(await rt.locator('#timeline a.vtitle').first().getAttribute('href') !== null && (await rt.locator('#timeline .vtitle').allTextContents()).some((t) => t.startsWith('Local page')), 'real titles/links rendered');
    await rt.click('#settings-btn');
    await rt.fill('#rules-text', JSON.stringify({ add: { privacy: { domains: ['127.0.0.1'] } } }));
    await rt.click('#rules-save');
    await rt.waitForFunction(() => document.querySelectorAll('#timeline .cat-privacy').length === 6);
    check(true, 'real history: user rule flags all local visits (privacy badge x6)');
    check(requests.length === 0 || requests.every((u) => u.startsWith(sbase)), 'no requests other than the local test server');
    const csp = errors.filter((e) => /Content Security Policy|CSP|Refused/i.test(e));
    check(errors.length === 0, `zero console errors/warnings and CSP violations${errors.length ? ': ' + errors.slice(0, 3).join(' | ') : ''}`);
    check(csp.length === 0, 'no CSP violations');
    server.close();
  } catch (e) {
    check(false, 'exception: ' + e.stack);
  } finally {
    await ctx.close().catch(() => {});
    console.log(results.join('\n'));
    console.log('screenshots:', shots);
    if (!process.env.KEEP) fs.rmSync(ext, { recursive: true, force: true });
    process.exit(results.some((r) => r.startsWith('FAIL')) ? 1 : 0);
  }
})();
