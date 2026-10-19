# Contributing

Thanks for helping. Bug reports, fixes, UI improvements, documentation and better category lists are all welcome.

## Privacy first

This tool reads browser history, which is personal data. **Never paste real browser history, URLs, page titles, `History` files or profile directories into issues, pull requests, commits or test data.** Redact anything that came from a real profile.

To reproduce a problem or write a test, use the synthetic fixture generator instead:

```
make fixture      # writes fake history to ./fixture
go run ./cmd/make-fixture -out DIR -days 90 -per-day 40 -seed 1
```

`fixture/` and `bin/` are git-ignored; keep it that way.

## Reporting security issues

Do not open a public issue for a vulnerability. See [SECURITY.md](SECURITY.md) for how to report one privately.

## Reporting bugs

Open an issue with your OS, browser and version, how you ran the tool, what you expected and what happened. Error text from the log is fine (logs contain only profile IDs and error messages). If a bug needs data to reproduce, describe it or generate it with `cmd/make-fixture`.

## Development setup

Option 1: the devcontainer on a local k3s cluster, through DevPod (see [.devcontainer/README.md](.devcontainer/README.md)):

```
export KUBECONFIG=$HOME/.kube/config
make dev-up       # create/start the pod (Go 1.22)
make dev-shell    # shell into it
make dev-down     # delete the pod, volume and namespace
```

Option 2: install Go 1.22 or newer locally and run the commands below directly.

## Build, test and lint

| Command | Purpose |
|---|---|
| `make build` | build `bin/chrome-history` (static, `CGO_ENABLED=0`) |
| `make test` | `go vet ./...` and `go test -count=1 ./...` |
| `make cross` | build linux/amd64, darwin/arm64, darwin/amd64 and windows/amd64 binaries into `dist/` |
| `gofmt -l .` | must print nothing |
| `go vet ./...` | must be clean |
| `go test -race ./...` | must pass |

### Chrome extension

The extension in `extension/` needs Node 22+ on the host and has no npm dependencies.

| Command | Purpose |
|---|---|
| `make extension-test` | unit tests (`node --test`), including manifest checks and the built-zip checks |
| `make extension-check` | fails if the extension's copy of `internal/classify/rules.json` is stale (`make extension-sync` fixes it) |
| `make extension-lint` | manifest lint |
| `make extension-zip` | reproducible zip in `dist/` |

Browser checks (`extension/scripts/chrome-check.cjs`, `ui-check.cjs`, `zip-check.cjs`) need `playwright-core` and its full Chromium (branded Chrome ignores `--load-extension`); see `extension/README.md`. They use synthetic fixture data only. Never put real history in extension tests or screenshots. Anything for tests (stub classifier, mock history, fixture page) lives in `extension/ui/test-support/`, which must never be shipped or imported from shipped code; the zip test enforces that.

Try the UI with `make run-fixture` and open the URL it prints (it contains the per-run access token). With curl, send `-H "X-Chrome-History-Token: <token>"`. The token cookie is `Secure`, so use a Chromium-based browser or Firefox for the UI (Safari does not send Secure cookies over http); the cookie must stay `Secure`.

### Which CI jobs run

A first job, `changes`, looks at the files a pull request (or a push to `main`) touches and skips work that cannot be affected. It uses job-level conditions rather than workflow `paths:` filters, so every workflow always reports its checks.

| Changed path | Go jobs (lint, test, cross-build, govulncheck, CodeQL go) | Extension job | CodeQL javascript-typescript |
|---|---|---|---|
| `**/*.go`, `go.mod`, `go.sum`, `cmd/**`, `internal/**` | yes | no | no |
| `web/**` | yes | no | yes |
| `extension/**` (except the docs below) | no | yes | yes |
| `Makefile`, `.gitattributes`, `.github/**`, `internal/classify/rules.json`, `internal/classify/vectors_test.go` | yes | yes | yes |
| root `*.md`, `LICENSE`, `.devcontainer/**`, `extension/store/**`, `extension/**/*.md` | no | no | no |
| anything else, or an empty or unknown diff | yes | yes | yes |

Scheduled and manual runs, new branches and force pushes run everything. The release workflow (tags `v*`) always builds and tests everything.

`ci-ok` (in CI) and `security-ok` (in Security) always run and pass when every job they depend on passed or was skipped, so each is a single stable check name to require in branch protection. The rules live in `.github/scripts/changes.sh`; `.github/scripts/changes_test.sh` tests them (run `sh .github/scripts/changes_test.sh`) and CI runs it, so update both when you add a new kind of path.

## Project layout

- `cmd/chrome-history`: the server binary and its flags.
- `cmd/make-fixture`: synthetic history generator.
- `internal/history`: profile discovery, read-only copy of the `History` database, queries and aggregations.
- `internal/classify`: sensitive-site categories and the bundled rules (`rules.json`).
- `internal/server`: the localhost HTTP server and JSON API.
- `web/`: the embedded UI (`web/static/`).

## Design principles

- Read-only: never open or modify the original `History` database; work on a temporary copy that is deleted afterwards (and unlinked right after opening where the OS allows). Refuse symlinks.
- Localhost only: bind `127.0.0.1` and reject other `Host` headers and cross-site requests, and require the per-run access token on every request (it must stay impossible to disable). Keep the strict CSP: no inline script or style, no `unsafe-inline`. No external requests from the server or the UI.
- Classification is offline and uses only the URL. Do not add network calls, and never log URLs, titles or paths.
- Dependencies: the Go standard library plus the single SQLite driver (`modernc.org/sqlite`). Discuss before adding another.
- UI: render history data with `textContent` only (never `innerHTML`); history titles and URLs are untrusted input.
- JSON field names are camelCase.

## Improving the category lists

The bundled lists in `internal/classify/rules.json` are a heuristic that favours precision over recall.

- Propose well-known sites only, as registrable domains (they match subdomains).
- Prefer domain rules over keywords. Keywords are rarely worth the false positives.
- Do not list large shared platforms such as search engines, social networks, video sites, or cloud and CDN hosts.
- Keep each list sorted and free of duplicates, and add or update tests in `internal/classify`.
- Users can fix misclassifications themselves with `add` and `allow` rules (see the README), so a borderline entry is better left out.

## Releasing

Releases are cut from `main` by pushing a version tag:

```sh
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

The `Release` workflow (`.github/workflows/release.yml`) then runs gofmt, `go vet` and `go test -race`, builds static (`CGO_ENABLED=0`, `-trimpath`) binaries for Linux, macOS and Windows on amd64 and arm64, and packages each as `.tar.gz` (`.zip` on Windows) together with `LICENSE` and `README.md`. It also builds the Chrome extension zip (`chrome-history-extension_vX.Y.Z.zip`; the tag must equal the manifest version), writes a `SHA256SUMS` file covering it too, adds build provenance attestations, and creates a **draft** GitHub Release with generated notes, which you publish by hand. The tag is injected as the version, so `chrome-history --version` prints it.

Once the workflow succeeds:

1. Open the draft under Releases and check that all archives and `SHA256SUMS` are attached and the generated notes read well.
2. Click **Publish release** in the GitHub UI, or run `gh release edit v1.2.3 --draft=false`.

Re-running the workflow for a tag whose release is still a draft replaces the draft's assets; if the release is already published the job fails with a clear message and changes nothing. Attestations are created at build time for the archives and `SHA256SUMS`, so the draft state does not affect them. Check a download with `sha256sum -c SHA256SUMS` or `gh attestation verify <file> --repo Jubblin/chrome-history`.

CI (`ci.yml`) and the security scans (`security.yml`) run on every pull request. Workflow actions are pinned to full commit SHAs and updated by Dependabot.

## Pull requests

Before opening a pull request:

- [ ] `gofmt -l .` prints nothing, `go vet ./...` and `go test -race ./...` pass
- [ ] `make build` succeeds (and `make cross` if you touched build or platform code)
- [ ] new behaviour has tests; UI changes were checked against `make run-fixture`
- [ ] README updated if flags, API or behaviour changed
- [ ] no real browser data anywhere in the change
- [ ] small, focused commits with clear messages

By contributing you agree that your contributions are licensed under the MIT licence (see [LICENSE](LICENSE)).
