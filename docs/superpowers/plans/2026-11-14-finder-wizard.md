# Finder Wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn finder enrollment into a five-step, photo-first wizard with D3 classification, DeepSeek photo-plan/prefill, Mapbox walking-distance drop points and a final disclaimer.

**Architecture:** A new `wizard` domain orchestrates D3 (images), a DeepSeek text planner (photo plan/prefill) and a Mapbox platform client (reverse geocoding + walking matrix). The HTTP layer exposes analyze/geo/drop-points endpoints; the existing `POST /api/items` remains the only write and the authority. Vue gets a finder-only wizard; the intermediary form is unchanged.

**Tech Stack:** Go 1.26 / Gin / SQLite / D3 / DeepSeek / Mapbox / Vue 3 + Vite + Tailwind.

**Spec:** `docs/superpowers/specs/2026-11-14-finder-wizard-design.md`

## Global Constraints

- Branch `feat/finder-wizard`; no new DB tables or item states.
- D3 is the only component that sees photos. DeepSeek receives sanitized structured text only, never images/PII.
- `MAPBOX_TOKEN` is server-side only; never in the browser, responses or logs. Geo/drop endpoints are `POST` so exact coordinates never enter the logged URL query.
- The final `POST /api/items` payload is unchanged; the wizard prefill is advisory.
- Wizard analyze: 1–4 photos, 8 MB each, JPEG/PNG/WebP; no persistence.
- Rate limits: analyze 20/min, reverse/drop-points 60/min per user.
- Demo timeouts: `demo/config.yml` read 10s / write 60s / shutdown 10s.
- All UI copy Italian; no exact coordinates or private profile in the summary.
- Existing privacy, custody, pass, fee, institution and auth rules remain.

## Review Focus

1. **Document first photo** — a document classification must stop additional photos and the private prefill, and never send document content to DeepSeek.
2. **Provider outage** — D3, DeepSeek or Mapbox down must leave a usable wizard, not a 500 or a dead step.
3. **Final write authority** — editing the category must not bypass the final D3 document/sensitive floor when `API_D3_ALLOW_IMAGES=true`.
4. **Drop-point operability** — every returned point must have an active account able to accept custody.
5. **User edits** — re-analysis (only on demand) must not overwrite prefilled fields the finder already edited.

---

### Task 1: Mapbox platform client + config

**Files:**
- Create: `internal/platform/mapbox/client.go`, `internal/platform/mapbox/client_test.go`
- Modify: `cmd/webapi/load-configuration.go`, `.env.example`

**Interfaces:**
- Produces: `mapbox.Point{Lat,Lng float64}`, `mapbox.Place{Zone,PlaceName,Source string}`, `mapbox.Leg{DistanceMeters,DurationSeconds float64}`, `mapbox.New(baseURL, token string, timeout time.Duration) *Client`, `(*Client).Reverse(ctx, Point) (Place, error)`, `(*Client).WalkingMatrix(ctx, origin Point, destinations []Point) ([]Leg, error)`.
- Config: `Mapbox.Token` from `MAPBOX_TOKEN`; base URL defaults to `https://api.mapbox.com`.

- [ ] **Step 1: Write failing tests** for reverse (path, query `types/language/limit/access_token`, first feature, empty features), matrix (path, `sources=0`, `annotations=distance,duration`, parse row 0, null leg), non-OK, timeout and missing token → `errs.ErrUnavailable`.
- [ ] **Step 2: Run `go test ./internal/platform/mapbox` and see failures.**
- [ ] **Step 3: Implement the client** with `http.Client{Timeout: timeout}`, `url.Values`, bounded response bodies, and `ErrUnavailable` wrapping. No logging, no token in errors.
- [ ] **Step 4: Add config** `Mapbox struct { Token string \`conf:"env:MAPBOX_TOKEN"\` }` and `MAPBOX_TOKEN=` in `.env.example`.
- [ ] **Step 5: Run tests + `go vet ./internal/platform/mapbox`; commit** `feat: add mapbox geocoding and walking matrix client`.

### Task 2: D3 `Analyze` + case color

**Files:**
- Modify: `internal/domain/item/catalog.go`, `item.go`, `service.go`, `service-interface.go`, `service_test.go`
- Maybe modify: test fakes implementing `item.Service`

**Interfaces:**
- Produces: `CatalogResult.CaseColor string`; `CatalogForm.CaseColor string`; `(item.Service).Analyze(ctx, photos []ImageInput, form CatalogForm) (CatalogResult, error)`.
- `Analyze` always sends images (wizard path); `Catalog` keeps the `AllowD3Images` flag.

- [ ] **Step 1: Write failing tests**: `case_color` is decoded from D3; `Analyze` sends images even when `Deps.AllowD3Images=false`; a document category still sends no images; existing catalog fallback remains.
- [ ] **Step 2: Run `go test ./internal/domain/item` and see failures.**
- [ ] **Step 3: Add `case_color` to `catalogQuestions` and `CatalogResult`; refactor the internal catalog call to take an `allowImages` argument; add `Analyze` to the interface and implementation.**
- [ ] **Step 4: Update fakes/compile errors; run the item tests.**
- [ ] **Step 5: Commit** `feat: add d3 analyze path and case color`.

### Task 3: Wizard domain — Analyze, planner, fallback, normalize

**Files:**
- Create: `internal/domain/wizard/{wizard.go,service.go,service-interface.go,planner.go,plan.go,normalize.go,deepseek.go}`
- Create: `internal/domain/wizard/service_test.go`, `planner_test.go`
- Modify: `internal/platform/deepseek` only if a small helper is needed (prefer `Complete` with tools).

**Interfaces:**
- Produces:
  - `wizard.AnalyzeInput{Photos []item.ImageInput; Slot string; Note string}`
  - `wizard.PhotoSlot{Key,Label,Instruction string; Required bool}`
  - `wizard.Prefill{Category,Color,Condition string}`
  - `wizard.Analysis{ObjectType string; Confidence float64; PhotoPlan []PhotoSlot; Prefill Prefill; PrivatePrefill string; Warnings []string; Source string; IsDocument,Sensitive bool}`
  - `wizard.PlannerInput{Category,Color,Condition,CaseColor string; HasCrack,HasSticker,HasCase,IsDocument bool; PhotoCount int}`
  - `wizard.Plan{PhotoPlan []PhotoSlot; Prefill Prefill; PrivatePrefill string; Warnings []string}`
  - `wizard.Deps{Items Analyzer; Planner Planner; Router Router; Geocoder Geocoder; Now func() time.Time}` with `Analyzer interface { Analyze(...) }`, `Planner`, `Router`, `Geocoder`.
  - `wizard.Service` with `Analyze`, `Nearest`, `Reverse` (Task 4 fills Nearest/Reverse; this task can stub them returning empty).
  - `wizard.New(db repository.Database, deps Deps) Service`
  - `wizard.NewDeepSeekPlanner(client *deepseek.Client) Planner`
- `Analyze` rules: validate 1–4 photos; D3 failure → manual analysis; document → no planner/no extra photos/no private text; planner failure → deterministic fallback; normalize D3 values to Italian UI values.

- [ ] **Step 1: Write failing tests** for: D3+planner path; document short-circuit (planner not called); planner failure fallback; analyzer failure manual; category/color/condition mapping; planner output truncated to 3 slots; no images in `PlannerInput`; unknown plan category falls back to D3.
- [ ] **Step 2: Run `go test ./internal/domain/wizard` and see failures.**
- [ ] **Step 3: Implement types, normalize table, deterministic plans, private-text builder, DeepSeek planner (tool `submit_wizard_analysis`, strict JSON, timeout/error fallback), and `Analyze`.**
- [ ] **Step 4: Run tests + `go vet`; commit** `feat: add finder wizard analysis and planner`.

### Task 4: Drop-off points, reverse geocode, seed

**Files:**
- Create: `internal/domain/wizard/dropoff.go`, `dropoff_test.go`
- Modify: `internal/seed/seed.go`, `seed_test.go`, `internal/domain/auth/service.go`, `webui/src/format.ts`

**Interfaces:**
- Produces:
  - `wizard.Point` alias? Use `mapbox.Point`.
  - `wizard.DropPoint{ID,Name,Address,Hours string; DistanceMeters,DurationSeconds float64; WalkingMinutes int; Approximate bool; Source string}`
  - `(Service).Nearest(ctx, at mapbox.Point, limit int) ([]DropPoint, error)`
  - `(Service).Reverse(ctx, at mapbox.Point) (mapbox.Place, error)`
- `Nearest`: active intermediaries only; Mapbox matrix when available; haversine ×1.3 fallback; sort duration/distance/name; cap 5 (max 10); if >24 points prefilter by haversine to 24 before matrix.
- `Reverse`: Mapbox geocoder; on error return `Place{Source:"unavailable"}` and nil.
- Seed: 7 active points (existing 2 + 5) and 5 intermediary users; auth demo users updated; format labels updated.

- [ ] **Step 1: Write failing tests** for active filtering, matrix sorting, top-5 cap, haversine fallback, approximate flag, empty list, reverse success/fallback; seed has 7 active points and 7 intermediary users.
- [ ] **Step 2: Run `go test ./internal/domain/wizard ./internal/seed` and see failures.**
- [ ] **Step 3: Implement `Nearest`/`Reverse`; add the seed rows/users with coordinates/addresses/hours; update `format.ts` labels.**
- [ ] **Step 4: Run tests; commit** `feat: add walking drop-off points and demo partners`.

### Task 5: HTTP routes, wiring, API spec

**Files:**
- Create: `internal/handler/route/wizard.go`, `internal/handler/dto/wizard.go`, `internal/handler/wizard_test.go`
- Modify: `internal/handler/api-handler.go`, `api.go`, `route/handler.go`, `cmd/webapi/main.go`, `internal/testserver/server.go`, `doc/api.yaml`

**Interfaces:**
- Produces:
  - `POST /api/finder/wizard/analyze` (finder/intermediary, multipart `payload`+`photos`) → `AnalysisResponse`
  - `POST /api/geo/reverse` (finder/intermediary, JSON `{lat,lng}`) → `{zone,placeName,source}`
  - `POST /api/drop-points` (finder/intermediary, JSON `{lat,lng,limit}`) → `{points:[...],source,approximate}`
- `handler.Config.WizardService` optional; if nil, the routes are not registered.
- `routes.New` carries it; `WebAPIHandler.Wizard`; limiters `wizardLimiter` (20/min), `geoLimiter` (60/min).
- Main builds: mapbox client from `cfg.Mapbox`, `wizard.New(db, Deps{Items: itemService, Planner: wizard.NewDeepSeekPlanner(deepseekClient), Router: mapboxClient, Geocoder: mapboxClient})`.
- Testserver builds the same with empty token (fallback).

- [ ] **Step 1: Write failing handler tests** for roles (401/403), analyze 1 photo + prefill JSON, 400 no photos/bad MIME/oversized, 429, no token/coordinates/private leak, reverse/drop auth + shape.
- [ ] **Step 2: Run `go test ./internal/handler -run Wizard` and see failures.**
- [ ] **Step 3: Implement DTOs/routes/wiring/limiters; update main/testserver; update `doc/api.yaml`.**
- [ ] **Step 4: Run handler tests + `go build ./cmd/webapi/` + `go test ./...`.**
- [ ] **Step 5: Commit** `feat: expose finder wizard and geo routes`.

### Task 6: Frontend API/types + wizard shell + step 1

**Files:**
- Modify: `webui/src/api.ts`, `types.ts`, `catalog.ts`, `format.ts`, `views/FinderEnrollView.vue`
- Create: `webui/src/views/FinderWizard.vue`, `webui/src/components/wizard/{WizardProgress,WizardStepPhotos,PhotoPlanSlot}.vue`

**Interfaces:**
- `analyzeWizardPhoto(payload, photos): Promise<WizardAnalysis>`
- `reverseGeocode(lat,lng): Promise<{zone,placeName,source}>`
- `nearestDropPoints(lat,lng,limit): Promise<DropPointResponse>`
- Types `WizardAnalysis`, `PhotoSlot`, `Prefill`, `DropPoint`, `ReverseGeocode`.
- `FinderWizard` state: step 1–5; photos; analysis; edited fields; location; private; drop point; consent.
- Step 1: first photo with `capture`, privacy banner, analyze button, plan slots, prefilled editable category/color/condition, brand/model empty with hint, "Rianalizza" only on demand.

- [ ] **Step 1: Add API methods/types; build the shell with progress and step navigation.**
- [ ] **Step 2: Build step 1** with photo capture, analyze loading/error/fallback, plan slots, prefill form. Use the existing resize helper; max 4 photos.
- [ ] **Step 3: Add new UI categories** (`laptop`, `gioielli`, `orologio`, `libro`, `bottiglia`) to `catalog.ts`.
- [ ] **Step 4: Run `npm --prefix webui run build`; fix type errors.**
- [ ] **Step 5: Commit** `feat: add finder wizard shell and photo step`.

### Task 7: Frontend steps 2–3 — location and private details

**Files:**
- Modify: `webui/src/views/FinderWizard.vue`, `webui/src/components/GeoPicker.vue`, `webui/src/format.ts`
- Create: `webui/src/components/wizard/{WizardStepLocation,WizardStepPrivate}.vue`

**Interfaces:**
- Step 2: GeoPicker lat/lng; debounced `reverseGeocode` sets zone; foundAt/notes; manual zone.
- Step 3: private textarea prefilled from `analysis.privatePrefill`; attribute chips append text; user edits; document warning.

- [ ] **Step 1: Build step 2** with geolocation, reverse geocode loading/fallback, zone/date/notes validation.
- [ ] **Step 2: Build step 3** with prefilled private text, chips (`custodia`, `crepa`, `adesivo`) and edit protection (only apply prefill once).
- [ ] **Step 3: Run webui build; commit** `feat: add wizard location and private detail steps`.

### Task 8: Frontend steps 4–5 + E2E/docs + full gates

**Files:**
- Create: `webui/src/components/wizard/{WizardStepDropPoint,WizardStepDisclaimer}.vue`
- Modify: `webui/src/views/FinderWizard.vue`, `webui/src/views/LoginView.vue`, `webui/src/router.ts` (if needed), `tests/e2e/demo_test.go` or new `tests/e2e/wizard_test.go`, `docs/demo-script.md`, `docs/demo-checklist.md`, `README.md`, `demo/config.yml`

**Interfaces:**
- Step 4: call `nearestDropPoints` when entering the step; cards with walking minutes/metres, "stima" badge, radio selection; retry on empty/error.
- Step 5: summary (object, photos count, zone/date, private presence, point/distance), disclaimer/rights checkbox, submit via `createItem`, receipt.
- E2E: with no D3/Mapbox token, analyze returns manual/fallback, drop points returns haversine, final submit still works.
- Demo config timeouts 10/60; docs mention the wizard; login groups extra intermediaries.

- [ ] **Step 1: Build step 4** with drop-point loading/error/empty and selection.
- [ ] **Step 2: Build step 5** with summary, disclaimer/rights, submit, receipt; ensure the finder cannot bypass the checkbox.
- [ ] **Step 3: Wire `FinderEnrollView` to `FinderWizard`; group extra intermediaries in the login view.**
- [ ] **Step 4: Add the E2E wizard fallback test; update demo docs/config.**
- [ ] **Step 5: Run full gates**: `go test ./... -count=1`, `go test ./tests/e2e/... -count=1`, `go vet ./...`, `npm --prefix webui run build`, `go build -tags webui ./cmd/webapi/`, `go build ./cmd/seed/`, `docker compose config -q`; headless/manual five-step smoke.
- [ ] **Step 6: Commit** `feat: complete finder wizard flow and demo docs`.

## Self-Review

- Spec coverage: mapbox (T1), D3/analyze (T2), wizard/planner (T3), drop-off/seed (T4), HTTP/wiring (T5), UI steps 1–5 (T6–T8), E2E/docs (T8). Document privacy, outages, final authority, operability and edit protection are in Review Focus and each has a test in its owning task.
- Type consistency: `item.ImageInput` reused; `wizard` types listed once; `mapbox.Point/Place/Leg` consumed by `wizard` and handlers; `WizardService` optional in `handler.Config`.
- Deliberate deferral: the item-service intermediary referential check remains the existing deferred minor; the wizard only offers active IDs. The final `POST /api/items` contract stays unchanged.
