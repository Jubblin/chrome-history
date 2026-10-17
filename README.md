# chrome-history

A local, read-only timeline viewer for Chromium-family browser history (Chrome, Chromium, Edge, Brave, Vivaldi, Opera).
It ships as one self-contained binary with the web UI embedded. No data leaves your machine.

## Quick start (try it on synthetic data)

Requires Go 1.22+.

```
make fixture      # writes fake history to ./fixture (no real data)
make run-fixture  # builds bin/chrome-history and serves the fixture at http://127.0.0.1:8080/
```

`make run-fixture` builds the binary and generates the fixture if missing, so on a fresh clone `make run-fixture` alone also works.
Open http://127.0.0.1:8080/.

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
  private temporary directory for each request, queried there, and the copy is deleted afterwards. This also works while the browser is running.
- Local-only: the server binds to `127.0.0.1` and rejects requests whose `Host` is not `localhost`, `127.0.0.1` or `::1`.
  The UI makes no external requests.
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

All endpoints are `GET`. Requests whose `Host` is not `localhost`, `127.0.0.1` or `::1` get `403`.
Errors are JSON: `{"error": "message", "status": 400}` (400 bad parameter, 404 unknown profile/endpoint,
405 wrong method, 422 unsupported DB schema, 500 other).
Profile IDs are opaque tokens from `/api/profiles`; `profile` may be omitted when only one profile exists.
Times are accepted as RFC3339, `YYYY-MM-DD` (UTC; as `end` it includes that whole day) or epoch ms.
`end` is exclusive otherwise.

| Endpoint | Response |
|---|---|
| `/api/profiles` | `{"profiles":[{"id","browser","channel?","name","email?","dir","kind","visitCount?"}]}` (`visitCount` = total visits; omitted if the database cannot be read) |
| `/api/timeline?profile=&start=&end=&q=&domain=&limit=&offset=` | `{"visits":[{"id","time","timeMs","url","title","domain","transition","durationMs","fromVisit"}],"total","limit","offset"}`, newest first; `limit` default 100, max 1000 |
| `/api/summary?profile=&start=&end=` | `{"totalVisits","perDay":[{"date","visits"}],"topDomains":[{"domain","visits"}],"sessions":[{"start","startMs","end","endMs","visits"}],"sessionGapMinutes"}` (days in UTC, top 20 domains) |
| `/api/domains?profile=&start=&end=` | `{"domains":[{"domain","visits"}]}` sorted by visits |

`time`, `start`, `end` in responses are ISO-8601 UTC; the `*Ms` fields are epoch milliseconds.
