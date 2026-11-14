# Registro unico degli oggetti smarriti — Design

**Data:** 13 novembre 2026
**Contesto:** AgenticHack Rome, sfida 03 “Oggetti smarriti”. Il sistema è pensato per funzionare oltre la demo: la demo è un traguardo, non il prodotto.
**Team:** unico codebase; il collega non ha un back-office separato, si merge qui.

---

## 1. Intento e perché

Oggi il Reparto Oggetti Rinvenuti pubblica gli oggetti ritrovati in un PDF per anno: l'elenco 2026 (1 gennaio - 11 novembre) è di 510 pagine non cercabili, con reperti n. 1–6940. Chi trova un oggetto deve consegnarlo alla Polizia Locale; quel passaggio è un muro e molti oggetti restano in strada. Chi lo perde deve scorrere l'elenco o telefonare. La restituzione dipende da un incontro casuale tra un oggetto custodito e una persona che lo cerca.

Il progetto costruisce il **registro unico degli oggetti smarriti**: chi trova può depositare l'oggetto in un punto civico vicino, che ne diventa il **ritrovatore legale** e lo custodisce per al massimo 7 giorni prima di consegnarlo alla PA; l'oggetto entra subito in un registro privato; un agente conversazionale cerca e verifica la titolarità; il custode consegna l'oggetto alla persona giusta. La Pubblica Amministrazione mantiene la responsabilità legale della denuncia e della pubblicazione; il sistema le restituisce un flusso operativo funzionante.

**Tesi:** non un catalogo con un chatbot sopra, ma un'infrastruttura di fiducia per il ritrovamento. Il catalogo esiste, ma è privato; l'informazione è asimmetrica; l'agente verifica prima di rivelare; la catena di custodia è tracciata.

---

## 2. Obiettivi, non-obiettivi, anti-obiettivi

### Obiettivi

- Quattro interfacce: finder, intermediario, searcher, istituzione.
- Un unico registro degli oggetti, con due fasi di custodia: intermediario (max 7 giorni) e PA.
- Finder con CIE/SPID (self-enrollment) e finder senza CIE/SPID (mediazione dell'intermediario).
- Cessione esplicita della qualità di ritrovatore dal finder all'intermediario, registrata come evento.
- Searcher conversazionale con agente + d3 per la probabilità di titolarità.
- Verifica proporzionale al rischio, con prove e soglie.
- Approvazione del ritiro da parte del custode.
- Pass QR firmato, monouso, legato a claim, oggetto, searcher, custode e pagamento.
- Interfaccia istituzione unica: lista ritiri, intake, review, fee, consegna, report.
- Restituzione diretta entro 7 giorni gratuita; dopo il trasferimento, pagamento solo alla PA.
- Premio art. 930: registrato e mostrato al searcher, pagamento fuori dal sistema.
- Non reclamati: dopo un anno offerti al ritrovatore legale (intermediario).
- Sistema funzionante: persistenza, auth, ruoli, notifiche, audit, pagamento test mode.
- Demo end-to-end delle quattro interfacce.

### Non-obiettivi

- Generare la denuncia di smarrimento o gestire l'albo pretorio: sono responsabilità della PA.
- Incassare o gestire il pagamento del premio art. 930: il sistema lo registra e lo mostra, il pagamento resta fuori.
- Applicare tariffe PA a una restituzione diretta avvenuta prima del trasferimento.
- Catalogo pubblico di foto o dettagli unici.
- Localizzatore GPS, assicurazione, marketplace.
- Provider di pagamento reale nella fase 0 (test mode).
- Multi-città nella fase 0.

### Anti-obiettivi

- Un “mago di Oz” che funziona solo in presentazione.
- Un muro pubblico di foto di oggetti trovati.
- Un agente che rivela dettagli o posizioni prima della verifica.
- Un sistema che scarica burocrazia sul negozio.
- Due sistemi o due database che raccontano storie diverse.

---

## 3. Vincoli

- Hackathon: evento di 20 ore, ma il progetto è pensato per durare; la roadmap distingue MVP demo e sistema reale.
- Modello legale: l'intermediario diventa ritrovatore per cessione del finder; 7 giorni max (24h sensibili); convenzione con la PA per rendere il termine compatibile con il “senza ritardo” dell'art. 927.
- d3: 26B multimodale, BF16 ≈ 52GB. Una 3090 da 24GB non basta per il modello pieno; opzioni: quantizzazione 4-bit, hosted Space/API, GPU 48GB+.
- CIE/SPID: integrazione reale o mock nel MVP; fallback con mediazione dell'intermediario per il finder e documento al custode per il searcher.
- Adozione PA e accordi con gli intermediari: prerequisiti reali; in demo mostriamo il protocollo.
- Privacy e dati personali: nessun dato pubblico; retention e audit.
- Open source, open data aggregato, nessun PII pubblico.

---

## Decision log — calls dopo il builder check

Queste decisioni sono mie e prevalgono sul testo successivo in caso di conflitto.

1. **Ordine pass/pagamento:** verifica → approvazione del custode → fee finale → pagamento PA → pass emesso → scan del custode → consegna. Il pass è prova di approvazione e pagamento. Se il pagamento avviene al banco, il pass è `pending_payment` e lo scan del custode verifica il pagamento prima di consegnare. Nell'MVP: pagamento online test mode prima del pass.
2. **Approvazione:** la policy auto-approva low/medium sopra soglia; high richiede approvazione esplicita del custode; sensitive richiede PA e ritiro. L'intermediario vede un summary (oggetto, searcher, fascia di confidenza, flag, fee/reward), non il report completo. Il report completo è della PA.
3. **Searcher senza SPID:** pass con `identity_required=true`; il custode verifica il documento al ritiro e registra tipo/numero (o hash). La chat resta guest. Il pass non è `validated` finché il documento non è controllato.
4. **Risk tier:** assegnato da una policy per categoria + suggerimento AI; finder/intermediario non possono abbassarlo; la PA può fare override con audit. Sensibili: documenti d'identità, carte di credito/debito, chiavi, dispositivi medici, SIM, documenti sanitari. Un contenuto sensibile visibile nelle foto alza il tier.
5. **Claim multipli:** ammessi; un solo claim approvato/attivo per oggetto; il primo pass redeemed vince; gli altri vanno a `rejected`/`cancelled` con messaggio non rivelatore. Rejection: l'oggetto torna `in_custody`; appello possibile; override PA. Pass scaduto/revocato: nuovo pass o claim riaperto; il trasferimento di custodia revoca il pass e ne emette uno nuovo per il nuovo custode.
6. **Enrollment e custodia:** gli oggetti `pending_custody` non sono ricercabili; solo `in_custody`. Se l'intermediario rifiuta, l'oggetto va a `custody_rejected`; il finder è notificato e può scegliere un altro punto; timeout di accettazione (24h) con notifica.
7. **PDF:** la migrazione completa delle 510 pagine non è in scope per l'MVP. Si parte da nuovi intake + un seed rappresentativo di reperti reali per la demo. La migrazione OCR/entity resolution è fase 2 o fuori scope se la PA fornisce dati strutturati.
8. **Modelli esterni:** niente immagini/documenti/identità grezze a provider esterni senza DPA. OCR/redazione locale; a d3 hosted si inviano solo valori redatti/booleani (es. `identity_match: {name: true, dob: true}`), non i campi grezzi. `VerificationEngine` impone il boundary. Con d3 locale, i dati restano nel perimetro.
9. **Hackathon:** deliverable e criteri del manuale: link online, repo pubblica, video 3 minuti, form; demo 3+3 minuti; criteri 33/33/33; nessuno stack sponsor obbligatorio. d3 e SPID reale non sono requisiti della giuria: sono scelte di prodotto. SPID può essere mock nell'MVP; d3 è un differenziatore con fallback.
10. **Team:** tu lead; aree modulari (dominio, agent/d3, quattro interfacce, PA/payment, demo). Il collega rivede il report e prende task in base alla review; il piano non assume un merge split rigido.
11. **Orizzonte:** solo hackathon, demo-first. Code freeze 13:30; submission 14:00; demo 14:30; video di backup entro 13:00. Il sistema reale è post-evento.
12. **Stack:** backend **Go 1.26 + Gin** sul boilerplate `fantastic-gin-dealcoholized` (layer `handler → domain → repository`, vendoring, goose); **SQLite** (GORM + `go-sqlite3`) per il relazionale; **Qdrant** in Docker + **vLLM embeddings** (`nvidia/Nemotron-3.5-Embed-8B-BF16`, 4096-d) per il vettoriale; **DeepSeek V4.1 Flash** come orchestrator/LLM; **D3 via API** come decision engine; frontend **Vue 3 + Vite + Tailwind**, embeddato con `-tags webui`. Deploy demo: Docker Compose + Cloudflare Tunnel.
13. **Modello legale:** il finder cede la qualità di ritrovatore all'intermediario al deposito; l'intermediario è il ritrovatore ex art. 927 e custodisce max 7 giorni (24h sensibili), poi consegna alla PA. Restituzione diretta entro 7 giorni gratuita; dopo il trasferimento, tariffa PA. Premio art. 930 registrato e mostrato al searcher, pagamento fuori dal sistema. Non reclamati: dopo un anno offerti al ritrovatore legale (intermediario). Da validare con un legale.

---

## 4. Approcci considerati

| Approccio | Cosa | Perché no / perché sì |
|---|---|---|
| A. Layer sul PDF esistente | Nuova ricerca + chat sopra il catalogo attuale | Veloce, ma non risolve il flusso, l'attrito e la custodia; la rete civica resta parallela e fragile. |
| B. Due sistemi paralleli | Catalogo ufficiale + rete civica separata | Semplice da dividere, ma due fonti di verità, due demo, narrazione debole. |
| C. Registro unico, quattro interfacce, ritrovatore intermediario, approvazione del custode | **Scelto** | Una fonte di verità, catena di fiducia completa, quattro attori con permessi diversi, modello legale coerente con art. 927-930. Più lavoro, ma il sistema è reale. |

**Perché C:** il prodotto non è il catalogo, è la catena di custodia e verifica. Un registro unico permette a finder, intermediario, searcher e PA di lavorare sullo stesso oggetto senza copie; il ritrovatore intermediario elimina l'attrito del conferimento immediato alla PA e rende la restituzione diretta possibile; il pass QR rende la consegna verificabile.

---

## 5. Utenti e ruoli

| Ruolo | Accesso | Può | Non può |
|---|---|---|---|
| Finder | CIE/SPID o mediazione intermediario | Enrollare, consegnare, cedere la qualità di ritrovatore, vedere la propria ricevuta | Vedere il registro, altri oggetti, il profilo privato |
| Intermediario | Account business + convenzione PA | Diventare ritrovatore legale, confermare custodia (max 7 giorni), enrollare, ricevere match, scansionare QR, consegnare, richiedere il premio | Vedere il report completo, approvare claim fuori dalla custodia |
| Searcher | Chat guest; claim con CIE/SPID o documento | Cercare, verificare, prenotare, pagare la PA, ritirare | Vedere dettagli privati o posizione prima della soglia |
| Istituzione | Account operatore | Lista ritiri, intake, review, fee, consegna, report, gestione non reclamati | Accedere a dati non necessari al ruolo (audit) |
| Admin | Account admin | Configurare punti, categorie, soglie, tariffe, ruoli | — |

Lo stesso `Item` è letto da tutti, ma ogni ruolo vede un sottoinsieme diverso. Non esistono copie del dato.

---

## 6. Le quattro interfacce

### 6.1 Finder

**Accesso**

- Con CIE/SPID: self-enrollment.
- Senza CIE/SPID: mappa/elenco dei punti aderenti; il finder porta l'oggetto e l'intermediario fa enrollment dal proprio account.

**Flusso**

1. Foto guidate per categoria: ombrello 1; telefono 3+ (fronte, retro, lati); portafoglio 2 (aperto/chiuso); documento 1 con oscuramento.
2. Geolocalizzazione: device + pin sulla mappa + correzione manuale.
3. Categoria e dettagli suggeriti dall'AI; il finder conferma o corregge.
4. Scelta dell'intermediario da lista o QR.
5. **Cessione della qualità di ritrovatore:** il finder accetta esplicitamente che l'intermediario diventi il ritrovatore legale, con i diritti che ne derivano (premio art. 930, acquisto dell'oggetto non reclamato dopo un anno, art. 929). L'accettazione è un evento di custodia.
6. Conferma e ricevuta: ID oggetto, stato, punto, cosa succede dopo, cessione registrata.
7. Contatto opzionale per aggiornamenti; nessun account obbligatorio.

**Vincoli**

- Il finder non vede il registro né altri oggetti.
- Il finder non può modificare l'oggetto dopo la conferma senza un evento di audit.
- Il finder senza CIE/SPID non può fare self-enrollment; la mediazione dell'intermediario è il percorso previsto.
- Senza cessione registrata, l'oggetto non entra in `in_custody`.

### 6.2 Intermediario

**Accesso**

- Account business verificato + convenzione con la PA.
- Badge “Punto Ritrovato” e visibilità nella mappa.

**Flusso**

1. Diventa il **ritrovatore legale** dell'oggetto (art. 927 c.c.) per cessione del finder; ne risponde fino alla consegna alla PA.
2. Conferma la presa in custodia: scansione ricevuta/QR del finder o dashboard.
3. Può fare enrollment di un oggetto trovato dal suo staff o da un finder senza identità digitale.
4. Custodisce per **al massimo 7 giorni** (24h per gli oggetti sensibili); allo scadere l'oggetto va in `pickup_due` e viene consegnato alla PA.
5. Riceve la notifica di match: oggetto, alias del searcher, finestra di ritiro, stato del pagamento/reward. Non vede il report privato.
6. Al ritiro, scansiona il QR del searcher; il sistema valida claim, approvazione e pagamento; l'intermediario conferma la consegna.
7. **Restituzione diretta entro i 7 giorni:** è il ritrovatore che rende la cosa; nessuna tariffa PA, restituzione gratuita per il searcher.
8. Può **richiedere il premio art. 930**: il sistema registra la richiesta e la mostra al searcher prima del ritiro; il pagamento del premio resta fuori dal sistema.
9. Dopo la consegna alla PA, l'oggetto esce dalla sua custodia; resta il diritto al premio e, dopo un anno, l'offerta dell'oggetto non reclamato.
10. Può rifiutare un oggetto con motivo (es. capacità, categoria non accettata).

**Vincoli**

- L'intermediario è ritrovatore e custode, non investigatore: vede solo l'autorizzazione al ritiro.
- Non può consegnare senza pass valido.
- Non può trattenere l'oggetto oltre 7 giorni (24h sensibili).
- Gli oggetti sensibili vengono ritirati dalla PA entro 24h.
- Non incassa tariffe PA dal searcher; l'eventuale premio è un rapporto separato, mostrato dal sistema ma pagato fuori.

### 6.3 Searcher

**Accesso**

- Chat guest.
- Claim con CIE/SPID; in alternativa, documento al custode.
- Il searcher non vede il registro; l'agente è l'unica interfaccia.

**Flusso**

1. “Ho perso un iPhone 17 in zona De Lolli ieri sera.”
2. L'agente estrae categoria, marca/modello, colore, zona, finestra temporale, segni particolari.
3. Cerca nel registro con filtri strutturati + similarità vettoriale.
4. Mostra solo l'esistenza di candidati compatibili, non i dettagli.
5. Fa domande aperte, una alla volta, generate dal profilo privato.
6. Usa d3 per calcolare la probabilità di titolarità; aggiorna la confidenza.
7. Sopra soglia, crea il claim e propone il ritiro; sotto soglia, chiede un'altra informazione o escala.
8. Se l'oggetto è ancora presso l'intermediario: ritiro diretto, gratuito (nessuna tariffa PA); eventuale premio richiesto mostrato prima del ritiro.
9. Se l'oggetto è già stato trasferito alla PA: ritiro in PA, pagamento della tariffa PA.
10. Riceve il pass QR, il punto di ritiro, le istruzioni e il dettaglio di fee/reward.

**Vincoli**

- Nessuna posizione o dettaglio privato prima della soglia.
- L'agente non corregge mai i dettagli (“non corrisponde”, non “no, è blu”).
- Tentativi limitati, rate limit, lockout, escalation umana.
- Il claim richiede identità; la chat no.
- Il pass è legato al custode corrente: se l'oggetto viene trasferito, il pass viene revocato e riemesso.

### 6.4 Istituzione

**Accesso**

- Un'unica interfaccia per Polizia Locale e Reparto, con permessi per ruolo operativo.
- Nessuna distinzione di prodotto tra i due enti.

**Flusso**

1. **Lista ritiri:** oggetti da ritirare dagli intermediari dopo 7 giorni (24h se sensibili), raggruppati per zona/esercizio, con priorità.
2. **Presa in carico:** segna “ritirato”; la custodia passa all'istituzione; l'intermediario è notificato; l'evento è registrato.
3. **Pubblicazione art. 928:** a cura della PA, fuori dal sistema; da questo momento decorre il termine di un anno dell'art. 929. Il sistema può registrare `transferred_at` e `legal_term_start` come riferimento.
4. **Intake ufficiale:** foto → descrizione AI → oggetto nel registro; stesso schema del finder.
5. **Review del match:** report con domande, risposte, probabilità d3, prove, flag; approva/rifiuta/chiedi altro.
6. **Fee e pagamento:** calcolo della tariffa PA, stato del pagamento, ricevuta.
7. **Consegna:** verifica del pass, documento, firma/conferma; chiusura della pratica.
8. **Non reclamati:** dopo un anno, l'oggetto viene offerto al ritrovatore legale (intermediario); se rifiuta, smaltimento.
9. **Report:** oggetti in custodia, restituzioni, pagamenti, giacenze, sensibili, tempi medi.

**Vincoli**

- La denuncia di smarrimento e l'albo pretorio restano fuori dal sistema.
- La PA vede il report completo; l'intermediario no.
- Ogni accesso è auditato.

---

## 7. Modello di dominio

### 7.1 Entità

**`Item`** — aggregato radice.

- `id`, `public_code`
- `source`: `finder_self` | `finder_mediated` | `operator`
- `custody_mode`: `intermediary` | `institution`
- `status`: `enrolled` | `pending_custody` | `custody_rejected` | `in_custody` | `matched` | `claim_pending` | `approved` | `returned` | `pickup_due` | `sensitive_pickup` | `transferred` | `unclaimed` | `offered_to_finder` | `finder_collected` | `disposed`
- `legal_finder_id` (intermediario o, se trovato da operatore, PA), `rights_assigned_at`
- `category`, `brand`, `model`, `color`, `condition`
- `private_profile` (JSON: dettagli unici, contenuto, campi documento, embedding testuali/visivi)
- `photos` (private), `found_at`, `found_location` (lat/lng, zona), `found_notes`
- `custodian_id`, `risk_tier` (`low` | `medium` | `high` | `sensitive`), `sensitive` flag
- `transferred_at`, `legal_term_start`, `offered_to_finder_at`, `disposed_at`
- `created_at`, `updated_at`

**`Custody`**

- `item_id`, `custodian_type` (`intermediary` | `institution`), `custodian_id`
- `legal_finder_id`, `rights_assigned_at`
- `approval_authority` (`intermediary` | `institution`)
- `max_custody_days` (7 | 1), `accepted_at`, `due_at`, `closed_at`

**`CustodyEvent`**

- `item_id`, `type`, `actor_id`, `timestamp`, `notes`, `evidence_ref`
- Tipi: `enrolled`, `rights_assigned`, `accepted`, `matched`, `verified`, `approved`, `handover`, `direct_return`, `pickup_scheduled`, `transferred`, `sensitive_notified`, `unclaimed`, `offered_to_finder`, `finder_collected`.

**`Claim`**

- `item_id`, `searcher_id` (o `guest_id` + identità al ritiro)
- `status`: `draft` | `searching` | `verifying` | `submitted` | `under_review` | `approved` | `rejected` | `more_info` | `scheduled` | `handed_over` | `closed`
- `return_channel`: `intermediary_direct` | `pa`
- `confidence`, `evidence`, `approval_by`, `approved_at`
- `pickup_pass_id`, `reward_notice_id`

**`VerificationSession`**

- `claim_id`, `questions` (d3), `answers`, `probabilities`, `rationale`, `model_version`, `flags`, `created_at`.

**`PickupPass`**

- `id`, `claim_id`, `item_id`, `searcher_id`, `custodian_id`
- `token` (firmato), `issued_at`, `expires_at`, `status` (`pending_payment` | `issued` | `validated` | `redeemed` | `expired` | `revoked`)
- `identity_required` (searcher senza SPID: documento al custode)
- `return_channel`, `payment_id`, `approval_id`, `fee_id`, `reward_notice_id`

**`Fee`**

- `item_id`, `applies` (`none_direct_return` | `pa_tariff`), `pa_tariff_id`, `custody_days`, `amount`, `currency`, `status` (`estimated` | `final` | `paid` | `waived`).

**`RewardNotice`**

- `id`, `item_id`, `legal_finder_id`, `requested`, `amount`, `shown_to_searcher_at`, `payment_status` (`outside_system` | `paid_directly` | `waived`).

**`Payment`**

- `fee_id`, `provider`, `provider_ref`, `status`, `paid_at`, `receipt_url`.

**`Intermediary`**

- `id`, `name`, `address`, `geo`, `hours`, `capacity`, `status`, `agreement_ref`, `contact`
- `agreement_ref` include il termine di 7 giorni e il modello di cessione.

**`LostReport`** (opzionale, match proattivo)

- `id`, `searcher_id`, `description`, `category`, `location`, `time_range`, `embedding`, `status`.

**`Notification`**

- `id`, `recipient_id`, `type`, `payload`, `read_at`, `created_at`.

**`AuditLog`**

- `id`, `actor_id`, `action`, `entity`, `entity_id`, `metadata`, `timestamp`.

**`Actor` / `Role`**

- `id`, `type` (`finder` | `searcher` | `intermediary` | `institution` | `admin`), `identity_ref`, `contact`.

### 7.2 Macchine a stati

```text
Fase 1 — presso l'intermediario (max 7 giorni)

Item:
  enrolled
    → pending_custody
      → in_custody
        → matched
          → claim_pending
            → approved
              → returned          (restituzione diretta, gratuita)
        → pickup_due (7° giorno)
          → transferred
        → sensitive_pickup (24h)
          → transferred
      → custody_rejected
        → pending_custody (reroute) | cancelled

Fase 2 — presso la PA (dopo transferred; pubblicazione art. 928 a cura PA)

Item:
  transferred
    → matched
      → claim_pending
        → approved
          → returned            (ritiro in PA, tariffa PA)
    → unclaimed (1 anno dall'ultima pubblicazione)
      → offered_to_finder
        → finder_collected
        → disposed

Claim:
  draft → searching → verifying → submitted → under_review
    → approved / rejected / more_info
      → scheduled → handed_over → closed
  approved → expired → scheduled
  approved → revoked → under_review
  rejected → under_review (appeal)

Pass:
  pending_payment → issued → validated → redeemed
  issued → expired
  issued → revoked
```

### 7.3 Invarianti

- Un solo custode corrente per oggetto; ogni trasferimento è un evento.
- Approva il custode; il sistema valida il pass.
- Nessun pass senza verifica, approvazione e pagamento.
- **Nessun oggetto entra in `in_custody` senza la cessione registrata della qualità di ritrovatore.**
- **L'intermediario custodisce al massimo 7 giorni; allo scadere l'oggetto va obbligatoriamente in `pickup_due` e trasferito alla PA.**
- **Gli oggetti sensibili: notifica immediata alla PA, ritiro entro 24h, consegna finale all'istituzione.**
- **Nessuna tariffa PA su una restituzione diretta avvenuta prima di `transferred`.**
- **`disposed` solo dopo `offered_to_finder`: dopo un anno l'oggetto spetta prima al ritrovatore legale (intermediario).**
- L'intermediario vede solo l'autorizzazione al ritiro, non il report completo.
- La PA vede il report completo.
- Le probabilità d3 e la versione del modello sono loggate; la policy è deterministica.
- Nessun dato pubblico oltre ciò che l'agente è autorizzato a rivelare.
- Ogni cambio di stato produce un evento di audit.

---

## 8. Agente e verifica

### 8.1 Architettura

- **LLM orchestrator:** dialogo, estrazione query, scelta candidati, generazione domande, tool calling, spiegazione.
- **d3 judge:** stato + domande → probabilità.
- **Policy engine:** probabilità + rischio + prove → decisione.

### 8.2 Tool dell'agente

- `search_items(filters, embedding)` → candidati.
- `get_private_profile(item_id)` → solo interno.
- `generate_question(item_id, attribute)` → domanda.
- `score_with_d3(state, questions)` → probabilità.
- `request_evidence(claim_id, type)` → prova.
- `check_identity_document(item_id, identity)` → match documentale.
- `compute_confidence(session_id)` → confidenza aggregata.
- `policy_decision(claim_id)` → auto/review/reject/more_info.
- `create_claim(item_id, searcher_id)` → claim.
- `generate_pass(claim_id)` → QR.
- `calculate_fee(item_id)` → tariffa PA (solo se `return_channel=pa`).
- `get_reward_notice(item_id)` → eventuale premio richiesto.
- `notify_intermediary(item_id)` / `notify_institution(item_id)`.
- `escalate(claim_id, reason)`.

### 8.3 Protocollo di verifica

1. Intake: cosa, dove, quando, descrizione, foto opzionale, prova opzionale.
2. Retrieval: categoria, zona, tempo, stato; similarità testo/immagine.
3. Ranking: match strutturato + similarità + prossimità + rischio.
4. Domande: aperte, una alla volta, attributi discriminanti; mai multiple choice.
5. Scoring: d3 restituisce probabilità per attributo; il sistema aggiorna la confidenza.
6. Prove: ricevuta, seriale, foto, documento; per i documenti, confronto con CIE/SPID.
7. Decisione: policy per rischio; sopra soglia → claim; sotto → altra domanda o escalation.
8. Report: domande, risposte, probabilità, prove, flag, rationale.

### 8.4 Soglie per rischio

- `low`: 1-2 domande, pass senza documento.
- `medium`: 3-5 domande, pass + documento al ritiro.
- `high`: intervista + prova + documento; review del custode.
- `sensitive`: notifica immediata PA, ritiro entro 24h, consegna finale all'istituzione.

### 8.5 d3: hosting e fallback

- d3 / Decision 3.0 (`vllm-sr/d3`): 26B multimodale, Apache-2.0, input testo/JSON + immagini/video, domande `choice`/`noul`/`score`, output probabilità, nessun testo generato.
- **Scelta stack:** d3 via API/Space ufficiale, niente GPU locale. BF16 26B ≈ 52GB: una 3090 24GB non basta; l'API evita il problema.
- `VerificationEngine` con `D3Client` e `LocalScorer`; la demo usa d3 se disponibile, il fallback garantisce continuità.
- Hosted: input redatti/booleani; documenti sensibili solo form/local, mai immagini a provider esterni.

---

## 9. Catena di fiducia e pass

```text
Identità → Cessione diritti → Custodia → Verifica → Approvazione → Fee (se PA) → Pagamento → Pass → Consegna → Audit
```

Il pass è emesso **dopo** l'approvazione e il pagamento (o con `pending_payment` se si paga al banco): è la prova di entrambi. Se il pagamento avviene al banco, lo scan del custode verifica il pagamento prima di consegnare.

- **Identità:** CIE/SPID per finder e searcher; mediazione intermediario per finder senza identità; documento al custode per searcher senza SPID.
- **Cessione diritti:** il finder cede la qualità di ritrovatore all'intermediario; l'evento è registrato.
- **Custodia:** ogni passaggio è un evento con attore, timestamp e prova; max 7 giorni (24h sensibili).
- **Verifica:** l'agente non rivela i dettagli; domande aperte, no correzioni, tentativi limitati.
- **Approvazione:** il custode approva; la PA interviene per ufficiale, sensibile, eccezioni.
- **Fee/pagamento:** nessuna tariffa PA per la restituzione diretta; tariffa PA solo dopo `transferred`.
- **Pass:** QR firmato, breve scadenza, monouso, legato a claim/oggetto/searcher/custode/pagamento.
- **Reward:** eventuale richiesta del ritrovatore registrata e mostrata; pagamento fuori dal sistema.
- **Audit:** ogni accesso e cambio di stato.

---

## 10. PA, fee, pagamento, premio, non reclamati

- Il sistema **non genera la denuncia di smarrimento** e non gestisce l'albo pretorio: sono responsabilità della PA.
- **Restituzione diretta entro 7 giorni:** è il ritrovatore (intermediario) che rende la cosa; nessuna tariffa PA; restituzione gratuita per il searcher.
- **Restituzione dopo il trasferimento:** il searcher paga **solo la PA**, secondo le tariffe vigenti (Delib. 421/2023).
- **Premio art. 930:** spetta al ritrovatore legale (intermediario) solo se lo chiede. Il sistema registra la richiesta e la mostra al searcher prima del ritiro; il pagamento del premio resta fuori dal sistema.
- **Non reclamati art. 929:** dopo un anno dall'ultima pubblicazione, l'oggetto viene offerto al ritrovatore legale (intermediario); se rifiuta, smaltimento.
- **Tariffa PA:** configurabile (giorni di custodia, eventuali diritti). `Fee` + `Payment`: stima, finale, pagamento, ricevuta.
- **Provider di pagamento:** astratto; test mode nella fase 0.
- L'intermediario ha una convenzione con la PA; non incassa tariffe PA dal searcher.
- La lista ritiri PA è generata da `Custody` + `due_at` + flag sensibile.

---

## 11. Privacy e sicurezza

- Registro privato; nessun catalogo pubblico di foto o dettagli unici.
- L'agente rivela solo ciò che serve al ritiro, dopo la verifica.
- Campi documento solo nel profilo privato; confronto con CIE/SPID; mai esposizione pubblica.
- Accesso per ruolo; intermediario minimo; PA report completo.
- Audit su ogni accesso e cambio di stato.
- Retention allineata ai termini; foto cancellate dopo restituzione o smaltimento.
- Prompt injection, false claims, tentativi multipli, rate limit, lockout, review umana.
- Open data solo aggregato e anonimo.

---

## 12. Requisiti funzionali

### Must (MVP)

- Auth e ruoli per le quattro interfacce.
- Finder self-enrollment con foto, geo, categoria, intermediario, **cessione diritti**.
- Finder mediato: mappa punti + enrollment intermediario.
- Intermediario: ritrovatore legale, conferma custodia, enrollment, notifiche match, scansione QR, consegna, **timer 7 giorni/24h**.
- Searcher: chat, ricerca, verifica, claim, pass.
- Agente con tool calling e stato conversazione.
- d3 o fallback deterministico, con report probabilità.
- Istituzione: lista ritiri, intake, review, fee, consegna.
- Pass QR firmato, monouso, validazione.
- Pagamento PA test mode + ricevuta; **nessuna fee PA per restituzione diretta**.
- **Reward notice** registrata e mostrata; pagamento fuori.
- **Non reclamati**: offerta al ritrovatore legale dopo un anno.
- Persistenza, migrazioni, seed.
- Audit log.
- Notifiche match/ritiro/lista.
- Test end-to-end del flusso demo.

### Should

- Match proattivo con `LostReport`.
- Vista catalogo minimale nella chat (categoria/data/zona) come safety net.
- Multilingua italiano/inglese.
- Admin per punti, categorie, soglie, tariffe.
- Export/report per la PA.
- Open data aggregato.

### Could

- Pagamento reale.
- Multi-città.
- Voce (ElevenLabs) per l'agente.
- App mobile.
- Analytics e dashboard impatto.

---

## 13. Requisiti non funzionali

- **Affidabilità:** la demo non si blocca; fallback d3, seed, gestione errori.
- **Prestazioni:** ricerca < 2s; risposta agente < 5s; scansione QR < 1s.
- **Sicurezza:** RBAC, token firmati, rate limit, audit, cifratura a riposo per i dati sensibili.
- **Privacy:** minimizzazione, retention, accesso per ruolo.
- **Accessibilità:** contrasto, tastiera, screen reader, messaggi chiari.
- **Internationalization:** italiano + inglese.
- **Testabilità:** API testabili, seed deterministico, test end-to-end.
- **Deployabilità:** ambiente demo pubblico + database persistente.
- **Osservabilità:** log strutturati, metriche base, audit.

---

## 14. Demo slice vs production path

**Demo:**

- Finder: telefono davanti alla Sapienza → 3 foto → geo → Bar X → cessione diritti → ricevuta.
- Intermediario: conferma custodia; timer 7 giorni.
- Searcher: chat “ho perso un iPhone 17 in zona De Lolli” → match → 3 domande → d3/fallback → pass.
- Custode: scan QR → restituzione diretta gratuita al Bar X.
- Istituzione: intake portafoglio / trasferimento → review → tariffa PA (contrasto con la restituzione gratuita).
- Video di 3 minuti; demo live con seed e fallback.

**Produzione:**

- Auth reale, notifiche, payment provider, audit, retention, accordi, pilota Sapienza + bar.
- PA: intake, lista ritiri, review, fee, report, non reclamati.
- Hardening: sicurezza, test, osservabilità, accessibilità.

---

## 15. Team e piano

- **Owner:** tu, lead su tutte le aree; il collega rivede il report e prende task in base alla review.
- **Aree modulari:** dominio/backend, agente/d3, frontend quattro interfacce, PA/pagamento, demo/seed/video.
- **Orizzonte:** solo hackathon. Il piano è demo-first; il sistema reale è post-evento.
- **Code freeze demo:** 13:30; submission 14:00; demo 14:30. Video di backup entro 13:00.
- Il piano di build dettaglia task, dipendenze e milestone.

---

## 16. Rischi e domande aperte

**Deciso:**

- Team: tu lead, aree modulari; il collega rivede e prende task in base alla review.
- Orizzonte: solo hackathon; piano demo-first.
- Stack: Go/Gin boilerplate, SQLite, Qdrant + vLLM Nemotron embeddings, DeepSeek V4.1 Flash, D3 API, Vue 3 embedded.
- Fallback obbligatori: SQLite search, LocalScorer, MockAgent.

**Esterni da validare:**

- **Cessione della qualità di ritrovatore:** da validare con un legale che sia efficace per premio (art. 930) e acquisto dopo un anno (art. 929).
- **Convenzione con la PA:** deve fissare il termine di 7 giorni (24h sensibili) e il modello di cessione.
- Adozione PA, accordi intermediari, titolare/retention, pagamento reale.
- Catalogo della sfida: l'agente è il catalogo; vista minimale come safety net.

**Risolte nel design:**

- Searcher senza SPID: pass `identity_required` + documento al custode.
- Sensibili: notifica PA + ritiro 24h.
- PDF: fuori scope MVP; seed rappresentativo.
- Dati personali: boundary per i modelli esterni.

---

## 17. Cosa resta al builder

Stack, schema fisico, UI, prompt, seed, deployment, piano di test e copione definitivo. Il design fissa il modello e i confini; il piano dirà come costruirlo.
