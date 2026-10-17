# chrome-history

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

## Use it on your real history

```
bin/chrome-history
```

Profiles are auto-discovered from the default locations for your OS. Use `--profile PATH` to point at a profile
directory, a user-data directory, or a `History` file (for example a copy). Use the profile picker in the UI to switch
between profiles; it shows each profile's visit count.

## Privacy

- Read-only: the original `History` database is never opened for writing. It is copied (with its `-wal`/`-journal`) to a
  private temporary directory for each request and queried there. On Linux and macOS the copy is unlinked as soon as it is open, so a crash leaves no copy of your history on disk; it is always removed when the request ends, on `Ctrl-C`/`SIGTERM` (the server shuts down gracefully), and leftover `chrome-history-*` directories older than 10 minutes in the temp dir are swept at startup. Symlinks and other non-regular files are refused. This also works while the browser is running.
- Local-only: the server binds to `127.0.0.1` and rejects requests whose `Host` is not `localhost`, `127.0.0.1` or `::1`.
  The UI makes no external requests.
- Access token: each run generates a random token that must accompany every request. The URL printed at startup (and opened in your browser unless `--no-open`) carries it once; the server turns it into an `HttpOnly`, `SameSite=Strict` cookie and redirects to the plain URL. API clients can send `X-Chrome-History-Token: <token>` or `Authorization: Bearer <token>`. There is no way to turn it off. Other web pages and other local users cannot read your history without it; treat the printed URL as secret (it also appears in the browser-launch command line while the browser starts). Your browser's history may keep the one-time `?token=` URL, but the token is only valid for the lifetime of that server run.
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
