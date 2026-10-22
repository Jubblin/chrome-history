# Security policy

history-timeline reads browser history, which is personal data, so security and privacy reports are taken seriously.

## Reporting a vulnerability

Please report vulnerabilities privately using GitHub private vulnerability reporting: open the **Security** tab of [github.com/Jubblin/history-timeline](https://github.com/Jubblin/history-timeline) and choose **Report a vulnerability**.

- **Never open a public issue, pull request or discussion for a vulnerability.**
- **Never include real browsing history** in a report: no real URLs, page titles, `History` files, `Local State` files or profile directories. Reproduce the problem with the synthetic fixture generator instead (`make fixture`, see [CONTRIBUTING.md](CONTRIBUTING.md)) and describe the steps.
- Do include the commit you tested, your OS and browser, what you did, what you expected and what happened.

## Supported versions

There are no tagged releases yet. Only the latest commit on `main` is supported; please check that the problem still exists there before reporting.

## What to expect

This is a hobby project, so these are best-effort targets, not guarantees:

- acknowledgement within 7 days;
- a status update within 30 days;
- coordinated disclosure: please give us a reasonable chance to fix the problem before publishing details, and we will agree a disclosure date with you;
- credit in the fix or advisory if you would like it.

## Built-in protections

- Binds `127.0.0.1` only and rejects other `Host` headers (DNS rebinding).
- A random per-run access token is required on every request. It is printed once at startup, exchanged for an `HttpOnly`, `Secure`, `SameSite=Strict` cookie, compared in constant time, and cannot be disabled. API clients may send `X-History-Timeline-Token` or `Authorization: Bearer`. The cookie is marked `Secure`: Chromium-based browsers (Chrome, Edge) and Firefox accept and send it over plain `http://127.0.0.1` and `http://localhost`, but Safari does not send Secure cookies over http, so the web UI will not authenticate in Safari. Use a Chromium-based browser or Firefox, or call the API with the token header. Your browser's history may keep the one-time `?token=` URL, but the token is only valid for the lifetime of that server run.
- Cross-site requests (`Sec-Fetch-Site: cross-site` or a foreign `Origin`) are refused.
- Every response sets a strict `Content-Security-Policy` (no inline script or style), `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Cross-Origin-Resource-Policy: same-origin`, `nosniff` and `no-store`.
- The database is read from a private temporary copy, unlinked right after opening on Linux and macOS, removed on exit (`SIGINT`/`SIGTERM`), with stale `history-timeline-*` directories swept at startup. Symlinked or non-regular History files are refused.
- `/api/profiles` does not return the account email or directory names; visit counts are cached so the endpoint does not copy every database on each call.
- The rules file is limited to 1 MiB.

## Automated scanning

- `govulncheck` checks the code and dependencies against the Go vulnerability database on every push, pull request and weekly.
- CodeQL analyses the Go code and the JavaScript in `web/static` on the same schedule; results appear in the repository's code scanning alerts.
- Renovate opens grouped minor/patch update pull requests for Go modules and GitHub Actions.
- Workflows use read-only permissions by default, pin actions to commit SHAs, use no secrets and never run untrusted pull request code with write access. Release archives carry build provenance attestations and a `SHA256SUMS` file.

## Scope

In scope:

- the localhost HTTP server (`internal/server`): the `Host` header check against DNS rebinding, profile-ID handling and path traversal, access from other web pages (CORS and CSRF-style requests), and response headers;
- reading of profile files (`internal/history`): the read-only guarantee, temporary-copy handling and cleanup, and symlink and path handling;
- rendering of untrusted history strings (URLs and titles) in the UI, for example cross-site scripting;
- CSV export, for example spreadsheet formula injection;
- the rules-file parser (`internal/classify`);
- bypasses of the "hide sensitive details" masking, where flagged text, URLs or domains still reach the page, the CSV export or the API results when masking is on.

Chrome extension (`extension/`):

- cross-site scripting through history strings (titles, URLs, domains) in the extension pages, and any weakening of the extension-page CSP (`default-src 'none'`, no inline script, `connect-src 'none'`);
- anything that makes the extension send data off the device, load remote code, or request more permissions than `history` and `storage` (no host permissions);
- the user rules parser (`extension/src/classify`): size limit, prototype pollution, regular-expression denial of service;
- bypasses of "Hide sensitive details" in the page or the CSV export;
- the release zip: files that should not ship (test-only code, fixture mode) being reachable from it.

Out of scope:

- attacks that need local code execution or access to the same user account: the tool reads files that user can already read;
- findings in third-party dependencies without a demonstrated impact on this project (please report those upstream);
- inaccuracy of the heuristic category lists (missed or wrongly flagged sites). Ordinary bugs can be reported as public issues, and users can adjust the lists with `add` and `allow` rules (see the README).
