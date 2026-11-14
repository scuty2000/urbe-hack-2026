# Fantastic Gin Dealcoholized

> A clean, Go REST API template built on [Gin](https://github.com/gin-gonic/gin),
> following a **layered architecture** and SOLID principles.  
> Note: this is not a production-ready implementation, but a learning tool and starting point for your own projects.

This project is a reimplementation of [Fantastic Coffee (decaffeinated)](https://github.com/sapienzaapps/fantastic-coffee-decaffeinated)
by [Enrico Bassetti](https://github.com/Enrico204); originally created for the
[Web and Software Architecture](http://gamificationlab.uniroma1.it/en/wasa/) course at Sapienza
University of Rome.

> 💡 **Looking for a full project example?**
> Check out [gin-fountains](https://github.com/Tommaso-Sgroi/fantastic-gin-dealcoholized/tree/gin-fountains) — a complete
> reimplementation of the WASA homework using this template as its backend foundation.

---

## What it does

The project exposes a small REST API (documented in [`doc/api.yaml`](doc/api.yaml)) to create,
read, update and delete **names** stored in a SQLite database:

| Method   | Path                  | Description                      |
|----------|-----------------------|----------------------------------|
| `GET`    | `/healthz`            | Liveness probe                   |
| `POST`   | `/names`              | Create a name                    |
| `GET`    | `/names/:id`          | Get a name by ID                 |
| `PUT`    | `/names/:id`          | Update a name by ID              |
| `DELETE` | `/names/:id`          | Delete a name by ID              |
| `POST`   | `/names/:id/replace`  | Atomically replace a name by ID  |

All error responses are JSON: `{"error": "<message>"}`.

---

## Demo (oggetti smarriti)

This repository now runs the full lost-and-found MVP: a Vue UI embedded in the
Go binary, a SQLite registry, finder/intermediary/searcher/institution roles,
privacy-safe claim verification and a single-use pickup pass.

The finder flow is a five-step photo-first wizard: D3 classifies the first
photo (the only component that sees images), DeepSeek turns the sanitized
result into a photo plan and prefills, Mapbox ranks the five nearest active
drop points by walking time, and the finder reviews every value before the
disclaimer and submit. Without D3/DeepSeek the wizard falls back to a manual
compilation; without `MAPBOX_TOKEN` the drop points show a **stima** badge.
Set `API_D3_ALLOW_IMAGES=true` if the final D3 enrollment pass must see the
photos too (the wizard analysis always does).

```bash
# first run only
cp .env.example .env

# local development with the demo dataset
scripts/dev-up.sh
# UI dev server in a second terminal
npm --prefix webui run dev

# or the container stack (embedded UI at /dashboard/)
docker compose up --build -d api qdrant
open http://localhost:3000/dashboard/
```

The compose stack keeps the SQLite database and the uploaded photos together in
the `./data` volume (mounted at `/data` inside the container), so a container
recreate does not lose either of them. Qdrant uses `./data/qdrant`.

- 3-minute demo script: [`docs/demo-script.md`](docs/demo-script.md)
- Submission checklist: [`docs/submission.md`](docs/submission.md)
- Full API end-to-end test: `go test ./tests/e2e/... -v`
- Reset between rehearsals: `scripts/demo-reset.sh`
- Optional public URL: `scripts/tunnel.sh` (requires a Cloudflare tunnel token)

The demo works without DeepSeek, D3, embeddings or a payment provider: search
falls back to SQLite, claims use the local scorer and the agent runs its
deterministic mock.

---

## Project structure

```
.
├── cmd/
│   ├── healthcheck/          # Standalone health-check daemon (for environments without native probes)
│   └── webapi/               # Main web API server
│       ├── main.go           # Entry point: wires all layers, starts server, handles shutdown
│       └── load-configuration.go
├── gormdb/
│   ├── fantastic-gin.gormdb      # SQLite database file (path set by demo/config.yml)
│   └── migrations/           # Reserved for future SQL migration scripts (currently empty)
├── demo/
│   └── config.yml            # Ready-to-use development configuration
├── doc/
│   └── api.yaml              # OpenAPI 3.1 specification
├── internal/
│   ├── domain/
│   │   ├── health/           # Health-check domain (service-interface.go, service.go)
│   │   └── name/             # Name entity domain (name.go, service-interface.go, service.go)
│   ├── errs/
│   │   └── errors.go         # Leaf package — shared sentinel errors, no internal imports
│   ├── handler/
│   │   ├── api.go            # Router interface, Config, New() — global middleware registered here
│   │   ├── api-handler.go    # Route registration + wrap() adapter (HandlerFunc → gin.HandlerFunc) adding additional features like request-context logger and error → HTTP status mapping
│   │   ├── dto/              # HTTP-boundary data-transfer objects (request / response)
│   │   ├── middleware/       # Gin middleware: CORS, recovery, request-context logger
│   │   ├── reqcontext/       # Per-request context wrapper (UUID + structured logger)
│   │   └── route/            # Route handlers and HTTP-level error mapping
│   └── repository/
│       ├── database.go       # appdbimpl (Repository) + trxdbimpl (Transactor)
│       ├── interface.go      # Repository & Transactor interface declarations
│       ├── dbquery/          # Raw SQL — QueryExecutor + RepositoryQueryExecutor
│       └── record/           # Plain struct projections of SQL rows (gormdb struct tags)
└── vendor/                   # Vendored dependencies (managed by go mod vendor)
```

### Architecture

The backend is organised in three strictly-separated layers:

| Layer | Package | Responsibility |
|---|---|---|
| **HTTP** | `internal/handler` | Gin routing, middleware, DTO mapping, HTTP error translation |
| **Domain** | `internal/domain` | Business rules, validation, use-case orchestration |
| **Repository** | `internal/repository` | SQLite adapter — implements domain-owned `Repository` / `Transactor` |

Typical request flow:

```
HTTP request
  → Gin router / middleware
    → handler/route (DTO decode, error → HTTP status)
      → domain use-case (validation, orchestration)
        → repository (SQL)
          → SQLite
```

Dependency direction (imports flow strictly downward):

```
handler → domain → repository
               ↑
             errs   (leaf — no internal imports)
```

Shared sentinel errors live in `internal/errs` so `errors.Is(...)` works consistently through
wrapped errors at every layer.

### Notable files

| File | Role |
|---|---|
| `cmd/webapi/main.go` | Entry point: loads config, wires DB → repository → domain services → HTTP router, starts the API server, handles graceful shutdown on **SIGTERM** and **SIGINT** |
| `cmd/webapi/load-configuration.go` | Declares `WebAPIConfiguration`; parses flags, env vars, and optional YAML file via `ardanlabs/conf` |
| `internal/errs/errors.go` | Leaf package of sentinel errors (`ErrNotFound`, `ErrInvalidInput`, `ErrConflict`, `ErrUnauthorized`, `ErrForbidden`, `ErrUnavailable`, `ErrTimeout`, `ErrInternal`). No internal imports |
| `internal/repository/database.go` | `appdbimpl` implements `Repository`; `trxdbimpl` wraps it to implement `Transactor`. `RunInTransaction` swaps the SQL executor to `*sql.Tx` so all query methods run inside the same transaction transparently |
| `internal/repository/interface.go` | `Repository` and `Transactor` interface declarations — defined here, consumed by the domain layer |
| `internal/repository/record/records.go` | Plain data-bag structs produced by SQL queries. `gormdb` struct tags describe column names for row scanning. No domain logic lives here |
| `internal/repository/dbquery/query.go` | Defines `QueryExecutor` (common interface for `*sql.DB` and `*sql.Tx`) and `RepositoryQueryExecutor` — the **only** type that holds and executes SQL, preventing any query method from accidentally bypassing a transaction |
| `internal/domain/name/service-interface.go` | Declares `ServiceName` — the public use-case interface consumed by the handler layer. Adding a new use-case starts here |
| `internal/domain/name/service.go` | Concrete `nameService`: validation (`maxNameLength = 255`), use-case orchestration |
| `internal/domain/name/name.go` | `Name` domain entity and the private `nameFromRow` mapper from `record.NameRecord` |
| `internal/handler/api.go` | Defines `Router` interface, `Config`, and `New()`. Configures the Gin engine, registers global middleware (CORS, recovery, request-context logger), validates required dependencies at startup |
| `internal/handler/api-handler.go` | Registers all routes on the Gin engine via `Handler()` and provides `wrap()` — the adapter that converts a framework-agnostic `routes.HandlerFunc` into a `gin.HandlerFunc` |
| `internal/handler/route/handler.go` | Defines `HandlerFunc` (the handler signature) and `WebAPIHandler` (the struct carrying shared domain service dependencies across all route handlers) |

---

## Quick start

```shell
go run ./cmd/webapi/ -c demo/config.yml
```

Or with environment variables only (no config file):

```shell
API_WEB_APIHOST=0.0.0.0:3000 API_DB_FILENAME=gormdb/fantastic-gin.gormdb go run ./cmd/webapi/
```

---

## Configuration

Configuration is applied in the following priority order (**lowest → highest**):

```
built-in defaults  <  YAML config file  <  environment variables  <  CLI flags
```

The YAML config file path can be set with `-c <path>` (short flag) or `API_CONFIG_PATH=<path>`.
The built-in default path is `/config/config.yml` (suitable for containerised deployments).
For local development, pass `demo/config.yml` explicitly:

```shell
go run ./cmd/webapi/ -c demo/config.yml
```

> **Note:** without a config file the SQLite database defaults to `/tmp/decaf.gormdb` and logs go to
> `/tmp/dealch.log`. The `demo/config.yml` file sets these to `gormdb/fantastic-gin.gormdb` and `stdout`.

Key settings and their **built-in** defaults (alongside the values used by `demo/config.yml`):

| Setting | Built-in default | `demo/config.yml` value | Description |
|---|---|---|---|
| `web.apihost` | `0.0.0.0:3000` | `0.0.0.0:3000` | API server bind address |
| `web.debughost` | `0.0.0.0:4000` | `0.0.0.0:4000` | Reserved pprof address (in config, not yet started) |
| `web.readtimeout` | `5s` | `5s` | Max time to read a full HTTP request |
| `web.writetimeout` | `5s` | `5s` | Max time to write a full HTTP response |
| `web.shutdowntimeout` | `5s` | `5s` | Graceful-shutdown deadline |
| `gormdb.filename` | `/tmp/decaf.gormdb` | `gormdb/fantastic-gin.gormdb` | SQLite database file path |
| `logger.level` | `info` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `logger.file` | `/tmp/dealch.log` | `stdout` | Log output (`stderr`, `stdout`, or a file path) |

CORS is fully configurable via the `cors.*` group (`allowallorigins`, `allowedorigins`,
`allowedmethods`, `allowedheaders`, `allowcredentials`, `maxage`).

All keys can be overridden via environment variables prefixed with `API_`, e.g.:

```shell
API_WEB_APIHOST=0.0.0.0:8080 API_LOGGER_LEVEL=debug go run ./cmd/webapi/ -c demo/config.yml
```

---

## Database

The app uses **SQLite** via [`go-sqlite3`](https://github.com/mattn/go-sqlite3) (CGo required).
The `gormdb/migrations/` directory is reserved for future SQL migration scripts; currently it is empty
and the schema must be pre-created in the SQLite file.

---

## How to build

**Backend only:**

```shell
go build ./cmd/webapi/
```


---

## How to run (development mode)

**Backend only:**

```shell
go run ./cmd/webapi/ -c demo/config.yml
```


---

## Go vendoring

This project uses [Go Vendoring](https://go.dev/ref/mod#vendoring).
After adding or updating a dependency:

```shell
go mod tidy && go mod vendor
```

Then commit everything under `vendor/`.

---


## License

MIT — see [LICENSE](LICENSE).

Originally authored by [Enrico Bassetti](https://github.com/Enrico204).
Reimplemented and extended by [Tommaso Sgroi](https://github.com/tommaso-sgroi).

