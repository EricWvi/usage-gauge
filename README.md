# usage-gauge

一个可自行托管的仪表盘，用于查询并显示多个服务的用量和配额。Go 后端与 React / shadcn/ui 前端会一起编译为单个静态二进制文件，并提供跟随系统设置的浅色/深色主题以及响应式单页布局。

- 后台定期（每 5 分钟）请求每个已配置的端点，使用端点专属的 JS 解析器解析响应，并将结果作为带时间戳的采样记录保存到 SQLite。请求失败也会被记录，且不会立即重试；下一次计划采样时会再次尝试。
- 页面从上到下列出所有端点，显示全局“上次更新时间”，并每 60 秒重新加载采样数据，不会因此触发上游请求。每张卡片显示平滑的配额曲线，并提供 3/6/12/24/48 小时预设。返回浏览器标签页时会立即重新加载最新采样数据。
- 每周配额卡片突出显示剩余百分比。随着配额消耗，进度条会从满格/绿色缩小为空/红色。历史图表仍显示已使用配额。主题始终跟随系统，不提供手动切换。
- 启动时以及每轮采样时都会删除早于 48 小时的采样记录。现有数据库会自动迁移；如果最新读数仍在保留期限内，则会保留该读数。历史数据来自实际采样，不会补录过去的数据。
- 失败或缺失的采样，以及配额重置，都会让曲线断开，避免显示具有误导性的零值或插值趋势。

## 配置

所有运行时状态都位于同一个配置目录下（`CONFIG_DIR`；本地默认为 `./config`，容器中默认为 `/app/config`）：

```
config/
├─ endpoints.yaml     # 端点列表（名称、URL、方法、请求头等）
├─ gauge.db           # SQLite 采样数据和最新结果（自动创建）
└─ parser/            # 可选的端点专属解析器（覆盖内置解析器）
    └─ zai.js
```

请参阅 [`endpoints.example.yaml`](./endpoints.example.yaml)。每个端点支持以下字段：

| 字段       | 必填 | 说明 |
|------------|----------|-------|
| `name`     | 是   | 显示名称，也作为默认解析器名称 |
| `url`      | 是   | 端点 URL |
| `methods`  | 否   | HTTP 方法（默认为 `GET`） |
| `headers`  | 否   | 请求头；zai 需要 Authorization，Codex 不需要 |
| `parser`   | 否   | 解析器名称（默认为 `name`） |
| `timeoutMs`| 否   | 请求超时时间（默认 10000；Codex 为 35000） |

对于 Codex，只需要名称和桥接服务 URL：

```yaml
endpoints:
  - name: codex
    url: http://127.0.0.1:55666/api/usage
```

如果有多个账号或需要自定义显示名称，请显式设置 `parser: codex`。端点名称必须唯一。运行仪表盘的机器或容器必须能够访问 Codex 桥接服务。

### 解析器

每个端点的响应 JSON 都会由解析器映射为统一的数据结构。解析器是普通的 JS 文件，需要提供顶层函数 `function parse(body, ctx)`，并由内置的 [goja](https://github.com/dop251/goja) 引擎执行（请使用 ES5.1 语法）。

```js
function parse(body, ctx) {
  // body：已解析的 JSON 对象；如果响应不是 JSON，则为 null
  // ctx：{ httpStatus, rawBody, endpoint: { name, url, methods } }
  return {
    status: "ok",            // "ok" | "expired" | "error"
    message: "plan name",    // 可选的显示文本
    tiers: [                 // 用量窗口
      { name: "five_hour", label: "5-hour window", utilization: 42.5, resetsAt: "2026-07-08T02:03:00.000Z" }
    ],
    queriedAt: new Date().getTime()
  };
}
```

解析器的查找顺序为：`config/parser/<name>.js`（你的覆盖解析器或新端点解析器）→ 同名的内置解析器。二进制文件内置 `zai` 和 `codex` 解析器。`tier` 的 `name` 是稳定的序列标识符；可选的 `label` 控制显示文本。Codex 同时支持旧版单配额桶和多配额桶速率限制响应，包括 5 小时和每周窗口。参考示例请参阅 [`examples/parser/zai.js`](./examples/parser/zai.js)。

## 本地运行

需要 Go 1.25+。默认后端只提供 `/api/usage`，不会嵌入或提供前端资源，也不需要 `dist` 目录。

```bash
mkdir -p config
cp endpoints.example.yaml config/endpoints.yaml   # 填入真实密钥
go run ./cmd/usage-gauge
# API：http://localhost:3000/api/usage
```

本地构建的 Go 二进制文件统一输出到 `build/`：

```bash
task build:usage-gauge
task build:codex-usage
```

环境变量：

| 变量                  | 默认值       | 说明 |
|-----------------------|--------------|-------------|
| `CONFIG_DIR`          | `./config`   | 配置目录 |
| `REFRESH_INTERVAL_MS` | `300000`     | 后台刷新间隔 |
| `PORT`                | `3000`       | HTTP 监听端口 |

## 前端开发

单独运行前端（Node.js 24+）：

```bash
npm --prefix web ci
npm --prefix web run dev
# 打开 http://localhost:5173
```

Vite 提供页面，并将 `/api` 代理到 3000 端口上的 Go 后端。开发期间两个进程都不需要构建前端。

Vite 的生产构建输出位于 `web/dist/`，该目录已被 Git 忽略，也会被排除在 Docker 构建上下文之外。Docker 会在 Node 阶段构建前端，将构建结果复制到 Go 阶段，然后使用 `-tags production` 编译。只有这个构建标签会启用 `web/embed.go` 和前端静态路由。请提交源代码和锁文件的变更，不要提交 dist。Docker 工作流不要求本地进行生产构建。

UI 使用 shadcn 的 Card、Button、Badge、Skeleton 和 Chart 组件，并使用 Recharts 绘制单调面积曲线。运行时不需要 CDN 资源。

验证：

```bash
go test -race ./...
go vet ./...
npm --prefix web test
cd web
npx playwright install chromium
npm run test:e2e
```

浏览器测试使用固定数据覆盖时间预设、轮询、主题、移动端布局、错误和空状态；不会查询真实账号。

## 使用 Docker 运行

```bash
docker build -t usage-gauge .

docker run -d --name usage-gauge -p 3000:3000 \
  -v "$PWD/config":/app/config usage-gauge
```

Docker 会在编译 Go 二进制文件前构建前端。

镜像中不会包含 `config/`（它已列入 `.dockerignore`），因此真实密钥会留在宿主机上。`gauge.db` 会持久化在挂载的卷中。

如果 Codex 桥接服务运行在 Docker 宿主机上，容器内的 `127.0.0.1` 并不是宿主机。请使用容器可以访问的地址运行桥接服务（例如 `-listen :55666`），使用 `http://host.docker.internal:55666/api/usage`，并在 Linux 上为 `docker run` 添加 `--add-host=host.docker.internal:host-gateway`。

## 端点

### 本地 Codex 配额桥接服务

在已安装 `codex` 且已使用 ChatGPT 账号登录（`codex login`）的机器上运行独立桥接服务：

```bash
go run ./cmd/codex-usage
curl http://127.0.0.1:55666/api/usage
```

该命令会在系统临时目录下创建一个全新的目录作为工作目录，并启动 `codex app-server`。它会继承本地 Codex 配置和登录状态，完成 RPC 握手后保持进程运行。每次 `GET /api/usage` 都会调用 `account/rateLimits/read`，并返回原始的 `result` JSON（包括 `rateLimits`，以及可用时的 `rateLimitsByLimitId`）。它不会启动模型对话，也不会缓存结果。请参阅[官方 App Server 协议](https://developers.openai.com/zh-Hans/docs/app-server)。

查询请求会串行执行，排队和请求处理总超时时间为 30 秒。Codex 错误会返回 HTTP 502 和 `{"error":"..."}`；超时会返回 HTTP 504。查询失败后，下一个请求会启动全新的 Codex 进程。SIGINT/SIGTERM 会停止子进程并删除临时目录。

参数：`-listen 127.0.0.1:55666`、`-codex codex`、`-timeout 30s`。使用 `-listen :55666` 可允许其他主机或容器访问；该端点没有身份验证，因此只能暴露在可信网络中。内置的 `codex` 解析器会将配额窗口映射为仪表盘格式。此桥接服务独立于仪表盘，也不属于仪表盘的 Docker 镜像。

### 仪表盘

- `GET /` — 仪表盘页面（仅生产构建提供；开发时使用 Vite）。
- `GET /api/usage` — JSON，包含 `serverTime`、`lastUpdatedAt`、`sampleIntervalMs`、`retentionHours` 和 `endpoints`。每个端点包含 `name`、`provider`、`latest`（或 `null`）以及按顺序排列的 `history` 采样记录。时间戳为 Unix 毫秒。不会暴露私有端点 URL 和请求头。
- `GET /assets/{file}` — 内嵌的 CSS/JS（仅生产构建提供）。

现在，用量 API 返回的是结构化数据，而不是旧版 HTML 片段。
