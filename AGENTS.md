# Fantastic Gin Dealcoholized - Agent Guidance

## Key Commands

**Run backend (dev):**
```bash
go run ./cmd/webapi/
```

**Run with custom config:**
```bash
go run ./cmd/webapi/ -c demo/demo.yml
# or
API_CONFIG_PATH=demo/demo.yml go run ./cmd/webapi/
```

**Build backend only:**
```bash
go build ./cmd/webapi/
```

**Build with embedded WebUI:**
```bash
./open-node.sh          # Enter Node container
yarn run build-embed    # Build web assets
exit                    # Exit container
go build -tags webui ./cmd/webapi/
```

**Production build (with WebUI):**
```bash
./open-node.sh
yarn run build-prod
```

**Manage dependencies (vendoring):**
```bash
go mod tidy && go mod vendor
# Commit all changes under vendor/
```

## Architecture Notes

- **Layered structure**: handler → domain → repository
- **Entry point**: `cmd/webapi/main.go`
- **Database**: SQLite (`gormdb/fantastic-gin.gormdb`) via go-sqlite3 (CGo)
  - Schema migrations are embedded and run on startup
- **Configuration**: Loaded in order: CLI flags → env vars → YAML file
- **Env vars**: Prefixed with `API_` (e.g., `API_WEB_APIHOST=0.0.0.0:3000`)
- **WebUI**: Vue.js app in `webui/`, embedded via `webui` build tag
- **Health check**: `GET /healthz` returns 200 OK when healthy
- **API base path**: All endpoints under `/` (e.g., `/name`, `/name/:id`)
- **Error format**: All errors return `{"error": "<message>"}` as JSON

## Important Files

- API specification: `doc/api.yaml` (OpenAPI 3.1)
- Demo config: `demo/config.yml` (can be overridden)
- Healthcheck daemon: `cmd/healthcheck/`
- Shared errors: `internal/errs/errors.go` (leaf package)
- Main server: `cmd/webapi/main.go`
- Database file: `gormdb/fantastic-gin.gormdb` (SQLite)
- Route handlers: `internal/handler/route/handler.go`
- Domain services: `internal/domain/*/*-service.go`
- Repository: `internal/repository/database.go`
