<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink, useRoute } from "vue-router";
import {
  ApiError,
  collectFromFinder,
  disposeItem,
  getInstitutionClaim,
  getItem,
  getItemFee,
  getRewardNotice,
  getTimeline,
  offerToFinder,
  reviewInstitutionClaim,
  transferToInstitution,
} from "../api";
import FeeSummary from "../components/FeeSummary.vue";
import RewardNoticePanel from "../components/RewardNotice.vue";
import StatusBadge from "../components/StatusBadge.vue";
import Timeline from "../components/Timeline.vue";
import VerificationReport from "../components/VerificationReport.vue";
import {
  categoryLabel,
  claimStatusLabels,
  confidenceBandLabels,
  conditionLabels,
  displayItemName,
  formatDateTime,
  organisationLabel,
  returnChannelLabels,
  riskLabels,
} from "../format";
import type {
  ClaimReport,
  Fee,
  Item,
  RewardNotice,
  TimelineEvent,
} from "../types";

const route = useRoute();

const item = ref<Item | null>(null);
const events = ref<TimelineEvent[]>([]);
const fee = ref<Fee | null>(null);
const reward = ref<RewardNotice | null>(null);
const loading = ref(true);
const error = ref("");
const notice = ref("");
const actionError = ref("");
const actionBusy = ref<null | "transfer" | "offer" | "collect" | "dispose">(null);
const confirmAction = ref<null | "offer" | "collect" | "dispose">(null);

const feeLoading = ref(false);
const feeError = ref("");
const rewardLoading = ref(false);
const rewardError = ref("");

const claimId = ref("");
const claim = ref<ClaimReport | null>(null);
const claimLoading = ref(false);
const claimError = ref("");
const reviewBusy = ref<null | "approved" | "rejected">(null);
const confirmDecision = ref<null | "approved" | "rejected">(null);

const itemId = computed(() => String(route.params.id ?? ""));

const status = computed(() => item.value?.status ?? "enrolled");
const canTransfer = computed(() =>
  status.value === "pickup_due" || status.value === "sensitive_pickup",
);
const canOffer = computed(
  () =>
    status.value === "transferred" ||
    status.value === "unclaimed" ||
    (status.value === "in_custody" && item.value?.custodyMode === "institution"),
);
const canCollect = computed(() => status.value === "offered_to_finder");
const canDispose = computed(() => status.value === "offered_to_finder");
const claimReviewable = computed(
  () =>
    !!claim.value &&
    ["submitted", "under_review", "more_info"].includes(claim.value.status),
);

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

onMounted(() => {
  void load();
  const queryClaim = typeof route.query.claim === "string" ? route.query.claim : "";
  if (queryClaim.trim()) {
    claimId.value = queryClaim.trim();
    void loadClaim();
  }
});

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [fetchedItem, timeline] = await Promise.all([
      getItem(itemId.value),
      getTimeline(itemId.value),
    ]);
    item.value = fetchedItem;
    events.value = timeline.events;
    void loadFee(fetchedItem.id);
    void loadReward(fetchedItem.id);
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 404) {
      error.value = "Oggetto non trovato.";
    } else if (cause instanceof ApiError && cause.status === 403) {
      error.value = "Non hai i permessi per vedere questo oggetto.";
    } else {
      error.value =
        cause instanceof ApiError
          ? cause.message
          : "Non riesco a caricare l'oggetto. Riprova.";
    }
  } finally {
    loading.value = false;
  }
}

async function refreshTimeline() {
  try {
    const timeline = await getTimeline(itemId.value);
    events.value = timeline.events;
  } catch {
    // The action already succeeded; a stale timeline is not worth an error.
  }
}

async function loadFee(id: string) {
  feeLoading.value = true;
  feeError.value = "";
  try {
    fee.value = await getItemFee(id);
  } catch (cause) {
    fee.value = null;
    feeError.value =
      cause instanceof ApiError ? cause.message : "Tariffa non disponibile.";
  } finally {
    feeLoading.value = false;
  }
}

async function loadReward(id: string) {
  rewardLoading.value = true;
  rewardError.value = "";
  try {
    reward.value = await getRewardNotice(id);
  } catch (cause) {
    reward.value = null;
    if (!(cause instanceof ApiError && cause.status === 404)) {
      rewardError.value =
        cause instanceof ApiError
          ? cause.message
          : "Ricompensa non disponibile.";
    }
  } finally {
    rewardLoading.value = false;
  }
}

async function runAction(kind: "transfer" | "offer" | "collect" | "dispose") {
  if (!item.value || actionBusy.value) {
    return;
  }
  actionBusy.value = kind;
  actionError.value = "";
  notice.value = "";
  confirmAction.value = null;
  const handlers = {
    transfer: transferToInstitution,
    offer: offerToFinder,
    collect: collectFromFinder,
    dispose: disposeItem,
  } as const;
  try {
    const updated = await handlers[kind](item.value.id);
    item.value = updated;
    const messages: Record<typeof kind, string> = {
      transfer: `${updated.publicCode} è ora in custodia della PA.`,
      offer: `${updated.publicCode} è stato offerto al finder legale.`,
      collect: `Ritiro del finder registrato per ${updated.publicCode}.`,
      dispose: `${updated.publicCode} è stato smaltito.`,
    };
    notice.value = messages[kind];
    await refreshTimeline();
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 400) {
      actionError.value =
        kind === "offer"
          ? "Il termine legale di un anno non è ancora trascorso: l'offerta al finder non è consentita."
          : cause.message;
    } else if (cause instanceof ApiError && cause.status === 409) {
      actionError.value =
        "L'oggetto non è in uno stato valido per questa azione. Aggiorna la scheda.";
    } else {
      actionError.value =
        cause instanceof ApiError
          ? cause.message
          : "Non riesco a completare l'azione. Riprova.";
    }
  } finally {
    actionBusy.value = null;
  }
}

async function loadClaim() {
  const id = claimId.value.trim();
  if (!id) {
    claimError.value = "Inserisci il codice della richiesta.";
    return;
  }
  claimLoading.value = true;
  claimError.value = "";
  confirmDecision.value = null;
  try {
    claim.value = await getInstitutionClaim(id);
    claimId.value = claim.value.id;
  } catch (cause) {
    claim.value = null;
    if (cause instanceof ApiError && cause.status === 404) {
      claimError.value = "Richiesta non trovata.";
    } else if (cause instanceof ApiError && cause.status === 403) {
      claimError.value = "Non hai i permessi per questa richiesta.";
    } else {
      claimError.value =
        cause instanceof ApiError
          ? cause.message
          : "Non riesco a leggere la richiesta. Riprova.";
    }
  } finally {
    claimLoading.value = false;
  }
}

async function review(decision: "approved" | "rejected") {
  if (!claim.value || reviewBusy.value) {
    return;
  }
  reviewBusy.value = decision;
  claimError.value = "";
  notice.value = "";
  confirmDecision.value = null;
  try {
    claim.value = await reviewInstitutionClaim(claim.value.id, decision);
    notice.value =
      decision === "approved"
        ? "Richiesta approvata: il searcher può generare il pass."
        : "Richiesta rifiutata.";
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 409) {
      claimError.value =
        "La richiesta non è più modificabile: potrebbe essere già stata decisa.";
      await loadClaim();
    } else {
      claimError.value =
        cause instanceof ApiError
          ? cause.message
          : "Non riesco a registrare la decisione. Riprova.";
    }
  } finally {
    reviewBusy.value = null;
  }
}
</script>

<template>
  <div class="mx-auto w-full max-w-5xl">
    <RouterLink to="/institution" class="link text-sm">← Coda di ritiro</RouterLink>

    <div v-if="loading" class="mt-4 card animate-pulse">
      <div class="h-6 w-1/2 rounded bg-stone-200"></div>
      <div class="mt-3 h-4 w-1/3 rounded bg-stone-200"></div>
      <div class="mt-6 h-32 rounded-xl bg-stone-100"></div>
    </div>

    <div v-else-if="error" class="empty-state mt-4">
      <p class="text-sm font-medium text-stone-700">{{ error }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="load">Riprova</button>
    </div>

    <template v-else-if="item">
      <header class="mt-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <p class="eyebrow">Scheda della PA</p>
          <h1 class="page-title mt-1">{{ displayItemName(item) }}</h1>
          <p class="mt-1 font-mono text-sm text-stone-600">{{ item.publicCode }}</p>
        </div>
        <StatusBadge :status="item.status" :sensitive="item.sensitive" />
      </header>

      <div class="mt-4 space-y-2">
        <p v-if="notice" class="banner-success" role="status">{{ notice }}</p>
        <p v-if="error" class="banner-error" role="alert">{{ error }}</p>
        <p v-if="actionError" class="banner-error" role="alert">{{ actionError }}</p>
      </div>

      <div
        v-if="item.sensitive"
        class="mt-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900"
      >
        <span class="font-semibold">Oggetto sensibile.</span> Limita l'accesso ai
        soli operatori autorizzati e non condividere foto o dettagli.
      </div>

      <section v-if="item.photos.length > 0" class="mt-5">
        <h2 class="section-title">Foto</h2>
        <div class="mt-2 grid grid-cols-2 gap-2.5 sm:grid-cols-3">
          <a
            v-for="(photo, index) in item.photos"
            :key="photo"
            :href="photo"
            target="_blank"
            rel="noopener"
            class="group relative aspect-square overflow-hidden rounded-xl border border-stone-200 bg-stone-100"
          >
            <img
              :src="photo"
              :alt="`Foto ${index + 1} di ${displayItemName(item)}`"
              class="h-full w-full object-cover transition group-hover:scale-[1.02]"
              loading="lazy"
            />
          </a>
        </div>
      </section>

      <div class="mt-5 grid gap-4 lg:grid-cols-[1.2fr_1fr]">
        <section class="card">
          <h2 class="section-title">Scheda</h2>
          <dl class="mt-4 grid grid-cols-2 gap-4 text-sm">
            <div>
              <dt class="data-label">Categoria</dt>
              <dd class="data-value">{{ categoryLabel(item.category) }}</dd>
            </div>
            <div>
              <dt class="data-label">Marca / modello</dt>
              <dd class="data-value">
                {{ [item.brand, item.model].filter(Boolean).join(" ") || "—" }}
              </dd>
            </div>
            <div>
              <dt class="data-label">Colore</dt>
              <dd class="data-value">{{ item.color || "—" }}</dd>
            </div>
            <div>
              <dt class="data-label">Condizioni</dt>
              <dd class="data-value">
                {{ conditionLabels[item.condition ?? ""] ?? item.condition ?? "—" }}
              </dd>
            </div>
            <div>
              <dt class="data-label">Zona</dt>
              <dd class="data-value">{{ item.foundZone || "—" }}</dd>
            </div>
            <div>
              <dt class="data-label">Ritrovato / consegnato il</dt>
              <dd class="data-value">{{ formatDateTime(item.foundAt) }}</dd>
            </div>
            <div class="col-span-2">
              <dt class="data-label">Note contestuali</dt>
              <dd class="data-value">{{ item.foundNotes || "—" }}</dd>
            </div>
            <div>
              <dt class="data-label">Custode</dt>
              <dd class="data-value">{{ organisationLabel(item.custodianId) }}</dd>
            </div>
            <div>
              <dt class="data-label">Finder legale</dt>
              <dd class="data-value">{{ organisationLabel(item.legalFinderId) }}</dd>
            </div>
            <div>
              <dt class="data-label">Trasferito il</dt>
              <dd class="data-value">{{ formatDateTime(item.transferredAt) }}</dd>
            </div>
            <div>
              <dt class="data-label">Termine legale dal</dt>
              <dd class="data-value">{{ formatDateTime(item.legalTermStart) }}</dd>
            </div>
            <div>
              <dt class="data-label">Rischio</dt>
              <dd class="data-value">
                {{ riskLabels[item.riskTier ?? ""] ?? item.riskTier ?? "—" }}
              </dd>
            </div>
            <div>
              <dt class="data-label">Registrato il</dt>
              <dd class="data-value">{{ formatDateTime(item.createdAt) }}</dd>
            </div>
          </dl>
        </section>

        <div class="space-y-4">
          <FeeSummary :fee="fee" :loading="feeLoading" :error="feeError" />
          <RewardNoticePanel
            :notice="reward"
            :loading="rewardLoading"
            :error="rewardError"
          />
        </div>
      </div>

      <section class="card mt-4">
        <h2 class="section-title">Azioni sull'oggetto</h2>
        <p class="mt-1 text-xs text-stone-500">
          Le azioni dipendono dallo stato attuale e sono registrate nella
          cronologia e nel registro di audit.
        </p>

        <div class="mt-3 flex flex-wrap gap-2">
          <template v-if="canTransfer">
            <button
              type="button"
              class="btn btn-primary"
              :disabled="actionBusy !== null"
              @click="runAction('transfer')"
            >
              {{ actionBusy === "transfer" ? "Trasferisco…" : "Trasferisci alla PA" }}
            </button>
          </template>

          <template v-if="canOffer">
            <template v-if="confirmAction === 'offer'">
              <button
                type="button"
                class="btn btn-primary"
                :disabled="actionBusy !== null"
                @click="runAction('offer')"
              >
                {{ actionBusy === "offer" ? "Offro…" : "Conferma offerta al finder" }}
              </button>
              <button
                type="button"
                class="btn btn-ghost"
                :disabled="actionBusy !== null"
                @click="confirmAction = null"
              >
                Annulla
              </button>
            </template>
            <button
              v-else
              type="button"
              class="btn btn-secondary"
              :disabled="actionBusy !== null"
              @click="confirmAction = 'offer'"
            >
              Offri al finder legale
            </button>
          </template>

          <template v-if="canCollect">
            <button
              type="button"
              class="btn btn-primary"
              :disabled="actionBusy !== null"
              @click="runAction('collect')"
            >
              {{ actionBusy === "collect" ? "Registro…" : "Registra ritiro del finder" }}
            </button>
          </template>

          <template v-if="canDispose">
            <template v-if="confirmAction === 'dispose'">
              <button
                type="button"
                class="btn btn-danger"
                :disabled="actionBusy !== null"
                @click="runAction('dispose')"
              >
                {{ actionBusy === "dispose" ? "Smaltisco…" : "Conferma smaltimento" }}
              </button>
              <button
                type="button"
                class="btn btn-ghost"
                :disabled="actionBusy !== null"
                @click="confirmAction = null"
              >
                Annulla
              </button>
            </template>
            <button
              v-else
              type="button"
              class="btn btn-danger"
              :disabled="actionBusy !== null"
              @click="confirmAction = 'dispose'"
            >
              Smaltisci
            </button>
          </template>

          <p
            v-if="!canTransfer && !canOffer && !canCollect && !canDispose"
            class="text-sm text-stone-600"
          >
            Nessuna azione disponibile per lo stato corrente.
          </p>
        </div>

        <p v-if="canOffer" class="mt-2 text-[11px] leading-5 text-stone-500">
          L'offerta al finder è consentita solo dopo un anno dal termine legale;
          un tentativo anticipato viene rifiutato dal server.
        </p>
      </section>

      <section class="card mt-4">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 class="section-title">Verifica della richiesta</h2>
            <p class="mt-1 text-xs text-stone-500">
              Apri una richiesta per esaminare la relazione di verifica e
              registrare la decisione della PA.
            </p>
          </div>
        </div>

        <form class="mt-3 flex flex-col gap-2 sm:flex-row" @submit.prevent="loadClaim">
          <label class="sr-only" for="claim-id">Codice richiesta</label>
          <input
            id="claim-id"
            v-model="claimId"
            class="input font-mono text-xs"
            type="text"
            autocomplete="off"
            spellcheck="false"
            placeholder="claim-…"
            :disabled="claimLoading"
          />
          <button
            type="submit"
            class="btn btn-secondary shrink-0"
            :disabled="claimLoading || !claimId.trim()"
          >
            {{ claimLoading ? "Apro…" : "Apri richiesta" }}
          </button>
        </form>

        <p v-if="claimError" class="mt-3 text-sm text-rose-700">{{ claimError }}</p>

        <div v-if="claim" class="mt-4">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="font-mono text-xs text-stone-500">{{ claim.id }}</p>
              <p class="mt-1 text-xs text-stone-600">
                Creata il {{ formatDateTime(claim.createdAt) }} ·
                {{ returnChannelLabels[claim.returnChannel] ?? claim.returnChannel }}
              </p>
            </div>
            <span class="badge" :class="claimTone">
              {{ claimStatusLabels[claim.status] ?? claim.status }}
            </span>
          </div>

          <dl class="mt-3 grid grid-cols-2 gap-3 text-sm">
            <div>
              <dt class="data-label">Confidenza</dt>
              <dd class="data-value">
                {{ claim.report ? `${Math.round(claim.confidence * 100)}%` : "—" }}
              </dd>
            </div>
            <div>
              <dt class="data-label">Fascia</dt>
              <dd class="data-value">
                {{ confidenceBandLabels[claim.confidenceBand] ?? claim.confidenceBand }}
              </dd>
            </div>
          </dl>

          <div v-if="claimReviewable" class="mt-4 rounded-xl border border-stone-200 bg-stone-50 px-3 py-3">
            <p class="text-xs font-medium text-stone-700">
              Decisione della pubblica amministrazione
            </p>
            <div v-if="confirmDecision" class="mt-2 flex flex-wrap gap-2">
              <p class="text-xs text-stone-600">
                Confermi la decisione?
              </p>
              <button
                type="button"
                class="btn btn-primary px-3 py-2 text-xs"
                :disabled="reviewBusy !== null"
                @click="review(confirmDecision)"
              >
                {{ reviewBusy ? "Registro…" : "Conferma" }}
              </button>
              <button
                type="button"
                class="btn btn-ghost px-3 py-2 text-xs"
                :disabled="reviewBusy !== null"
                @click="confirmDecision = null"
              >
                Annulla
              </button>
            </div>
            <div v-else class="mt-2 flex flex-wrap gap-2">
              <button
                type="button"
                class="btn btn-primary px-3 py-2 text-xs"
                :disabled="reviewBusy !== null"
                @click="confirmDecision = 'approved'"
              >
                Approva richiesta
              </button>
              <button
                type="button"
                class="btn btn-danger px-3 py-2 text-xs"
                :disabled="reviewBusy !== null"
                @click="confirmDecision = 'rejected'"
              >
                Rifiuta richiesta
              </button>
            </div>
          </div>

          <div v-else class="mt-3 text-xs text-stone-500">
            La richiesta è {{ (claimStatusLabels[claim.status] ?? claim.status).toLowerCase() }}:
            non è più modificabile.
          </div>

          <p v-if="!claim.report" class="mt-3 text-xs text-stone-500">
            Nessuna relazione di verifica disponibile per questa richiesta.
          </p>
        </div>
      </section>

      <div v-if="claim && claim.report" class="mt-4">
        <VerificationReport :report="claim.report" />
      </div>

      <section class="card mt-4">
        <h2 class="section-title">Cronologia</h2>
        <p class="mt-1 text-xs text-stone-500">
          Eventi di custodia senza dati privati o coordinate esatte.
        </p>
        <div class="mt-4">
          <Timeline :events="events" />
        </div>
      </section>
    </template>
  </div>
</template>
