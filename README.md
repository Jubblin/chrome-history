# chrome-history

[![CI](https://github.com/Jubblin/chrome-history/actions/workflows/ci.yml/badge.svg)](https://github.com/Jubblin/chrome-history/actions/workflows/ci.yml)
[![Security](https://github.com/Jubblin/chrome-history/actions/workflows/security.yml/badge.svg)](https://github.com/Jubblin/chrome-history/actions/workflows/security.yml)

A local, read-only timeline viewer for Chromium-family browser history (Chrome, Chromium, Edge, Brave, Vivaldi, Opera).
It ships as one self-contained binary with the web UI embedded. No data leaves your machine.

## Quick start (try it on synthetic data)

Requires Go 1.22+.

```
make fixture      # writes fake history to ./fixture (no real data)
make run-fixture  # builds bin/chrome-history and serves the fixture on 127.0.0.1:8080
```

`make run-fixture` builds the binary and generates the fixture if missing, so on a fresh clone `make run-fixture` alone also works.
Open the URL the server prints (it ends in `/?token=...`; the token is random per run). Without it every request gets `401`.

## Build

| Command | Result |
|---|---|
| `make build` | `bin/chrome-history`, a static binary (`CGO_ENABLED=0`, pure-Go SQLite) |
| `make test` | `go vet` and `go test ./...` |
| `make cross` | binaries for linux/amd64, darwin/arm64, darwin/amd64, windows/amd64 in `dist/` |
| `make fixture` | generates `./fixture` (flags: `go run ./cmd/make-fixture -out DIR -days 90 -per-day 40 -seed 1`) |

## macOS setup

macOS denies terminal apps access to other apps' data under `~/Library/Application Support` until you grant it, so a first run against real profiles can fail with `permission denied`. These steps use the release archives (the exact menu names may differ between macOS versions; System Settings was reorganised in macOS 13).

1. Download the `darwin_arm64` (Apple Silicon) or `darwin_amd64` (Intel) archive and `SHA256SUMS` from the Releases page, then check and unpack it:
   ```
   grep darwin_arm64 SHA256SUMS | shasum -a 256 -c   # or darwin_amd64
   tar -xzf chrome-history_*_darwin_arm64.tar.gz   # or darwin_amd64
   cd chrome-history_*_darwin_*
   ```
   The `grep` limits the check to the archive you downloaded. Alternatively run `gh attestation verify <archive> --repo Jubblin/chrome-history`.
2. The binaries are unsigned and not notarised, so Gatekeeper may block the first run. Control-click the file and choose Open, or run `xattr -d com.apple.quarantine ./chrome-history`. The checksum or attestation from step 1 is how you know the file is genuine.
3. Run `./chrome-history --list-profiles` to see which profiles are found and why any directory was skipped.
4. If it reports `permission denied`, open System Settings > Privacy & Security > Full Disk Access, enable your terminal app (Terminal, iTerm, ...), quit it completely (Cmd-Q) and reopen it, then run again.

Alternative without Full Disk Access: quit the browser, copy only the profile's `History` file (and `History-wal` if it exists) into a folder you can read, and run `./chrome-history --profile /path/to/that/folder`. The profile folder is named `Default`, `Profile 1`, and so on; `chrome://version` shows the exact Profile Path.

The tool only reads the `History` database (see [Privacy](#privacy)), so you can turn Full Disk Access off again afterwards. The web UI works in Chromium-based browsers and Firefox but not in Safari, because the access cookie is `Secure` and Safari does not send it over plain http.

## Chromebook (ChromeOS)

Not tested on a Chromebook; this is based on how ChromeOS's Linux support is documented to work.

- **Works:** ChromeOS can run Linux apps in a container (Settings > Advanced > Developers > Linux development environment). Inside it, use the `linux_amd64` release archive, or `linux_arm64` on ARM models (`uname -m` prints `x86_64` or `aarch64`). It can audit a Chrome or other Chromium browser installed inside that container, or a `History` file you copy into the container and open with `--profile PATH`.
- **Does not work with this app:** the history of the Chrome you use on ChromeOS itself. To audit that, install the [Chrome extension](#chrome-extension) in ChromeOS Chrome instead; it reads history through Chrome's own `history` API, so it can see what the Linux container cannot. ChromeOS keeps that profile in encrypted per-user storage that the Linux container cannot read, and the Files app does not expose it. The same goes for Chrome on Android on a Chromebook. Don't weaken device security (for example with developer mode) to get at it.
- **Opening the UI:** the container has no browser, so run `./chrome-history --no-open` and open the printed URL, which contains the access token, in ChromeOS Chrome. ChromeOS normally forwards `localhost` ports from the container, so `http://localhost:8080/?token=...` should work. The server listens on `127.0.0.1` only and accepts only the hosts `localhost`, `127.0.0.1` and `[::1]`, so a container hostname such as `penguin.linux.test` is rejected. If forwarding misbehaves, keep a fixed port with `--port` (default 8080; `--port 0` picks a random one).
- **Google Takeout:** its Chrome history export is JSON, which this tool does not import.

## Chrome extension

The `extension/` folder is a Manifest V3 Chrome extension that shows the same kind of audit timeline (days and sessions, activity chart, top sites, filters, sensitive-category flags with a hide toggle, CSV export) for the Chrome profile it is installed in. It reads history with Chrome's `history` permission, makes no network requests (strict CSP, no host permissions) and stores only your optional custom rules in `chrome.storage.local`. Privacy policy: [`extension/store/PRIVACY.md`](extension/store/PRIVACY.md). It has not been published to the Chrome Web Store yet.

**Install unpacked:** step-by-step guide, with checksum verification, updating and troubleshooting: [`docs/INSTALL-EXTENSION.md`](docs/INSTALL-EXTENSION.md). In short: download and verify the release zip, unzip it into a new, EMPTY, permanent folder, then in `chrome://extensions` turn on Developer mode and choose **Load unpacked**. (From a checkout, `make extension-zip` builds the same zip.)

**ChromeOS:** this is the way to audit ChromeOS's own Chrome, which the Linux app cannot reach. Load it as above in ChromeOS Chrome (Developer mode in `chrome://extensions` is a Chrome setting, not ChromeOS developer mode, and does not weaken device security).

**Limits:**
- Current profile only (install it in each profile you want to audit); incognito visits are not recorded by Chrome and the extension does not run in incognito.
- No visit durations (Chrome's API does not provide them); days use your local time zone.
- Managed devices (school or work policies) may block unpacked or unlisted extensions, or the `history` permission.
- Not tested on a Chromebook; automated checks run in Playwright's Chromium.
- Large histories load progressively; the first screen appears quickly and totals fill in as loading finishes.

## Use it on your real history

```
bin/chrome-history
```

Profiles are auto-discovered from the default locations for your OS. Use `--profile PATH` to point at a profile
directory, a user-data directory, or a `History` file (for example a copy). Use the profile picker in the UI to switch
between profiles; it shows each profile's visit count.

### Profiles not found?

Run `chrome-history --list-profiles`. It prints each profile (browser, name, visit count; no emails, URLs or titles)
and every directory searched with the reason it was skipped (`not found`, `no History file`, `permission denied`),
then exits. If nothing is found, the normal start-up error prints the same list.

- **macOS: `permission denied`.** Grant your terminal Full Disk Access or copy the `History` file; see [macOS setup](#macos-setup).
- **`not found` for every browser:** the browser may use a custom profile location, a different `$HOME`, or a
  sandboxed/packaged install. Pass the profile (or its `History` file) with `--profile PATH`.
- Browsers are searched in the standard locations for the OS (macOS: `~/Library/Application Support/<browser>`).

## Privacy

- Read-only: the original `History` database is never opened for writing. It is copied (with its `-wal`/`-journal`) to a
  private temporary directory for each request and queried there. On Linux and macOS the copy is unlinked as soon as it is open, so a crash leaves no copy of your history on disk; it is always removed when the request ends, on `Ctrl-C`/`SIGTERM` (the server shuts down gracefully), and leftover `chrome-history-*` directories older than 10 minutes in the temp dir are swept at startup. Symlinks and other non-regular files are refused. This also works while the browser is running.
- Local-only: the server binds to `127.0.0.1` and rejects requests whose `Host` is not `localhost`, `127.0.0.1` or `::1`.
  The UI makes no external requests.
- Access token: each run generates a random token that must accompany every request. The URL printed at startup (and opened in your browser unless `--no-open`) carries it once; the server turns it into an `HttpOnly`, `Secure`, `SameSite=Strict` cookie and redirects to the plain URL. The cookie is marked `Secure`: Chromium-based browsers (Chrome, Edge) and Firefox accept and send it over plain `http://127.0.0.1` and `http://localhost`, but Safari does not send Secure cookies over http, so the web UI will not authenticate in Safari. Use a Chromium-based browser or Firefox, or call the API with the token header. API clients can send `X-Chrome-History-Token: <token>` or `Authorization: Bearer <token>`. There is no way to turn it off. Other web pages and other local users cannot read your history without it; treat the printed URL as secret (it also appears in the browser-launch command line while the browser starts). Your browser's history may keep the one-time `?token=` URL, but the token is only valid for the lifetime of that server run.
- Cross-site requests (`Sec-Fetch-Site: cross-site`, or an `Origin` other than the server's own) are refused with `403`. Responses carry a strict `Content-Security-Policy` (no inline script or style), `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer` and `Cross-Origin-Resource-Policy: same-origin`.
- The rules file is limited to 1 MiB.
- Logs contain only profile IDs and error text, never URLs, titles or paths.
- Do not commit real browser history to this repo; `fixture/` and `bin/` are git-ignored.

## Dev environment (devcontainer on k3s)

Development runs in a pod on the local k3s cluster through DevPod; see [.devcontainer/README.md](.devcontainer/README.md).

```
export KUBECONFIG=$HOME/.kube/config
make dev-up                 # create/start the pod (Go 1.22)
devpod ssh chrome-history   # shell in the pod; run make test / make build there
# serve from the pod and reach it on the host:
devpod ssh chrome-history --command 'cd /workspaces/chrome-history && make run-fixture'   # port 8080 is forwarded
# copy a built binary out (use a Linux build, or make cross for other OSes):
devpod ssh chrome-history --command 'cat /workspaces/chrome-history/dist/chrome-history-linux-amd64' > chrome-history && chmod +x chrome-history
make dev-down               # delete the pod, volume and namespace
```

The built binary is self-contained and runs on the host against real profiles; the pod only ever sees fixture data.

## Running from source

```
go run ./cmd/chrome-history [--profile PATH] [--port N] [--no-open]
```

Binds `127.0.0.1` only (default port 8080, `0` = random) and opens the browser unless `--no-open`.
`--profile` accepts a profile dir, a user-data dir, or a `History` file; without it, profiles are auto-discovered.
The UI in `web/static/` is embedded into the binary.

## JSON API

All endpoints are `GET`. Every request needs the access token (cookie, `X-Chrome-History-Token` or `Authorization: Bearer`); without it the answer is JSON `401`. Requests whose `Host` is not `localhost`, `127.0.0.1` or `::1`, and cross-site requests, get `403`.
Errors are JSON: `{"error": "message", "status": 400}` (400 bad parameter, 404 unknown profile/endpoint,
401 missing or wrong token, 403 bad Host or cross-site, 405 wrong method, 422 unsupported DB schema, 500 other).
Profile IDs are opaque tokens from `/api/profiles`; `profile` may be omitted when only one profile exists.
Times are accepted as RFC3339, `YYYY-MM-DD` (UTC; as `end` it includes that whole day) or epoch ms.
`end` is exclusive otherwise.

| Endpoint | Response |
|---|---|
| `/api/profiles` | `{"profiles":[{"id","browser","channel?","name","kind","visitCount?"}]}` (`visitCount` = total visits, cached until the History files change; omitted if the database cannot be read). The account email and directory names are not returned |
| `/api/timeline?profile=&start=&end=&q=&domain=&category=&limit=&offset=` | `{"visits":[{"id","time","timeMs","url","title","domain","transition","durationMs","fromVisit","categories"}],"total","limit","offset"}`, newest first; `limit` default 100, max 1000; `categories` is an array (empty if unflagged) |
| `/api/summary?profile=&start=&end=&category=&maskFlagged=` | `{"totalVisits","perDay":[{"date","visits","flagged?","flaggedVisits"}],"topDomains":[{"domain","visits","categories"}],"sessions":[{"start","startMs","end","endMs","visits"}],"sessionGapMinutes"}` (days in UTC, top 20 domains) |
| `/api/domains?profile=&start=&end=&maskFlagged=` | `{"domains":[{"domain","visits"}]}` sorted by visits |
| `/api/categories?profile=&start=&end=&maskFlagged=` | `{"categories":[{"id","label","visits"}],"flaggedVisits"}`: all six categories with visit counts in the date range (the `category` filter is ignored here) |

`category` is repeatable or comma-separated (`category=gambling,privacy`); a visit matches if it has any listed category. Valid IDs: `adult`, `finance`, `gambling`, `health`, `news_politics`, `privacy`; anything else is a `400`. In `perDay`, `flagged` maps category to visit count for that day and `flaggedVisits` counts visits with at least one category.

`maskFlagged=true` (the UI sends it while **Hide sensitive details** is on) stops the text search `q` and the `domain` filter from matching flagged visits' title, URL or domain, so a flagged visit never shows up (or is counted in `total`) because of a hidden value; flagged visits still appear through non-hidden fields such as date or `category`. It also leaves flagged visits out of `topDomains` and `/api/domains`. Without it (or `false`) behaviour is unchanged; any other value is a `400`.

`time`, `start`, `end` in responses are ISO-8601 UTC; the `*Ms` fields are epoch milliseconds.

## Sensitive-site flags

Visits are tagged locally with up to six categories: `adult`, `gambling`, `health`, `finance`, `privacy` (VPN, proxy and Tor sites; not incognito mode, which Chrome does not record) and `news_politics`. This is a **heuristic**: the bundled list (`internal/classify/rules.json`, one domain per line, sorted) favours precision over recall, so it misses many sites and can occasionally be wrong. Approximate size: adult ~80 domains, gambling ~120, health ~130, finance ~210, privacy ~80 and news_politics ~180 (about 800 in total), plus 3 or 4 keywords per category (none for news_politics). Domains are well-known registrable domains and match their subdomains; large shared platforms (search engines, social networks, video sites, cloud and CDN hosts) are deliberately not listed, and a small bundled allow list exempts a few benign subdomains and developer or encyclopedia hosts such as `wikipedia.org` and `github.com` from keyword hits. Bundled keywords only match a whole host word (split at `.` and `-`, so `poker-room.example` matches but `pokerface.example` does not) or a whole path segment (`/poker/` matches, `/poker-hand-evaluator` does not); your own keywords remain plain substrings. Coverage is strongest for UK, US and major EU names. Treat flags as hints, not a verdict, and fix mistakes with `add` and `allow` below. `news_politics` is the broadest and most subjective category (news outlets, parties, campaigns, advocacy groups). Classification uses only the URL, makes no network requests and logs no URLs.

In the UI, each flagged visit shows a badge per category (icon plus text), the category chips filter the timeline and the day-bar chart (striped part of a bar = flagged visits), and **Hide sensitive details** (remembered in the browser) replaces the title, URL and domain of flagged visits with a placeholder until you click Reveal. With it on, Search and Domain no longer match flagged visits' hidden values and flagged domains are left out of the top-domains list, and a note under the filters says sensitive visits are hidden and not searched while Search or Domain is in use; CSV export masks those columns as `[hidden]`; the CSV always has a `categories` column.

Edit the rules with a JSON file at `<user config dir>/chrome-history/rules.json` (or `--rules PATH`). A broken file stops startup with an error naming the file and the problem.

```json
{
  "add":   {"gambling": {"domains": ["lucky.example"], "keywords": ["lotto"]}},
  "allow": {"news_politics": {"domains": ["bbc.co.uk"]}}
}
```

`domains` match the domain and its subdomains. `keywords` (3+ characters) match anywhere in the host or path, not the query (unlike the bundled keywords, which must be whole words). `allow` removes matches from a category and wins over bundled rules and your own `add` rules.

## Licence

MIT. See [LICENSE](LICENSE). Contributions are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md). To report a vulnerability, see [SECURITY.md](SECURITY.md).

The SQLite driver, `modernc.org/sqlite`, is licensed under BSD-3-Clause.
