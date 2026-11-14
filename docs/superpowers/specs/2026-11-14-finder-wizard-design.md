# Finder wizard — design

Date: 2026-11-14
Branch: `feat/finder-wizard`
Status: approach approved in conversation (A); design awaiting builder check and user approval.

## Intent

Turn the finder enrollment form into a guided, photo-first wizard. The finder
takes one photo, the system recognizes the object and asks for the few
additional photos that actually help identify it, pre-fills the object, location
and private-detail cards, proposes the five nearest drop-off points by walking
distance, and ends with the legal/privacy disclaimer and submit. The finder
reviews and edits every prefilled value; the AI is an assistant, never the
authority.

The wizard is the "step-up" demo feature: it should feel like a product, not a
form with a spinner, while preserving the existing privacy, custody and legal
rules.

## Why now

The MVP works end to end. The current finder form asks for category, brand,
model, color and condition manually and offers two hard-coded drop points; it
does not use the photo beyond attachment. The next demo needs a visible leap in
finder experience without destabilizing the custody/pass/PA flows.

## Goals

- Finder-only five-step wizard; the intermediary form stays as it is.
- First photo triggers classification and a photo plan; the plan says how many
  additional photos are needed and what each must show.
- Step 1 pre-fills the object card: category, color, condition, photos and
  private-detail hints. Brand and model remain editable and are **not**
  hallucinated.
- Step 2 reverse-geocodes the browser position into a found zone; manual entry
  remains available.
- Step 3 pre-fills the private-details text from visible attributes (case color,
  cracks, stickers); the finder edits it.
- Step 4 shows the **top five active drop-off points** sorted by walking
  distance/time from the report position, with an explicit "stima" badge when
  the routing provider is unavailable.
- Step 5 shows a summary, the legal/privacy disclaimer, the rights-assignment
  consent and submit.
- Every AI or network step has a deterministic fallback; no provider outage
  blocks enrollment.
- No new database tables, no item state changes, no change to the final
  `POST /api/items` contract. The final write re-runs D3 (with images enabled
  in the demo) and remains the authority; the wizard analysis is UX only.

## Non-goals

- No wizard for the intermediary enrollment (it stays the current form).
- No public catalog, no public photos, no exposure of exact finder coordinates.
- No owner/searcher claim flow changes; no pass/fee/PA changes.
- No automatic brand/model recognition. D3 cannot read open text and DeepSeek
  must not guess an identity attribute for a legal registry; the fields stay
  editable with a clear "da confermare" hint.
- No map rendering or route polyline; the drop-off step is a ranked list.
- No real payment, no video, no deployment changes.

## Anti-goals

- Sending raw document images or document data to DeepSeek.
- Asking the finder to unlock a phone, show notifications, or photograph the
  inside of a wallet/document. The plan asks only for visible physical details.
- Blocking the wizard when D3, DeepSeek or Mapbox is down.
- Showing the finder's exact coordinates in any response or public DTO.
- Weakening the existing rights-assignment, sensitive-item or custody rules.

## Constraints

- Existing Go/Gin/SQLite/Vue 3 stack; no new database tables.
- D3 (`decision.vllm-sr.ai`) is the official decision/perimeter engine and the
  only component allowed to see the item photos. DeepSeek receives structured
  text only, never images. The first photo may accidentally contain a document;
  the UI warns against it and D3 is the trusted engine. The wizard never asks
  for document contents.
- DeepSeek is the planner/prefill writer; it must return structured JSON and
  must not invent private attributes. D3 is the factual source.
- Mapbox token is read server-side as `MAPBOX_TOKEN` (already in `.env`); it is
  never exposed to the browser or logged. Geocoding and the walking matrix are
  backend calls.
- The final `POST /api/items` remains the only write and the authority; the
  wizard prefill is advisory. The final item service still applies the risk
  policy and the document/sensitive floor. The demo `.env` sets
  `API_D3_ALLOW_IMAGES=true` so the final D3 call sees the photos too; no new
  request field carries the AI result, so a client cannot forge it.
- Existing privacy rules, `.env` hygiene, role checks and rate limits remain.

## Approaches considered

### A — D3 is the eye, DeepSeek is the director (chosen)

D3 classifies the photos with the existing catalog question set (extended with
`case_color`). DeepSeek receives only the sanitized D3 result and writes the
photo plan, the card-1 prefill and the private-detail draft. Mapbox serves
reverse geocoding and the walking matrix. Deterministic fallbacks cover every
provider.

Why: D3 is already the image decision engine; DeepSeek is already the text/tool
orchestrator; the privacy boundary stays intact; the free Mapbox tier is enough
for the demo.

### B — DeepSeek vision for everything

Rejected: the current DeepSeek client is text/tool only; the configured model's
image support is unverified; sending item photos (and potentially documents) to
a public LLM would break the privacy boundary.

### C — Server-side wizard draft/session

Rejected: a draft table/session would let the final submit trust a cached
analysis, but it adds state, expiry and cleanup for a flow that already has the
photos on the client. The final submit re-runs D3 (or uses the form values)
authoritatively; the wizard analysis is UX only.

### Routing options

- **Mapbox Matrix walking (chosen)**: one backend call, real walking
  distance/duration, free tier, token already available.
- Per-destination Mapbox Directions: more calls, no benefit for seven points.
- Haversine only: cheap but not walking; kept as the explicit fallback with an
  `approximate` badge.

## Flow

```
1  Foto → analisi → piano foto → card oggetto precompilata → Avanti
2  Posizione → zona ritrovamento auto → Avanti
3  Dettagli riservati precompilati → Avanti
4  Punti di raccolta: top 5 a piedi → Avanti
5  Riepilogo + disclaimer/diritti + Registra
```

The stepper is client-side inside one route. State survives back/next; a page
reload restarts the wizard (acceptable; no server draft).

### Step 1 — photo and object

- The finder takes/selects the first photo. The UI shows a privacy banner:
  "Non fotografare documenti, notifiche o dati personali".
- `POST /api/finder/wizard/analyze` runs **once on the first photo** and returns
  the D3 classification, the additional photo plan, the prefill and warnings.
  Additional slot photos are attached without another AI call; a "Rianalizza"
  action appears if the finder replaces the first photo or materially changes
  the category. The plan is frozen after the first analysis unless the finder
  asks for a new one; the finder's edits to prefill/private text are never
  overwritten.
- The UI shows the first photo plus the additional slots. Each slot has a label,
  an instruction, a required/optional flag and a camera/file input. Optional
  slots can be skipped.
- The object fields (category, color, condition) are prefilled and editable.
  Brand/model are empty with "D3 non li legge: conferma o compila".
- If D3 says document: the plan stops, no additional photo slots are shown, the
  category prefills `documenti`, a sensitive warning appears, and the private
  details are not prefilled. The finder can continue to the drop-off step.
- If D3 or DeepSeek fails: the wizard shows the manual form with a generic
  1–2 photo plan and a warning; it does not block.
- "Avanti" validates at least the first photo, a category, and every required
  slot.

### Step 2 — where and when

- "Usa la mia posizione" fills lat/lng; a debounced backend reverse geocode
  fills the zone (neighborhood/locality).
- The zone remains editable; if geolocation or Mapbox fails, the finder types
  it.
- Found date/time defaults to now; notes optional.

### Step 3 — private details

- DeepSeek writes a short Italian draft from D3's visible attributes (case
  color, crack, sticker, case presence), e.g. "Custodia blu, crepa sullo
  schermo, adesivo sul retro".
- The finder edits the text. The draft never includes documents, names, phone
  numbers or contents; for document items the field starts empty with a warning.
- The final payload stores it as `privateProfile.details` (existing shape).

### Step 4 — drop-off point

- `POST /api/drop-points` with `{lat,lng,limit}` returns the active points
  sorted by walking duration (then distance, then name).
- Each card shows name, address, opening hours, walking minutes, metres and an
  "stima" badge when the routing source is the haversine fallback. There is no
  separate `area` field: the seed address includes the neighborhood.
- The first point is preselected; the finder can choose another. If there is no
  location, the endpoint returns active points sorted by name with no distance
  and the UI asks for a location or manual selection.
- All returned points are active partner accounts, so the selected point can
  accept custody.

### Step 5 — summary and disclaimer

- Summary: object, photos count, zone/date, private details presence, chosen
  point and walking time.
- Legal/privacy disclaimer and rights-assignment consent in one explicit
  checkbox; submit is disabled until accepted.
- Submit uses the existing multipart `POST /api/items` with all photos and the
  edited payload. The existing receipt and redirect are unchanged.

## Backend design

### New domain package: `internal/domain/wizard`

Types:

```go
type ImageInput struct { Data []byte; MediaType string } // bytes only, never persisted
type AnalysisInput struct { Photos []item.ImageInput; Slot string; Note string }
type PhotoSlot struct { Key, Label, Instruction string; Required bool }
type Prefill struct { Category, Color, Condition string }
type Analysis struct {
    ObjectType      string
    Confidence      float64
    PhotoPlan       []PhotoSlot // additional slots, max 3
    Prefill         Prefill
    PrivatePrefill  string
    Warnings        []string
    Source          string // "d3+deepseek", "d3+fallback", "manual"
    IsDocument      bool
    Sensitive       bool
}
```

Ports:

```go
type Analyzer interface { Analyze(ctx context.Context, photos []item.ImageInput, form item.CatalogForm) (item.CatalogResult, error) }
type Planner interface { Plan(ctx context.Context, input PlannerInput) (Plan, error) }
type Router interface { Walking(ctx context.Context, origin Point, destinations []Point) ([]Leg, error) }
type Geocoder interface { Reverse(ctx context.Context, lat, lng float64) (Place, error) }
```

`Analyze`:

1. Validate 1–4 photos, MIME JPEG/PNG/WebP, size limit (8 MB each) before any
   provider call.
2. Call the D3 analyzer (always with images for this path). On failure return a
   manual analysis with a generic plan and a warning.
3. If `IsDocument`: return a sensitive document analysis, no additional photo
   slots, no planner call, no private prefill. This is the privacy override.
4. Build a sanitized `PlannerInput` (category, color, condition, case color,
   flags, photo count; no images, no PII) and call DeepSeek.
5. On planner failure/timeout: use the deterministic plan table and derive the
   prefill/private text from D3.
6. Normalize D3's English category/color/condition to the UI values; if the
   planner suggests an unknown category, keep D3's value.
7. Return the analysis. Never include raw photo bytes, provider tokens or
   internal errors in the response.

The endpoint is called once per item (first photo). A repeated call is the
explicit "Rianalizza" action and returns a complete new analysis; the client
keeps its edited values except for fields the finder has not touched. The
20/min limit therefore covers one normal call and a small number of retries, not
one call per slot.

Deterministic plan table:

| Category | Additional slots |
|---|---|
| smartphone | Retro (required); custodia/segno particolare (optional) |
| wallet | Esterno (required); interno **senza documenti** (optional) |
| keys | Mazzo completo (required); portachiavi (optional) |
| document | none; warning "non aggiungere foto del contenuto" |
| backpack/bag | Esterno (required); dettaglio (optional) |
| glasses/headphones/watch/jewelry | Dettaglio (optional) |
| generic | Dettaglio/segno particolare (optional) |

The first photo is always required and is slot 1; the plan lists only the
additional slots. Maximum 4 photos total.

DeepSeek planner: one tool/JSON call with a strict schema:
`photoPlan[]` (max 3), `prefill{category,color,condition}`, `privatePrefill`,
`warnings[]`. The system prompt says: only visible physical attributes, no
brand/model guessing, no document contents, no names/phones/addresses, one
instruction per slot, Italian copy. On any parse/timeout error, fallback.

### D3 classification changes

- Add a `case_color` choice question (same palette as `color`) and
  `CatalogResult.CaseColor`.
- Add `item.Service.Analyze(ctx, photos, form) (CatalogResult, error)` that runs
  the same question set **with images** for the wizard. The existing `Catalog`
  keeps the `AllowD3Images` deployment flag for enrollment.
- The D3 result remains the factual source; the wizard never trusts the
  planner for `is_document`/`sensitive`.

### Category and color normalization

D3 returns English category values. The wizard maps every one of them to an
existing or new UI category; nothing falls back to `altro` merely because the
UI lacked an option (which would lower laptop/jewelry/watch from high to the
default medium risk).

| D3 | UI value | UI label | Risk policy |
|---|---|---|---|
| smartphone | smartphone | Smartphone | high |
| laptop | laptop | Laptop | high |
| wallet | portafoglio | Portafoglio | high |
| keys | chiavi | Chiavi | sensitive |
| document | documenti | Documenti / carta d'identità | sensitive |
| jewelry | gioielli | Gioielli | high |
| watch | orologio | Orologio | high |
| glasses | occhiali | Occhiali | medium |
| backpack | zaino | Zaino / borsa | medium |
| headphones | cuffie | Cuffie / auricolari | medium |
| book | libro | Libro / quaderno | low |
| bottle | bottiglia | Bottiglia / borraccia | low |
| clothing | abbigliamento | Abbigliamento | low |
| umbrella | ombrello | Ombrello | low |
| other | altro | Altro | default medium |

`itemCategories` gains `laptop`, `gioielli`, `orologio`, `libro`, `bottiglia`.
The colors map `black→nero`, `white→bianco`, `gray→grigio`, `red→rosso`,
`blue→blu`, `green→verde`, `yellow→giallo`, `brown→marrone`, `pink→rosa`,
`other→multicolore`; conditions map `new→like_new`, `good→good`, `worn→fair`,
`damaged→poor`. The `Prefill` type has no custom-category field; `other` maps to
`altro` and the finder can type a custom category only if they choose it.

### Drop-off domain: `internal/domain/dropoff`

- `Nearest(ctx, lat, lng, limit) ([]DropPoint, error)`.
- Load `ListIntermediaries`, keep `status == "active"` and valid coordinates.
- Use the `Router` port. Mapbox Matrix when configured and healthy; otherwise
  haversine × 1.3 walking factor, `approximate=true`, `source="haversine"`.
- Matrix limit: one call for up to 24 destinations (the seeded set is 7). If a
  deployment has more, prefilter to the 24 nearest by haversine before the
  matrix call; the "top five" remains exact for the demo scale and is labeled
  approximate only when the provider is down.
- Sort by duration, then distance, then name; cap at `limit` (default 5, max
  10). Return an empty list if no active points.
- `DropPoint` response: id, name, address, hours, distance metres, duration
  seconds, walking minutes, approximate, source.

### Mapbox platform: `internal/platform/mapbox`

- Server-side client with `MAPBOX_TOKEN`; base URL configurable for tests.
- `Reverse(ctx, lat, lng)`: Geocoding v5
  `/{lng},{lat}.json?types=neighborhood,locality,place&language=it&limit=1`.
  Return the first feature's `text` as zone plus `place_name`; empty when no
  feature.
- `WalkingMatrix(ctx, origin, destinations)`: Directions Matrix v1,
  `mapbox/walking`, `sources=0`, `annotations=distance,duration`. Parse row 0.
- Timeouts: 3 s geocoding, 5 s matrix; `ErrUnavailable` on transport/non-OK.
- Token in the query string is never logged. In-memory caches: geocode keyed by
  5-decimal lat/lng for 1 hour; matrix keyed by 4-decimal origin + destination
  IDs for 5 minutes. Cache misses are best-effort; no external dependency fails
  the request because the service falls back.
- Demo timeouts: `demo/config.yml` raises `readtimeout` to 10 s and
  `writetimeout` to 60 s (matching the local `.env`), so the 10 s D3 + 15 s
  planner budget fits the analyze request; the final submit also fits with the
  same write budget. If the demo config is used without `.env`, these values
  still apply.

### HTTP

- `POST /api/finder/wizard/analyze` — roles finder/intermediary; multipart
  `payload` JSON `{slot?, note?}` + repeated `photos`; returns `Analysis`.
  Max 4 photos, 8 MB each, JPEG/PNG/WebP; no persistence; 10 s D3 + 15 s
  planner bounds; rate limit 20/min per user.
- `POST /api/geo/reverse` with `{lat,lng}` — roles finder/intermediary; returns
  `{zone, placeName, source}`; 60/min. POST keeps exact coordinates out of the
  request URL, which the shared request logger records.
- `POST /api/drop-points` with `{lat,lng,limit}` — roles finder/intermediary;
  returns `{points:[...], source, approximate}`; 60/min.
- Existing `POST /api/items` unchanged; the wizard sends the same payload/photos.
- Update `doc/api.yaml` with the three paths and schemas.

### Config

- `Mapbox.Token` from `MAPBOX_TOKEN` (explicit `env:` tag; no `API_` prefix);
  `.env.example` gets `MAPBOX_TOKEN=`.
- No new D3 flag: the wizard path explicitly uses images; the enrollment catalog
  keeps `API_D3_ALLOW_IMAGES` as today.
- No new database migration.

### Item validation hardening

The wizard makes the drop point dynamic. Add a referential check in
`item.CreateItem`: the selected intermediary must exist and be `active` before
the item/custody transaction. This closes the deferred Task-4 minor and prevents
a finder from submitting an unknown point.

### Seed

Add five active partner points and matching intermediary users so every point
returned by `/api/drop-points` can accept custody. Suggested demo data:

| id | name | address | lat/lng |
|---|---|---|---|
| bar-x | Bar X | Via dei Volsci 12, San Lorenzo | 41.9015, 12.5150 |
| portineria-sapienza | Portineria Sapienza | Piazzale Aldo Moro 5 | 41.9028, 12.5144 |
| tabaccheria-termini | Tabaccheria Termini | Via Giolitti 34 | 41.9010, 12.5015 |
| caffe-trastevere | Caffè Trastevere | Piazza Mastai 9 | 41.8890, 12.4690 |
| farmacia-san-giovanni | Farmacia San Giovanni | Via Appia Nuova 20 | 41.8850, 12.5090 |
| punto-prati | Punto Prati | Via Cola di Rienzo 100 | 41.9080, 12.4620 |
| mercato-ostiense | Mercato Ostiense | Via Ostiense 95 | 41.8720, 12.4780 |

All status `active`, capacity > 0, demo agreement reference. The login screen
groups the intermediaries (the two demo points visible, the others in a
collapsible "Altri punti di raccolta") so every selectable point is operable.
The existing demo `bar-x` remains the presentation choice.

## Frontend design

- `FinderEnrollView` uses a new `FinderWizard.vue`; `ItemEnrollForm.vue` remains
  for the intermediary enrollment.
- Wizard shell: progress (1–5), back/next, step validation, focus management,
  loading/error/empty states, mobile-first.
- Step components: `WizardStepPhotos`, `WizardStepLocation`,
  `WizardStepPrivate`, `WizardStepDropPoint`, `WizardStepDisclaimer`.
- Photo slot component: camera/file input (`capture="environment"`), preview,
  replace/remove, instruction, required/optional badge. Reuse the existing
  client-side JPEG resize helper; max 4 photos.
- Location step reuses `GeoPicker`; the parent watches lat/lng and calls
  `/api/geo/reverse` with debounce; the zone input remains editable.
- Drop-point cards show walking minutes/metres, "stima" badge when approximate,
  accessibile radio group; default first.
- Summary step shows the final values, then the disclaimer/rights checkbox.
- All copy in Italian; no private profile or exact coordinates in the summary
  beyond the zone (the exact coordinates are hidden, as today).
- API client additions: `analyzeWizardPhoto`, `reverseGeocode`, `nearestDropPoints`;
  types for `WizardAnalysis`, `DropPoint`, `ReverseGeocode`.

## Failure modes and fallbacks

| Failure | Behavior |
|---|---|
| D3 down/timeout | Manual analysis: generic 1–2 photo plan, no prefill, warning; the wizard continues. |
| DeepSeek down/timeout/JSON error | D3 prefill + deterministic plan + private draft; warning "suggerimenti parziali". |
| D3 says document | No additional photos, no planner, sensitive warning, category `documenti`; the finder can still submit. |
| Mapbox geocoding down | Zone empty, manual entry; `source="unavailable"`. |
| Mapbox matrix down/no token | Haversine × 1.3 fallback, sorted by distance, `approximate=true`. |
| No active drop points | Step 4 shows an error and a retry; submit is blocked (no legal finder). |
| Rate limit | 429 with an Italian "attendi un minuto" message; no retry storm. |
| Bad/oversized photo | Step 1 inline error; no provider call. |

## Privacy and security

- DeepSeek never receives photos, private profile values, exact coordinates,
  document fields or the finder's identity. The planner input is the sanitized
  D3 result only.
- D3 is the official decision engine; the wizard never asks for document
  contents. The first-photo banner warns; a document classification stops the
  photo plan and the private prefill.
- Mapbox receives the origin and public shop coordinates only; no finder
  identity. The token stays server-side and out of logs/responses.
- The analyze endpoint authenticates finder/intermediary, bounds body size,
  validates MIME, persists nothing, rate-limits and never echoes internal
  errors.
- The final item write remains role-checked and policy-checked; the drop point
  referential check prevents a forged point.

## Testing

- Mapbox client: geocoding/matrix happy paths, empty features, non-OK, timeout,
  missing token, no token leak.
- Wizard service: D3+planner path, document override, planner fallback, D3
  fallback, mapping, max slots, no images/PII in planner input, request limits.
- Drop-off service: active filtering, sorting, top-5 cap, haversine fallback,
  approximate flag, empty list.
- Handlers: roles, multipart limits, MIME, 429, response privacy, reverse
  geocode/drop points auth, no token in JSON.
- E2E: with no D3/Mapbox token, the analyze endpoint returns manual/fallback,
  drop points returns haversine, and the final submit still creates the item.
- Frontend: `npm --prefix webui run build`; a headless-Chrome smoke of the five
  steps with fallback providers; manual smoke with the real `.env` (D3 +
  DeepSeek + Mapbox).
- Full gates: `go test ./...`, targeted `-race`, `go vet`, both Go builds,
  webui build, `docker compose config -q`.

## Acceptance criteria

1. Finder logs in and sees a five-step wizard, not the old single form.
2. First photo returns an object classification, a photo plan with the right
   number/instructions and card-1 prefill; the finder can edit every value.
3. A document photo stops additional photo requests and shows the sensitive
   path; no document content is requested or sent to DeepSeek.
4. Geolocation fills a zone, or the finder can type it.
5. Private details are prefilled from visible attributes and editable.
6. Step 4 returns five active points with walking minutes and a fallback badge
   when approximate; selecting one is required.
7. Summary + disclaimer + consent; submit creates the item and the receipt as
   before.
8. With D3, DeepSeek or Mapbox unavailable, the wizard still completes.
9. Intermediary enrollment and all existing flows/tests remain unchanged.
10. No secrets, tokens, exact coordinates or private profiles leak to the UI,
    logs or DeepSeek.

## Risks

- **Latency/cost**: two AI calls per analysis; mitigate with timeouts, rate
  limits, caching, and the analysis being optional. The final submit remains
  authoritative.
- **Planner reliability**: strict JSON/tool schema + deterministic fallback.
- **Mapbox free tier**: cache, one matrix call per origin, haversine fallback.
- **Document privacy**: the first photo may contain a document; the banner,
  early stop and trusted D3 boundary are the mitigation. If the user wants a
  stricter rule, the first step would need a pre-photo "non è un documento"
  gate; not chosen.
- **Brand/model**: not inferred; the finder confirms them. This is deliberate.
- **Demo accounts**: five extra partners; grouped login and seed tests keep the
  demo usable.

## Builder-check resolutions

The independent builder check raised seven questions; all are resolved here.

1. **Analyze frequency.** Analyze runs once on the first photo; additional slot
   photos are attached without an AI call. A "Rianalizza" action re-runs it
   when the first photo is replaced or the category materially changes. The plan
   is frozen until then; the finder's edited prefill/private text is never
   overwritten.
2. **Final write.** `POST /api/items` is unchanged; no new request field carries
   the AI result. The demo `.env` sets `API_D3_ALLOW_IMAGES=true`, so the final
   item service re-runs D3 with the photos and remains the authoritative
   document/sensitive floor. A forged category cannot bypass it.
3. **Categories.** The five missing UI categories are added and every D3
   category has an explicit mapping (table above); no high-risk category is
   downgraded to `altro`/medium.
4. **`area`.** Removed from the drop-point DTO/cards; the seeded address includes
   the neighborhood. No migration.
5. **Coordinates in logs.** Reverse geocoding and drop points are `POST` with a
   JSON body, so exact coordinates never appear in the logged URL query.
6. **Matrix size.** One Mapbox Matrix call handles up to 24 destinations; the
   seeded set is 7. Above 24, prefilter by haversine to the 24 nearest before
   the matrix call; the demo's top five is exact.
7. **Timeouts.** `demo/config.yml` uses 10 s read / 60 s write / 10 s shutdown,
   matching the `.env`; the 10 s D3 + 15 s planner budget fits, and the final
   submit fits the same write budget.

## What is left to the builder

- Exact DeepSeek system prompt/tool schema and the deterministic plan copy.
- File/component split and names beyond the ones above.
- Cache TTL constants and limiter values within the stated ranges.
- Seed capacity/hours/agreement values for the five new points.
- Login grouping presentation for the extra intermediaries.
- Exact Italian error/disclaimer copy and the summary layout.
- Test file organization.

Everything else above is decided.
