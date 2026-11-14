<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { ApiError, getClaimSummary, getItem, getTimeline, reviewClaim } from "../api";
import StatusBadge from "../components/StatusBadge.vue";
import Timeline from "../components/Timeline.vue";
import {
  claimStatusLabels,
  conditionLabels,
  confidenceBandLabels,
  displayItemName,
  formatDateTime,
  returnChannelLabels,
  riskLabels,
} from "../format";
import type { ClaimSummary, Item, TimelineEvent } from "../types";

const route = useRoute();
const item = ref<Item | null>(null);
const events = ref<TimelineEvent[]>([]);
const loading = ref(true);
const error = ref("");

const claim = ref<ClaimSummary | null>(null);
const claimIDInput = ref("");
const claimError = ref("");
const claimBusy = ref(false);

const canReview = computed(
  () =>
    claim.value !== null &&
    ["submitted", "under_review", "more_info"].includes(claim.value.status),
);

onMounted(() => {
  const query = route.query.claim;
  if (typeof query === "string" && query.trim() !== "") {
    claimIDInput.value = query.trim();
    void loadClaim();
  }
  void load();
});

async function load() {
  loading.value = true;
  error.value = "";
  const id = String(route.params.id ?? "");
  try {
    const [fetchedItem, timeline] = await Promise.all([
      getItem(id),
      getTimeline(id),
    ]);
    item.value = fetchedItem;
    events.value = timeline.events;
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

async function loadClaim() {
  const id = claimIDInput.value.trim();
  if (id === "") {
    claim.value = null;
    claimError.value = "";
    return;
  }
  claimBusy.value = true;
  claimError.value = "";
  try {
    claim.value = await getClaimSummary(id);
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
          : "Non riesco a caricare la richiesta. Riprova.";
    }
  } finally {
    claimBusy.value = false;
  }
}

async function review(decision: "approved" | "rejected") {
  if (!claim.value) {
    return;
  }
  claimBusy.value = true;
  claimError.value = "";
  try {
    claim.value = await reviewClaim(claim.value.id, decision);
  } catch (cause) {
    claimError.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a registrare la decisione. Riprova.";
  } finally {
    claimBusy.value = false;
  }
}
</script>

<template>
  <div class="mx-auto w-full max-w-4xl">
    <RouterLink to="/intermediary" class="link text-sm">← Dashboard</RouterLink>

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
          <p class="eyebrow">Dettaglio oggetto</p>
          <h1 class="page-title mt-1">{{ displayItemName(item) }}</h1>
          <p class="mt-1 font-mono text-sm text-stone-600">{{ item.publicCode }}</p>
        </div>
        <StatusBadge :status="item.status" :sensitive="item.sensitive" />
      </header>

      <div
        v-if="item.sensitive"
        class="mt-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900"
      >
        <span class="font-semibold">Oggetto sensibile.</span> Termine di custodia
        di 24 ore. Non condividere foto o dettagli con persone non autorizzate.
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
              <dd class="data-value">{{ item.category || "—" }}</dd>
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
              <dt class="data-label">Ritrovato il</dt>
              <dd class="data-value">{{ formatDateTime(item.foundAt) }}</dd>
            </div>
            <div class="col-span-2">
              <dt class="data-label">Note contestuali</dt>
              <dd class="data-value">{{ item.foundNotes || "—" }}</dd>
            </div>
            <div>
              <dt class="data-label">Registrato il</dt>
              <dd class="data-value">{{ formatDateTime(item.createdAt) }}</dd>
            </div>
            <div>
              <dt class="data-label">Custode</dt>
              <dd class="data-value">{{ item.custodianId || "—" }}</dd>
            </div>
            <div>
              <dt class="data-label">Finder legale</dt>
              <dd class="data-value">{{ item.legalFinderId || "—" }}</dd>
            </div>
            <div>
              <dt class="data-label">Termine custodia</dt>
              <dd class="data-value">
                <template v-if="item.dueAt">
                  {{ formatDateTime(item.dueAt) }}
                  <span v-if="item.daysLeft !== undefined" class="text-stone-500">
                    · {{ item.daysLeft }} {{ item.daysLeft === 1 ? "giorno" : "giorni" }}
                  </span>
                </template>
                <span v-else>Termine non disponibile</span>
              </dd>
            </div>
            <div>
              <dt class="data-label">Rischio</dt>
              <dd class="data-value">
                {{ riskLabels[item.riskTier ?? ""] ?? item.riskTier ?? "—" }}
              </dd>
            </div>
          </dl>
        </section>

        <section class="card">
          <h2 class="section-title">Cronologia</h2>
          <p class="mt-1 text-xs text-stone-500">
            Eventi di custodia senza dati privati o coordinate esatte.
          </p>
          <div class="mt-4">
            <Timeline :events="events" />
          </div>
        </section>
      </div>

      <section class="mt-5 card">
        <div class="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 class="section-title">Richiesta di verifica</h2>
            <p class="mt-1 text-xs text-stone-500">
              Apri una richiesta per id. Vedi stato, fascia di confidenza, prove e
              segnali: il report completo resta alla PA.
            </p>
          </div>
          <div class="flex gap-2">
            <input
              v-model="claimIDInput"
              type="text"
              class="input w-56 font-mono text-xs"
              placeholder="ID richiesta"
              aria-label="ID richiesta"
              @keyup.enter="loadClaim"
            />
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="claimBusy"
              @click="loadClaim"
            >
              Apri
            </button>
          </div>
        </div>

        <p v-if="claimError" class="mt-3 text-sm text-rose-700">{{ claimError }}</p>

        <div v-if="claim" class="mt-4 rounded-xl border border-stone-200 p-4">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p class="data-label">Stato</p>
              <p class="data-value">
                {{ claimStatusLabels[claim.status] ?? claim.status }}
              </p>
            </div>
            <div>
              <p class="data-label">Confidenza</p>
              <p class="data-value">
                {{ confidenceBandLabels[claim.confidenceBand] ?? claim.confidenceBand }}
              </p>
            </div>
            <div>
              <p class="data-label">Canale</p>
              <p class="data-value">
                {{ returnChannelLabels[claim.returnChannel] ?? claim.returnChannel }}
              </p>
            </div>
          </div>

          <div class="mt-3 grid gap-3 sm:grid-cols-2">
            <div>
              <p class="data-label">Prove</p>
              <p v-if="claim.evidence.length === 0" class="data-value text-stone-500">
                Nessuna prova allegata
              </p>
              <ul v-else class="mt-1 flex flex-wrap gap-1.5">
                <li
                  v-for="entry in claim.evidence"
                  :key="entry"
                  class="rounded-full bg-stone-100 px-2.5 py-0.5 text-xs text-stone-700"
                >
                  {{ entry }}
                </li>
              </ul>
            </div>
            <div>
              <p class="data-label">Segnali</p>
              <p v-if="claim.flags.length === 0" class="data-value text-stone-500">
                Nessun segnale
              </p>
              <ul v-else class="mt-1 flex flex-wrap gap-1.5">
                <li
                  v-for="flag in claim.flags"
                  :key="flag"
                  class="rounded-full bg-amber-50 px-2.5 py-0.5 text-xs text-amber-800 ring-1 ring-amber-200"
                >
                  {{ flag }}
                </li>
              </ul>
            </div>
          </div>

          <div v-if="canReview" class="mt-4 flex flex-wrap gap-2">
            <button
              type="button"
              class="btn btn-primary"
              :disabled="claimBusy"
              @click="review('approved')"
            >
              Approva
            </button>
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="claimBusy"
              @click="review('rejected')"
            >
              Rifiuta
            </button>
          </div>
        </div>
      </section>

      <div class="mt-5 flex flex-wrap gap-2">
        <RouterLink to="/intermediary" class="btn btn-secondary">
          Torna alla dashboard
        </RouterLink>
        <RouterLink to="/intermediary/scan" class="btn btn-primary">
          Scansiona un pass
        </RouterLink>
      </div>
    </template>
  </div>
</template>
