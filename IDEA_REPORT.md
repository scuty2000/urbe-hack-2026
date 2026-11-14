# IDEA REPORT — Registro unico degli oggetti smarriti

**Data:** 13 novembre 2026
**Stato:** idea + architettura logica (pre-design doc)
**Contesto:** AgenticHack Rome, sfida 03 “Oggetti smarriti”

---

## 1. Executive summary

Ogni anno Roma raccoglie migliaia di oggetti smarriti. Il Reparto Oggetti Rinvenuti li pubblica in un PDF per anno: l'elenco 2026, dal 1 gennaio all'11 novembre, è di 510 pagine non cercabili (reperti n. 1–6940). Chi trova un oggetto deve consegnarlo alla Polizia Locale; quel passaggio è un muro e molti oggetti restano in strada. Chi lo perde deve scorrere l'elenco o telefonare. La restituzione dipende da un incontro casuale tra un oggetto custodito e una persona che lo cerca.

Il progetto costruisce il **registro unico degli oggetti smarriti**: un sistema in cui chi trova può depositare l'oggetto in un punto civico vicino, che ne diventa il ritrovatore legale e lo custodisce per al massimo 7 giorni prima di consegnarlo alla PA; l'oggetto entra subito in un registro privato, un agente conversazionale cerca e verifica la titolarità, e il custode consegna l'oggetto alla persona giusta. La Pubblica Amministrazione mantiene la responsabilità legale della denuncia e della pubblicazione; il sistema le restituisce un flusso operativo funzionante.

**La tesi:** non un catalogo con un chatbot sopra, ma un'infrastruttura di fiducia per il ritrovamento. Il catalogo esiste, ma è privato; l'informazione è asimmetrica; l'agente verifica prima di rivelare; la catena di custodia è tracciata.

---

## 2. Il problema

- **Per chi trova:** consegnare alla Polizia Locale richiede tempo, spostamento e burocrazia. Il risultato è che molti oggetti non entrano mai nel sistema.
- **Per chi perde:** l'elenco è un PDF di 510 pagine, non cercabile; bisogna scorrere, telefonare, sperare. Non c'è un modo semplice per descrivere l'oggetto e ricevere una risposta.
- **Per la PA:** il Reparto riceve oggetti in modo discontinuo, con descrizioni eterogenee, e deve gestire custodia, pratiche, restituzioni e pubblicità legale. La qualità del dato è bassa.
- **Per la privacy:** mostrare foto e dettagli degli oggetti trovati può esporre documenti, volti, targhe e informazioni personali; un catalogo pubblico è anche un bersaglio per false claims.
- **Per la fiducia:** chi trova non sa se l'oggetto arriverà al proprietario; chi perde non sa se il sistema sta facendo qualcosa; la PA non ha una catena di custodia digitale.

---

## 3. La tesi di prodotto

Il sistema non deve pubblicare un muro di oggetti. Deve **mettere in sicurezza l'oggetto, verificare la persona e tracciare la consegna**. Il valore sta nella catena di fiducia:

1. **Identità** — finder e searcher sono identificati (CIE/SPID o mediazione dell'intermediario).
2. **Custodia** — l'oggetto è in un punto autorizzato, con una presa in carico digitale.
3. **Verifica** — l'agente confronta la descrizione del searcher con il profilo privato dell'oggetto e calcola una probabilità di titolarità.
4. **Approvazione** — il custode approva la consegna; la PA interviene dove serve.
5. **Pass** — un QR monouso lega claim, oggetto, searcher, custode e pagamento.
6. **Audit** — ogni evento è registrato; la PA può ricostruire la storia.

---

## 4. Gli attori e le quattro interfacce

| Interfaccia | Attore | Accesso | Funzione |
|---|---|---|---|
| **Finder** | chi trova | CIE/SPID per self-enrollment; senza CIE/SPID, mappa dei punti aderenti | Foto, geolocalizzazione, categoria, consegna all'intermediario |
| **Intermediario** | bar, negozio, portineria | Account business + convenzione con la PA | Diventa il ritrovatore legale; conferma custodia (max 7 giorni), enrollment al posto del finder, notifiche match, scansione QR, consegna, passaggio alla PA |
| **Searcher** | chi ha perso | Chat guest; claim con CIE/SPID o documento al custode | Conversazione, ricerca, verifica, ritiro |
| **Istituzione** | PA | Account operatore | Lista ritiri, intake ufficiale, review, fee, consegna, report |

### Finder

Due modi di accesso:

- **Con CIE/SPID:** il finder fa tutto da solo. Scatta N foto (una per un ombrello, tre o più per un telefono: fronte, retro, lati), la posizione viene rilevata, sceglie l'intermediario, conferma. Riceve una ricevuta con ID oggetto e stato.
- **Senza CIE/SPID:** vede una mappa/elenco di punti aderenti, porta l'oggetto al punto, e l'intermediario fa l'enrollment dal proprio account. Il finder può lasciare un contatto; non è obbligatorio.

In entrambi i casi l'oggetto entra nel registro con la stessa qualità: categoria, foto, posizione, custode, data.

In entrambi i casi, al momento del deposito il finder **accetta esplicitamente di cedere all'intermediario la qualità di ritrovatore**, insieme ai diritti che ne derivano: il premio (art. 930 c.c.) e l'acquisto dell'oggetto non reclamato dopo un anno (art. 929 c.c.). L'accettazione è registrata come evento di custodia.

### Intermediario

- Diventa il **ritrovatore legale** dell'oggetto (art. 927 c.c.): ne risponde fino alla consegna alla PA.
- Custodisce l'oggetto per **al massimo 7 giorni** (24h per gli oggetti sensibili), poi lo consegna alla PA. Il termine è fissato nella convenzione con la PA, che lo rende compatibile con il "senza ritardo" dell'art. 927.
- Conferma la presa in custodia (scansione ricevuta/QR del finder o dashboard).
- Può fare enrollment di un oggetto trovato dal suo staff o da un finder senza identità digitale.
- Riceve la notifica quando c'è un match: oggetto, alias del searcher, finestra di ritiro. Non vede il report privato.
- Scansiona il QR del searcher al momento della consegna; il sistema valida claim, approvazione e pagamento; l'intermediario conferma.
- Ha una lista degli oggetti in custodia, giorni residui, ritiri programmati dalla PA.

### Searcher

- Apre una chat: “ho perso un iPhone 17 in zona De Lolli”.
- L'agente estrae categoria, zona, tempo, descrizione; cerca nel registro privato.
- Fa domande aperte, una alla volta, e usa d3 per calcolare la probabilità di titolarità.
- Sopra soglia, propone il ritiro; sotto soglia, chiede un'altra informazione o escala.
- Il claim richiede identità: CIE/SPID, oppure documento al custode. La chat può essere guest.
- Riceve un pass QR con il punto di ritiro e le istruzioni.

### Istituzione

Un'unica interfaccia per Polizia Locale e Reparto, con gli stessi dati e permessi differenziati solo per ruolo operativo.

- Lista quotidiana degli oggetti da ritirare dagli intermediari, raggruppata per zona/esercizio.
- Presa in carico: segna “ritirato”, la custodia passa all'istituzione, l'intermediario è notificato.
- Intake ufficiale: foto → descrizione AI → oggetto nel registro.
- Review del match: report con domande, risposte, probabilità d3, prove, flag; approva/rifiuta/chiedi altro.
- Calcolo e incasso del pagamento alla PA.
- Consegna e chiusura della pratica.
- Ricerca e report.

---

## 5. Il flusso end-to-end

```text
FINDER con CIE/SPID
  └─ foto + geo + intermediario → pending

FINDER senza CIE/SPID
  └─ mappa punti → intermediario → enrollment dal suo account

INTERMEDIARIO (diventa il ritrovatore legale)
  └─ conferma custodia + cessione dei diritti del finder → in_custody
  └─ entro 7 giorni (24h se sensibile): restituzione diretta gratuita
     oppure consegna alla PA

SEARCHER
  └─ chat → LLM orchestrator → search registro
       └─ d3 judge → domande → probabilità
            └─ policy → claim → approvazione del custode
                 └─ PickupPass + pagamento PA

CUSTODE
  └─ scan QR → consegna → returned → timeline

ISTITUZIONE
  └─ lista ritiri → presa in carico → transferred
       └─ pubblicazione all'Albo (art. 928) → parte il termine di 1 anno (art. 929)
  └─ intake ufficiale → stesso registro
  └─ review, fee, consegna, report
```

Il punto chiave: **l'intermediario è un canale di acquisizione e custodia**, non un passaggio burocratico. Può fare enrollment al posto del finder, confermare la custodia e consegnare dopo la scansione del pass. La PA non deve approvare ogni ritiro: interviene dove serve.

---

## 6. Il modello logico

L'aggregato radice è `Item`. Intorno:

- `Custody` — custode corrente, storia, autorità di approvazione.
- `CustodyEvent` — catena di custodia.
- `Claim` — searcher, stato, confidenza, prove, approvazione, pass.
- `VerificationSession` — domande d3, probabilità, risposte, rationale, versione del modello.
- `PickupPass` — QR firmato, breve scadenza, monouso.
- `Fee` — tariffa PA, giorni di custodia, totale; pagamento.
- `Payment` — stato, metodo, ricevuta.
- `Intermediary` — accordo con la PA, indirizzo, orari, capacità.
- `LostReport` — opzionale, per il match proattivo.
- `Notification`, `AuditLog`, `Actor`/`Role`.

**Stati:**

```text
Fase 1 — presso l'intermediario (max 7 giorni)
Item:    enrolled → in_custody → matched → claim_pending → approved → returned
                         │      (restituzione diretta del ritrovatore, art. 927 c.1: gratuita)
                         ├→ pickup_due (al 7° giorno) → transferred
                         └→ sensitive_pickup (entro 24h) → transferred

Fase 2 — presso la PA (dopo transferred: pubblicazione art. 928)
Item:    transferred → matched → claim_pending → approved → returned
                         │      (ritiro in PA, con le tariffe della Delib. 421/2023)
                         └→ unclaimed (1 anno dall'ultima pubblicazione, art. 929)
                              → offered_to_finder (al ritrovatore, cioè l'intermediario)
                                   → finder_collected | disposed

Claim:   draft → searching → verifying → submitted → under_review
              → approved / rejected / more_info → scheduled → handed_over → closed
```

**Invarianti:**

- Un solo custode corrente; ogni trasferimento è un evento.
- Approva il custode; il sistema valida il pass.
- Nessun pass senza verifica, approvazione e pagamento.
- L'intermediario custodisce al massimo 7 giorni; allo scadere l'oggetto va obbligatoriamente in `pickup_due`.
- Oggetti sensibili: notifica immediata alla PA, ritiro entro 24h, consegna finale all'istituzione.
- Nessun oggetto entra in `in_custody` senza la cessione registrata della qualità di ritrovatore.
- Nessuna tariffa PA su una restituzione avvenuta prima di `transferred`.
- `disposed` solo dopo `offered_to_finder`: dopo un anno l'oggetto spetta prima al ritrovatore (art. 929).
- L'intermediario vede solo l'autorizzazione al ritiro, non il report completo.
- La PA vede il report completo.
- Le probabilità d3 e la versione del modello sono loggate; la policy è deterministica.
- Nessun dato pubblico oltre ciò che l'agente è autorizzato a rivelare.

---

## 7. Il motore di verifica: LLM + d3

### Perché due modelli

d3 (Decision 3.0, `vllm-sr/d3`) non è un chatbot: è un modello decisionale multimodale da 26B che prende uno stato (testo/JSON + immagini/video) e un set di domande (`choice`, `noul` sì/no, `score`) e restituisce una probabilità per ogni risposta, senza generare testo. Licenza Apache-2.0.

- **LLM orchestrator:** conduce la chat, estrae la query, sceglie i candidati, genera le domande, chiama i tool, spiega l'esito.
- **d3 judge:** riceve stato + domande, restituisce probabilità.
- **Policy engine:** mappa probabilità + classe di rischio + prove in auto-approva / review / rifiuta / chiedi altro.

### Hosting

- BF16 26B ≈ 52GB: **una 3090 da 24GB non basta** per il modello pieno.
- Opzioni: quantizzazione 4-bit (può stare in 24GB, ma qualità e visione da testare), FP8 (~26GB, comunque tight), hosted Space/API ufficiale (`vllm-sr/Decision-3.0`), o GPU 48GB+.
- Per un sistema funzionante: `VerificationEngine` con due implementazioni, `D3Client` e `LocalScorer`. La demo usa d3 se disponibile; il fallback deterministico garantisce continuità.

### Schema di verifica

```json
{
  "state": {
    "item": {"category": "smartphone", "brand": "Apple", "color": "nero", "case": "blu", "crack": "basso-sinistra"},
    "searcher": {"color": "nero", "case": "blu", "crack": "basso-sinistra"},
    "identity": {"name": "...", "dob": "..."},
    "evidence": ["foto_1.jpg", "ricevuta.jpg"]
  },
  "questions": {
    "color_match": {"type": "noul", "instructions": "La descrizione del colore corrisponde?"},
    "case_match": {"type": "noul", "instructions": "La cover corrisponde?"},
    "damage_match": {"type": "score", "instructions": "Quanto è specifico il dettaglio del difetto?", "criteria": ["generico", "plausibile", "specifico", "unico"]},
    "document_match": {"type": "noul", "instructions": "Il documento appartiene alla persona identificata?"}
  }
}
```

Il report alla PA contiene domande, probabilità, risposte, prove e rationale: la decisione è spiegabile.

---

## 8. La catena di fiducia

- **Identità:** CIE/SPID per finder e searcher; mediazione dell'intermediario per il finder senza identità digitale; documento al custode per il searcher senza SPID.
- **Custodia:** ogni passaggio è un evento con attore, timestamp e prova.
- **Verifica:** l'agente non rivela i dettagli privati; usa domande aperte, non corregge, limita i tentativi, escala.
- **Approvazione:** il custode approva; la PA interviene per ufficiale, sensibile, eccezioni.
- **Pass:** QR firmato, breve scadenza, monouso, legato a claim/oggetto/searcher/custode/pagamento.
- **Audit:** ogni accesso e cambio di stato è registrato.

---

## 9. Modello legale, PA e soldi

- Il sistema **non genera la denuncia di smarrimento**: la denuncia e la pubblicazione legale sono responsabilità della PA. Il sistema gestisce l'oggetto, la custodia, il match, la verifica e la restituzione.
- **Modello legale:** l'intermediario diventa il ritrovatore (art. 927 c.c.), per cessione esplicita del finder al deposito. Entro 7 giorni può restituire direttamente l'oggetto al proprietario (art. 927, primo comma); altrimenti lo consegna alla PA, e da quel momento partono la pubblicazione (art. 928) e il termine di un anno (art. 929).
- **Restituzione entro i 7 giorni:** è il ritrovatore che rende la cosa; l'oggetto non è mai stato "ricevuto dal servizio", quindi le tariffe della Delib. 421/2023 **non si applicano** e la restituzione è gratuita per il searcher.
- **Restituzione dopo il trasferimento:** il searcher paga **solo la PA**, secondo le tariffe vigenti.
- **Premio (art. 930):** spetta al ritrovatore, cioè all'intermediario, solo se lo chiede. Il sistema registra la richiesta e la mostra al searcher prima del ritiro; il pagamento del premio resta fuori dal sistema.
- **Oggetti non reclamati (art. 929):** dopo un anno vengono offerti all'intermediario come ritrovatore; è un incentivo ad aderire, insieme al premio.
- Il rimborso all'intermediario è oggetto della convenzione con la PA, fuori dal pagamento del searcher.
- La tariffa è configurata dalla PA (giorni di custodia, eventuali diritti). Il sistema calcola il totale, mostra il dettaglio, gestisce il pagamento (provider astratto; test mode in demo) e produce la ricevuta.
- L'intermediario ha un accordo con la PA; non incassa dal searcher.

---

## 10. Privacy e sicurezza

- Registro privato; nessun catalogo pubblico di foto o dettagli unici.
- L'agente rivela solo ciò che serve al ritiro, dopo la verifica.
- Campi dei documenti (nome, cognome, data di nascita, numero) solo nel profilo privato; confronto con CIE/SPID; mai esposizione pubblica.
- Accesso per ruolo; l'intermediario vede il minimo; la PA vede il report completo.
- Audit su ogni accesso e cambio di stato.
- Retention allineata ai termini; foto cancellate dopo restituzione o smaltimento.
- Resistenza a prompt injection, false claims, tentativi multipli, rate limit, lockout, review umana.
- Open data solo aggregato e anonimo, se richiesto.

---

## 11. Perché è diverso

- **Dal PDF:** ricerca conversazionale, dati strutturati, verifica, custodia tracciata.
- **Dai gruppi Facebook:** identità, responsabilità, catena di custodia, niente leak.
- **Dai lost & found universitari:** rete di punti, registro unico, agente, PA.
- **Dai tracker commerciali:** non serve un dispositivo sull'oggetto; funziona su oggetti trovati da altri.
- **Dal catalogo con chatbot:** il catalogo è privato; il prodotto è la fiducia, non la vetrina.

---

## 12. Cosa rende il sistema “funzionante”

Non un mago di Oz per la presentazione. Il sistema deve avere:

- persistenza reale (database, migrazioni, seed);
- autenticazione e ruoli (finder, intermediario, searcher, istituzione, admin);
- API e UI per le quattro interfacce;
- upload foto e storage;
- ricerca strutturata + vettoriale;
- agente con tool calling e stato della conversazione;
- d3 o fallback deterministico, con report delle probabilità;
- QR firmato, monouso, con validazione;
- pagamento PA (test mode) e ricevuta;
- notifiche (match, ritiro, lista quotidiana);
- audit log e gestione errori;
- test end-to-end e dati di demo realistici.

La demo è un traguardo, non il prodotto.

---

## 13. Rischi e domande aperte

- **d3 hosting:** 3090 insufficiente per BF16; quantizzazione o API/GPU 48GB+. Da decidere.
- **Searcher senza SPID:** chat guest, claim con documento al custode. Da confermare.
- **Catalogo della sfida:** l'agente è il catalogo; una vista catalogo minimale nella chat è la safety net.
- **Adozione PA:** la denuncia e la pubblicazione restano loro; il sistema deve essere adottabile come registro operativo.
- **Accordi intermediari:** convenzione con la PA; senza accordo, il punto non è autorizzato. La convenzione deve fissare il termine di 7 giorni (l'art. 927 dice solo "senza ritardo") e il modello di cessione della qualità di ritrovatore.
- **Cessione dei diritti del finder:** da validare con un legale che la cessione al deposito sia efficace per premio (art. 930) e acquisto dopo un anno (art. 929).
- **Sensibili:** ritiro PA entro 24h; disponibilità operativa.
- **Pagamento:** test mode in demo; provider reale dopo.
- **Dati personali:** titolare, retention, audit; da validare.
- **Tempo:** quattro interfacce, un codebase; la roadmap deve essere realistica.

---

## 14. Roadmap

- **Fase 0 — MVP demo:** quattro interfacce, registro, agente, d3/fallback, QR, fee PA, seed, video.
- **Fase 1 — Pilota:** Sapienza + pochi bar, auth reale, notifiche, audit, test end-to-end.
- **Fase 2 — PA:** intake ufficiale, lista ritiri, review, pagamento, report.
- **Fase 3 — Scala:** multi-città, match proattivo, open data, accordi, hardening.

---

## 15. Conclusione

L'idea è forte perché non compete con il catalogo: lo sostituisce nel flusso operativo e lascia alla PA la responsabilità legale. Il moat è la catena di fiducia: identità, custodia, verifica, approvazione, pass, audit. Il prossimo passo è il design doc e il piano di build.
