# Registro unico oggetti smarriti — Go/Gin MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a working four-interface lost-and-found registry on the `fantastic-gin-dealcoholized` Go/Gin boilerplate: SQLite for relational data, Qdrant + vLLM Nemotron embeddings for vector search, DeepSeek V4.1 Flash as orchestrator, D3 as structured decision engine, and a Vue 3 SPA embedded in the Go binary.

**Architecture:** Keep the boilerplate’s layered `handler → domain → repository` structure. Add domain packages for `auth`, `item`, `custody`, `claim`, `verification`, `pass`, `fee`, `institution`, `search`, `agent`, and `platform`. The item is the aggregate root; every custody/claim change writes `custody_events` and `audit_logs`. Qdrant stores 4096-d text embeddings from the vLLM Nemotron endpoint; if either is unavailable, search falls back to SQLite. DeepSeek runs the chat/tool loop; D3 returns probabilities; both have deterministic fallbacks.

**Tech Stack:** Go 1.26, Gin, GORM, SQLite (`go-sqlite3`, CGO), goose migrations, Qdrant, vLLM embeddings (`nvidia/Nemotron-3.5-Embed-8B-BF16`, 4096-d), DeepSeek V4.1 Flash (OpenAI-compatible API), D3 (`/v1/systemone`), Vue 3 + Vite + TypeScript + Tailwind 4, Docker Compose, Cloudflare Tunnel.

**Spec:** `docs/superpowers/specs/2026-11-13-oggetti-smarriti-design.md`

## Global Constraints

- Base repo: copy `https://github.com/Tommaso-Sgroi/fantastic-gin-dealcoholized` into the project root. Keep its module path `github.com/tommaso-sgroi/fantastic-gin-dealcoholized` for speed; optional rename later.
- Follow the boilerplate’s `AGENTS.md`: layered architecture, `go mod tidy && go mod vendor` after any dependency change, goose migrations, `webui` build tag.
- Go 1.26; `CGO_ENABLED=1`; SQLite via `go-sqlite3`. Enable `PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;` on startup.
- SQLite file: `db/lostfound.db` (config `API_DB_FILENAME`). Never commit the DB file.
- Qdrant: `API_QDRANT_URL` (default `http://localhost:6333`), collection `items`, vector size **4096**, cosine.
- Embeddings: `API_EMBEDDINGS_URL` (default `http://www.lucascutigliani.it:8000/v1/embeddings`), `API_EMBEDDINGS_MODEL=nvidia/Nemotron-3.5-Embed-8B-BF16`, optional `API_EMBEDDINGS_KEY`. The endpoint is an OpenAI-compatible vLLM instance; it returns 4096-d vectors. For production, use HTTPS and an internal endpoint.
- DeepSeek: `API_DEEPSEEK_KEY`, `API_DEEPSEEK_URL` (default `https://api.deepseek.com`), `API_DEEPSEEK_MODEL=deepseek-v4.1-flash`. OpenAI-compatible `/chat/completions` with tools.
- D3: `API_D3_URL`, `API_D3_KEY`; POST to `${D3_URL}/v1/systemone` with `{model, state, questions}`. If unset or failing, use `LocalScorer`.
- Sessions: server-side `sessions` table, random 32-byte token, HttpOnly cookie `lf_session`, 12h TTL. Roles: `finder`, `searcher`, `intermediary`, `institution`, `admin`.
- Uploads: `API_STORAGE_DATA_DIR` (default `./data`), `data/uploads/<uuid>.jpg`, max 8MB, JPEG/PNG/WebP. Serve only via authenticated `/api/uploads/:id`.
- No raw identity/document images or fields to DeepSeek/D3. Use form-entered document fields and boolean `identity_match`. Redact logs.
- No denuncia di smarrimento, no albo pretorio, no 510-page PDF import. Demo uses a representative seed.
- Direct return before `transferred` is free. PA tariff only after `transferred`. Reward is a recorded notice, never charged by the system.
- Every state change writes `custody_events` + `audit_logs`. Every API route authenticates and authorizes by role.
- Tests: `go test ./...`; use `internal/testutil.NewTestDB(t)` with a temp SQLite + goose. Qdrant/embeddings/DeepSeek/D3 integration tests skip unless env vars are set.
- Frontend: Vue 3 + Vite + TypeScript + Tailwind 4 under `webui/`; build to `webui/dist`; serve at `/dashboard/`; API at `/api/`. Use `npm --prefix webui`.
- Demo deployment: Docker Compose (`api`, `qdrant`) + Cloudflare Tunnel; code freeze 13:30, submission 14:00, demo 14:30.

## Review Focus

These are the failure modes most likely to break a real user; each has a test in the owning task:

1. **Custody boundary at 7 days / 24h.** An item matched at 6d23h must not be handed over after the intermediary deadline; the pass is revoked and reissued for the PA. Task 5 + Task 9.
2. **Pass replay, expiry, wrong custodian, unpaid.** A pass must never redeem twice, at the wrong custodian, after expiry, or before payment. Task 9.
3. **Privacy leak.** Qdrant payloads, search DTOs, and agent responses must never contain `private_profile`, exact coordinates, or expected attribute values before the threshold. Task 6 + Task 8.
4. **External dependency failure.** Qdrant, the embeddings endpoint, DeepSeek, or D3 down must degrade to SQLite search, `LocalScorer`, and `MockAgent`, not crash or block the demo. Task 6 + Task 7 + Task 8.
5. **Rights assignment and sensitive routing.** No `in_custody` without a registered rights assignment; sensitive items go to the PA within 24h and cannot be handed over by the intermediary. Task 4 + Task 5 + Task 11.

---

### Task 1: Boilerplate, Docker Compose, configuration, health

**Files:**
- Copy: boilerplate repo into project root (preserve `docs/`, `IDEA_REPORT.md`)
- Create: `docker-compose.yml`, `Dockerfile`, `.env.example`, `scripts/dev-up.sh`
- Modify: `cmd/webapi/load-configuration.go`, `cmd/webapi/main.go`, `demo/config.yml`, `.gitignore`
- Test: `internal/health/health_test.go` or a `cmd/webapi` smoke test

**Interfaces:**
- Produces: `WebAPIConfiguration` fields `DeepSeek{Key,URL,Model}`, `D3{URL,Key}`, `Qdrant{URL,Collection}`, `Embeddings{URL,Model,Key}`, `Auth{SessionSecret,PassSecret,SessionTTL}`, `Storage{DataDir,MaxUploadMB}`, `App{PublicURL}`.
- Produces: `GET /healthz` from the boilerplate; `GET /api/health` returns `{ok, db, qdrant, embeddings}`.

- [ ] **Step 1: Copy the boilerplate**

Run:
```bash
git clone --depth 1 https://github.com/Tommaso-Sgroi/fantastic-gin-dealcoholized /tmp/fgd
rm -rf /tmp/fgd/.git
cp -a /tmp/fgd/. .
```
Expected: `cmd/`, `internal/`, `db/`, `webui/`, `vendor/`, `go.mod` present; `docs/` and `IDEA_REPORT.md` preserved.

- [ ] **Step 2: Add Docker Compose**

`docker-compose.yml`:
```yaml
services:
  api:
    build: .
    ports: ["3000:3000"]
    env_file: .env
    environment:
      API_QDRANT_URL: http://qdrant:6333
    volumes: ["./data:/data", "./db:/app/db"]
    depends_on: [qdrant]
  qdrant:
    image: qdrant/qdrant:latest
    ports: ["6333:6333"]
    volumes: ["./data/qdrant:/qdrant/storage"]
  tunnel:
    image: cloudflare/cloudflared:latest
    command: tunnel --no-autoupdate --url http://api:3000
    environment:
      TUNNEL_TOKEN: ${CLOUDFLARE_TUNNEL_TOKEN}
    profiles: ["tunnel"]
```

`scripts/dev-up.sh` loads `.env` with a literal dotenv loop (not `source`), because the CORS values contain semicolons; it then starts Qdrant and runs the API. The local `.env` is never printed or committed.

`Dockerfile` (multi-stage):
```dockerfile
FROM node:22-bookworm AS web
WORKDIR /web
COPY webui/package*.json ./
RUN npm install
COPY webui/ ./
RUN npm run build

FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev
WORKDIR /src
COPY . .
COPY --from=web /web/dist ./webui/dist
RUN CGO_ENABLED=1 go build -tags webui -o /out/webapi ./cmd/webapi

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/webapi .
COPY db ./db
COPY demo ./demo
ENV API_DB_FILENAME=/data/lostfound.db API_STORAGE_DATA_DIR=/data
EXPOSE 3000
CMD ["./webapi", "-c", "demo/config.yml"]
```

- [ ] **Step 3: Extend configuration**

Add config structs in `cmd/webapi/load-configuration.go` matching `.env.example` (DeepSeek, D3, Embeddings, Qdrant, Auth, Storage, Custody, Fees, Payment, Demo, Features, App) with `conf` tags and defaults. Set `demo/config.yml` `db.filename: db/lostfound.db`. Keep `.env.example` as the contract; `.env` stays gitignored.

- [ ] **Step 4: SQLite pragmas + health**

In `cmd/webapi/main.go`, after `gorm.Open`, execute:
```go
gormDB.Exec("PRAGMA journal_mode=WAL;")
gormDB.Exec("PRAGMA busy_timeout=5000;")
```
Add `/api/health` handler that pings SQLite/Qdrant with 1s timeouts and probes embeddings cheaply via `GET /v1/models` (3s, short cache; no inference), returning status booleans and never failing the request.

- [ ] **Step 5: Verify and commit**

Run: `go build ./cmd/webapi/ && go run ./cmd/webapi/ -c demo/config.yml` then `curl localhost:3000/api/health`.
Expected: `{ "ok": true, ... }`.

```bash
git add -A && git commit -m "chore: adopt gin boilerplate with sqlite, qdrant, embeddings config"
```

---

### Task 2: Migrations, records, repositories, test helper

**Files:**
- Create: `db/migrations/002_lost_found.sql`
- Create: `internal/repository/record/{user,session,intermediary,item,custody,event,claim,verification,pass,fee,payment,reward,notification,audit,tariff}.go`
- Modify: `internal/repository/interface.go`, `internal/repository/database.go`
- Create: `internal/repository/{user,session,item,custody,claim,pass,fee,notification,audit}.go`
- Create: `internal/testutil/db.go`
- Test: `internal/repository/repository_test.go`

**Interfaces:**
- Produces: `Repository` interface composed of `UserRepository`, `ItemRepository`, `CustodyRepository`, `ClaimRepository`, `PassRepository`, `FeeRepository`, `NotificationRepository`, `AuditRepository`.
- Produces: `testutil.NewTestDB(t) repository.Database` that creates a temp SQLite file, runs goose migrations, and returns a `Database`.
- Tables: `users`, `sessions`, `intermediaries`, `items`, `custody`, `custody_events`, `claims`, `verification_sessions`, `pickup_passes`, `fees`, `payments`, `reward_notices`, `notifications`, `audit_logs`, `lost_reports`, `tariffs`.

- [ ] **Step 1: Write the migration**

Create all tables with `TEXT` UUID ids, ISO-8601 timestamps, `TEXT` JSON fields, `INTEGER` booleans, and `INTEGER` cents. Add indexes on `items(status, category, found_zone)`, `custody(due_at)`, `claims(item_id)`, `pickup_passes(token_hash)`, `sessions(expires_at)`.

- [ ] **Step 2: Write the failing repository test**

```go
func TestCreateAndGetItem(t *testing.T) {
    db := testutil.NewTestDB(t)
    rec := record.ItemRecord{ID: "item-1", Status: "pending_custody", Category: "smartphone", RiskTier: "high", CreatedAt: time.Now().UTC()}
    if err := db.CreateItem(context.Background(), rec); err != nil { t.Fatal(err) }
    got, err := db.GetItemByID(context.Background(), "item-1")
    if err != nil { t.Fatal(err) }
    if got.Category != "smartphone" { t.Fatalf("got %q", got.Category) }
}
```

- [ ] **Step 3: Run and see it fail**

Run: `go test ./internal/repository -run TestCreateAndGetItem -v`
Expected: FAIL because the schema/method is missing.

- [ ] **Step 4: Implement records and repository methods**

Mirror the design doc §7.1 fields. Use GORM `Create`, `First`, `Where`, `Updates`, `Find`. Add `TableName()` methods. Add transaction helpers where a state change must be atomic (`CreateItemWithCustody`, `IssuePass`, `RedeemPass`).

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/repository/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add sqlite schema and repositories"
```

---

### Task 3: Auth, sessions, role middleware, demo login

**Files:**
- Create: `internal/domain/auth/{auth.go,service.go,service-interface.go}`
- Create: `internal/repository/user.go`, `internal/repository/session.go`
- Create: `internal/handler/middleware/auth.go`
- Create: `internal/handler/route/auth.go`, `internal/handler/dto/auth.go`
- Modify: `internal/handler/reqcontext/context.go` (add `User *auth.User`)
- Modify: `internal/handler/api.go`, `internal/handler/api-handler.go`
- Test: `internal/domain/auth/service_test.go`

**Interfaces:**
- Produces: `Login(ctx, userID string) (token string, user auth.User, err error)`; `Logout(ctx, token string) error`; `Authenticate(ctx, token string) (auth.User, error)`; `RequireRole(role string) gin.HandlerFunc`.
- Routes: `POST /api/auth/login` `{userId}`; `POST /api/auth/logout`; `GET /api/auth/me`.

- [ ] **Step 1: Write the failing auth test**

```go
func TestLoginCreatesSessionAndRejectsWrongRole(t *testing.T) {
    db := testutil.NewTestDB(t)
    svc := auth.New(db)
    token, user, err := svc.Login(context.Background(), "finder-demo")
    if err != nil || user.Role != "finder" { t.Fatal(err) }
    got, err := svc.Authenticate(context.Background(), token)
    if err != nil || got.ID != "finder-demo" { t.Fatal(err) }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/auth -v`
Expected: FAIL.

- [ ] **Step 3: Implement sessions and middleware**

`Login` creates a random 32-byte token (hex), stores its hash + `user_id` + `expires_at`, returns the raw token. `Authenticate` hashes and loads the session, checks expiry, loads the user. Middleware reads `lf_session` cookie, sets `reqcontext.User`, and `RequireRole` returns 403 JSON for wrong roles. Seed demo users: `finder-demo`, `searcher-demo`, `bar-x`, `portineria-sapienza`, `institution-demo`, `admin-demo`.

- [ ] **Step 4: Wire routes and CORS credentials**

Register `/api/auth/*`; set CORS `AllowCredentials=true` and the dev origin `http://localhost:5173`.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/domain/auth/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add session auth and role middleware"
```

---

### Task 4: Item enrollment, uploads, D3 catalog, rights assignment

**Files:**
- Create: `internal/domain/item/{item.go,service.go,service-interface.go,catalog.go,risk.go}`
- Create: `internal/platform/d3/client.go`, `internal/platform/storage/local.go`
- Create: `internal/repository/item.go`, `internal/repository/custody.go`
- Create: `internal/handler/route/item.go`, `internal/handler/dto/item.go`
- Create: `internal/handler/route/upload.go`
- Test: `internal/domain/item/service_test.go`, `internal/platform/d3/client_test.go`

**Interfaces:**
- Produces: `CreateItem(ctx, input CreateItemInput) (Item, error)`; `GetItem(ctx, id, actor) (Item, error)`; `ListItemsByCustodian(ctx, custodianID) ([]Item, error)`; `Catalog(ctx, photos []ImageInput) (CatalogResult, error)`.
- `CreateItemInput`: `{Source, FinderID, IntermediaryID?, LegalFinderID, RightsAssigned, Category, Brand, Model, Color, Condition, PrivateProfile, Photos, FoundAt, FoundLat, FoundLng, FoundZone}`.
- `CreateItem` rejects if `RightsAssigned` is false; for non-`operator` source, requires `IntermediaryID == LegalFinderID`; writes `item` + `custody` + `custody_events(rights_assigned)` + `audit_logs`.
- `Catalog` asks D3 choice/noul questions over the photos (category, color, condition, cracks, stickers, case, document type). If D3 is unavailable, returns the form values with `source: "form"`.

- [ ] **Step 1: Write the failing item test**

```go
func TestCreateItemRequiresRightsAssignment(t *testing.T) {
    db := testutil.NewTestDB(t)
    svc := item.New(db, item.Deps{})
    _, err := svc.CreateItem(context.Background(), item.CreateItemInput{
        Source: "finder_self", FinderID: "finder-demo", IntermediaryID: "bar-x",
        LegalFinderID: "bar-x", RightsAssigned: false, Category: "smartphone",
        Photos: []string{"data/uploads/a.jpg"}, FoundAt: time.Now().UTC(),
        FoundLat: 41.9, FoundLng: 12.5, FoundZone: "San Lorenzo",
    })
    if !errors.Is(err, errs.ErrInvalidInput) { t.Fatalf("want invalid input, got %v", err) }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/item -v`
Expected: FAIL.

- [ ] **Step 3: Implement storage and D3 catalog**

`LocalStorage.Save(dataURL or multipart file)` writes to `data/uploads/<uuid>.jpg` and returns a relative path. `D3Client` POSTs `{model: "d3", state: {images: [base64...]}, questions: {...}}` to `/v1/systemone`; parse probabilities. On timeout/error, return `ErrUnavailable` and let the service fall back to form values.

- [ ] **Step 4: Implement CreateItem + routes**

`POST /api/items` (multipart: `photos`, JSON `payload`), role finder/intermediary; `GET /api/items/:id`; `GET /api/items?custodian=me` for intermediary. Return DTOs without `private_profile` or exact coordinates for searcher roles.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/domain/item/... ./internal/platform/d3/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add item enrollment, uploads and d3 catalog"
```

---

### Task 5: Custody accept/refuse, 7-day/24h deadlines, pickup-due

**Files:**
- Create: `internal/domain/custody/{custody.go,service.go,service-interface.go}`
- Create: `internal/repository/custody.go`
- Create: `internal/handler/route/custody.go`, `internal/handler/dto/custody.go`
- Test: `internal/domain/custody/service_test.go`

**Interfaces:**
- Produces: `Accept(ctx, itemID, actorID string, now time.Time) (Custody, error)`; `Refuse(ctx, itemID, actorID, reason string) (Item, error)`; `MarkPickupDue(ctx, now time.Time) ([]Item, error)`; `DaysLeft(item, now) int`.
- `Accept`: only `pending_custody`; checks `legal_finder_id == actorID` and a `rights_assigned` event; sets `in_custody`, `due_at = now+7d` or `+24h` if sensitive; writes event/audit.
- `MarkPickupDue`: `in_custody` with `due_at <= now` → `pickup_due`; sensitive → `sensitive_pickup`; writes events.

- [ ] **Step 1: Write the failing custody test**

```go
func TestCustodyDeadlinesAndPickupDue(t *testing.T) {
    db := testutil.NewTestDB(t)
    svc := custody.New(db)
    now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
    c, err := svc.Accept(context.Background(), "item-umbrella", "bar-x", now)
    if err != nil { t.Fatal(err) }
    if !c.DueAt.Equal(now.Add(7*24*time.Hour)) { t.Fatalf("due %v", c.DueAt) }
    due, err := svc.MarkPickupDue(context.Background(), now.Add(8*24*time.Hour))
    if err != nil || len(due) == 0 { t.Fatal(err) }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/custody -v`
Expected: FAIL.

- [ ] **Step 3: Implement custody logic and routes**

Routes: `POST /api/items/:id/custody/accept`, `POST /api/items/:id/custody/refuse`, `POST /api/institution/tick` (demo/admin only) to call `MarkPickupDue`. Use transactions for item + event + audit.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/domain/custody/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add custody deadlines and pickup-due transitions"
```

---

### Task 6: Search, Qdrant, vLLM embeddings, SQLite fallback, seed

**Files:**
- Create: `internal/platform/embeddings/client.go`, `internal/platform/qdrant/client.go`
- Create: `internal/domain/search/{search.go,service.go,service-interface.go,fallback.go}`
- Create: `internal/repository/item_search.go`
- Create: `internal/seed/seed.go`, `cmd/seed/main.go`
- Test: `internal/domain/search/service_test.go`, `internal/platform/qdrant/client_test.go`

**Interfaces:**
- Produces: `Search(ctx, Query) ([]SearchResult, error)`; `IndexItem(ctx, Item) error`; `DeleteItem(ctx, id) error`.
- `SearchResult`: `{ItemID, Score, Category, FoundZone, FoundAt, CustodianType}` only. No `private_profile`, no exact lat/lng.
- `EmbeddingClient.Embed(ctx, text) ([]float32, error)`; `VectorStore.Upsert(ctx, point) error`; `Search(ctx, vector, filter, limit) ([]Point, error)`.
- `Search` uses Qdrant + the vLLM embeddings endpoint if available; on error, `SQLiteSearch` by category/zone/time and token overlap.

- [ ] **Step 1: Write the failing search test**

```go
func TestSearchFallsBackToSQLiteAndHidesPrivateData(t *testing.T) {
    db := testutil.NewTestDB(t)
    seed.Demo(t, db)
    svc := search.New(db, search.Deps{QdrantURL: "", EmbeddingsURL: ""})
    got, err := svc.Search(context.Background(), search.Query{Text: "iphone nero", Category: "smartphone", Zone: "San Lorenzo"})
    if err != nil || len(got) == 0 { t.Fatal(err) }
    if got[0].ItemID != "item-phone" { t.Fatalf("got %+v", got[0]) }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/search -v`
Expected: FAIL.

- [ ] **Step 3: Implement vLLM embeddings/Qdrant clients and fallback**

Embeddings: `POST ${API_EMBEDDINGS_URL}` with `{model: "nvidia/Nemotron-3.5-Embed-8B-BF16", input: [text1, text2]}`; parse `data[i].embedding` (4096 floats). Batch up to 32 inputs, 5s timeout; on error return `ErrUnavailable`. Qdrant: `PUT /collections/items/points` with `{points:[{id, vector, payload}]}`; `POST /collections/items/points/search` with `{vector, filter, limit}`. Payload only `{item_id, category, zone, status, custodian_type, description}`. On startup, `EnsureCollection` with size **4096**, cosine; log and continue on failure.

- [ ] **Step 4: Seed demo data**

Seed 6 users, 2 intermediaries, 4 items (phone pending at Bar X, wallet transferred, umbrella pending, document sensitive), claims, events, tariffs. `cmd/seed` runs `seed.Demo`. `POST /api/demo/reset` truncates and re-seeds (admin + `API_DEMO_RESET_SECRET`). Wire `IndexItem` into custody accept and the seed; on transfer, update the Qdrant payload.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/domain/search/... ./internal/platform/qdrant/... && go build ./cmd/webapi/ ./cmd/seed/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add hybrid search with qdrant, vllm embeddings and sqlite fallback"
```

---

### Task 7: Verification engine, policy, claims

**Files:**
- Create: `internal/domain/verification/{verification.go,d3.go,local.go,policy.go}`
- Create: `internal/domain/claim/{claim.go,service.go,service-interface.go}`
- Create: `internal/repository/claim.go`
- Create: `internal/handler/route/claim.go`, `internal/handler/dto/claim.go`
- Test: `internal/domain/verification/policy_test.go`, `internal/domain/claim/service_test.go`

**Interfaces:**
- Produces: `D3Result` with per-question probabilities; `LocalScore(state, questions) D3Result`; `Decision(riskTier, confidence, evidence, flags) PolicyDecision`; `CreateClaim(ctx, input) (Claim, error)`.
- `PolicyDecision`: `auto_approve` / `review` / `reject` / `more_info`.
- `CreateClaim` writes `claim` + `verification_sessions` + `custody_events(claim_created)` + `audit`. No pass yet.

- [ ] **Step 1: Write the failing verification test**

```go
func TestLocalScoreAndPolicy(t *testing.T) {
    state := verification.State{Item: map[string]any{"color": "nero", "case": "blu"}, Searcher: map[string]any{"color": "nero", "case": "blu"}}
    q := verification.Questions{"color": {Type: "noul"}, "case": {Type: "noul"}}
    res := verification.LocalScore(state, q)
    if res.Answers["color"].Noul < 0.8 { t.Fatalf("low score: %+v", res) }
    if verification.Decision("medium", 0.9, nil, nil).Decision != "auto_approve" { t.Fatal("want auto") }
    if verification.Decision("high", 0.9, nil, nil).Decision != "review" { t.Fatal("want review") }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/verification -v`
Expected: FAIL.

- [ ] **Step 3: Implement D3 client, local scorer, policy**

`D3Client.Score(ctx, state, questions)` posts to `/v1/systemone`; 5s timeout; on error return `ErrUnavailable`. `LocalScore` compares normalized values: match 0.95, mismatch 0.1, `score` 0.9 for unique/exact, 0.5 for partial. `Decision` thresholds: low/medium ≥0.75 auto, high ≥0.85 review, sensitive always review. `redactIdentity` maps raw fields to booleans before D3.

- [ ] **Step 4: Implement claims and routes**

`POST /api/claims` `{itemId, searcherId, returnChannel, confidence, session}`; `GET /api/claims/:id` (owner/institution). Claim status `submitted`/`under_review`/`approved` according to policy; high/sensitive stays `under_review`.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/domain/verification/... ./internal/domain/claim/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add d3/local verification, policy and claims"
```

---

### Task 8: Agent, DeepSeek orchestrator, tools, fallback

**Files:**
- Create: `internal/platform/deepseek/client.go`
- Create: `internal/domain/agent/{agent.go,service.go,service-interface.go,tools.go,mock.go}`
- Create: `internal/handler/route/chat.go`, `internal/handler/dto/chat.go`
- Test: `internal/domain/agent/service_test.go`, `internal/platform/deepseek/client_test.go`

**Interfaces:**
- Produces: `Chat(ctx, searcherID string, messages []Message) (Reply, error)`; `Tool` definitions `search_items`, `get_private_profile`, `score_with_d3`, `create_claim`, `escalate`.
- `Reply`: `{Message string, Candidates []SearchResult, ClaimID string, Done bool}`.
- `DeepSeekClient` posts OpenAI-compatible `/chat/completions` with `tools`; parse `tool_calls`; loop max 5 tool rounds. On error/unset key, use `MockAgent` (scripted: search, ask 3 questions, score, create claim).

- [ ] **Step 1: Write the failing agent test**

```go
func TestMockAgentDoesNotLeakPrivateData(t *testing.T) {
    db := testutil.NewTestDB(t)
    seed.Demo(t, db)
    svc := agent.New(db, agent.Deps{DeepSeekKey: ""})
    reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"}})
    if err != nil { t.Fatal(err) }
    if strings.Contains(reply.Message, "basso-sinistra") { t.Fatal("leaked private detail") }
    if len(reply.Candidates) == 0 { t.Fatal("expected candidates") }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/agent -v`
Expected: FAIL.

- [ ] **Step 3: Implement DeepSeek client and tool loop**

System prompt: ask one open question at a time; never reveal private details, exact location, or expected values; never correct a wrong answer; if confidence below threshold, ask another question or escalate. Tools call domain services. Tool results sent back as `tool` messages. Cap at 5 rounds and 20 messages.

- [ ] **Step 4: Implement SSE/JSON chat route**

`POST /api/chat` `{messages}` with session auth. Return JSON `{message, candidates, claimId, done}`. Rate limit 20 messages/min per session. The Vue chat can poll/send; streaming is optional.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/domain/agent/... ./internal/platform/deepseek/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add deepseek agent with tools and mock fallback"
```

---

### Task 9: Pass QR, validation, redemption

**Files:**
- Create: `internal/domain/pass/{pass.go,service.go,service-interface.go,token.go}`
- Create: `internal/repository/pass.go`
- Create: `internal/handler/route/pass.go`, `internal/handler/dto/pass.go`
- Test: `internal/domain/pass/service_test.go`

**Interfaces:**
- Produces: `Issue(ctx, claimID string, now time.Time) (Pass, error)`; `Validate(ctx, token, custodianID string, now time.Time) (Validation, error)`; `Redeem(ctx, token, custodianID, actorID string, now time.Time) (Item, error)`.
- Token: base64url JSON `{passId, claimId, itemId, custodianId, returnChannel, exp}` + HMAC-SHA256 signature with `Auth.PassSecret`.
- Validation reasons: `expired`, `wrong_custodian`, `unpaid`, `revoked`, `already_redeemed`, `item_not_in_custody`.

- [ ] **Step 1: Write the failing pass test**

```go
func TestPassRedeemsOnceAndRejectsWrongCustodian(t *testing.T) {
    db := testutil.NewTestDB(t)
    seed.Demo(t, db)
    svc := pass.New(db, pass.Deps{PassSecret: "test-secret"})
    p, err := svc.Issue(context.Background(), "claim-phone", time.Now().UTC())
    if err != nil { t.Fatal(err) }
    if _, err := svc.Validate(context.Background(), p.Token, "other-shop", time.Now().UTC()); !errors.Is(err, errs.ErrForbidden) { t.Fatal("want wrong custodian") }
    if _, err := svc.Redeem(context.Background(), p.Token, "bar-x", "bar-x", time.Now().UTC()); err != nil { t.Fatal(err) }
    if _, err := svc.Validate(context.Background(), p.Token, "bar-x", time.Now().UTC()); !errors.Is(err, errs.ErrConflict) { t.Fatal("want already redeemed") }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/pass -v`
Expected: FAIL.

- [ ] **Step 3: Implement pass issue/validate/redeem**

`Issue` requires claim approved, item in custody/transferred, fee paid or amount 0; sets `pending_payment` if due. `Validate` checks signature, expiry, status, claim, item, custodian, payment. `Redeem` validates, then transaction: item `returned`, pass `redeemed`, claim `handed_over`, event/audit.

- [ ] **Step 4: API routes**

`POST /api/passes` `{claimId}`; `POST /api/passes/validate` `{token}` (intermediary/institution); `POST /api/passes/redeem` `{token}`. Return DTO with item summary and instructions.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/domain/pass/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add signed pickup passes with qr validation"
```

---

### Task 10: PA fees, mock payment, reward notice

**Files:**
- Create: `internal/domain/fee/{fee.go,service.go,service-interface.go}`
- Create: `internal/domain/reward/{reward.go,service.go}`
- Create: `internal/repository/fee.go`
- Create: `internal/handler/route/fee.go`, `internal/handler/dto/fee.go`
- Test: `internal/domain/fee/service_test.go`, `internal/domain/reward/service_test.go`

**Interfaces:**
- Produces: `Calculate(ctx, itemID) (Fee, error)`; `Pay(ctx, feeID) (Payment, error)`; `Request(ctx, itemID, legalFinderID) (RewardNotice, error)`; `Get(ctx, itemID) (RewardNotice, error)`.
- `Fee.Applies`: `none_direct_return` (amount 0) or `pa_tariff` (days × daily + fixed). `RewardNotice.payment_status` always `outside_system`.

- [ ] **Step 1: Write the failing fee test**

```go
func TestDirectReturnIsFreeAndPATariffAppliesAfterTransfer(t *testing.T) {
    db := testutil.NewTestDB(t)
    seed.Demo(t, db)
    svc := fee.New(db)
    direct, err := svc.Calculate(context.Background(), "item-phone")
    if err != nil || direct.Applies != "none_direct_return" || direct.AmountCents != 0 { t.Fatal(err) }
    pa, err := svc.Calculate(context.Background(), "item-wallet")
    if err != nil || pa.Applies != "pa_tariff" || pa.AmountCents == 0 { t.Fatal(err) }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/fee -v`
Expected: FAIL.

- [ ] **Step 3: Implement fee/reward + mock payment**

Use a seeded tariff (`daily_rate_cents`, `fixed_cents`, `currency`). `Pay` creates a `paid` payment with a mock provider and receipt URL. `Request` only allows the legal finder; records amount if provided; never charges.

- [ ] **Step 4: API routes and wiring**

`GET /api/items/:id/fee`, `POST /api/fees/:id/pay`, `POST /api/items/:id/reward-notice`, `GET /api/items/:id/reward-notice`. Pass validation checks `payment_id`/fee status.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/domain/fee/... ./internal/domain/reward/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add PA fees, mock payment and reward notices"
```

---

### Task 11: Institution pickups, intake, review, transfer, unclaimed

**Files:**
- Create: `internal/domain/institution/{institution.go,service.go,service-interface.go}`
- Create: `internal/repository/institution.go`
- Create: `internal/handler/route/institution.go`, `internal/handler/dto/institution.go`
- Test: `internal/domain/institution/service_test.go`

**Interfaces:**
- Produces: `ListPickups(ctx) ([]PickupGroup, error)`; `Transfer(ctx, itemID, actorID, now) (Custody, error)`; `CreateOfficialItem(ctx, input) (Item, error)`; `ReviewClaim(ctx, claimID, decision, actorID) (Claim, error)`; `OfferUnclaimed(ctx, itemID, now) (Item, error)`; `CollectByLegalFinder(ctx, itemID, actorID) (Item, error)`; `Dispose(ctx, itemID, actorID) (Item, error)`.
- `ListPickups` groups `pickup_due` and `sensitive_pickup` by intermediary/zone, sensitive first.
- `Transfer` closes intermediary custody, opens institution custody, sets `transferred`, revokes open passes.
- `OfferUnclaimed` only after `legal_term_start + 1 year`; `Dispose` only after offer.

- [ ] **Step 1: Write the failing institution test**

```go
func TestInstitutionTransferAndUnclaimedOrder(t *testing.T) {
    db := testutil.NewTestDB(t)
    seed.Demo(t, db)
    now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
    cs := custody.New(db)
    if _, err := cs.Accept(context.Background(), "item-umbrella", "bar-x", now); err != nil { t.Fatal(err) }
    if _, err := cs.MarkPickupDue(context.Background(), now.Add(8*24*time.Hour)); err != nil { t.Fatal(err) }
    svc := institution.New(db)
    if _, err := svc.Transfer(context.Background(), "item-umbrella", "institution-demo", now); err != nil { t.Fatal(err) }
    if _, err := svc.OfferUnclaimed(context.Background(), "item-wallet", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, errs.ErrInvalidInput) { t.Fatal("want too early") }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/institution -v`
Expected: FAIL.

- [ ] **Step 3: Implement institution logic and routes**

Routes: `GET /api/institution/pickups`, `POST /api/institution/items/:id/transfer`, `POST /api/institution/items` (official intake), `POST /api/institution/claims/:id/review`, `POST /api/institution/items/:id/offer`, `POST /api/institution/items/:id/collect`, `POST /api/institution/items/:id/dispose`. All institution role.

- [ ] **Step 4: Verification report**

`GET /api/institution/claims/:id` returns questions, answers, probabilities, evidence, flags, rationale, model version. No raw PII beyond what the institution needs.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/domain/institution/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add institution pickups, intake and review"
```

---

### Task 12: Notifications, audit, timeline

**Files:**
- Create: `internal/domain/platform/{notification.go,audit.go}`
- Create: `internal/repository/notification.go`, `internal/repository/audit.go`
- Create: `internal/handler/route/notification.go`, `internal/handler/dto/notification.go`
- Test: `internal/domain/platform/platform_test.go`

**Interfaces:**
- Produces: `Notify(ctx, userID, type string, payload map[string]any) error`; `Audit(ctx, actorID, action, entity, entityID string, metadata map[string]any) error`; `ListNotifications(ctx, userID) ([]Notification, error)`; `Timeline(ctx, itemID) ([]CustodyEvent, error)`.
- Types: `match`, `custody_accepted`, `pickup_due`, `pass_issued`, `handover`, `transfer`, `sensitive`, `fee_paid`.

- [ ] **Step 1: Write the failing test**

```go
func TestNotifyAndAuditPersist(t *testing.T) {
    db := testutil.NewTestDB(t)
    svc := platform.New(db)
    if err := svc.Notify(context.Background(), "bar-x", "match", map[string]any{"itemId": "item-phone"}); err != nil { t.Fatal(err) }
    if err := svc.Audit(context.Background(), "bar-x", "custody_accepted", "item", "item-phone", nil); err != nil { t.Fatal(err) }
    if n, _ := svc.ListNotifications(context.Background(), "bar-x"); len(n) != 1 { t.Fatalf("got %d", len(n)) }
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./internal/domain/platform -v`
Expected: FAIL.

- [ ] **Step 3: Implement and wire**

Add notification/audit calls to custody, claim, pass, fee, transfer services. `GET /api/notifications`, `POST /api/notifications/:id/read`, `GET /api/items/:id/timeline`.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/domain/platform/... && go build ./cmd/webapi/`
Expected: PASS.

```bash
git add -A && git commit -m "feat: add notifications, audit log and timeline"
```

---

### Task 13: Vue 3 frontend shell, auth, finder, intermediary

**Files:**
- Create: `webui/package.json`, `webui/vite.config.ts`, `webui/index.html`, `webui/tsconfig.json`
- Create: `webui/src/main.ts`, `webui/src/App.vue`, `webui/src/router.ts`, `webui/src/api.ts`, `webui/src/style.css`
- Create: `webui/src/views/{LoginView,FinderEnrollView,FinderReceiptView,IntermediaryDashboardView,IntermediaryEnrollView,IntermediaryScanView}.vue`
- Create: `webui/src/components/{PhotoUpload,GeoPicker,RightsAssignment,RoleNav,StatusBadge,Timeline}.vue`
- Modify: `cmd/webapi/register-web-ui.go` for SPA fallback to `index.html`
- Test: `npm --prefix webui run build`; `go build -tags webui ./cmd/webapi/`; manual login/finder/intermediary smoke

**Interfaces:**
- Consumes: `/api/auth/*`, `/api/items`, `/api/items/:id`, `/api/items/:id/custody/*`, `/api/uploads/:id`.
- Produces: SPA routes `/dashboard/login`, `/dashboard/finder`, `/dashboard/finder/receipt/:id`, `/dashboard/intermediary`, `/dashboard/intermediary/enroll`, `/dashboard/intermediary/scan`.

- [ ] **Step 1: Scaffold Vue + Vite + Tailwind**

`webui/package.json` scripts: `dev`, `build`. Use `vue`, `vue-router`, `qrcode`, `html5-qrcode`, `tailwindcss`, `@tailwindcss/vite`, `vite`, `@vitejs/plugin-vue`, `typescript`, `vue-tsc`. Set `base: "/dashboard/"` in `vite.config.ts` so assets resolve under the Gin mount.

- [ ] **Step 2: Build auth + role shell**

`api.ts` uses `credentials: "include"`. Router guard calls `/api/auth/me`; redirects to `/dashboard/login`. `RoleNav` shows the role’s links.

- [ ] **Step 3: Finder enrollment + receipt**

Photo upload (1-3, client-side resize to 1200px), geolocation button + manual lat/lng, category/color/condition form, intermediary select, rights-assignment checkbox with legal copy. Submit multipart; receipt shows item code/status.

- [ ] **Step 4: Intermediary dashboard + enrollment + scan**

Dashboard lists pending/in-custody items with days left and sensitive badge; accept/refuse. Enrollment reuses the finder form. Scan view uses `html5-qrcode` + manual token input; calls validate/redeem.

- [ ] **Step 5: Verify and commit**

Run: `npm --prefix webui run build && go build -tags webui ./cmd/webapi/`
Expected: build succeeds; `/dashboard/` serves the SPA.

```bash
git add -A && git commit -m "feat: add vue shell, finder and intermediary ui"
```

---

### Task 14: Vue searcher chat, pass QR, institution dashboard

**Files:**
- Create: `webui/src/views/{SearcherChatView,SearcherPassView,InstitutionDashboardView,InstitutionIntakeView,InstitutionItemView}.vue`
- Create: `webui/src/components/{ChatMessage,CandidateCard,QRCode,FeeSummary,RewardNotice,VerificationReport}.vue`
- Test: `npm --prefix webui run build`; manual chat/pass/institution smoke

**Interfaces:**
- Consumes: `/api/chat`, `/api/claims`, `/api/passes`, `/api/items/:id/fee`, `/api/items/:id/reward-notice`, `/api/institution/*`.
- Produces: SPA routes `/dashboard/searcher`, `/dashboard/searcher/pass/:id`, `/dashboard/institution`, `/dashboard/institution/intake`, `/dashboard/institution/items/:id`.

- [ ] **Step 1: Searcher chat**

Chat messages, send box, candidate cards (category/zone/date only), question prompts, "claim" button when ready. Show "d3 / fallback" badge from `/api/health`.

- [ ] **Step 2: Pass page**

Render QR with `qrcode` from the pass token; show pickup instructions, fee summary, reward notice, documents required. "Paga" calls the mock payment endpoint when a fee is due.

- [ ] **Step 3: Institution dashboard + intake + item detail**

Pickup groups with sensitive badge and transfer button; intake form with photo + AI description; item detail with timeline, verification report, fee/payment, approve/reject, handover, unclaimed actions.

- [ ] **Step 4: Verify and commit**

Run: `npm --prefix webui run build && go build -tags webui ./cmd/webapi/`
Expected: build succeeds; manual flows work.

```bash
git add -A && git commit -m "feat: add searcher chat, pass qr and institution ui"
```

---

### Task 15: Demo seed, API E2E, Docker, tunnel, video

**Files:**
- Create: `tests/e2e/demo_test.go` (Go `httptest` against the router or a running server)
- Create: `scripts/demo-reset.sh`, `scripts/tunnel.sh`
- Create: `docs/demo-script.md`
- Modify: `cmd/webapi/main.go` to serve `webui/dist` in dev/build
- Test: `go test ./tests/e2e/...`; `docker compose up --build`; manual demo rehearsal

**Interfaces:**
- Produces: deterministic demo flow: reset → finder enroll phone → intermediary accept → searcher chat/claim → approve → pass → redeem → returned; institution wallet flow with PA fee.

- [ ] **Step 1: Write the failing E2E test**

```go
func TestDemoFlow(t *testing.T) {
    srv := testutil.NewServer(t)
    testutil.ResetDemo(t, srv)
    // login finder, POST /api/items with rightsAssigned=true, login bar-x, accept custody
    // login searcher, POST /api/chat / /api/claims, login bar-x, approve, issue pass, redeem
    // assert item status returned
}
```

- [ ] **Step 2: Run and see it fail**

Run: `go test ./tests/e2e -run TestDemoFlow -v`
Expected: FAIL because the flow is incomplete.

- [ ] **Step 3: Fix seed and flow**

Seed `claim-phone` approved, phone pending at Bar X, wallet transferred to PA, umbrella pending, document sensitive. Ensure reset truncates all tables and re-seeds. Add a `POST /api/demo/reset` protected by `API_DEMO_RESET_SECRET`.

- [ ] **Step 4: Build, run with Docker, tunnel**

`docker compose up -d qdrant`, `docker compose up --build api`. Use `docker compose --profile tunnel up tunnel` for a public URL. Record the 3-minute video as backup.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add demo seed, e2e flow and docker deployment"
```

---

### Task 16: Hardening, fallbacks, accessibility, rehearsal

**Files:**
- Modify: handlers for error/loading states; Vue error/empty states
- Create: `docs/demo-checklist.md`
- Modify: `/api/health` to include DeepSeek/D3 status

- [ ] **Step 1: Fallbacks**

Verify: no Qdrant/embeddings → SQLite search; no DeepSeek key → `MockAgent`; no D3 → `LocalScorer`; no payment provider → mock. Add a UI badge for degraded mode.

- [ ] **Step 2: Rate limit and privacy logs**

Chat 20 msg/min/session; no `private_profile`, document numbers, or raw identity in logs. Add a test that searches and chat responses do not contain a seeded private value.

- [ ] **Step 3: Mobile/accessibility pass**

Finder/searcher at 375px; labels; focus; contrast. Institution/intermediary desktop tables responsive.

- [ ] **Step 4: Final verification**

Run: `go test ./... && go vet ./... && npm --prefix webui run build && go build -tags webui ./cmd/webapi/`
Expected: all pass.

- [ ] **Step 5: Rehearse and commit**

Reset, run the 3-minute demo twice, record backup video.

```bash
git add -A && git commit -m "chore: harden demo, fallbacks and accessibility"
```

---

## Cut Line

If time is short, ship in this order:

1. **P0 (core demo):** Tasks 1–9 + 13 + 15. Finder → intermediary → searcher → agent → pass → handover, with SQLite + Qdrant/embeddings fallback.
2. **P1 (50/50 demo):** Tasks 10–11 + 14. PA fees, institution dashboard, official intake, review.
3. **P2 (polish):** Tasks 12, 16. Notifications, audit UI, hardening, accessibility.

Never cut: rights assignment, custody deadline, pass validation, privacy redaction, demo reset, SQLite/Qdrant fallback.
