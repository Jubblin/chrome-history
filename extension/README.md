# Chrome extension (Manifest V3)

An audit timeline for the browsing history of the profile it is installed in. It reads history through `chrome.history`, makes no network requests, and requests only the `history` permission. Incognito browsing is not recorded by Chrome and is not accessible to the extension.

Status: scaffold, data layer and timeline UI. The classifier and packaging come in separate changes.

## Load unpacked
1. Open `chrome://extensions`, enable Developer mode.
2. Choose "Load unpacked" and select this `extension/` folder.
3. Click the toolbar button to open the timeline in a tab.

## Layout
- `manifest.json` MV3 manifest, strict CSP (`default-src 'none'; script-src 'self'; ...; connect-src 'none'`).
- `src/background.js` minimal service worker (opens the timeline tab).
- `src/history/` data layer (ES modules, no dependencies):
  - `loadVisits(chrome.history, {startTime, endTime, pageSize, concurrency, signal, onBatch, onProgress})`
    pages `history.search` by time and calls `getVisits` per URL with bounded concurrency. Returns visits
    `{visitId, time, url, title, domain, transition, referringVisitId, isLocal}`, newest first. Cancel with an
    `AbortSignal`; the rejection is an `AbortError` whose `.visits` holds what was loaded. There is no visit duration in the API.
  - `visitsPerDay`, `topDomains`, `groupSessions` (default gap 30 minutes, inclusive) match the Go app.
    **Days use the browser's local time zone.**
  - `filterVisits` (date range, text in url/title, domain or parent domain, transition types).
  - `createMockHistory(entries)` in-memory `chrome.history` for tests and offline/fixture mode.
- `scripts/make-fixture.js` deterministic synthetic history generator (10k+ visits).

## Tests
- `make extension-test` (from the repo root) runs `node:test` unit tests; Node 22+, no dependencies. Includes manifest checks
  (MV3, permissions, CSP, no network/eval).
- `scripts/chrome-check.cjs` loads the unpacked extension in headless Chromium with a throwaway profile, browses local pages to
  create real history, and checks the service worker and data layer. Needs `playwright-core` and its Chromium
  (branded Chrome ignores `--load-extension`):
  `PLAYWRIGHT_BROWSERS_PATH=<dir> NODE_PATH=<dir>/node_modules node scripts/chrome-check.cjs`.

## Timeline UI (`ui/`)
- `timeline.html` + `ui/app.js` (view) + `ui/model.js` (pure, unit-tested filtering/layout/CSV) + `ui/timeline.css` (copied from `web/static/app.css`, plus extension additions).
- Loads all history progressively via `loadVisits`, classifies each visit with the classifier (single integration point: `ui/classify-import.js`, which re-exports `src/classify/index.js`), and renders day/session groups 200 rows at a time.
- User rules are edited in the Settings panel and stored as raw text in `chrome.storage.local` under `userRulesText` (needs the `storage` permission; without it they only last for the page's lifetime).
- Fixture mode: open `timeline.html?fixture=1` to run against a synthetic 12k-visit mock history (`ui/test-support/`, test-only, must be excluded from the shipped zip; without those files the page reports fixture mode is unavailable).
- `scripts/ui-check.cjs` drives the UI in headless Chromium (fixture mode, then real history from local pages), writes screenshots of synthetic data, and checks masking, CSV, rules, keyboard, CSP and network. Same Playwright setup as `chrome-check.cjs`.
