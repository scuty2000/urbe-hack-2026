# Demo script — 3 minutes

This is the recorded/live script for the Oggetti Smarriti MVP. It assumes the
demo dataset was reset and all four demo roles are available from the login
screen.

## Before you record

1. Reset the dataset:

   ```bash
   scripts/demo-reset.sh
   # or, with no API running:
   scripts/demo-reset.sh --seed
   ```

   The script never prints `API_DEMO_RESET_SECRET`.

2. Start the application:

   ```bash
   # first run only
   cp .env.example .env

   # Docker: the embedded UI is served at /dashboard/
   docker compose up -d qdrant
   docker compose up --build -d api

   # or local backend, then the Vite UI in a second terminal
   scripts/dev-up.sh
   npm --prefix webui run dev
   ```

3. Open the demo UI at `http://localhost:3000/dashboard/` (embedded build) or
   `http://localhost:5173/dashboard/` (Vite dev server).

4. Prepare four browser windows/profiles, one per role, already on the login
   screen:

   | Role | Demo user button |
   |---|---|
   | Finder | Giulia Ferrari (`finder-demo`) |
   | Intermediary | Bar X (`bar-x`) |
   | Searcher | Marco Bianchi (`searcher-demo`) |
   | Institution | Comune di Roma (`institution-demo`) |

5. Set browser zoom around 110%, disable notifications, close personal tabs and
   keep the secret-bearing terminal off screen.

## The 3-minute arc

### 0:00 — The problem (18 s)

Show the welcome/login screen. Say: "Oggetti smarriti oggi finiscono in tre
posti diversi: il bar che li trova, il comando di polizia locale e chi li ha
persi. Nessuno sa dove sia l'oggetto, chi lo custodisce o quando scade il
termine. Questo registro unico lo risolve."

Point at the four role cards: finder, intermediary, searcher, institution.

### 0:20 — Finder enrolls an item (30 s)

1. Sign in as **Giulia Ferrari** (finder).
2. Open **Segnala un oggetto** / `/finder`.
3. Fill in just the fields needed for the story:
   - category: **Ombrello**
   - colour: **nero**
   - zone: **San Lorenzo**
   - private detail: **custodia blu** (this is the detail only the owner should
     know; it is never shown to searchers)
   - one photo
   - collection point: **Bar X**
   - accept the rights assignment checkbox
4. Submit. Show the receipt with the public code and the handover instructions.

Say: "Il finder non consegna i suoi dati a nessuno: cede la custodia legale al
punto di raccolta e riceve una ricevuta."

### 0:50 — Intermediary accepts custody (20 s)

1. Switch to **Bar X**.
2. On the dashboard, find the new item under **Da accettare**.
3. Click **Accetta custodia**.

Point at the 7-day deadline. Say: "Da qui l'oggetto è tracciato. La custodia dura
sette giorni, o ventiquattro ore per i documenti sensibili; alla scadenza entra
nella coda della pubblica amministrazione."

### 1:10 — Searcher chat and human review (55 s)

The chat uses the already-seeded iPhone in Bar X custody (`item-phone`) so the
search and verification steps are deterministic and independent from the
umbrella just created. Say: "Ora cambio ruolo: Marco cerca il suo telefono, già
consegnato al Bar X."

1. Switch to **Marco Bianchi** (searcher) and open **Ritrova il tuo oggetto**.
2. First message: *"Ho perso un telefono nero in zona San Lorenzo"*.
   The assistant asks one open question and shows the compatible candidate.
3. Second message: *"È un iPhone 17 nero con custodia blu, una crepa e un
   adesivo a forma di gatto"*.

Show that the reply does not echo the private details, that only public fields
appear in the candidate card, and that the claim status is **Servono altre
informazioni** with a **Media** confidence band. Say: "L'assistente non vede il
profilo privato e non approva da solo: servono altre informazioni e decide una
persona."

4. Switch to **Comune di Roma** and open the direct item URL
   `/dashboard/institution/items/item-phone?claim=<claim code>` (the claim code
   is printed on the searcher's claim card). The pickup dashboard does not list
   an in-custody item, so use this URL rather than looking for it in the queue.
   The claim panel is already open; click **Approva richiesta**, then
   **Conferma**.

Say: "La revisione umana vede il report, le risposte e i flag; il searcher non
vede mai il profilo privato."

If the fallback badge is visible, say: "Qui gira il percorso locale, senza
dipendere da un provider esterno: la demo funziona anche offline."

### 2:10 — Pass and QR (25 s)

1. Return to the searcher window. The claim id is kept in the URL after the
   chat creates it, so **reloading the page (F5)** re-fetches
   `/api/claims/:id` and the approved state (and the pass button) appears. If the
   URL does not contain the claim id, open
   `/dashboard/searcher?claim=<claim code>` instead.
2. Click **Genera pass di ritiro**. Show the QR and the one-time token.
3. Switch to **Bar X**, open **Scansiona pass** (`/intermediary/scan`).
4. Scan the QR, or paste the token in the manual field.
5. Click **Verifica**, then **Conferma ritiro**.

Show the item status **Restituito** and the timeline. Say: "Il pass è firmato,
monouso e legato a quell'oggetto e a quel custode. Un secondo tentativo viene
rifiutato."

### 2:35 — Institution and the legal tail (15 s)

1. Switch to **Comune di Roma**.
2. Open the wallet (`item-wallet`) or the pickup queue.
3. Show:
   - the PA pickup/transfer state,
   - the fee summary (15,00 € for the demo tariff),
   - the verification report and the claim review controls,
   - the legal-term actions: **offerta al finder**, **ritiro**, **smaltimento**.

Say: "La PA vede solo ciò che le serve: coda di ritiro, verifiche, tariffa e
termine legale. Dopo un anno l'oggetto viene offerto a chi lo ha trovato e solo
dopo può essere smaltito."

### 2:50 — Sensitive items and close (10 s)

Open the seeded **documento** card or the official intake page. Say: "I
documenti sono sensibili: custodia di 24 ore, intake ufficiale e nessun dettaglio
identity nella ricerca. Chiudi con: un registro unico, quattro ruoli, un pass
monouso, zero dati personali esposti."

## If something goes wrong

- **API or tunnel down:** use the recorded video. For a live recovery, start the
  Docker stack (`docker compose up --build -d api qdrant`, open
  `http://localhost:3000/dashboard/`) or rebuild the embedded UI and run it
  locally:

  ```bash
  npm --prefix webui run build
  go run -tags webui ./cmd/webapi/ -c demo/config.yml
  ```

  `go run ./cmd/webapi/` without `-tags webui` serves the API only and does not
  mount `/dashboard/`; in that case use the Vite path (`scripts/dev-up.sh` plus
  `npm --prefix webui run dev`, open `http://localhost:5173/dashboard/`).
- **Chat, claim or review stalls:** open
  `/dashboard/searcher?claim=<claim code>` with the claim code from the chat; the
  seeded `claim-phone` belongs to the fixture account `searcher-alt`, not to the
  demo searcher.
- **Dataset dirty:** `scripts/demo-reset.sh --seed` (or `scripts/demo-reset.sh`
  against the running API). Reset before every rehearsal and recording.
- **Institution queue empty:** the seeded wallet is already in PA custody; show
  its item page instead of the transfer action. The umbrella path can be
  rehearsed separately with a second item.
- **Camera unavailable:** the scan page always accepts the token manually; use
  that in the recording.
- **Qdrant/DeepSeek/D3 unavailable:** the UI shows **modalità fallback** and the
  demo still works through SQLite, the local scorer and the mock agent. Do not
  spend time debugging providers on stage.

## Recording checklist

- [ ] 3:00 maximum, 16:9, readable at 1080p.
- [ ] No `.env`, API keys, tunnel tokens, terminal history or personal data on
      screen.
- [ ] Show all four roles and the full return, not only the happy-path chat.
- [ ] Record one clean take, then verify the file plays start to finish.
- [ ] Upload it (unlisted YouTube/Drive is fine) and add the link to
      `docs/submission.md`.
