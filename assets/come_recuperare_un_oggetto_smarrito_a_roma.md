# CATALOGO FONTI — Ritirare un oggetto smarrito (Roma Capitale)

Data dello scraping e della verifica: 13/11/2026

## 1. Fonti consultate

| ID | Fonte | Tipo | Origine | Stato |
|---|---|---|---|---|
| F1 | Scheda servizio "Ritirare un oggetto smarrito" | Pagina ufficiale Comune | comune.roma.it, contentId=INF39645 (copia locale `leggi/ritirare un oggetto smarrito.html`) | ✔ letta |
| F2 | Scheda ufficio "Reparto Oggetti Rinvenuti" | Pagina ufficiale Comune | comune.roma.it (copia locale `leggi/Reparto Oggetti Rinvenuti.html`) | ✔ letta |
| F3 | "Servizio Riscossione Reversali" | Pagina ufficiale Comune | comune.roma.it (copia locale `leggi/servizio riscossione reversali.html`) | ✔ letta |
| F4 | Mod. 1 Rinuncia | Modulo ufficiale | `NEWModulo_1_oggetti_rinvenuti_rinuncia.pdf` | ✔ letto |
| F5 | Mod. 2 Delega al ritiro tramite corriere | Modulo ufficiale | `NEWModulo_2_oggetti_rinvenuti_corriere.pdf` | ✔ letto |
| F6 | Mod. 3 Delega al ritiro | Modulo ufficiale | `Modulo_3_oggetti_rinvenuti_delega_ritiro.pdf` | ✔ letto |
| F7 | Informativa Privacy della Polizia Locale | Documento ufficiale | `RH20240162525_Informativa__Polizia_Locale.pdf` | ✔ letta |
| F8 | Elenco reperti dall'01/01/2026 all'11/11/2026 (510 pagine, generato il 12/11/2026) | Avviso ex art. 928 c.c. | `Rinvenimenti_01012026_11112026.pdf` | ✔ letto (vedi nota sul testo) |
| F9 | Elenco reperti dall'01/01/2025 al 31/12/2025 | Avviso ex art. 928 c.c. | `Rinvenimenti_01012025_31122025.pdf` | ✔ ha lo stesso formato di F8 |
| F10 | Codice Civile (R.D. 16/03/1942 n. 262), artt. 927–933 | Normativa | Normattiva, testo vigente al 13/11/2026 (aggiornato al 23/10/2026) | ✔ scaricato dall'endpoint `caricaArticolo` |
| F11 | Delib. della Giunta Capitolina n. 421 del 15/12/2023 (tariffe 2024) | Normativa comunale | `Deliberazione_Giunta_Capitolina_n._421_del_15_dicembre_2023.pdf` (89 pagine) | ✔ letta (Allegato A, sezione Polizia Locale) |

> Nota su `leggi/decreto1942-03-16;262.html`: la copia salvata contiene **solo l'indice** del Codice Civile, perché Normattiva carica il testo degli articoli via AJAX. Gli articoli sono stati riscaricati a parte (F10).

> Nota su F8 e F9: i PDF usano font Type3 con una codifica rotta, quindi il testo estratto è illeggibile e non si può cercare. Si leggono solo come immagine o con OCR.

---

## 2. Dati estratti, divisi per fonte

### F1 — Scheda servizio
- Il servizio è regolato dagli **artt. 927 e seguenti del Codice Civile**.
- Si restituisce l'oggetto "previa verifica della proprietà e presentazione dei documenti richiesti".
- Per i cittadini stranieri: "l'elenco viene comunicato alle **Ambasciate e/o rappresentanze consolari** del paese di appartenenza, che sono **esenti dal pagamento dei diritti di custodia**".
- "I **documenti d'identità o di riconoscimento** sono restituiti **senza alcun costo**."
- Prima di usare il servizio bisogna leggere l'Informativa Privacy.
- Per ritirare l'oggetto servono:
  - un documento d'identità valido;
  - un'**eventuale** denuncia di furto o smarrimento con la descrizione dettagliata dell'oggetto ("non necessaria per oggetti senza valore o privi di interesse commerciale");
  - gli elementi e i documenti utili a individuare il bene e a dimostrarne la proprietà.
- Le spese sono fissate dalla **Delib. G.C. n. 421 del 15/12/2023**.
- Ritiro tramite delegato: servono il documento del delegato, l'**originale della delega**, la copia del documento del delegante e l'eventuale copia della denuncia.
- Costi:
  - custodia **€ 4,60**;
  - istruttoria e ricerca **€ 5,80**;
  - deposito **€ 17,40**, se si ritira **oltre 30 giorni dall'acquisizione**.
- Rinuncia: bisogna inviare una dichiarazione di rinuncia.
- Consegna a domicilio: si possono ricevere gli oggetti a casa pagando prima i diritti. Le spese di spedizione sono a carico dell'interessato.
- Pagamento: con la reversale generata dall'applicativo **SIREWEB** "in uso al Reparto".
- Elenchi: pubblicati periodicamente all'Albo Pretorio e nel box Allegati della scheda, dove restano **per un anno solare**.

### F2 — Reparto Oggetti Rinvenuti
- Sede: Circonvallazione Ostiense 191, 00154 Roma.
- Orari:
  - lun, mar, mer, ven 8:30–13:00;
  - gio 8:30–17:00 (orario continuato);
  - dal 25/06 al 10/09: lun–ven 8:30–13:00.
- Telefono: 06 6769 3220 / 3268 / 3214.
- Email: oggettismarriti@comune.roma.it
- PEC: protocollo.polizialocale.com@pec.comune.roma.it

### F3 — Reversali e PagoPA
- Il pagamento richiede l'IUV.
- Online con il pulsante "Paga", anche per utenti non registrati: carta di credito, addebito in conto, MyBank.
- Home banking: tramite CBILL.
- Ufficio postale:
  - con il bollettino premarcato;
  - se il bollettino è perso o rovinato, con un **bollettino bianco PA** compilato così: CF dell'Ente 02438750586, CF del contribuente, IUV, importo, c/c **20046033**.
- Prestatori aderenti a PagoPA (Sisal, Lottomatica, banche, GDO): si usa il lato sinistro dell'avviso.
- "Attualmente non è possibile effettuare **bonifici SEPA** né utilizzare bollettini bianchi diversi da quelli PA."
- L'importo segue la delibera annuale delle tariffe per i servizi a domanda individuale.

### F4, F5, F6 — Moduli
- **Mod. 1 Rinuncia**:
  - si invia per email oppure per PEC;
  - campi: dati anagrafici, telefono, n. di repertorio, data di acquisizione, eventuale dichiarazione;
  - allegato: copia del documento.
- **Mod. 2 Corriere**:
  - si invia per email oppure per PEC;
  - campi: dati anagrafici, telefono, n. di repertorio, data di acquisizione, domicilio, nome del corriere, data in cui il corriere ritira presso "i vostri locali di Roma **Via Prospero Alpino n. 63/A**";
  - allegati: copia del documento e **copia della ricevuta di versamento dei diritti**.
- **Mod. 3 Delega**:
  - campi: proprietario, n. reperto, nominativo del delegato;
  - il delegato porta la copia del documento del proprietario;
  - firmano sia il proprietario sia il delegato.

### F7 — Informativa Privacy
- Il titolare del trattamento è Roma Capitale. Il DPO risponde a dpo@comune.roma.it.
- §5: "Il conferimento dei suoi dati personali è **obbligatorio** […] Il loro mancato conferimento comporta la **mancata erogazione del servizio**."

### F8 e F9 — Elenchi dei reperti
- Intestazione: Polizia Roma Capitale, Comando Generale, U.O. Servizi Amministrativi e Affari Generali, Sezione Procedimenti C.d.S. e Oggetti Rinvenuti, Reparto Oggetti Rinvenuti.
- Titolo: "AVVISO — Ai sensi dell'Art. 928 del C.C. …"
- Colonne: **REPERTO | DESCRIZIONE | DATA ACQUISIZIONE**. I numeri dei documenti sono parzialmente oscurati (es. "Nr: 5813\* \* \* \*").
- Il footer riporta **entrambi** gli indirizzi: Circonvallazione Ostiense 191 **e** Via Prospero Alpino 63a, 00154 Roma. Il tel. +39 06 67693214 conferma lo stesso complesso.

### F11 — Delib. G.C. 421/2023, Allegato A, "Diritti di custodia per oggetti rinvenuti"

| Voce | Tariffa 2023 | Tariffa 2024 | Riferimento |
|---|---|---|---|
| A) Diritto fisso (anche per i reperti il cui proprietario è subito identificabile o rintracciabile) | € 4,00 | **€ 4,60** | artt. 927 e segg. c.c.; Delib. G.C. 643 del 30/12/1914 |
| B) Diritti di istruttoria e ricerca ("per i reperti ritirati entro 30 giorni dal ricevimento da parte del servizio") | € 5,00 | **€ 5,80** | idem |
| C) Deposito ("per i reperti oltre 30 giorni dal ricevimento da parte del servizio") | € 15,00 | **€ 17,40** | idem |
| Recapito a domicilio su richiesta, a cura e spese dell'interessato | — | **A+B e/o A+B+C** | — |
| Documenti d'identità o di riconoscimento | — | "senza alcun onere" | — |

- La voce B dice "entro 30 giorni", ma la formula del recapito (A+B+C) e la scheda F1 trattano C come **aggiuntiva**. Il totale oltre 30 giorni è quindi € 27,80.
- "Diritti di custodia" è il **titolo dell'intera sezione** (A+B+C). Per questo l'esenzione di Ambasciate e Consolati citata in F1 è ambigua: potrebbe valere solo per A oppure per tutte le voci.
- La delibera **non** cita l'esenzione consolare.
- Le tariffe valgono per l'**anno 2024**. A novembre 2026 F1 cita ancora questa delibera.
- Le prime pagine (il verbale di seduta) hanno il testo codificato male. L'Allegato A invece si legge.

### F10 — Codice Civile (testo vigente)
- **927**: chi trova una cosa mobile la restituisce al proprietario. Se non lo conosce, la consegna "senza ritardo al **podestà** del luogo in cui l'ha trovata", indicando le circostanze del ritrovamento.
- **928**: la consegna si pubblica all'albo pretorio "per due domeniche successive" e resta affissa "per tre giorni ogni volta".
- **929**: trascorso un anno dall'ultimo giorno di pubblicazione senza che il proprietario si presenti, la cosa (o il suo prezzo) appartiene a chi l'ha trovata. Sia il proprietario sia il ritrovatore pagano le spese occorse.
- **930**: il premio spetta al ritrovatore **se lo richiede**. È pari a 1/10 della somma o del prezzo; per la parte che eccede **10.000 lire** è solo 1/20. Se la cosa non ha valore commerciale, il premio lo fissa il giudice.
- **931**: il possessore e il detentore sono equiparati al proprietario, "secondo le circostanze".
- **932 e 933**: riguardano il tesoro e i rigetti del mare. **Non** interessano questo flusso.

---

## 3. VERIFICA di `aglritmo burocratico.md`

Legenda:
- ✔ confermato da una fonte
- ⚠ impreciso o va precisato
- ❓ non presente nelle fonti ufficiali consultate (inferenza o conoscenza esterna)
- ✘ errato o incoerente

### 3.1 Costanti e tariffe (§0)

| Affermazione | Esito | Fonte / nota |
|---|---|---|
| Ufficio: Polizia Locale, Comando Generale, Reparto Oggetti Rinvenuti | ✔ | F8 (intestazione) |
| Sportello: Circonvallazione Ostiense 191 | ✔ | F2 |
| "MAGAZZINO" in Via Prospero Alpino 63/A | ⚠ | F5 parla di "vostri locali" e F8 lo riporta come secondo indirizzo. L'etichetta "magazzino" è un'inferenza. Probabilmente è lo stesso complesso con un secondo accesso, ma le fonti non lo dicono. |
| Orari | ✔ | F2 |
| Telefono, email e PEC | ✔ | F2, F4, F5, F8 |
| Base legale: artt. 927–931 c.c. | ✔ | F1 dice "927 e seguenti"; gli artt. 932–933 non sono pertinenti |
| Tariffe: Delib. G.C. 421 del 15/12/2023 | ✔ | F11, Allegato A (tariffe 2024) |
| Importi € 4,60 / € 5,80 / € 17,40 | ✔ | F1 |
| Totali € 10,40 / € 27,80 | ✔ | Il calcolo è corretto. |
| Documenti d'identità: € 0 | ✔ | F1 |
| "Rappresentanze consolari: esenti dai diritti di custodia" | ⚠ | F1 dice "Ambasciate **e/o** rappresentanze consolari": vanno aggiunte le Ambasciate. |
| Pagamento con reversale SIREWEB tramite PagoPA (IUV) | ✔ | F1, F3 |
| Moduli 1, 2, 3 | ✔ | F4–F6 |

### 3.2 Flusso (§1–§2)

| Passo | Esito | Nota |
|---|---|---|
| **A. Pre-check sui gestori** (treni, mezzi pubblici, taxi, aeroporti, enti privati) e la frase "può tenere l'oggetto senza mai consegnarlo al Comune" | ❓ | Nessuna delle fonti consultate ne parla. Può essere un consiglio sensato, ma va marcato come **non ufficiale** oppure supportato da una fonte (es. le pagine lost & found di ATAC, Trenitalia, ADR). |
| A4: se l'oggetto è perso fuori da Roma è competente il **sindaco** del luogo | ⚠ | Il testo vigente dell'art. 927 dice "**podestà**". Leggerlo come "sindaco" è l'interpretazione corrente, ma conviene esplicitarlo. Attenzione anche al criterio: conta il **luogo del ritrovamento**, non quello dello smarrimento. Un oggetto perso a Roma e trovato fuori Roma finisce in un altro comune, e il flusso non lo gestisce. |
| B1: denuncia per "documento **o** oggetto di valore" | ⚠ | F1 esenta solo gli oggetti "senza valore o privi di interesse commerciale". Il caso "documento" non è previsto da F1 come motivo autonomo. Anche "Polizia o Carabinieri" non è citato in F1 (che parla genericamente di denuncia). |
| B4: prove di proprietà | ✔ | F1 ("elementi e documenti utili per l'individuazione esatta del bene e per l'effettiva proprietà") |
| C1: elenchi su Albo e Allegati, validi un anno solare | ✔ | F1 |
| Formato REPERTO / DESCRIZIONE / DATA ACQUISIZIONE, numeri oscurati | ✔ | F8 |
| "Il testo dei PDF non si può cercare" | ✔ | Verificato: la codifica dei font è rotta. |
| C3: chiamare o scrivere al Reparto | ⚠ | I contatti sono ufficiali, ma F1 non descrive una procedura di ricerca su richiesta. Anche "consegna giorni o settimane dopo" è ❓. |
| **T0/TEND: timeout di 1 anno sul ramo "non trovato"** | ✘ | **Errore logico.** Il termine dell'art. 929 decorre dall'ultima pubblicazione **di quel reperto**. Se l'oggetto non compare negli elenchi non è mai stato consegnato o pubblicato, quindi il termine non può partire. Il timeout va spostato **dopo C4**: se il reperto è in elenco e il termine è già scaduto, il proprietario perde il diritto. Ora il flusso lascia ritirare un reperto scaduto e applica il timeout a un oggetto mai pubblicato. |
| D: deposito "oltre 30 giorni dall'acquisizione" | ✔ | F1 |
| D2/D3 Consolato: nel diagramma porta direttamente al costo, nel testo restano istruttoria e deposito | ✘ | **Incoerenza interna** tra diagramma e testo. Nessuna delle due letture è certa: in F11 "Diritti di custodia" è il titolo dell'intera sezione A+B+C. **Corretto** marcando l'ambito come "da confermare con il Reparto". |
| D: "se manca poco al 30° giorno conviene ritirare subito" | ✔ | Deriva correttamente dalle tariffe. |

### 3.3 Rami E, P, S, T (§3–§6)

| Affermazione | Esito | Nota |
|---|---|---|
| E1: Informativa obbligatoria, senza dati il servizio non viene erogato | ✔ | F7 §5 |
| E1/V1: verifica di identità e proprietà | ✔ | F1 |
| E1: il Reparto genera la reversale | ✔ | F1 ("applicativo … in uso al Reparto") |
| E1 punto 4 "mostra la ricevuta" e punto 5 "**firma del verbale di restituzione**" | ❓ | Sono plausibili ma non compaiono in nessuna fonte. |
| E2: delega in originale, documento del delegato, copia del documento del delegante, eventuale denuncia | ✔ | F1, F6 |
| E2: delega "firmata dal proprietario e dal delegato, con n. reperto" | ✔ | F6 |
| E3: diritti pagati prima della spedizione e ricevuta allegata | ✔ | F1, F5 |
| E3: campi del Mod. 2, invio per email o PEC, ritiro in Via Prospero Alpino 63/A | ✔ | F5 |
| E3 punto 1: "chiedi al Reparto di generare la reversale" | ⚠ | È un'inferenza (e il documento la segnala già tra parentesi). |
| E4: campi del Mod. 1, allegato, invio per email o PEC | ✔ | F4 |
| E4: "nessun costo dovuto" | ❓ | Nessuna fonte lo dice: F1 e F4 tacciono sui costi in caso di rinuncia. |
| Ramo P (P1–P4, c/c 20046033, CF 02438750586, niente SEPA, niente bollettini non-PA) | ✔ | F3 |
| S1: elenco comunicato a Ambasciata o Consolato | ✔ | F1 |
| S2: documenti a costo 0, con il corriere si paga solo la spedizione | ✔ | F1 (deduzione corretta) |
| S2: nota sul duplicato non più valido | ❓ | Viene da conoscenza esterna, non dalle fonti consultate. |
| S3: premio "10% fino a € 5,16 + 5% sull'eccedenza" | ✔ | Art. 930: 10.000 lire / 1936,27 = **€ 5,16**, conversione corretta. Conviene dire che il testo vigente è ancora in **lire** e che l'importo in euro è una conversione. |
| S3: premio "**se lo chiede**", senza valore commerciale lo fissa il giudice | ✔ | Art. 930 |
| S3: chi riprende la cosa paga le spese | ✔ | Art. 929 c. 2 (vale anche per il ritrovatore) |
| S4: possessore e detentore equiparati | ✔ | Art. 931 ("secondo le circostanze") |
| S4: "servono prove del titolo (contratto, lettera)" | ❓ | È un'inferenza ragionevole, ma non è nelle fonti. |
| T: 1 anno dall'ultima pubblicazione, poi la cosa o il prezzo va al ritrovatore, che paga le spese | ✔ | Art. 929 |
| T: "se il ritrovatore non la reclama segue il **Regolamento comunale sugli oggetti rinvenuti**" | ❓ | Nessun regolamento capitolino del genere è citato in F1–F3. Va trovato e citato, oppure rimosso. |
| §7 tabella degli articoli | ✔ | F10 (vedi la nota "podestà" e la nota "lire") |

### 3.4 Correzioni applicate ad `aglritmo burocratico.md` (13/11/2026)

1. Il timeout dell'art. 929 è stato spostato dopo C4: ora vale solo per un reperto pubblicato.
2. L'esenzione di Ambasciate e Consolati è indicata come ambigua (F11), nel diagramma e nel testo.
3. Aggiunte le Ambasciate; "podestà" = sindaco; competenza secondo il luogo del ritrovamento.
4. La condizione per la denuncia non dipende più dall'essere un "documento".
5. "MAGAZZINO" è diventato "SECONDO INDIRIZZO".
6. Tariffe A, B, C con i nomi e le condizioni della delibera; nota che sono le tariffe 2024.
7. Rimossi perché senza fonte: "firma del verbale", "nessun costo" per la rinuncia, "Regolamento comunale". Marcati **[consiglio]**: il pre-check sui gestori, la richiesta su chiamata, la nota sul duplicato, le prove del titolo.
8. Aggiunti i dati dell'elenco 2026 (reperti 1–6940, 510 pagine, ordine crescente) e la nota "lire → € 5,16".

### 3.5 Riepilogo

- **Corretto nella sostanza**: costanti, tariffe, moduli, canali, pagamenti e norme del c.c.
- **2 errori da correggere** (✘):
  1. Il timeout dell'art. 929 è attaccato al ramo "oggetto non trovato": va spostato dopo C4 (reperto trovato ma già scaduto).
  2. L'esenzione del consolato nel diagramma (D3) non è coerente con il testo (§2.D): manca il costo residuo.
- **Da precisare** (⚠):
  - aggiungere le Ambasciate oltre ai Consolati;
  - "podestà" → sindaco;
  - la competenza dipende dal luogo del **ritrovamento**;
  - la condizione per la denuncia non dipende dall'essere un "documento";
  - l'etichetta "magazzino".
  - (La Delib. 421/2023 è stata poi verificata, vedi F11.)
- **Stato attuale**: tutte le correzioni sono applicate (§3.4).
- **Senza fonte ufficiale** (❓), da marcare come "consiglio" o da documentare:
  - pre-check sui gestori dei trasporti;
  - firma del verbale;
  - costo nullo della rinuncia;
  - nota sul duplicato;
  - prove del titolo per il detentore;
  - regolamento comunale citato nel ramo T.
