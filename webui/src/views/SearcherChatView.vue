<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  ApiError,
  chat,
  createPass,
  getClaim,
  user,
} from "../api";
import CandidateCard from "../components/CandidateCard.vue";
import ChatMessage from "../components/ChatMessage.vue";
import {
  claimStatusLabels,
  confidenceBandLabels,
  returnChannelLabels,
} from "../format";
import { savePass } from "../passStore";
import type {
  ChatCandidate,
  ChatMessage as ChatTurn,
  ClaimOwner,
} from "../types";

const router = useRouter();
const route = useRoute();

const messages = ref<ChatTurn[]>([]);
const candidates = ref<ChatCandidate[]>([]);
const claim = ref<ClaimOwner | null>(null);
const input = ref("");
const sending = ref(false);
const generating = ref(false);
const done = ref(false);
const error = ref("");
const notice = ref("");
const lookupId = ref("");
const lookupBusy = ref(false);
const transcript = ref<HTMLElement | null>(null);

const claimTone = computed(() => {
  switch (claim.value?.status) {
    case "approved":
      return "bg-emerald-50 text-emerald-800 ring-emerald-200";
    case "rejected":
      return "bg-rose-50 text-rose-800 ring-rose-200";
    case "more_info":
      return "bg-amber-50 text-amber-800 ring-amber-200";
    default:
      return "bg-sky-50 text-sky-800 ring-sky-200";
  }
});

const canGeneratePass = computed(() => claim.value?.status === "approved");

const emptyConversation = computed(
  () => messages.value.length === 0 && !sending.value,
);

onMounted(() => {
  const queryClaim = typeof route.query.claim === "string" ? route.query.claim : "";
  if (queryClaim.trim()) {
    lookupId.value = queryClaim.trim();
    void loadClaim(lookupId.value);
  }
});

async function scrollToBottom() {
  await nextTick();
  if (transcript.value) {
    transcript.value.scrollTop = transcript.value.scrollHeight;
  }
}

async function send() {
  const text = input.value.trim();
  if (!text || sending.value || done.value) {
    return;
  }
  error.value = "";
  notice.value = "";

  const turn: ChatTurn = { role: "user", content: text };
  messages.value = [...messages.value, turn];
  input.value = "";
  sending.value = true;
  void scrollToBottom();

  try {
    const reply = await chat(messages.value);
    messages.value = [...messages.value, { role: "assistant", content: reply.message }];
    // Always replace the candidate list: an empty reply means the new search
    // found nothing, so stale cards from a previous turn must disappear.
    candidates.value = reply.candidates;
    if (reply.claimId) {
      await loadClaim(reply.claimId, true);
      notice.value = claim.value
        ? `Richiesta ${reply.claimId} registrata.`
        : `Richiesta ${reply.claimId} registrata: non riesco a leggerne lo stato.`;
    } else if (reply.done) {
      notice.value = "La conversazione è stata inoltrata a un operatore.";
    }
    done.value = reply.done;
  } catch (cause) {
    // The turn was not recorded server-side when the request failed: put the
    // text back so the searcher can retry without duplicating a bubble.
    messages.value = messages.value.filter((message) => message !== turn);
    input.value = text;
    if (cause instanceof ApiError && cause.status === 429) {
      error.value =
        "Hai inviato troppi messaggi in poco tempo. Attendi un minuto e riprova.";
    } else if (cause instanceof ApiError && cause.status === 400) {
      error.value =
        "La conversazione è troppo lunga. Avvia una nuova ricerca per continuare.";
      done.value = true;
    } else if (cause instanceof ApiError) {
      error.value = cause.message;
    } else {
      error.value = "Non riesco a contattare l'assistente. Riprova.";
    }
  } finally {
    sending.value = false;
    void scrollToBottom();
  }
}

async function loadClaim(claimId: string, quiet = false) {
  const id = claimId.trim();
  if (!id) {
    error.value = "Inserisci il codice della richiesta.";
    return;
  }
  if (!quiet) {
    lookupBusy.value = true;
  }
  error.value = "";
  try {
    const found = await getClaim(id);
    claim.value = found;
    lookupId.value = found.id;
    done.value = true;
    // Keep the claim id in the URL so a reload re-fetches it. This is what
    // makes the pass button survive the institution approval step: the searcher
    // can refresh this page instead of losing the claim card.
    if (route.query.claim !== found.id) {
      await router.replace({ query: { ...route.query, claim: found.id } });
    }
  } catch (cause) {
    claim.value = null;
    if (cause instanceof ApiError && cause.status === 404) {
      error.value = "Richiesta non trovata. Controlla il codice e riprova.";
    } else if (cause instanceof ApiError && cause.status === 403) {
      error.value = "Questa richiesta appartiene a un altro profilo.";
    } else if (!quiet) {
      error.value =
        cause instanceof ApiError
          ? cause.message
          : "Non riesco a leggere la richiesta. Riprova.";
    }
  } finally {
    lookupBusy.value = false;
    void scrollToBottom();
  }
}

async function generatePass() {
  if (!claim.value || !canGeneratePass.value || generating.value) {
    return;
  }
  generating.value = true;
  error.value = "";
  try {
    const owner = user.value?.id;
    if (!owner) {
      error.value = "La sessione non è più valida. Accedi di nuovo per generare il pass.";
      return;
    }
    const pass = await createPass(claim.value.id);
    if (!savePass(pass, owner)) {
      error.value =
        "Non riesco a conservare il pass in questa sessione. Verifica le impostazioni del browser e riprova.";
      return;
    }
    await router.push({ name: "searcher-pass", params: { id: pass.passId } });
  } catch (cause) {
    error.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a generare il pass. Riprova.";
  } finally {
    generating.value = false;
  }
}

function resetConversation() {
  messages.value = [];
  candidates.value = [];
  claim.value = null;
  input.value = "";
  done.value = false;
  error.value = "";
  notice.value = "";
  lookupId.value = "";
  if (route.query.claim) {
    const query = { ...route.query };
    delete query.claim;
    void router.replace({ query });
  }
  void scrollToBottom();
}
</script>

<template>
  <div>
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p class="eyebrow">Chi cerca</p>
        <h1 class="page-title mt-1">Ritrova il tuo oggetto</h1>
        <p class="page-subtitle">
          Racconta cosa hai perso: l'assistente cerca tra gli oggetti ritrovati e
          ti guida nella verifica, una domanda alla volta.
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-if="messages.length > 0 || claim"
          type="button"
          class="btn btn-secondary"
          @click="resetConversation"
        >
          Nuova ricerca
        </button>
      </div>
    </div>

    <div class="mt-4 space-y-2">
      <p v-if="notice" class="banner-success" role="status">{{ notice }}</p>
      <p v-if="error" class="banner-error" role="alert">{{ error }}</p>
    </div>

    <div class="mt-5 grid gap-4 lg:grid-cols-[1.25fr_1fr]">
      <section class="card flex min-h-[28rem] flex-col p-0">
        <div class="border-b border-stone-100 px-4 py-3 sm:px-5">
          <div class="flex items-center justify-between gap-3">
            <h2 class="section-title">Conversazione</h2>
            <span class="text-xs text-stone-500">
              {{ done ? "Conclusa" : sending ? "L'assistente sta scrivendo…" : "Una domanda alla volta" }}
            </span>
          </div>
        </div>

        <div
          ref="transcript"
          class="flex-1 space-y-4 overflow-y-auto px-4 py-4 sm:px-5"
          aria-live="polite"
        >
          <div v-if="emptyConversation" class="space-y-3">
            <p class="text-sm leading-6 text-stone-700">
              Ciao! Sono l'assistente del registro oggetti smarriti. Descrivi
              l'oggetto che hai perso: tipo, colore e la zona in cui pensi di
              averlo perso.
            </p>
            <div class="flex flex-wrap gap-2">
              <button
                type="button"
                class="badge bg-stone-100 text-stone-700 ring-stone-200 hover:bg-stone-200"
                @click="input = 'Ho perso un telefono nero in zona San Lorenzo'"
              >
                Ho perso un telefono
              </button>
              <button
                type="button"
                class="badge bg-stone-100 text-stone-700 ring-stone-200 hover:bg-stone-200"
                @click="input = 'Ho perso un portafoglio marrone in centro'"
              >
                Ho perso un portafoglio
              </button>
              <button
                type="button"
                class="badge bg-stone-100 text-stone-700 ring-stone-200 hover:bg-stone-200"
                @click="input = 'Ho perso un ombrello blu'"
              >
                Ho perso un ombrello
              </button>
            </div>
          </div>

          <ChatMessage
            v-for="(message, index) in messages"
            :key="`${index}-${message.role}`"
            :role="message.role"
            :content="message.content"
          />

          <div v-if="sending" class="flex items-center gap-2 text-xs text-stone-500">
            <span class="h-4 w-4 animate-spin rounded-full border-2 border-emerald-600 border-t-transparent"></span>
            Cerco tra gli oggetti ritrovati…
          </div>
        </div>

        <div class="border-t border-stone-100 px-4 py-3 sm:px-5">
          <div v-if="done" class="flex flex-wrap items-center justify-between gap-2">
            <p class="text-xs text-stone-600">
              <template v-if="claim">
                La richiesta è stata registrata. Puoi aprire il pass dalla scheda
                a destra quando è approvata.
              </template>
              <template v-else>
                La conversazione è conclusa. Avvia una nuova ricerca per
                ricominciare.
              </template>
            </p>
            <button type="button" class="btn btn-secondary px-3 py-2" @click="resetConversation">
              Nuova ricerca
            </button>
          </div>
          <form v-else class="flex items-end gap-2" @submit.prevent="send">
            <label class="sr-only" for="chat-input">Messaggio</label>
            <textarea
              id="chat-input"
              v-model="input"
              rows="2"
              maxlength="800"
              class="input max-h-32 min-h-11 resize-y py-2.5"
              placeholder="Scrivi un messaggio… (Invio per inviare)"
              :disabled="sending"
              @keydown.enter.exact.prevent="send"
            ></textarea>
            <button
              type="submit"
              class="btn btn-primary shrink-0 px-3 py-2.5"
              :disabled="sending || !input.trim()"
            >
              <span v-if="sending" class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
              <span v-else>Invia</span>
              <span class="sr-only">Invia il messaggio</span>
            </button>
          </form>
          <p class="mt-1.5 text-[11px] text-stone-500">
            Non inserire dati sensibili non richiesti: la verifica usa solo
            dettagli dell'oggetto.
          </p>
        </div>
      </section>

      <div class="space-y-4">
        <section v-if="claim" class="card">
          <div class="flex items-start justify-between gap-3">
            <div>
              <h2 class="section-title">La tua richiesta</h2>
              <p class="mt-0.5 font-mono text-xs text-stone-500">{{ claim.id }}</p>
            </div>
            <span class="badge shrink-0" :class="claimTone">
              {{ claimStatusLabels[claim.status] ?? claim.status }}
            </span>
          </div>

          <dl class="mt-4 grid grid-cols-2 gap-3 text-sm">
            <div>
              <dt class="data-label">Confidenza</dt>
              <dd class="data-value">
                {{ confidenceBandLabels[claim.confidenceBand] ?? claim.confidenceBand }}
              </dd>
            </div>
            <div>
              <dt class="data-label">Ritiro</dt>
              <dd class="data-value text-xs">
                {{ returnChannelLabels[claim.returnChannel] ?? claim.returnChannel }}
              </dd>
            </div>
          </dl>

          <div v-if="claim.status === 'approved'" class="mt-4">
            <button
              type="button"
              class="btn btn-primary w-full"
              :disabled="generating"
              @click="generatePass"
            >
              <span
                v-if="generating"
                class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"
                aria-hidden="true"
              ></span>
              {{ generating ? "Genero il pass…" : "Genera pass di ritiro" }}
            </button>
            <p class="mt-2 text-[11px] leading-4 text-stone-500">
              Il codice QR viene mostrato una sola volta: aprirlo da questa
              sessione. Non condividerlo.
            </p>
          </div>

          <div
            v-else
            class="mt-4 rounded-xl bg-stone-50 px-3 py-2 text-xs leading-5 text-stone-600"
          >
            <template v-if="claim.status === 'rejected'">
              La richiesta non è stata approvata. Se pensi ci sia un errore,
              contatta il punto di raccolta.
            </template>
            <template v-else-if="claim.status === 'more_info'">
              La verifica richiede altre informazioni. Contatta il punto di
              raccolta o la pubblica amministrazione per completare la pratica.
            </template>
            <template v-else>
              La richiesta è in verifica. Il pass comparirà qui dopo
              l'approvazione del custode.
            </template>
          </div>
        </section>

        <section v-if="candidates.length > 0" class="card">
          <div class="flex items-baseline justify-between gap-3">
            <div>
              <h2 class="section-title">Oggetti compatibili</h2>
              <p class="mt-0.5 text-xs text-stone-500">
                Solo categoria, zona e data: nessun dettaglio privato.
              </p>
            </div>
            <span class="text-xs text-stone-500">{{ candidates.length }}</span>
          </div>
          <div class="mt-3 space-y-2.5">
            <CandidateCard
              v-for="(candidate, index) in candidates"
              :key="candidate.itemId"
              :candidate="candidate"
              :index="index"
            />
          </div>
        </section>

        <details class="card" :open="!claim && Boolean(route.query.claim)">
          <summary class="cursor-pointer text-sm font-semibold text-stone-900">
            Riprendi una richiesta esistente
          </summary>
          <p class="mt-1 text-xs leading-5 text-stone-500">
            Inserisci il codice della richiesta (per esempio quella creata in una
            sessione precedente) per vedere stato e generare il pass.
          </p>
          <form class="mt-3 flex gap-2" @submit.prevent="loadClaim(lookupId)">
            <label class="sr-only" for="claim-id">Codice richiesta</label>
            <input
              id="claim-id"
              v-model="lookupId"
              class="input font-mono text-xs"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="claim-…"
              :disabled="lookupBusy"
            />
            <button
              type="submit"
              class="btn btn-secondary shrink-0 px-3 py-2"
              :disabled="lookupBusy || !lookupId.trim()"
            >
              {{ lookupBusy ? "Apro…" : "Apri" }}
            </button>
          </form>
        </details>
      </div>
    </div>
  </div>
</template>
