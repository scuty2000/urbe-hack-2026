# Demo rehearsal checklist

Final pre-recording checklist for the Oggetti Smarriti MVP. Use it once before
every rehearsal and recording. The narrative is in `docs/demo-script.md`; this
file is the operational sequence.

> **Honesty rule.** This repository contains no recorded video and no public
> deployment URL. Recording the backup video and publishing a tunnel are human
> steps on the presenter's machine and account. Do not fill in a video/URL that
> has not been verified from outside the local network.

## 0. Prerequisites (once)

- [ ] `.env` exists (`cp .env.example .env`) and is **not** committed:

  ```bash
  git check-ignore -v .env
  ```

- [ ] `API_AUTH_SESSION_SECRET` and `API_AUTH_PASS_SECRET` are set.
- [ ] `API_DEEPSEEK_TIMEOUT` is below `API_WEB_WRITETIMEOUT` (30–45 s is a
      good demo value). A provider that hangs must not outlive the HTTP write
      timeout: the agent falls back to the mock only when its request returns
      or its client timeout fires.
- [ ] Optional providers are configured or deliberately left empty. The demo
      works either way:
  - DeepSeek key empty → mock agent.
  - D3 empty/off → local scorer.
  - Qdrant/embeddings empty/off → SQLite search.
  - Payment provider `mock` → no real payment.
- [ ] Docker daemon is running (for the container path) and port 3000 is free.
- [ ] `.env`, `data/`, `db/*.db*`, `webui/dist/` and `webui/node_modules/` are
      ignored by git.

## 1. Reset the dataset

Run the reset immediately before rehearsal. It never prints the demo secret.

```bash
scripts/demo-reset.sh
# no API running yet:
scripts/demo-reset.sh --seed
```

Verify the seed before opening the UI. `/api/health` only reports dependencies;
it does not prove the dataset was reset. Log in as the demo searcher and read
the seeded claim instead:

```bash
curl -sS -c /tmp/ritrova-cookies -H 'Content-Type: application/json' \
  -d '{"userId":"searcher-demo"}' http://localhost:3000/api/auth/login >/dev/null
curl -sS -b /tmp/ritrova-cookies http://localhost:3000/api/claims/claim-phone | jq
# expected: the seeded approved claim for item-phone
```

- [ ] Reset exits 0 and prints either the HTTP reset or the local seed path.
- [ ] The admin login still works (`admin-demo`).
- [ ] The login screen lists finder, intermediary, searcher and institution.

If reset fails with 403, `.env` and the running API use different
`API_DEMO_RESET_SECRET` values. Do not print the secret while debugging.

## 2. Start the stack

Container path (embedded UI at `/dashboard/`):

```bash
docker compose up --build -d api qdrant
open http://localhost:3000/dashboard/
```

Local path (backend + Vite; the Go binary has no embedded UI unless it was built
with `-tags webui`):

```bash
scripts/dev-up.sh
npm --prefix webui run dev   # open http://localhost:5173/
```

- [ ] `/healthz` returns 200.
- [ ] `/api/health` returns JSON, and the **fallback** badge on the searcher
      chat screen says **modalità fallback** when optional providers are not
      configured.
- [ ] Opening the four role homes works (embedded `/dashboard/...` or the Vite
      `:5173` equivalents).

## 3. Fallback rehearsal (5 minutes)

Do this once with providers stopped/disabled, so a stage outage is boring.
Provider settings (`API_DEEPSEEK_*`, `API_D3_*`, `API_QDRANT_*`,
`API_EMBEDDINGS_*`) are read only at startup: after changing any of them, stop
and restart the API before testing.

- [ ] Stop Qdrant (`docker compose stop qdrant`) or unset `API_QDRANT_URL`.
      Search still returns candidates; nothing 500s.
- [ ] Unset `API_DEEPSEEK_KEY` (or use an invalid URL). Chat answers with the
      deterministic mock agent; the badge says **modalità fallback**.
- [ ] Unset `API_D3_URL` (or stop D3). Claims are scored locally; review still
      works.
- [ ] Keep the mock payment provider; fees settle without a payment key.
- [ ] Restore the normal configuration before recording.

The chat badge on the searcher screen is honest about configuration: green
means D3 and DeepSeek are configured; amber means at least one of them is not,
and the local path is in use. Provider errors always fall back to SQLite, the
local scorer and the mock agent rather than returning 500.

## 4. Four-flow rehearsal

Rehearse with four separate browser profiles/windows already on the login
screen. Reset first, then follow the arc in `docs/demo-script.md`.

| # | Role | Flow | Expected visible result |
|---|---|---|---|
| 1 | Finder (`finder-demo`) | `/dashboard/finder` → enroll umbrella → choose Bar X → accept rights → submit | Receipt with public code, pending custody, finder cannot reopen the item |
| 2 | Intermediary (`bar-x`) | Dashboard → **Accetta custodia** | Item in custody, 7-day deadline (24 h for sensitive items) |
| 3 | Searcher (`searcher-demo`) | `/dashboard/searcher` → two chat turns → claim card | Public candidates only, no private profile/documents/exact coordinates, claim in review |
| 4 | Institution (`institution-demo`) | Open the claim via the item URL → approve → confirm | Review recorded; searcher can reload the claim and generate the pass |
| 5 | Intermediary (`bar-x`) | Searcher generates pass → **Scansiona** → validate → redeem | QR shown once; redemption moves the item to **Restituito**; a second redemption is rejected |

Also rehearse the legal tail: PA pickup queue, transfer, fee summary, offer,
collect and dispose (see the seeded wallet/umbrella).

Privacy spot check after the chat:

```bash
# with a live API: nothing private in the reply
curl -sS -b cookies.txt -H 'Content-Type: application/json' \
  -d '{"messages":[{"role":"user","content":"Ho perso un telefono nero in zona San Lorenzo"}]}' \
  http://localhost:3000/api/chat | grep -E 'privateProfile|case_color|CA00000AA|Mario Rossi' || echo clean
```

- [ ] No 500 and no spinner left running in any interface.
- [ ] Empty states are visible on a freshly reset dataset (no pending items,
      no pickups, no notifications).
- [ ] Error states appear when the API is stopped (friendly message, retry).
- [ ] Chat rate limit: the 21st turn in a minute returns a readable
      "troppi messaggi" message, not a generic failure.
- [ ] Pass/scan double click does not redeem twice (buttons disable while the
      request is in flight; the token is single-use server-side).

## 5. Mobile / accessibility pass (375 px)

Use browser devtools at 375 px width on the finder and searcher screens:

- [ ] Finder form is one column, fields keep visible labels and errors.
- [ ] Chat input stays reachable above the on-screen keyboard; transcript scrolls.
- [ ] Candidate cards and claim panel stack without horizontal scrolling.
- [ ] Pass QR is fully visible.
- [ ] Every icon-only button has an accessible name; Tab reaches all controls.
- [ ] Focus ring is visible (emerald outline); text contrast is readable in the
      header, banners, hints and footer.

## 6. Backup before recording

- [ ] Run `go test ./... && go test ./tests/e2e/...` once after the last change.
- [ ] Record the backup video now (3:00 max, 16:9, no secrets or personal data
      on screen). **Human step:** the file must exist and play start to finish;
      the repository ships none.
- [ ] Keep a local copy outside the repo (e.g. Desktop) and upload it as an
      unlisted video.
- [ ] Add the verified link to `docs/submission.md` only after playback works.

If the live demo fails:

1. Switch to the recorded video.
2. Or use the Docker stack (already verified):
   `docker compose up --build -d api qdrant`, then open
   `http://localhost:3000/dashboard/`.
3. Or rebuild the embedded UI and run it locally:

   ```bash
   npm --prefix webui run build
   go run -tags webui ./cmd/webapi/ -c demo/config.yml
   open http://localhost:3000/dashboard/
   ```

   Without `-tags webui` the binary serves the API only (no `/dashboard/`): in
   that case use the Vite path instead (`scripts/dev-up.sh` +
   `npm --prefix webui run dev`, open `http://localhost:5173/`).
4. Or use the seeded fast path with either UI:
   `http://localhost:3000/dashboard/searcher?claim=claim-phone` (embedded) or
   `http://localhost:5173/searcher?claim=claim-phone` (Vite).

## 7. Public URL (human/account step)

Only after the local demo is stable:

```bash
# .env must contain CLOUDFLARE_TUNNEL_TOKEN and DEMO_PUBLIC_URL
scripts/tunnel.sh
```

- [ ] Cloudflare named tunnel routes to `http://api:3000` (configured in the
      Cloudflare dashboard, not in this repository).
- [ ] The printed URL was opened from a different network and `/healthz`
      returned 200.
- [ ] `scripts/demo-reset.sh` succeeds against the public API.
- [ ] The URL is written to `docs/submission.md` only after that check.
- [ ] No token, tunnel command line or `.env` is visible in the video.

If no tunnel is available, submit the local URL and say so explicitly. Never
invent a public URL or a video link.

## 8. Final sign-off

- [ ] Dataset reset.
- [ ] `go test ./...` and `go test ./tests/e2e/...` pass.
- [ ] `npm --prefix webui run build` and `go build -tags webui ./cmd/webapi/` pass.
- [ ] `docker compose config` is valid.
- [ ] Four roles rehearsed twice end to end.
- [ ] Fallback badge understood and explained in one sentence.
- [ ] Backup video recorded and linked.
- [ ] Public URL (or an explicit local-only note) recorded.
