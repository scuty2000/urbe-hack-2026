# Registro unico oggetti smarriti — Sponsor Integration Follow-up Plan

> **For agentic workers:** this is a **delta plan** on top of `assets/2026-11-13-oggetti-smarriti-mvp.md` (the "MVP plan"). Read the MVP plan, `assets/2026-11-13-oggetti-smarriti-design.md` (the "design") and `assets/INTEGRAZIONI_SPONSOR.md` (the sponsor report, rationale only) first. Everything in the MVP plan still applies unless a section below says **AMENDS** or **SUPERSEDES**. Use superpowers:subagent-driven-development or superpowers:executing-plans; steps use checkbox (`- [ ]`) syntax.

**Goal:** Integrate four sponsor services, each with one precise role:

| Sponsor | Role | One-line scope |
|---|---|---|
| **Lovable** | Automated frontend development | Rebuilds from scratch, in React, the existing Vue app of MVP Tasks 13–14 (all four interfaces) and replaces it in `webui/`. |
| **Exa** | Knowledge outside the knowledge base | Agent tool used only when the private registry and the local verified KB cannot answer; always cited. |
| **ElevenLabs** | Accessibility for people with disabilities | Voice mode (STT + TTS) across finder/searcher flows, spoken pickup code, live captions at the counter. |
| **Avalanche (Team1)** | Verifiable audit | Merkle-root anchoring of `custody_events` on Avalanche C-Chain (Fuji testnet) + public proof/verification. |

**Architecture rule (unchanged):** `handler → domain → repository`. Every sponsor is a client under `internal/platform/<name>/` behind an interface, with a deterministic fallback, a health probe, and a test that skips without credentials. No sponsor is on the critical path of the demo.

**Assumed starting state:** the MVP plan is implemented through Task 14 — backend (Tasks 1–12) and the Vue 3 app in `webui/` (Tasks 13–14). Tasks 15–16 may still be open. This plan changes that codebase; follow the **Execution order** below.

---

## Global Constraints — AMENDS MVP "Global Constraints"

- **Frontend (SUPERSEDES the Vue bullet):** React 18 + Vite + TypeScript + Tailwind + shadcn/ui, **generated from scratch by Lovable**. **Lovable's scope is only the frontend under `webui/`**: it never creates or edits anything outside `webui/` (backend, migrations, scripts, docs, Docker are done by the agent). Lovable works on the dedicated repo `lostfound-webui`, whose root **is** the content of `webui/`; it is brought into the main repo with `git subtree` at prefix `webui/` (no submodule, no extra folder). **No Supabase, no Lovable Cloud/auth, no client-side DB.**
- **Existing Vue frontend = reference only.** Assume the Vue 3 app in `webui/` (boilerplate + MVP Tasks 13–14) exists. Lovable **rebuilds it from scratch** in React: screens, flows, copy and API usage are taken from the Vue app; no Vue code is ported or converted. The Vue app keeps running until the React app reaches parity (S5.5 cutover), then its sources are removed.
- **`webui/static-resources.go` is the only contact point with Go** (`//go:build webui`, `//go:embed "dist/*"` → `webui.Dist`). It stays in `webui/` before and after cutover. After cutover `webui/` = Lovable React app + `static-resources.go` + the generated, gitignored `webui/dist/`.
- **Build:** `scripts/build-webui.sh` = `npm --prefix webui ci && npm --prefix webui run build` (works for both the Vue app and the React app; both use npm and commit `package-lock.json`). Vite keeps its default `outDir: "dist"`, which is exactly `webui/dist` → embedded by `webui/static-resources.go`; Lovable's preview keeps working. Then `go build -tags webui ./cmd/webapi/`; served at `/dashboard/` as in the MVP plan.
- **npm everywhere** (as in the MVP plan). The boilerplate `AGENTS.md` still mentions `yarn` / `./open-node.sh`: at cutover, update `AGENTS.md` to say `webui/` is the Lovable React app (subtree of `lostfound-webui`), built with `scripts/build-webui.sh`.
- **Business logic stays server-side.** The SPA never computes fees, states, deadlines, thresholds or confidence. It renders DTOs.
- **Egress privacy rule (new, never cut):** no `private_profile` value, photo, document field, exact coordinate, audio recording or identity datum may leave the process towards Exa, ElevenLabs or Avalanche. Enforced by `internal/platform/egress` (Task S1) and tested.
- **On-chain rule:** only 32-byte hashes go on chain. No item IDs, user IDs, names, zones or timestamps in calldata or events.
- **Audio rule:** audio is never written to disk or DB; only the transcript is kept, and only inside the chat history it belongs to.
- **New config** (add to `cmd/webapi/load-configuration.go` and `.env.example`, all optional; empty = feature disabled + fallback):

```text
API_EXA_KEY=
API_EXA_URL=https://api.exa.ai
API_EXA_CACHE_TTL=24h

API_ELEVENLABS_KEY=
API_ELEVENLABS_URL=https://api.elevenlabs.io
API_ELEVENLABS_VOICE_ID=
API_ELEVENLABS_TTS_MODEL=eleven_flash_v2_5
API_ELEVENLABS_STT_MODEL=scribe_v1

API_AVAX_RPC_URL=https://api.avax-test.network/ext/bc/C/rpc
API_AVAX_CHAIN_ID=43113
API_AVAX_PRIVATE_KEY=          # dedicated Fuji testnet wallet, never a real one
API_AVAX_CONTRACT=             # CustodyAnchor address after deploy
API_AVAX_EXPLORER_TX_URL=https://testnet.snowtrace.io/tx/
API_AVAX_BATCH_MAX=256

API_CORS_EXTRA_ORIGINS=        # Lovable preview origin(s), dev only, ';'-separated
API_COOKIE_CROSS_SITE=false    # true only in dev with Lovable preview → SameSite=None; Secure
```

- **Verify external APIs before coding each client.** Endpoints, headers, model ids and field names below reflect the state known at planning time. Before writing each client, fetch current docs (Context7 MCP: `resolve-library-id` → `query-docs`) for Exa, ElevenLabs, go-ethereum and Foundry, and adjust. Keep the client interfaces in this plan unchanged even if the wire format differs.
- **Dependencies:** `github.com/ethereum/go-ethereum` (ethclient, crypto, accounts/abi/bind). After adding: `go mod tidy && go mod vendor` (boilerplate `AGENTS.md`). Foundry (`forge`, `cast`) only for the contract, not needed at runtime.

## Review Focus — AMENDS MVP "Review Focus" (add items 6–10)

6. **Egress leak.** Payloads sent to Exa/ElevenLabs and leaves sent to Avalanche never contain a seeded private value. Task S1 + S2 + S3 + S4.
7. **Spoken pickup code brute force.** The 6-char short code is bound to the custodian, expires with the pass, and locks after 5 failed attempts per custodian per 15 min. Task S3.
8. **Anchor integrity.** Canonical leaf serialization is deterministic; editing any anchored event in SQLite makes verification fail. Task S4.
9. **Frontend contract drift.** Every route used by the SPA exists in the contract and in the Gin router; no forbidden patterns (`@supabase`, client-side fee math, tokens in `localStorage`). Task S0 + S5.
10. **Accessibility.** The full searcher flow (describe → 3 questions → claim → pass code) is completable by voice only and by keyboard + screen reader only. Task S3 + S5.

---

## Execution order

```text
S1 (egress + migration 003) → S0 (contract, CORS, public endpoint)
   ├── Lovable track (human + agent, parallel): S5.0 inventory of the Vue app → S5.1–S5.3 (Lovable with mock data)
   └── Backend track: S2 (Exa + KB) → S3 (voice + short code) → S4 (Avalanche)
→ S5.4 (SPA fallback, preview React against the real API) → S5.5 (parity + cutover) → S6 (health, compose, e2e, demo) → MVP 15 → MVP 16
```

MVP Tasks 1–14 are the existing code. The Vue app (Tasks 13–14) is the functional reference for S5 and keeps serving `/dashboard/` until S5.5. Every "Depends on" below refers to code that already exists.

---

### Task S0: API contract, cross-site dev auth, public intermediaries endpoint

**Depends on:** MVP Task 3. **Unblocks:** Lovable track.

**Files:**
- Create: `docs/api-contract.md`
- Create: `internal/handler/route/public.go`, `internal/handler/dto/public.go`
- Modify: `internal/handler/api.go` (register `/api/public/*`), CORS setup, cookie writer in `internal/handler/route/auth.go`
- Test: `internal/handler/contract_test.go`, `internal/handler/route/public_test.go`

**Interfaces:**
- `GET /api/public/intermediaries` (no auth) → `[{id, name, address, zone, lat, lng, hours, acceptsCategories, accessibility: {stepFree: bool, notes: string}}]`. Intermediary business coordinates are public data; **item** coordinates never are. Add `accessibility` JSON column to `intermediaries` in migration `003_sponsors.sql` (Task S1 creates the file; S0 appends).
- `docs/api-contract.md`: for **every** route of MVP Tasks 3–12 plus this plan: method, path, role(s), request JSON, response JSON example, error codes. Use the DTO structs as the source of truth. Header section states: base URL `VITE_API_URL`, `credentials: "include"`, error shape `{"error": "<message>", "code": "<snake_case>"}` — the boilerplate's `{"error": "<message>"}` with an added `code` field (`error` stays a string). Make the shared error writer emit this shape.
- Cookie: `lf_session` stays `HttpOnly`. If `API_COOKIE_CROSS_SITE=true` → `SameSite=None; Secure`; else `SameSite=Lax`.
- CORS: `AllowCredentials=true`; origins = `http://localhost:5173` + `API_CORS_EXTRA_ORIGINS`. Never `*`.

- [ ] **Step 1: Write failing tests**

```go
// contract_test.go: every Gin route under /api is documented, and every documented route exists.
func TestAPIContractMatchesRouter(t *testing.T) {
    r := testutil.NewRouter(t)
    documented := contract.ParseRoutes(t, "../../docs/api-contract.md") // lines like "### GET /api/items/:id"
    for _, rt := range r.Routes() {
        if strings.HasPrefix(rt.Path, "/api/") && !documented[rt.Method+" "+rt.Path] {
            t.Errorf("undocumented route %s %s", rt.Method, rt.Path)
        }
    }
    for k := range documented {
        if !hasRoute(r, k) { t.Errorf("documented but missing: %s", k) }
    }
}

func TestPublicIntermediariesNeedsNoAuthAndHasNoItems(t *testing.T) {
    srv := testutil.NewServer(t); testutil.ResetDemo(t, srv)
    res := srv.GET("/api/public/intermediaries")
    if res.Code != 200 { t.Fatal(res.Code) }
    if strings.Contains(res.Body.String(), "item-") { t.Fatal("item data leaked") }
}
```

- [ ] **Step 2: Run, see fail** — `go test ./internal/handler/... -run 'Contract|Public' -v`
- [ ] **Step 3: Implement** route, DTO, CORS/cookie switches, and write `docs/api-contract.md`. Route headings must use the exact format `### METHOD /path` so the parser works. Routes added by later tasks of this plan (S2–S4) do not exist yet: mark them `### GET /api/... (planned)` and make the parser skip `(planned)` in the "documented but missing" direction; remove the marker when the route is implemented.
- [ ] **Step 4: Verify and commit** — `go test ./internal/handler/... && go build ./cmd/webapi/`

```bash
git add -A && git commit -m "feat: add api contract, public intermediaries and cross-site dev auth"
```

---

### Task S1: Egress guard and migration 003

**Depends on:** MVP Task 2. **Used by:** S2, S3, S4.

**Files:**
- Create: `internal/platform/egress/guard.go`, `internal/platform/egress/guard_test.go`
- Create: `db/migrations/003_sponsors.sql`
- Modify: `internal/seed/seed.go` (export `seed.PrivateValues()` returning all seeded private strings)

**Interfaces:**
- `type Guard interface { Check(dest string, payload []byte) error }` — returns `ErrEgressBlocked` (wrapping `errs.ErrForbidden`) if the payload contains any forbidden value.
- `egress.New(src ForbiddenSource)`; `ForbiddenSource.ForbiddenValues(ctx) []string` loads, from SQLite, all string leaves of every `items.private_profile` JSON with length ≥ 4, plus document fields. Cache 60s; invalidate on item create/update.
- Matching: case-insensitive, whitespace-normalized substring. Also block any `lat,lng` pair with ≥ 4 decimals.
- Every outbound sponsor client receives a `Guard` and calls `Check` on the serialized request body **before** sending. On block: do not send, log `egress_blocked` (no payload in the log), audit, return error → caller uses its fallback.
- Migration `003_sponsors.sql` (goose up/down), created now and appended by later tasks:

```sql
-- +goose Up
CREATE TABLE external_sources (
  id TEXT PRIMARY KEY, intent TEXT NOT NULL, query_hash TEXT NOT NULL,
  title TEXT NOT NULL, url TEXT NOT NULL, snippet TEXT NOT NULL,
  retrieved_at TEXT NOT NULL, expires_at TEXT NOT NULL
);
CREATE INDEX idx_external_sources_q ON external_sources(intent, query_hash);
ALTER TABLE users ADD COLUMN a11y_prefs TEXT NOT NULL DEFAULT '{}';
ALTER TABLE intermediaries ADD COLUMN accessibility TEXT NOT NULL DEFAULT '{}';
ALTER TABLE pickup_passes ADD COLUMN short_code_hash TEXT;
CREATE TABLE short_code_attempts (custodian_id TEXT NOT NULL, at TEXT NOT NULL, ok INTEGER NOT NULL);
CREATE TABLE anchors (
  batch_id INTEGER PRIMARY KEY, root TEXT NOT NULL, leaf_count INTEGER NOT NULL,
  status TEXT NOT NULL,            -- pending | submitted | confirmed | failed
  tx_hash TEXT, block_number INTEGER, created_at TEXT NOT NULL, confirmed_at TEXT, last_error TEXT
);
ALTER TABLE custody_events ADD COLUMN leaf_hash TEXT;
ALTER TABLE custody_events ADD COLUMN anchor_salt TEXT;
ALTER TABLE custody_events ADD COLUMN anchor_batch_id INTEGER;
ALTER TABLE custody_events ADD COLUMN leaf_index INTEGER;
-- +goose Down
-- drop in reverse order (SQLite ≥3.35 supports DROP COLUMN)
```

- [ ] **Step 1: Failing test**

```go
func TestGuardBlocksSeededPrivateValues(t *testing.T) {
    db := testutil.NewTestDB(t); seed.Demo(t, db)
    g := egress.New(db)
    for _, v := range seed.PrivateValues() {
        if err := g.Check("exa", []byte(`{"query":"iphone `+strings.ToUpper(v)+`"}`)); !errors.Is(err, errs.ErrForbidden) {
            t.Fatalf("value %q not blocked", v)
        }
    }
    if err := g.Check("exa", []byte(`{"query":"lost and found ATAC metro B"}`)); err != nil { t.Fatal(err) }
    if err := g.Check("exa", []byte(`{"q":"41.90281,12.49640"}`)); err == nil { t.Fatal("coords not blocked") }
}
```

- [ ] **Step 2: Run, see fail** — `go test ./internal/platform/egress -v`
- [ ] **Step 3: Implement** guard + migration + `seed.PrivateValues()`.
- [ ] **Step 4: Verify and commit** — `go test ./internal/platform/egress/... ./internal/repository/...`

```bash
git add -A && git commit -m "feat: add egress privacy guard and sponsor migration"
```

---

### Task S2: Local knowledge base + Exa external knowledge tool

**Depends on:** MVP Task 8, S1.

**Purpose:** the agent answers procedural questions from a **verified local KB** first; it uses **Exa** only for information outside it (operators' lost & found, other municipalities, embassies/consulates, product variants, new tariff resolutions), always citing the source and labelling it as external.

**Files:**
- Create: `internal/domain/knowledge/{knowledge.go,kb.yaml,service.go}` (`kb.yaml` embedded with `//go:embed`)
- Create: `internal/platform/exa/client.go`, `internal/platform/exa/client_test.go`
- Create: `internal/domain/knowledge/external.go` (intent → domains, cache, fallback)
- Modify: `internal/domain/agent/tools.go`, agent system prompt, `internal/domain/agent/mock.go`
- Test: `internal/domain/knowledge/service_test.go`

**Local KB (`kb.yaml`)** — derive **only** from `assets/come_recuperare_un_oggetto_smarrito_a_roma.md`, sources F1–F11, items marked ✔. Do **not** include anything marked ❓. Each entry:

```yaml
- id: tariffe
  topics: [costi, tariffe, prezzo, pagare]
  answer_it: "Ritiro presso la PA: diritto fisso € 4,60 + istruttoria € 5,80; oltre 30 giorni dall'acquisizione si aggiunge il deposito € 17,40 (totale € 27,80). Tariffe 2024, Delib. G.C. 421/2023. I documenti d'identità si restituiscono gratis. La restituzione diretta presso un Punto Ritrovato entro 7 giorni è gratuita."
  sources: [F1, F11]
```

Minimum entries: `sede_orari_contatti` (F2), `documenti_ritiro` (F1), `delega` (F1, F6), `corriere` (F1, F5), `rinuncia` (F4), `tariffe` (F1, F11), `pagamento_pagopa` (F3), `stranieri_consolati` (F1, ambiguity of exemption stated), `premio_art930` (F10, € 5,16 conversion note), `termine_un_anno_art929` (F10), `competenza_luogo_ritrovamento` (F10, "podestà" = sindaco), `punti_ritrovato` (design: 7 days / 24h, free direct return).

**Interfaces:**
- `knowledge.Lookup(ctx, text, lang) (KBAnswer, bool)` — keyword/topic match (no network); `KBAnswer{ID, Answer, Sources []string}`.
- `exa.Client.Search(ctx, SearchRequest) ([]Result, error)`; `SearchRequest{Query string; IncludeDomains []string; NumResults int; MaxChars int}`; `Result{Title, URL, Snippet string}`. Wire format (verify with Context7): `POST {API_EXA_URL}/search`, header `x-api-key`, body `{query, numResults, includeDomains, contents: {text: {maxCharacters}}}`. Timeout 5s → `ErrUnavailable`. Calls `Guard.Check("exa", body)` first.
- `knowledge.External(ctx, Intent, Params) ([]ExternalSource, error)` with:

| Intent | Params | Include domains | Query template |
|---|---|---|---|
| `transport_lost_found` | `operator` or `place` | `atac.roma.it, trenitalia.com, italotreno.com, adr.it, cotral.it, romamobilita.it` | `"oggetti smarriti {operator}"` |
| `other_municipality` | `municipality` | `comune.{slug}.it`, `*.gov.it` | `"ufficio oggetti rinvenuti comune di {municipality}"` |
| `embassy` | `country` | `esteri.it`, `.gov` embassy sites | `"ambasciata {country} Roma contatti"` |
| `product_variants` | `brand, model` | manufacturer domains map (`apple.com`, `samsung.com`, …) | `"{brand} {model} colori finiture"` |
| `tariff_update` | — | `comune.roma.it` | `"deliberazione tariffe oggetti rinvenuti Roma Capitale"` |

  Params are **closed-vocabulary or public** strings (operator, municipality, country, brand, model). Never free text from the private profile. Results cached in `external_sources` (`query_hash = sha256(intent|normalized params)`, TTL `API_EXA_CACHE_TTL`). Fallback when Exa is disabled/fails: static list in `kb.yaml` under `external_fallback` (official URLs for ATAC, Trenitalia, Italo, ADR, Cotral, esteri.it).
- **Agent tools** (add to MVP Task 8 tool list):
  - `lookup_knowledge({question})` → `KBAnswer` or `{found:false}`.
  - `search_external_knowledge({intent, operator?, place?, municipality?, country?, brand?, model?})` → `[{title, url, snippet, external:true}]`.
- **System prompt additions:** order of use is registry → `lookup_knowledge` → `search_external_knowledge`. External results must be presented as "Fonte esterna:" + URL, never as registry data. Never pass item details obtained from `get_private_profile` into `search_external_knowledge`. `product_variants` is used only to *formulate* verification questions, never shown to the searcher.
- `product_variants` is also callable from MVP Task 4 `Catalog` to suggest attributes to the finder/intake (optional, P1).
- `MockAgent`: if the user message mentions metro/bus/treno/aeroporto/taxi and the registry has no candidate, call `External(transport_lost_found)` and return the cited link.

- [ ] **Step 1: Failing tests**

```go
func TestKBAnswersTariffsWithoutNetwork(t *testing.T) {
    svc := knowledge.New(knowledge.Deps{})
    a, ok := svc.Lookup(context.Background(), "quanto costa ritirare dopo 40 giorni?", "it")
    if !ok || !strings.Contains(a.Answer, "27,80") || !slices.Contains(a.Sources, "F11") { t.Fatalf("%+v", a) }
}

func TestExternalFallsBackToStaticAndCaches(t *testing.T) {
    db := testutil.NewTestDB(t)
    calls := 0
    exaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        calls++; json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"title": "ATAC oggetti smarriti", "url": "https://www.atac.roma.it/x", "text": "..."}}})
    }))
    defer exaSrv.Close()
    svc := knowledge.New(knowledge.Deps{DB: db, Exa: exa.New(exaSrv.URL, "k", egress.New(db))})
    for i := 0; i < 2; i++ {
        got, err := svc.External(context.Background(), knowledge.TransportLostFound, knowledge.Params{Operator: "ATAC"})
        if err != nil || len(got) == 0 || !strings.HasPrefix(got[0].URL, "https://www.atac.roma.it") { t.Fatal(err, got) }
    }
    if calls != 1 { t.Fatalf("cache miss: %d calls", calls) }
    off := knowledge.New(knowledge.Deps{DB: db}) // no Exa
    if got, _ := off.External(context.Background(), knowledge.TransportLostFound, knowledge.Params{Operator: "Trenitalia"}); len(got) == 0 { t.Fatal("no static fallback") }
}

func TestAgentNeverSendsPrivateValuesToExa(t *testing.T) {
    // seed + spy Exa server recording bodies; run MockAgent and DeepSeek-stub flows for item-phone;
    // assert no recorded body contains any seed.PrivateValues().
}
```

- [ ] **Step 2: Run, see fail** — `go test ./internal/domain/knowledge/... ./internal/platform/exa/... ./internal/domain/agent/... -v`
- [ ] **Step 3: Implement** KB, Exa client, external service, tools, prompt, mock behaviour. Integration test against real Exa skips unless `API_EXA_KEY` is set.
- [ ] **Step 4: Verify and commit**

```bash
git add -A && git commit -m "feat: add verified local kb and exa external knowledge tool"
```

---

### Task S3: Accessibility voice mode (ElevenLabs), preferences, spoken pickup code

**Depends on:** MVP Tasks 8 and 9, S1.

**Purpose:** make the service usable by blind/low-vision, motor-impaired, dyslexic/cognitively impaired, deaf/hard-of-hearing and elderly users. Voice is an **additional channel**; keyboard + screen reader support remains mandatory (S5, MVP 16).

**Files:**
- Create: `internal/platform/elevenlabs/client.go`, `internal/platform/elevenlabs/client_test.go`
- Create: `internal/domain/voice/{voice.go,service.go}`
- Create: `internal/handler/route/voice.go`, `internal/handler/route/preferences.go`, DTOs
- Modify: `internal/domain/agent` (plain-language mode, voice flag), `internal/domain/pass` (short code), `internal/handler/route/pass.go`
- Test: `internal/domain/voice/service_test.go`, `internal/domain/pass/shortcode_test.go`, `internal/domain/agent/plain_language_test.go`

**Interfaces:**
- `elevenlabs.Client.Transcribe(ctx, audio io.Reader, mime, lang string) (string, error)` — verify with Context7; expected `POST {URL}/v1/speech-to-text`, header `xi-api-key`, multipart `file` + `model_id` (`API_ELEVENLABS_STT_MODEL`) + optional `language_code`. Timeout 15s.
- `elevenlabs.Client.Speak(ctx, text, lang string, speed float64) (io.ReadCloser, string /*mime*/, error)` — expected `POST {URL}/v1/text-to-speech/{voice_id}` body `{text, model_id, voice_settings: {speed}}`, returns `audio/mpeg`. Timeout 10s. `Guard.Check("elevenlabs", body)` before sending.
- Routes (all authenticated, same rate limit bucket as chat: 20/min/session):
  - `POST /api/voice/transcribe` — multipart `audio` (≤ 10 MB, ≤ 60 s, `audio/webm|ogg|mpeg|wav|mp4`), `lang` → `{text}`. Audio stays in memory; never persisted or logged.
  - `POST /api/voice/speak` — `{source: "chat"|"ui", messageId?, key?, lang}` → `audio/mpeg` stream. **No arbitrary text:** `source=chat` speaks the stored assistant message `messageId` of the caller's own session (already privacy-filtered); `source=ui` speaks a whitelisted UI string `key` from `internal/domain/voice/strings_it.yaml` / `strings_en.yaml` (e.g. `pass.instructions`, `finder.rights_assignment`, `error.generic`). This prevents credit abuse and leaks.
  - `GET /api/me/preferences`, `PUT /api/me/preferences` — `a11y_prefs = {voiceMode: bool, speechRate: 0.7..1.2, plainLanguage: bool, highContrast: bool, captions: bool, lang: "it"|"en"}`.
  - `GET /api/voice/status` → `{tts: "elevenlabs"|"browser"|"none", stt: ...}` so the SPA picks a fallback.
- `POST /api/chat` **AMENDS** MVP Task 8: request accepts `voice: bool`; reply adds `messageId` (assistant messages are now stored server-side per chat session for `speak`) and `speakable: bool`. If the user has `plainLanguage`, the agent uses a plain-language prompt variant: sentences ≤ 15 words, one instruction per message, no legal jargon (article numbers moved to a "Dettagli" line).
- **Spoken pickup code — AMENDS MVP Task 9:**
  - `Issue` also generates `short_code`: 6 chars from the alphabet `ACDEFHJKMNPRTUVWXY3479` (no ambiguous glyphs), formatted `XXX-XXX`; store only `short_code_hash = HMAC(PassSecret, custodianID|code)`. Return the code once in the pass DTO.
  - `ValidateShortCode(ctx, code, custodianID, now)` — same checks as `Validate`; looks up by hash with the caller's custodian ID, so a code is valid only at its custodian. Every attempt writes `short_code_attempts`; after 5 failures in 15 min for that custodian → `ErrRateLimited` for 15 min. Audit every attempt.
  - `POST /api/passes/validate` and `/redeem` accept `{token}` **or** `{shortCode}`.
  - The pass page reads the code aloud via `speak(source=ui, key=pass.code)` with the code spelled using the Italian phonetic alphabet (A come Ancona…), generated server-side from the pass the caller owns.
- **Live captions at the counter (P1):** intermediary/institution handover screen can record 5-second chunks → `/api/voice/transcribe` → append to an on-screen caption panel. Nothing stored.
- **Fallbacks:** no ElevenLabs key or error → `voice/status` reports `browser`; the SPA uses Web Speech API (`speechSynthesis`, `SpeechRecognition` where available); otherwise text only. The chat must remain fully usable with `aria-live` and a screen reader.

- [ ] **Step 1: Failing tests**

```go
func TestSpeakRejectsArbitraryTextAndForeignMessages(t *testing.T) {
    // speak(source="chat", messageId=<other user's message>) → ErrForbidden
    // speak(source="ui", key="not.whitelisted") → ErrInvalidInput
}

func TestTranscribeNeverPersistsAudio(t *testing.T) {
    // stub ElevenLabs; POST audio; assert data dir unchanged and no DB row contains audio bytes.
}

func TestShortCodeBoundToCustodianAndRateLimited(t *testing.T) {
    db := testutil.NewTestDB(t); seed.Demo(t, db)
    svc := pass.New(db, pass.Deps{PassSecret: "s"})
    now := time.Now().UTC()
    p, _ := svc.Issue(context.Background(), "claim-phone", now)
    if _, err := svc.ValidateShortCode(context.Background(), p.ShortCode, "other-shop", now); err == nil { t.Fatal("wrong custodian accepted") }
    for i := 0; i < 5; i++ { svc.ValidateShortCode(context.Background(), "AAA-AAA", "bar-x", now) }
    if _, err := svc.ValidateShortCode(context.Background(), p.ShortCode, "bar-x", now); !errors.Is(err, errs.ErrRateLimited) { t.Fatal("no lockout") }
    if _, err := svc.ValidateShortCode(context.Background(), p.ShortCode, "bar-x", now.Add(16*time.Minute)); err != nil { t.Fatal(err) }
}

func TestPlainLanguageReplyIsShort(t *testing.T) {
    // MockAgent with plainLanguage=true: every sentence ≤ 15 words, no "art." in main text.
}
```

- [ ] **Step 2: Run, see fail** — `go test ./internal/domain/voice/... ./internal/domain/pass/... ./internal/domain/agent/... -v`
- [ ] **Step 3: Implement** client, voice service, whitelisted strings (IT + EN), preferences, chat amendments, short code. Add `ErrRateLimited` to `errs` if missing (HTTP 429).
- [ ] **Step 4: Verify and commit**

```bash
git add -A && git commit -m "feat: add accessible voice mode, preferences and spoken pickup code"
```

---

### Task S4: Avalanche custody anchoring and public verification

**Depends on:** MVP Tasks 5 and 12, S1.

**Files:**
- Create: `contracts/foundry.toml`, `contracts/src/CustodyAnchor.sol`, `contracts/test/CustodyAnchor.t.sol`, `contracts/script/Deploy.s.sol`
- Create: `internal/platform/avalanche/{anchor.go,custodyanchor_bind.go}` (`abigen` output)
- Create: `internal/domain/anchoring/{leaf.go,merkle.go,service.go}`
- Create: `internal/handler/route/proof.go`, DTOs
- Modify: `internal/domain/custody` tick (`/api/institution/tick` also calls `AnchorPending`), custody-event writer (compute leaf on insert), `Timeline` DTO
- Test: `internal/domain/anchoring/{merkle_test.go,service_test.go}`, `forge test`

**Contract** (Solidity ^0.8.24, no dependencies):

```solidity
contract CustodyAnchor {
    address public immutable operator;
    mapping(uint64 => bytes32) public roots;
    event Anchored(uint64 indexed batchId, bytes32 root, uint32 leafCount);
    constructor() { operator = msg.sender; }
    function anchor(uint64 batchId, bytes32 root, uint32 leafCount) external {
        require(msg.sender == operator, "not operator");
        require(roots[batchId] == bytes32(0), "batch exists");
        require(root != bytes32(0), "empty root");
        roots[batchId] = root;
        emit Anchored(batchId, root, leafCount);
    }
}
```

Forge tests: only operator, no overwrite, event emitted. Deploy to Fuji with `forge script script/Deploy.s.sol --rpc-url $API_AVAX_RPC_URL --private-key $API_AVAX_PRIVATE_KEY --broadcast`; write the address into `.env` (`API_AVAX_CONTRACT`) and `docs/avalanche.md`. Fund the wallet from the Fuji faucet.

**Leaf (deterministic, no PII on chain):**
- On every `custody_events` insert: `anchor_salt = 32 random bytes (hex)`; `leaf_hash = keccak256(0x00 || canonical)` where `canonical` is UTF-8 JSON with **fixed key order** `{"v":1,"id":..,"item_id":..,"type":..,"actor_id":..,"timestamp":<RFC3339Nano UTC>,"notes_sha256":..,"evidence_ref":..,"salt":..}`. Implement with a struct + `json.Marshal` (Go preserves struct field order); `notes_sha256` so free-text notes are not part of the preimage in clear.
- The salt makes leaves unlinkable to guessable event content; the leaf, not its preimage, is what leaves the process.

**Merkle tree:** internal node `keccak256(0x01 || min(a,b) || max(a,b))` (sorted pairs, domain-separated from leaves); odd node promoted unchanged. `Root(leaves) bytes32`, `Proof(leaves, i) [][32]byte`, `Verify(leaf, proof, root) bool`.

**Service:**
- `AnchorPending(ctx, now)`: select events with `anchor_batch_id IS NULL` ordered by `(timestamp, id)`, up to `API_AVAX_BATCH_MAX`; create `anchors` row (`pending`), assign `anchor_batch_id`/`leaf_index` in one transaction; then send `anchor(batchId, root, count)` via go-ethereum (`bind.NewKeyedTransactorWithChainID`, chain id 43113), set `submitted` + `tx_hash`; `confirmed` + `block_number` once the receipt has status 1 (poll on next tick; no blocking > 10 s). On any RPC error: keep `pending`/`submitted`, set `last_error`, retry next tick. If Avalanche is disabled: batches are still built (`status=pending`, local root) so verification of local integrity still works.
- `GuardCheck`: `Guard.Check("avalanche", calldata)` is trivially satisfied (hashes only) but call it anyway for uniformity.
- `VerifyEvent(ctx, eventID) (Verification, error)` → recompute canonical leaf from the DB row, check proof against the stored batch root, and (if enabled) compare with `roots(batchId)` read on chain. `Verification{EventID, LeafHash, BatchID, Root, Proof []string, TxHash, ExplorerURL, LocalOK, OnChainOK *bool, Status}`.

**Routes:**
- `GET /api/items/:id/proof` (institution, or the item's current custodian) → `[]Verification` for the item's events.
- `GET /api/public/proofs/:eventId` (no auth) → `{leafHash, batchId, root, proof, txHash, explorerUrl, onChainOK}` only — no item, actor or type fields.
- Timeline DTO (MVP Task 12) adds per event `anchor: {status, txHash, explorerUrl, verified}`.

- [ ] **Step 1: Failing tests**

```go
func TestMerkleProofsVerify(t *testing.T) {
    leaves := anchoring.RandomLeaves(t, 7)
    root := anchoring.Root(leaves)
    for i := range leaves {
        if !anchoring.Verify(leaves[i], anchoring.Proof(leaves, i), root) { t.Fatalf("leaf %d", i) }
    }
}

func TestCanonicalLeafIsDeterministic(t *testing.T) {
    ev := record.CustodyEventRecord{ID: "e1", ItemID: "item-phone", Type: "accepted", ActorID: "bar-x",
        Timestamp: time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC), AnchorSalt: strings.Repeat("ab", 32)}
    a, b := anchoring.Leaf(ev), anchoring.Leaf(ev)
    if a != b { t.Fatal("non deterministic") }
}

func TestTamperingBreaksVerification(t *testing.T) {
    db := testutil.NewTestDB(t); seed.Demo(t, db)
    svc := anchoring.New(db, anchoring.Deps{}) // chain disabled: local root only
    if _, err := svc.AnchorPending(context.Background(), time.Now().UTC()); err != nil { t.Fatal(err) }
    evID := testutil.FirstEventID(t, db, "item-phone")
    if v, _ := svc.VerifyEvent(context.Background(), evID); !v.LocalOK { t.Fatal("should verify") }
    db.Exec("UPDATE custody_events SET actor_id = 'mallory' WHERE id = ?", evID)
    if v, _ := svc.VerifyEvent(context.Background(), evID); v.LocalOK { t.Fatal("tampering not detected") }
}

func TestPublicProofHasNoPII(t *testing.T) {
    // GET /api/public/proofs/:eventId body must not contain item id, actor id, event type or any seed.PrivateValues().
}
```

Integration test (skips unless `API_AVAX_PRIVATE_KEY` and `API_AVAX_CONTRACT` are set): anchor one batch on Fuji, wait for receipt, `roots(batchId) == root`.

- [ ] **Step 2: Run, see fail** — `go test ./internal/domain/anchoring/... -v` and `cd contracts && forge test`
- [ ] **Step 3: Implement** contract, bindings (`abigen --abi ... --pkg avalanche --type CustodyAnchor`), leaf hook in the event writer, Merkle, service, routes. `go mod tidy && go mod vendor`.
- [ ] **Step 4: Verify and commit**

```bash
git add -A && git commit -m "feat: anchor custody events on avalanche with public merkle proofs"
```

---

### Task S5: Lovable frontend rebuilt from scratch — SUPERSEDES MVP Tasks 13–14

**Depends on:** S0 (start), then all backend tasks for S5.4.

**Starting point:** the existing Vue 3 app in `webui/` (views, components, router, `api.ts`, copy). It is the **functional reference**: Lovable starts from an empty React project and rebuilds the same screens and flows, improved where this plan asks (voice, accessibility, short code, anchor badge, public map). No Vue code is converted. Go-side reuse:
- `webui/static-resources.go` — embeds `dist/*` as `webui.Dist` under the `webui` build tag;
- `cmd/webapi/register-web-ui.go` — mounts `webui.Dist/dist` at `/dashboard/` (`http.StripPrefix` + `http.FileServer`, **no SPA fallback** today);
- `cmd/webapi/register-web-ui-stub.go` — no-op when building without the tag.

**S5.0 — Reference inventory (agent, before any Lovable prompt).** From the Vue app, write `docs/lovable/reference-inventory.md`:
- **Routes:** every `router.ts` route → view component → role guard.
- **Per view:** purpose, sections/fields (label, type, required, validation), actions and the exact API call each makes (method + path + body), states shown (loading/empty/error/success), Italian copy strings verbatim.
- **Shared components:** `PhotoUpload`, `GeoPicker`, `RightsAssignment`, `RoleNav`, `StatusBadge`, `Timeline`, `ChatMessage`, `CandidateCard`, `QRCode`, `FeeSummary`, `RewardNotice`, `VerificationReport` → props, behaviour, where used.
- **Gaps vs this plan:** what the Vue app lacks (voice, a11y toggle, short code, anchor badge, external-source label, public map) → tagged `NEW`.
- **Screenshots:** one per view at 375 px and 1280 px in `docs/lovable/screens/<view>.png`, taken from the running Vue app with the demo seed. Attach them to the matching Lovable prompt.

This inventory, the contract and the screenshots are the whole input to Lovable.

**Division of labour:** a human operates Lovable (prompts in the Lovable UI, GitHub sync). The implementing agent (a) writes the Knowledge and prompt files, (b) reviews and fixes the generated code in `lostfound-webui` (i.e. `webui/`), (c) does the Go integration. If Lovable is unavailable, the agent implements the same prompts by hand with the same stack; the acceptance criteria do not change.

**Repository layout:** Lovable syncs to the dedicated GitHub repo `lostfound-webui`; its root mirrors `webui/`. Lovable never pushes to the main repo. The repo also contains `static-resources.go` (copied from the boilerplate on day 1, untouched): Knowledge forbids Lovable to edit or delete it, and `check-webui.sh` fails if it is missing or changed. In the main repo `webui/` is updated only with `git subtree pull --prefix webui lostfound-webui main --squash` (run by the agent). `webui/dist/` is gitignored. Until cutover (S5.5) `webui/` still holds the Vue app; the React app is previewed with `scripts/preview-react.sh` (see S5.4). The Dockerfile already builds `webui/` (`COPY webui/package*.json ./` + `COPY webui/ ./`), so it does not change; a normal `git clone` is enough.

**Files (main repo):**
- Create: `docs/lovable/reference-inventory.md`, `docs/lovable/screens/*.png`, `docs/lovable/knowledge.md`, `docs/lovable/prompts.md`, `docs/lovable/review-checklist.md`, `docs/lovable/parity.md`
- Create: `scripts/check-webui.sh`, `scripts/static-resources.sha256`, `scripts/build-webui.sh`, `scripts/preview-react.sh`
- Create: `cmd/webapi/register_web_ui_test.go` (`//go:build webui`) + `cmd/webapi/testdata/dist/{index.html,assets/x.js}`
- Delete (S5.5): Vue sources in `webui/` except `static-resources.go`
- Modify: `.gitignore` (`webui/dist/`, `.lovable-preview/`), `cmd/webapi/register-web-ui.go` (SPA fallback, cache headers, `/` redirect), `AGENTS.md` (npm + Lovable note), MVP Task 15 E2E smoke
- Test: `scripts/check-webui.sh`, `scripts/build-webui.sh`, `go test -tags webui ./cmd/webapi/`, `go build -tags webui ./cmd/webapi/`, manual smoke + a11y checks

**S5.1 — `docs/lovable/knowledge.md`** (paste into the Lovable project Knowledge). Must contain, verbatim in intent:
1. Product summary (5 lines) and the four roles. State explicitly: "This is a from-scratch React rebuild of an existing Vue app. Reproduce the screens, flows and copy described in the reference inventory and screenshots; do not imitate Vue patterns; add the items tagged NEW." Paste `docs/lovable/reference-inventory.md` as an appendix.
2. Hard rules: you build **only the frontend**; never create backend code, server functions, databases or Go files, and never edit or delete `static-resources.go` in the repo root. Errors from the API have the shape `{"error": "<message>", "code": "<snake_case>"}`; show `error` to the user, branch on `code`. Use npm and keep `package-lock.json` committed (no `bun.lockb`). No Supabase / Lovable Cloud / auth providers; no mock data in committed code except behind `import.meta.env.VITE_USE_MOCKS === "true"`; all calls through `src/lib/api.ts` using `fetch(\`${import.meta.env.VITE_API_URL ?? ""}/api/...\`, { credentials: "include" })`; never store tokens (the session is an HttpOnly cookie); never compute fees, deadlines, statuses or confidence client-side; display server values only.
3. Routing: React Router with `basename="/dashboard"`; Vite `base: "/dashboard/"`. Route list (same as MVP Tasks 13–14): `/login`, `/finder`, `/finder/receipt/:id`, `/intermediary`, `/intermediary/enroll`, `/intermediary/scan`, `/searcher`, `/searcher/pass/:id`, `/institution`, `/institution/intake`, `/institution/items/:id`, plus public `/` (landing) and `/punti` (map).
4. Privacy UI rules: searcher sees only category, zone, date; never item photos or details; external links from the agent shown with a "Fonte esterna" label.
5. Accessibility (WCAG 2.1 AA, Legge Stanca): semantic HTML, every input labelled, visible focus, contrast ≥ 4.5:1, no colour-only status, touch targets ≥ 44 px, `aria-live="polite"` on the chat log, skip-link, all actions keyboard-reachable, respects `prefers-reduced-motion`; global "Modalità accessibile" toggle bound to `/api/me/preferences` (voice mode, speech rate, plain language, high contrast, captions).
6. The full API contract: paste `docs/api-contract.md`.
7. Design: Italian UI copy, sober civic look, mobile-first for finder/searcher (375 px), desktop tables for intermediary/institution.

**S5.2 — `docs/lovable/prompts.md`**: one prompt per block, each with acceptance criteria the human checks before moving on:

| # | Block | Must include |
|---|---|---|
| 1 | Shell | `api.ts`, auth guard on `/api/auth/me`, role nav, demo login (pick user), error/empty/loading states, accessibility toggle, health/degraded badge from `/api/health` |
| 2 | Finder | 1–3 photos with client resize to 1200 px, geolocation + manual fallback, category/colour/condition, intermediary picker (from `/api/public/intermediaries`), rights-assignment checkbox with legal copy (design §6.1), receipt page; voice dictation button for the description |
| 3 | Intermediary | custody list with days left + sensitive badge, accept/refuse with reason, enroll (reuses finder form), scan page: `html5-qrcode` **and** short-code input `XXX-XXX`, validate → redeem; captions panel (P1) |
| 4 | Searcher | chat with `aria-live`, candidate cards (category/zone/date), claim CTA, push-to-talk **and** tap-to-toggle mic, "ascolta" button per assistant message (`/api/voice/speak`), Web Speech fallback per `/api/voice/status`; pass page with QR (`qrcode`), large short code, "leggi il codice" button, fee summary, reward notice, required documents, mock pay button |
| 5 | Institution | pickups grouped by zone (sensitive first) + transfer, intake form, item detail with timeline (anchor badge + explorer link + "verifica integrità"), verification report, approve/reject/more-info, handover, unclaimed actions |
| 6 | Public | landing (problem: 510-page PDF → solution), `/punti` map (Leaflet + OSM tiles) with step-free badge from `accessibility`, CTA "Ho perso qualcosa" → `/dashboard/searcher`, "Ho trovato qualcosa" → `/dashboard/finder` |

During blocks 1–5 the human may run Lovable with `VITE_USE_MOCKS=true` and fixtures copied from the contract examples.

**S5.3 — Review gate.** `scripts/check-webui.sh` takes a directory (default `webui/`; `.lovable-preview/` before cutover) and fails (exit 1) if `static-resources.go` is missing or its SHA-256 differs from `scripts/static-resources.sha256` (recorded from the boilerplate file); if any other `.go` file, a `supabase/` folder or a top-level entry outside the allow-list exists (allow-list: `src/ public/ index.html package.json package-lock.json vite.config.ts tsconfig*.json tailwind.config.* postcss.config.* components.json eslint.config.* README.md .gitignore static-resources.go`); if `package-lock.json` is missing; or if the tree (excluding `node_modules/` and `dist/`) contains any of: `@supabase`, `supabase`, `localStorage.setItem(` with `token|session`, `firebase`, hard-coded tariff numbers (`4.60|4,60|5.80|5,80|17.40|17,40`), `dangerouslySetInnerHTML`, `fetch(` outside `src/lib/api.ts`, or a `vite.config.ts` without `base: "/dashboard/"`. `docs/lovable/review-checklist.md` lists the manual checks (Review Focus 9–10). The agent fixes violations directly in the Lovable repo (changes sync back to Lovable).

**S5.4 — Integration.**
- [ ] Add `webui/dist/` and `.lovable-preview/` to `.gitignore`.
- [ ] Write `scripts/build-webui.sh` (see Global Constraints) with `set -euo pipefail`. Build with `VITE_API_URL=""` (same origin).
- [ ] Write `scripts/preview-react.sh` (pre-cutover only): clone or pull `lostfound-webui` into `.lovable-preview/`, run `check-webui.sh .lovable-preview`, `npm --prefix .lovable-preview ci && npm --prefix .lovable-preview run build`, then replace `webui/dist` with `.lovable-preview/dist` and `go build -tags webui ./cmd/webapi/`. The Vue sources in `webui/` are not touched; running `scripts/build-webui.sh` again restores the Vue build.
- [ ] **SPA fallback** in `cmd/webapi/register-web-ui.go`. Keep the existing `/dashboard/` prefix check and `fs.Sub(webui.Dist, "dist")`, then:
  - strip `/dashboard/`; if the path is empty or `fs.Stat` says it does not exist → serve `index.html` with `Cache-Control: no-cache`;
  - if it exists under `assets/` (Vite hashed files) → serve it with `Cache-Control: public, max-age=31536000, immutable`;
  - `GET /` exactly → `302` to `/dashboard/`; every other path (including `/api/*`, `/healthz`) → `hdl` unchanged.
  - Use `r.URL.Path`, not `r.RequestURI` (the boilerplate uses `RequestURI`, which includes the query string).
- [ ] Write the failing test first, `cmd/webapi/register_web_ui_test.go` (`//go:build webui`). Make the FS injectable (`newWebUIHandler(dist fs.FS, hdl http.Handler)`, with `registerWebUI` calling it with `webui.Dist`) so the test uses `testdata/dist`:

```go
func TestWebUISPAFallback(t *testing.T) {
    dist, _ := fs.Sub(os.DirFS("testdata"), "dist")
    api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(299) })
    h := newWebUIHandler(dist, api)
    cases := []struct{ path string; code int; body string }{
        {"/dashboard/searcher", 200, "<!doctype html>"},
        {"/dashboard/searcher/pass/abc?x=1", 200, "<!doctype html>"},
        {"/dashboard/assets/x.js", 200, "console"},
        {"/api/health", 299, ""},
        {"/", 302, ""},
    }
    for _, c := range cases {
        rec := httptest.NewRecorder()
        h.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
        if rec.Code != c.code || !strings.Contains(strings.ToLower(rec.Body.String()), c.body) {
            t.Errorf("%s → %d %q", c.path, rec.Code, rec.Body.String())
        }
    }
}
```

- [ ] Run `scripts/preview-react.sh && go test -tags webui ./cmd/webapi/`.
- [ ] Manual smoke per role on `http://localhost:3000/dashboard/`, then at 375 px; keyboard-only pass of the searcher flow; screen-reader pass (Orca on Linux, or NVDA/VoiceOver) of the searcher flow.
- [ ] Lighthouse accessibility ≥ 95 on `/dashboard/searcher` and `/dashboard/finder`.

```bash
git add -A && git commit -m "feat: add spa fallback and lovable preview tooling"
```

**S5.5 — Parity check and cutover.**
- [ ] Generate `docs/lovable/parity.md` from the inventory: one row per Vue route/action → React equivalent → ✔/✘. Every non-`NEW` row must be ✔; every `NEW` row must be ✔ or explicitly deferred per the Cut Line.
- [ ] Run the MVP Task 15 E2E smoke and the manual role smoke on the React build; compare with the Vue screenshots.
- [ ] Cutover: `git remote add lostfound-webui <repo url>` (once) and `git fetch lostfound-webui`; `git rm -r webui && git commit -m "chore: remove vue frontend"`, then `git subtree add --prefix webui lostfound-webui main --squash` (brings back `static-resources.go` from the Lovable repo together with the React app). Verify `git diff <before>:webui/static-resources.go HEAD:webui/static-resources.go` is empty. Update `AGENTS.md` (`webui/` = Lovable React app via subtree, `scripts/build-webui.sh`, npm). Then `scripts/check-webui.sh && scripts/build-webui.sh && go build -tags webui ./cmd/webapi/`.
- [ ] Later Lovable changes: `git subtree pull --prefix webui lostfound-webui main --squash`, then the same checks.
- [ ] Rollback = `git revert -m 1 <subtree merge commit>` then `git revert <"remove vue frontend" commit>` → the Vue app comes back intact.

```bash
git add -A && git commit -m "docs: agents.md points to the lovable react frontend"
```

---

### Task S6: Health, compose, E2E and demo — AMENDS MVP Tasks 1, 15, 16

**Files:**
- Modify: `/api/health` handler, `.env.example`, `docker-compose.yml`, `tests/e2e/demo_test.go`, `docs/demo-script.md`, `docs/demo-checklist.md`, `scripts/demo-reset.sh`

**Changes:**
- `/api/health` returns `{ok, db, qdrant, embeddings, deepseek, d3, exa, elevenlabs, avalanche: {enabled, rpc, lastAnchor: {batchId, status}}}`. Probes: Exa and ElevenLabs = "key present" + cached last-call success (no paid call on health); Avalanche = `eth_chainId` equals 43113 (2 s timeout). Never fails the request.
- `docker-compose.yml`: pass the new env vars to `api`; no new services.
- `demo/reset` also: clears `external_sources`, `short_code_attempts`, resets `anchors` and anchor columns, then runs `AnchorPending` once so the seed timeline shows anchored events (chain-enabled → real Fuji tx; disabled → `pending` local root).
- **E2E (`TestDemoFlow`) additions**, all with sponsors stubbed by `httptest` servers:
  1. Searcher asks "ho perso lo zaino in metro B" with no registry match → reply contains an `atac.roma.it` URL labelled external.
  2. Searcher asks the cost → reply from KB contains "4,60" and no Exa call is made.
  3. Phone flow completes using **short code** instead of QR token.
  4. `/api/voice/speak` for the assistant message returns `audio/mpeg` (stub); for another user's message returns 403.
  5. After redeem, `/api/items/item-phone/proof` returns `LocalOK=true` for every event; after manual DB tamper, `false`.
  6. Spy servers recorded no `seed.PrivateValues()` in any outbound body.
- **Demo script (3 min) — sponsor beats:**
  - 0:20 Finder enrolls the phone by **voice** (accessibility toggle on).
  - 0:50 Searcher with screen reader + voice: describe → 3 questions → claim → **code read aloud**.
  - 1:30 Bar X redeems by **short code**; timeline shows "✓ ancorato su Avalanche" → open Snowtrace.
  - 1:50 "Ho perso lo zaino in metro" → **Exa** returns the official ATAC page as "Fonte esterna".
  - 2:20 Institution wallet flow with PA tariff from the **KB**; "verifica integrità" green.
  - 2:45 Closing line: "UI generated with **Lovable** from the API contract."
- `docs/demo-checklist.md`: `webui/` up to date (`git subtree pull` done), `scripts/build-webui.sh` run before the Go build, Fuji wallet funded (≥ 1 AVAX test), contract address set, ElevenLabs voice id set, Exa key set, each sponsor toggled off once to confirm fallback.

- [ ] **Step 1:** extend E2E test (fails). - [ ] **Step 2:** implement changes. - [ ] **Step 3:** `go test ./... && go vet ./... && (cd contracts && forge test) && scripts/check-webui.sh && scripts/build-webui.sh && go test -tags webui ./cmd/webapi/ && go build -tags webui ./cmd/webapi/`. - [ ] **Step 4:** rehearse twice with all sponsors on, once with all sponsor keys empty.

```bash
git add -A && git commit -m "chore: sponsor health, e2e coverage and demo script"
```

---

## Cut Line — AMENDS MVP "Cut Line"

1. **P0:** S1 (egress), S0 (contract), S5.0 + S5 blocks 1–5 + S5.4 + S5.5 (Lovable UI), S2 (KB + Exa `transport_lost_found` + fallback), S3 core (transcribe/speak/preferences/short code, searcher voice), S6 health + E2E items 1–3, 6.
2. **P1:** S4 (Avalanche anchoring + proofs + timeline badge), S3 finder dictation + counter captions, S2 other intents (`other_municipality`, `embassy`, `product_variants`), S5 block 6 (landing + map).
3. **P2:** S2 `tariff_update` monitor, ElevenLabs Agents full-duplex (custom LLM → our `/api/chat`), Avalanche L1 multi-city (slide only).

**Frontend fallback:** if S5.5 parity is not reached by code freeze, do not cut over — demo on the Vue app (it already works) and show the Lovable build via `scripts/preview-react.sh` or the Lovable preview; adjust the 2:45 demo line accordingly.

**Never cut (adds to MVP list):** egress guard, KB-before-Exa order, no arbitrary-text TTS, short-code custodian binding + lockout, no PII on chain, `check-webui.sh` gate, keyboard + screen-reader completion of the searcher flow.

## Open items for the human (not for the agent)

- Create accounts/keys: Lovable (GitHub-connected), Exa, ElevenLabs (choose an Italian voice → `API_ELEVENLABS_VOICE_ID`), Fuji wallet + faucet.
- Create the `lostfound-webui` GitHub repo with only `static-resources.go` (copied from the boilerplate `webui/`) at the root, and connect it to a **new** Lovable project.
- Paste `docs/lovable/knowledge.md` into the project Knowledge, then run the 6 prompts of `docs/lovable/prompts.md` in order, checking each block's acceptance criteria before the next.
- Confirm with Team1/Avalanche judges whether Fuji testnet is acceptable for the demo (default: yes).
