# 🧩 Report integrazioni sponsor — Lovable · Exa · ElevenLabs · Team1/Avalanche

> Contesto: AgenticHack Rome, sfida 03. Backend deciso: Go/Gin + SQLite + Qdrant + DeepSeek + D3.
> Ogni sponsor ha **un ruolo preciso** nel sistema:
> 💜 Lovable **costruisce** il frontend · 🔎 Exa **porta conoscenza da fuori** · 🎙️ ElevenLabs **rende accessibile** · 🔺 Avalanche **rende verificabile**.

---

## ⚡ TL;DR (30 secondi)

| # | Sponsor | Ruolo | Dove entra | Sforzo | Priorità |
|---|---|---|---|---|---|
| 1 | 💜 **Lovable** | 🏗️ Sviluppo frontend automatico | **Tutte e 4 le interfacce**: rifà da zero in React l'app Vue esistente (Task 13–14) | 🟡 ~4–6h (al posto di ~8h a mano) | **P0** |
| 2 | 🔎 **Exa** | 🌐 Info fuori dalla knowledge base | Tool dell'agente quando il registro e la KB non bastano | 🟢 ~2–3h | **P0** |
| 3 | 🎙️ **ElevenLabs** | ♿ Accessibilità | Modalità voce completa per persone con disabilità | 🟡 ~3–4h | **P0** |
| 4 | 🔺 **Avalanche (Team1)** | 🔐 Audit verificabile | Ancoraggio on-chain della catena di custodia | 🟡 ~3–4h | **P1** (forte) |

**Regola d'oro per tutti e 4:** 🔒 *nessun `private_profile`, foto, documento o dato personale esce verso uno sponsor o finisce on-chain.*

---

## 💜 1. Lovable — il frontend lo genera Lovable

### Cos'è (1 riga)
Builder AI di frontend: genera app **React + Vite + TypeScript + Tailwind + shadcn/ui** da prompt, con sync bidirezionale su GitHub.

### 🔄 Cosa cambia nel piano
- 📌 L'app **Vue 3** esistente (Task 13–14) resta il **riferimento**: schermate, flussi e testi. Il codice Vue non si converte.
- ✅ Lovable la rifà **da zero in React**, solo dentro `webui/` (repo `lostfound-webui`, importato con `git subtree`).
- 🔁 Finché React non fa tutto quello che faceva Vue, la demo resta su Vue.
- ✅ Il resto **non cambia**: `vite build` → `webui/dist` → embed nel binario Go con `-tags webui`, servito su `/dashboard/`.

### 🧠 Principio: "contratto prima, UI dopo"
Lovable scrive **solo UI**. Tutta la logica (stati, custodia, verifica, privacy, fee) resta nel backend Go.
1. Il backend espone le rotte del piano (`/api/auth/*`, `/api/items`, `/api/chat`, `/api/passes/*`, `/api/institution/*`…).
2. Si genera un **contratto API** (`docs/api-contract.md` o OpenAPI) con DTO e esempi JSON.
3. Il contratto va nella **Knowledge** del progetto Lovable → Lovable genera il client `src/api.ts` e le schermate.

### 📋 Knowledge del progetto Lovable (regole fisse)
- 🚫 **Niente Supabase**, niente auth Lovable, niente DB lato client.
- Tutte le chiamate: `fetch(\`${import.meta.env.VITE_API_URL}/api/...\`, { credentials: "include" })`.
- `base: "/dashboard/"` in `vite.config.ts`; router con basename `/dashboard`.
- Ruoli: finder, intermediary, searcher, institution → guard su `/api/auth/me`.
- Il searcher **non vede mai** dettagli privati: mostra solo categoria/zona/data.
- Accessibilità obbligatoria (vedi §3): label, focus visibile, contrasto AA, `aria-live` per la chat.

### 🧱 Prompt per blocchi (uno per sessione Lovable)
1. Shell + login demo + navigazione per ruolo.
2. **Finder:** foto (1–3, resize 1200px), geolocalizzazione, form categoria, scelta punto, checkbox cessione diritti, ricevuta.
3. **Intermediario:** dashboard custodia con giorni residui + badge sensibile, accetta/rifiuta, enrollment, scanner QR (`html5-qrcode`).
4. **Searcher:** chat, card candidati, pass QR, fee/premio, **pulsante voce** (§3).
5. **Istituzione:** lista ritiri per zona, intake, review con report D3, timeline con badge Avalanche (§4).
6. **Pubblico:** landing + mappa "Punti Ritrovato" (`GET /api/public/intermediaries`).

### ⚠️ Rischi e come evitarli
| Rischio | Contromisura |
|---|---|
| Lovable tocca il backend o rompe `main` | Lovable lavora solo sul repo `lostfound-webui` (= `webui/`); il repo principale lo importa con `git subtree pull`; `check-webui.sh` blocca file non frontend |
| Preview Lovable su un altro dominio → cookie di sessione bloccati | In dev: CORS con l'origin della preview + cookie `SameSite=None; Secure`; in demo: tutto servito da Gin (stesso dominio) |
| Logica di business che scivola nel client | Regola in Knowledge + review: il client non calcola fee, stati o soglie |
| Mock data finiti in produzione | Knowledge: "nessun dato fittizio, solo API" |

### 🎬 Momento demo / pitch
> "Il frontend delle 4 interfacce è stato generato con Lovable a partire dal contratto API: il team ha passato il tempo sul dominio, la fiducia e la verifica."

---

## 🔎 2. Exa — conoscenza oltre la knowledge base

### Cos'è (1 riga)
API di ricerca web neurale: query in linguaggio naturale → pagine + contenuto + citazioni, con filtri per dominio e data.

### 🧠 Principio: "prima la KB, poi il web"
```text
domanda del searcher
  → 1. registro privato (Qdrant / SQLite)
  → 2. KB locale (catalogo fonti verificate: c.c. 927–931, Delib. 421/2023, moduli, orari, PagoPA)
  → 3. Exa  ← solo se 1 e 2 non rispondono, con fonte citata
```
La KB locale nasce da `come_recuperare_un_oggetto_smarrito_a_roma.md` (fonti F1–F11 già verificate).

### 🎯 Cosa porta Exa che la KB non ha
1. 🚇 **Lost & found dei gestori** (ATAC, Trenitalia, Italo, ADR, Cotral, taxi): oggi marcato ❓ "senza fonte" nel catalogo → Exa lo trova e lo cita.
2. 🗺️ **Oggetto trovato fuori Roma:** è competente il comune del **ritrovamento** (art. 927) → Exa trova l'ufficio oggetti rinvenuti di quel comune.
3. 🌍 **Cittadini stranieri:** contatti dell'Ambasciata/Consolato del paese (F1 dice che ricevono l'elenco).
4. 📱 **Specifiche prodotto:** D3 dice "iPhone, nero" → Exa trova varianti e segni distintivi → domande di verifica più discriminanti.
5. 📜 **Aggiornamenti normativi:** nuove delibere tariffe su `comune.roma.it` (oggi vale ancora la 421/2023 = tariffe 2024) → avviso all'admin.

### 🛠️ Come
- `internal/platform/exa/client.go` → `Search(ctx, query, includeDomains) ([]Result, error)`; timeout 5s; `ErrUnavailable`.
- Tool dell'agente (Task 8): `search_external_knowledge(topic, place, category)` → `{title, url, snippet, retrieved_at}`.
- Domini whitelistati per tipo di domanda (gestori trasporto, siti dei comuni, `esteri.it`/ambasciate, siti dei produttori).
- System prompt: "le info da Exa sono **esterne**: citale sempre, non sono il registro".
- Cache in SQLite (`external_sources`, TTL 24h) → costi bassi e demo stabile.
- Config: `API_EXA_KEY`, `API_EXA_DOMAINS`.
- Fallback: lista statica di link ufficiali nel seed.

### ⚠️ Rischi / NO
- ❌ Niente ricerca sui social ("ho perso portafoglio a San Lorenzo"): privacy + false claims.
- Query solo **generiche** (categoria + luogo pubblico), mai dettagli privati dell'oggetto o della persona.

### 🎬 Momento demo
> "Non è nel registro… ma l'hai perso sul treno: ecco la pagina ufficiale lost & found di Trenitalia [fonte]."

---

## 🎙️ 3. ElevenLabs — accessibilità per persone con disabilità

### Cos'è (1 riga)
TTS (voci naturali, italiano incluso), STT (Scribe) e Agents conversazionali.

### 🎯 Perché è un tema di accessibilità, non un gadget
- Il servizio è di una **PA**: deve rispettare la **Legge Stanca** e le linee guida AgID (WCAG 2.1 AA, verso la 2.2). L'**European Accessibility Act** è in vigore dal 28/06/2025.
- Oggi il percorso ufficiale è un **PDF di 510 pagine non cercabile**: per una persona cieca è di fatto inaccessibile.

### ♿ Chi aiuta e come
| Persona | Barriera di oggi | Cosa fa la voce |
|---|---|---|
| 👁️ Cieca / ipovedente | PDF non leggibile, form, QR | Conversazione completa a voce; istruzioni del pass lette ad alta voce; **codice di ritiro letto** come alternativa al QR |
| ✋ Disabilità motoria | Digitare, compilare form | Uso **a mani libere**: finder e searcher dettano |
| 🧠 Dislessia / disabilità cognitive | Testi burocratici lunghi | Risposte lette in **linguaggio semplice**, velocità regolabile |
| 👂 Sorda / ipoacusica | Telefonare al Reparto | Chat testuale + **sottotitoli live** (Scribe) al banco del custode |
| 👵 Anziani / poco digitali | App complesse | Si parla come al telefono |
| 🌍 Turisti stranieri | Lingua | Stessa voce in altre lingue (beneficio secondario) |

### 🛠️ Come
- **Architettura:** STT + TTS **attorno al nostro agente** → DeepSeek resta l'orchestrator; le regole di privacy restano nel nostro codice.
  - `internal/platform/elevenlabs/client.go` → `Transcribe(audio, lang)`, `Speak(text, lang, rate)`.
  - Rotte: `POST /api/voice/transcribe`, `POST /api/voice/speak` (stessa auth e rate limit della chat).
- **UI (generata da Lovable):**
  - toggle **"Modalità accessibile"** sempre visibile, salvato tra le preferenze;
  - pulsante voce grande (tieni premuto **e** tocca per iniziare/finire: niente gesti difficili);
  - velocità di lettura e "linguaggio semplice" regolabili;
  - `aria-live` sulla chat → compatibile anche con gli screen reader esistenti (NVDA, VoiceOver).
- **In ogni interfaccia:**
  - **Finder:** dettatura della descrizione → campi del form; conferma letta ad alta voce.
  - **Searcher:** domande di verifica lette, risposte parlate; pass e documenti richiesti letti.
  - **Intermediario / Istituzione:** sottotitoli live per searcher sordi al banco.
- Il TTS legge **solo** `reply.Message`, già filtrato → nessun nuovo rischio di leak.
- Config: `API_ELEVENLABS_KEY`, `API_ELEVENLABS_VOICE_ID`, `API_ELEVENLABS_MODEL` (modello a bassa latenza, per stare sotto i 5s del NFR).
- Fallback: Web Speech API del browser → altrimenti solo testo + screen reader.
- Stretch: ElevenLabs Agents con "custom LLM" verso il nostro backend (conversazione full-duplex).

### ⚠️ Rischi
- L'audio è dato personale → **non si salva**, si tiene solo il transcript; niente lettura di documenti d'identità.
- La voce non sostituisce l'accessibilità di base: tastiera, contrasto, label restano obbligatori (Task 16).

### 🎬 Momento demo
> Una persona non vedente ritrova il telefono **solo con la voce**: descrive, risponde a 3 domande, sente il codice di ritiro. Dal PDF di 510 pagine a una conversazione.

---

## 🔺 4. Team1 / Avalanche — la catena di custodia diventa verificabile

### Cos'è (1 riga)
Team1 è la community builder di Avalanche (grant, hackathon). Integrarlo vuol dire costruire su **Avalanche C-Chain** (EVM), testnet **Fuji**.

### 🎯 Dove entra
**Notarizzazione dell'audit.** La tesi del progetto è "catena di fiducia + audit". Oggi l'audit sta in SQLite: chi ha accesso al DB può riscriverlo. Con Avalanche:
- ogni N minuti (o a fine giornata) si calcola la **Merkle root** dei nuovi `custody_events`;
- la root va on-chain con un contratto minimo `CustodyAnchor.anchor(bytes32 root, uint64 batchId)`;
- ogni evento conserva la propria Merkle proof → la PA (o un giudice) può verificare che la storia non sia stata toccata.

### 🛠️ Come
- Contratto Solidity di ~20 righe, deploy su Fuji (chainId 43113) con Foundry o Hardhat.
- Go: `internal/platform/avalanche/anchor.go` con `go-ethereum` (`ethclient` + binding `abigen`).
- Tabella `anchors` (`batch_id, root, tx_hash, anchored_at`) + colonna `anchor_batch_id` su `custody_events`.
- Job `AnchorPending(ctx)` chiamato da `/api/institution/tick` (già previsto nel Task 5).
- Endpoint `GET /api/items/:id/proof` → evento, proof, root, tx (verificabile da chiunque).
- UI: nella timeline, badge **"✓ ancorato su Avalanche"** con link all'explorer.
- Config: `API_AVAX_RPC_URL`, `API_AVAX_PRIVATE_KEY` (wallet testnet dedicato), `API_AVAX_CONTRACT`.
- Fallback: se la chain non risponde, i batch restano `pending` e si riprova al tick successivo.

### ⚠️ Rischi / NO
- ❌ **Zero PII on-chain**: solo hash, con salt per evento. GDPR: la blockchain non si cancella.
- ❌ **Niente crypto per le tariffe PA**: la PA incassa solo via PagoPA (F3).
- ❌ Premio art. 930 in stablecoin: il design lo tiene *fuori dal sistema* → al massimo una slide "futuro".
- 💬 Storia di produzione (solo pitch): un **Avalanche L1 permissioned** tra Comuni per un registro multi-città (Fase 3 della roadmap).

### 🎬 Momento demo
> "Questo telefono è passato da Bar X alla Polizia Locale. Ogni passaggio è ancorato: ecco la transazione."

---

## 🧭 Impatto sul piano MVP

| Task | Sponsor | Cosa cambia |
|---|---|---|
| Task 1 (config/health) | tutti | nuove chiavi in `.env.example`; `/api/health` con lo stato di ogni sponsor |
| Task 3 (auth) | Lovable | CORS per l'origin della preview Lovable; cookie `SameSite=None; Secure` in dev |
| Task 5 (custody/tick) | Avalanche | `AnchorPending` nel tick |
| Task 8 (agente) | Exa, ElevenLabs | tool `search_external_knowledge`; KB locale; rotte voice |
| Task 12 (audit/timeline) | Avalanche | `anchor_batch_id`, Merkle proof, `/api/items/:id/proof` |
| **Task 13–14 (frontend)** | **Lovable** | **rifatti da zero:** inventario dell'app Vue + contratto API → Knowledge Lovable → 6 blocchi di prompt → verifica parità → sostituzione in `webui/` → embed Go |
| Task 16 (hardening/a11y) | ElevenLabs, tutti | test di accessibilità con la modalità voce; ogni sponsor giù → degrada senza crash |
| Nuovo | Lovable | `docs/api-contract.md` + `GET /api/public/intermediaries` |

## 🗓️ Ordine di lavoro (in parallelo)
- 👤 **Persona A (backend):** Task 1–9 → contratto API il prima possibile (sblocca Lovable).
- 👤 **Persona B (frontend):** Lovable con le API mock dal contratto → poi le API vere.
- Poi: Exa → ElevenLabs → Avalanche.

## ✂️ Cut line sponsor
- **P0:** Lovable (4 interfacce) · Exa tool + KB · ElevenLabs modalità voce per il searcher.
- **P1:** Avalanche anchoring + proof · voce anche per il finder · sottotitoli live al banco.
- **P2:** ElevenLabs Agents full-duplex · monitor delibere con Exa · L1 multi-città (solo slide).

## ✅ Checklist veloce
- [ ] Account e chiavi: Lovable, Exa, ElevenLabs, wallet Fuji (faucet)
- [ ] `docs/api-contract.md` pronto **prima** del primo prompt Lovable
- [ ] Knowledge Lovable con le regole fisse (no Supabase, credentials include, base `/dashboard/`, a11y)
- [ ] Verificare le API aggiornate con Context7 quando si implementa (endpoint e modelli possono essere cambiati)
- [ ] Test "nessun dato privato" esteso ai payload verso Exa, ElevenLabs e la chain
- [ ] `/api/health` mostra 🟢/🔴 per ogni sponsor
- [ ] Una frase di pitch per sponsor nel video da 3 minuti

## 🔍 Verifica (quando si implementa)
- `go test ./...`: i test degli sponsor si saltano senza chiavi; test di privacy sui payload in uscita.
- `scripts/check-webui.sh && scripts/build-webui.sh && go build -tags webui ./cmd/webapi/` → `/dashboard/` serve il frontend Lovable.
- Demo manuale: flusso solo a voce con lo screen reader acceso → match → link Trenitalia via Exa → handover → badge "ancorato" con link all'explorer funzionante.
- Spegnere le chiavi una alla volta → l'app continua a funzionare (fallback).

> Dettagli di implementazione: `assets/2026-11-14-oggetti-smarriti-sponsor-followup.md`.
