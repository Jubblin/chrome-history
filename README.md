# chrome-history

A new project created with Intent.

## Running

```
go run ./cmd/chrome-history [--profile PATH] [--port N] [--no-open]
```

Binds `127.0.0.1` only (default port 8080, `0` = random) and opens the browser unless `--no-open`.
`--profile` accepts a profile dir, a user-data dir, or a `History` file; without it, profiles are auto-discovered.
The UI in `web/` is embedded into the binary.

## JSON API

All endpoints are `GET`. Requests whose `Host` is not `localhost`, `127.0.0.1` or `::1` get `403`.
Errors are JSON: `{"error": "message", "status": 400}` (400 bad parameter, 404 unknown profile/endpoint,
405 wrong method, 422 unsupported DB schema, 500 other).
Profile IDs are opaque tokens from `/api/profiles`; `profile` may be omitted when only one profile exists.
Times are accepted as RFC3339, `YYYY-MM-DD` (UTC; as `end` it includes that whole day) or epoch ms.
`end` is exclusive otherwise.

| Endpoint | Response |
|---|---|
| `/api/profiles` | `{"profiles":[{"id","browser","channel?","name","email?","dir","kind"}]}` |
| `/api/timeline?profile=&start=&end=&q=&domain=&limit=&offset=` | `{"visits":[{"id","time","timeMs","url","title","domain","transition","durationMs","fromVisit"}],"total","limit","offset"}`, newest first; `limit` default 100, max 1000 |
| `/api/summary?profile=&start=&end=` | `{"totalVisits","perDay":[{"date","visits"}],"topDomains":[{"domain","visits"}],"sessions":[{"start","startMs","end","endMs","visits"}],"sessionGapMinutes"}` (days in UTC, top 20 domains) |
| `/api/domains?profile=&start=&end=` | `{"domains":[{"domain","visits"}]}` sorted by visits |

`time`, `start`, `end` in responses are ISO-8601 UTC; the `*Ms` fields are epoch milliseconds.
