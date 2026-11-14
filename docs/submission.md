# Submission checklist

Use this checklist for the hackathon form. Do not invent a public URL, a video
link or a recording: the repository has no committed deployment URL and no
recorded video, because both are human steps that must be performed on the
presenter's machine/account.

## What is ready in the repository

- [x] Full backend: SQLite schema/migrations, demo users, reset endpoint.
- [x] Vue UI embedded in the Go binary with `-tags webui`.
- [x] Dockerfile (Node UI build → Go CGO build → slim runtime).
- [x] `docker-compose.yml` with `api`, `qdrant` and the optional `tunnel`
      profile.
- [x] `scripts/demo-reset.sh` (safe `.env` loader, admin session + secret,
      local seed fallback).
- [x] `scripts/tunnel.sh` (requires `CLOUDFLARE_TUNNEL_TOKEN`; never prints it).
- [x] `docs/demo-script.md` with the 3-minute arc and backup plan.
- [x] `tests/e2e/demo_test.go`: full API flow against real SQLite.

## What a human must do before submitting

### 1. Public repository

- [ ] Push the branch to GitHub/GitLab and confirm the repository is **public**.
- [ ] Confirm the history contains no `.env`, database, upload or secret file:

  ```bash
  git ls-files | grep -E '(^|/)\.env($|\.)|\.db($|-)|^data/'
  ```

  Only `.env.example` may match. If anything else matches, stop and remove it
  from the history before publishing.
- [ ] Put the repository URL here: `<public repository URL>`
- [ ] Verify a fresh clone builds:

  ```bash
  go build ./cmd/webapi/
  go test ./...
  npm --prefix webui ci && npm --prefix webui run build
  go build -tags webui ./cmd/webapi/
  ```

### 2. Online demo URL

The demo uses a Cloudflare **named** tunnel. The public hostname must route to
`http://api:3000`.

1. In Cloudflare Zero Trust → Networks → Tunnels, create the tunnel and a public
   hostname.
2. Put the tunnel token in `.env` as `CLOUDFLARE_TUNNEL_TOKEN=...` (never commit
   it).
3. Start the stack and the tunnel:

   ```bash
   docker compose up --build -d api qdrant
   scripts/tunnel.sh
   ```

4. Set `DEMO_PUBLIC_URL=https://...` in `.env` so the script prints it, or read
   it from the dashboard.
5. Verify from a different network:

   ```bash
   curl -fsS https://<host>/healthz
   curl -fsS https://<host>/api/health | jq
   # Open https://<host>/dashboard/
   ```

- [ ] Public URL works from outside the local network.
- [ ] `/healthz` returns 200 and `/api/health` shows the expected fallback mode.
- [ ] The four demo logins work and `scripts/demo-reset.sh` succeeds against the
      public API.
- [ ] Put the URL here: `<demo URL>`

If you cannot expose a tunnel, submit the local URL
(`http://localhost:3000/dashboard/`) and say so explicitly in the form; do not
claim a public deployment that does not exist.

### 3. Three-minute video

- [ ] Follow `docs/demo-script.md` and rehearse at least once after a reset.
- [ ] Record a single take of at most 3:00 showing: problem, finder, intermediary,
      searcher chat, pass/QR, institution, sensitive/legal close.
- [ ] Re-watch the file; check that no `.env`, token, API key or personal data is
      visible.
- [ ] Upload it (unlisted YouTube/Drive is fine) and set sharing so reviewers can
      open it.
- [ ] Put the link here: `<video URL>`

### 4. Form

- [ ] Project name and one-line pitch: "Un registro unico per oggetti smarriti,
      con custodia tracciata, verifica privacy-safe e pass di ritiro monouso."
- [ ] Public repository URL.
- [ ] Demo URL (or an explicit note that only the local demo is available).
- [ ] 3-minute video URL.
- [ ] Team members and contacts.
- [ ] Technology list: Go, Gin, GORM, SQLite, Vue 3, TypeScript, Tailwind,
      Docker, optional Qdrant/DeepSeek/D3 with deterministic fallbacks.
- [ ] Confirm that the demo does not require a payment provider key and uses the
      mock payment path.

## Demo cheat sheet

```bash
# reset the dataset (the secret is loaded from .env and never printed)
scripts/demo-reset.sh

# full local stack
scripts/dev-up.sh

# container stack
docker compose up --build -d api qdrant

# public URL
scripts/tunnel.sh

# end-to-end verification
go test ./tests/e2e/... -v
```

Demo users are passwordless buttons on the login screen: `finder-demo`,
`bar-x`, `portineria-sapienza`, `searcher-demo`, `institution-demo`,
`admin-demo`.

## Known limitations (state them honestly)

- The demo payment provider is a deterministic mock; no money moves.
- DeepSeek, D3 and embeddings are optional. Without keys the UI shows
  "modalità fallback" and search/verification use SQLite and the local scorer.
- The tunnel is not committed and is not started automatically; it requires the
  operator's Cloudflare token.
- The repository is a single-process SQLite MVP; SQLite is not a multi-node
  production database.
