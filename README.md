# usage-gauge

A self-hosted dashboard that queries and displays the usage / quota of multiple
services. The backend is written in Go; the frontend is server-rendered HTML +
a sprinkle of JS. Ships as a single static binary and supports light/dark mode.

- Periodically (every 5 minutes) fetches each configured endpoint in the
  background, parses the response with a per-endpoint JS parser, and stores the
  result in SQLite. Failures are recorded but never retried.
- The page lists every endpoint top-to-bottom, shows a global "last updated"
  time, and refreshes from the cache every 60 seconds.

## Configuration

All runtime state lives under a single config directory (`CONFIG_DIR`,
defaults to `./config` locally and `/app/config` in the container):

```
config/
├─ endpoints.yaml     # list of endpoints (name, url, methods, headers, ...)
├─ gauge.db           # SQLite cache (auto-created)
└─ parser/            # optional per-endpoint parsers (override built-ins)
    └─ zai.js
```

See [`endpoints.example.yaml`](./endpoints.example.yaml). Each endpoint:

| field      | required | notes |
|------------|----------|-------|
| `name`     | yes      | display name + default parser name |
| `url`      | yes      | endpoint URL |
| `methods`  | yes      | HTTP method, e.g. `GET` |
| `headers`  | yes      | request headers (Authorization lives here) |
| `parser`   | no       | parser name (defaults to `name`) |
| `timeoutMs`| no       | request timeout (default 10000) |

### Parsers

Each endpoint's response JSON is mapped to a common shape by a parser. Parsers
are plain JS files exposing a top-level `function parse(body, ctx)` and are run
by the embedded [goja](https://github.com/dop251/goja) engine (use ES5.1).

```js
function parse(body, ctx) {
  // body: parsed JSON object (or null if the body was not JSON)
  // ctx:  { httpStatus, rawBody, endpoint: { name, url, methods } }
  return {
    status: "ok",            // "ok" | "expired" | "error"
    message: "plan name",    // optional display text
    tiers: [                 // usage windows
      { name: "five_hour", utilization: 42.5, resetsAt: "2026-07-08T02:03:00.000Z" }
    ],
    queriedAt: new Date().getTime()
  };
}
```

Resolution order: `config/parser/<name>.js` (your override / new endpoint) → the
built-in parser of the same name. A built-in `zai` parser ships with the binary,
so the zai endpoint works out of the box. See
[`examples/parser/zai.js`](./examples/parser/zai.js) for a reference.

## Run locally

Requires Go 1.25+.

```bash
cp endpoints.example.yaml config/endpoints.yaml   # fill in real keys
go run ./cmd/usage-gauge
# open http://localhost:3000
```

Environment variables:

| var                   | default      | description |
|-----------------------|--------------|-------------|
| `CONFIG_DIR`          | `./config`   | config directory |
| `REFRESH_INTERVAL_MS` | `300000`     | background refresh interval |
| `PORT`                | `3000`       | HTTP listen port |

## Run with Docker

```bash
docker build -t usage-gauge .

docker run -d --name usage-gauge -p 3000:3000 \
  -v "$PWD/config":/app/config usage-gauge
```

The image never contains `config/` (it is in `.dockerignore`), so real keys stay
on the host. `gauge.db` is persisted in the mounted volume.

## Endpoints

- `GET /` — the dashboard page.
- `GET /api/usage` — `{ lastUpdatedAt, lastUpdatedText, html }` (polled by the page).
- `GET /static/{file}` — embedded CSS/JS.
