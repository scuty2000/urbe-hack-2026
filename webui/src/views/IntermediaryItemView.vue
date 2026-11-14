<script setup lang="ts">
import { onMounted, ref } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { ApiError, getItem, getTimeline } from "../api";
import StatusBadge from "../components/StatusBadge.vue";
import Timeline from "../components/Timeline.vue";
import {
  conditionLabels,
  displayItemName,
  formatDateTime,
} from "../format";
import type { Item, TimelineEvent } from "../types";

const route = useRoute();
const item = ref<Item | null>(null);
const events = ref<TimelineEvent[]>([]);
const loading = ref(true);
const error = ref("");

onMounted(() => {
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
              <dt class="data-label">Rischio</dt>
              <dd class="data-value capitalize">{{ item.riskTier || "—" }}</dd>
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
