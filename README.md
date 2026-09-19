# usage-gauge

A self-hosted dashboard that queries and displays the usage / quota of multiple
services. A Go backend and a React / shadcn/ui frontend ship together as a
single static binary, with system-following light/dark themes and a responsive single-page layout.

- Periodically (every 5 minutes) fetches each configured endpoint in the
  background, parses the response with a per-endpoint JS parser, and stores the
  result in SQLite as a timestamped sample. Failures are also recorded, with no
  immediate retries; the next scheduled sample tries again.
- The page lists every endpoint top-to-bottom, shows a global "last updated"
  time, and reloads sampled data every 60 seconds without triggering upstream
  requests. Each card shows smooth quota curves and 3/6/12/24/48-hour presets.
  Returning to the tab reloads the latest sampled data immediately.
- Weekly quota cards emphasize the percentage left. Their bars shrink from
  full/green to empty/red as the quota is consumed. Historical charts continue
  to show used quota. Themes always follow the system, with no manual override.
- Samples older than 48 hours are removed at startup and every sampling round.
  Existing databases are migrated automatically, preserving their latest
  reading if it is still within the retention window. History accumulates from
  actual samples; past data is not backfilled.
- Failed/missing samples and quota resets break the curve instead of producing
  misleading zeroes or interpolated trends.

## Configuration

All runtime state lives under a single config directory (`CONFIG_DIR`,
defaults to `./config` locally and `/app/config` in the container):

```
config/
├─ endpoints.yaml     # list of endpoints (name, url, methods, headers, ...)
├─ gauge.db           # SQLite samples + latest results (auto-created)
└─ parser/            # optional per-endpoint parsers (override built-ins)
    └─ zai.js
```

See [`endpoints.example.yaml`](./endpoints.example.yaml). Each endpoint:

| field      | required | notes |
|------------|----------|-------|
| `name`     | yes      | display name + default parser name |
| `url`      | yes      | endpoint URL |
| `methods`  | no       | HTTP method (defaults to `GET`) |
| `headers`  | no       | request headers; zai requires Authorization, Codex does not |
| `parser`   | no       | parser name (defaults to `name`) |
| `timeoutMs`| no       | request timeout (default 10000; Codex 35000) |

For Codex, only the name and bridge URL are needed:

```yaml
endpoints:
  - name: codex
    url: http://127.0.0.1:55666/api/usage
```

For multiple accounts or a custom display name, set `parser: codex` explicitly.
Endpoint names must be unique. The Codex bridge must be reachable from the
machine/container running the dashboard.

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
      { name: "five_hour", label: "5-hour window", utilization: 42.5, resetsAt: "2026-07-08T02:03:00.000Z" }
    ],
    queriedAt: new Date().getTime()
  };
}
```

Resolution order: `config/parser/<name>.js` (your override / new endpoint) → the
built-in parser of the same name. Built-in `zai` and `codex` parsers ship with
the binary. Tier `name` is a stable series identifier; optional `label` controls
display text. Codex supports both the legacy single bucket and the multi-bucket
rate-limit response, including five-hour and weekly windows. See
[`examples/parser/zai.js`](./examples/parser/zai.js) for a reference.

## Run locally

Requires Go 1.25+. The default backend serves only `/api/usage`; it does not
embed or serve frontend assets and does not require a `dist` directory.

```bash
mkdir -p config
cp endpoints.example.yaml config/endpoints.yaml   # fill in real keys
go run ./cmd/usage-gauge
# API: http://localhost:3000/api/usage
```

Environment variables:

| var                   | default      | description |
|-----------------------|--------------|-------------|
| `CONFIG_DIR`          | `./config`   | config directory |
| `REFRESH_INTERVAL_MS` | `300000`     | background refresh interval |
| `PORT`                | `3000`       | HTTP listen port |

## Frontend development

Run the frontend separately (Node.js 24+):

```bash
npm --prefix web ci
npm --prefix web run dev
# open http://localhost:5173
```

Vite serves the page and proxies `/api` to the Go backend on port 3000. Neither
process requires a frontend build during development.

Vite's production output is `web/dist/`, ignored by Git and excluded from the
Docker build context. Docker builds it in the Node stage, copies it into the Go
stage, then compiles with `-tags production`. Only that build tag enables
`web/embed.go` and frontend static routes. Commit source and lockfile changes,
never dist. No local production build is required for the Docker workflow.

The UI uses shadcn Card, Button, Badge, Skeleton and Chart components,
with Recharts monotone area curves. No CDN resources are required at runtime.

Validation:

```bash
go test -race ./...
go vet ./...
npm --prefix web test
cd web
npx playwright install chromium
npm run test:e2e
```

Browser tests cover time presets, polling, themes, mobile layout, errors and empty states
using fixture data; they do not query real accounts.

## Run with Docker

```bash
docker build -t usage-gauge .

docker run -d --name usage-gauge -p 3000:3000 \
  -v "$PWD/config":/app/config usage-gauge
```

Docker builds the frontend before compiling the Go binary.

The image never contains `config/` (it is in `.dockerignore`), so real keys stay
on the host. `gauge.db` is persisted in the mounted volume.

For a Codex bridge running on the Docker host, `127.0.0.1` inside the container
is not the host. Run the bridge with an address reachable from the container
(e.g. `-listen :55666`), use `http://host.docker.internal:55666/api/usage`, and on
Linux add `--add-host=host.docker.internal:host-gateway` to `docker run`.

## Endpoints

### Local Codex quota bridge

Run the separate bridge on a machine with `codex` installed and already logged
in with a ChatGPT account (`codex login`):

```bash
go run ./cmd/codex-usage
curl http://127.0.0.1:55666/api/usage
```

The command starts `codex app-server` with a fresh directory under the system
temporary directory as its working directory. It inherits the local Codex
configuration and login, completes the RPC handshake, and keeps the process
running. Every `GET /api/usage` calls `account/rateLimits/read` and returns its
raw `result` JSON (including `rateLimits` and, when available,
`rateLimitsByLimitId`). It does not start a model conversation or cache results.
See the [official App Server protocol](https://developers.openai.com/zh-Hans/docs/app-server).

Queries are serialized and have a 30-second timeout including queueing.
Codex errors return HTTP 502 with `{"error":"..."}`; timeouts return HTTP 504.
After a failed query the next request starts a fresh Codex process.
SIGINT/SIGTERM stops the child process and removes the temporary directory.

Flags: `-listen 127.0.0.1:55666`, `-codex codex`, `-timeout 30s`.
Use `-listen :55666` to allow access from other hosts or containers; this
endpoint has no authentication, so only expose it on a trusted network.
The built-in `codex` parser maps the quota windows to the dashboard format.
This bridge is separate from the dashboard and its Docker image.

### Dashboard

- `GET /` — the dashboard page (production builds only; use Vite in development).
- `GET /api/usage` — JSON containing `serverTime`, `lastUpdatedAt`,
  `sampleIntervalMs`, `retentionHours` and `endpoints`. Each endpoint contains
  `name`, `provider`, `latest` (or `null`) and its ordered `history` samples.
  Timestamps are Unix milliseconds. Private endpoint URLs and headers are not exposed.
- `GET /assets/{file}` — embedded CSS/JS (production builds only).

The usage API now returns structured data rather than the old HTML fragment.
