// Generates the 1280x800 Web Store screenshots from SYNTHETIC fixture data, loading the
// UNPACKED extension (not the zip) via its test-support fixture page. Sensitive-looking
// rows are masked with "Hide sensitive details" so no brand-name examples are shown.
// Usage: PLAYWRIGHT_BROWSERS_PATH=<dir> NODE_PATH=<dir>/node_modules node store/make-screenshots.cjs OUTDIR
const { chromium } = require('playwright-core');
const fs = require('fs'), os = require('os'), path = require('path');

const ext = path.resolve(__dirname, '..');
const out = path.resolve(process.argv[2] || path.join(__dirname, 'screenshots'));
fs.mkdirSync(out, { recursive: true });

async function open(scheme) {
  const ctx = await chromium.launchPersistentContext(fs.mkdtempSync(path.join(os.tmpdir(), 'shot-')), {
    headless: false, channel: 'chromium', colorScheme: scheme, viewport: { width: 1280, height: 800 },
    ignoreDefaultArgs: ['--disable-extensions'],
    args: ['--headless=new', '--no-sandbox', `--disable-extensions-except=${ext}`, `--load-extension=${ext}`,
      '--disable-features=DisableLoadExtensionCommandLineSwitch'],
  });
  let [sw] = ctx.serviceWorkers();
  if (!sw) sw = await ctx.waitForEvent('serviceworker', { timeout: 15000 });
  const page = await ctx.newPage();
  await page.goto(`chrome-extension://${new URL(sw.url()).host}/ui/test-support/timeline-fixture.html?clean=1`);
  await page.waitForFunction(() => /visits? loaded/.test(document.getElementById('status').textContent), null, { timeout: 60000 });
  await page.check('#f-hide');
  await page.waitForTimeout(400);
  return { ctx, page };
}
const shot = (page, name) => page.screenshot({ path: path.join(out, name) });

(async () => {
  let { ctx, page } = await open('light');
  await shot(page, '1-timeline-light.png');
  const chip = page.locator('#cat-chips button, #cat-chips input, #cat-chips label').first();
  if (await chip.count()) { await chip.click(); await page.waitForTimeout(500); await shot(page, '2-category-filter-light.png'); await chip.click(); }
  await page.click('#settings-btn');
  await page.fill('#rules-text', JSON.stringify({ add: { finance: { domains: ['mybank.example'] } }, allow: { news_politics: { domains: ['example.org'] } } }, null, 1));
  await page.waitForTimeout(300);
  await shot(page, '3-rules-settings-light.png');
  await ctx.close();
  ({ ctx, page } = await open('dark'));
  await shot(page, '4-timeline-dark.png');
  await ctx.close();
})().catch((e) => { console.error(e); process.exit(1); });
