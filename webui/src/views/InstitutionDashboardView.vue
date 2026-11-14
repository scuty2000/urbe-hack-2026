<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import {
  ApiError,
  listInstitutionPickups,
  transferToInstitution,
} from "../api";
import StatusBadge from "../components/StatusBadge.vue";
import {
  displayItemName,
  formatDateTime,
  organisationLabel,
} from "../format";
import type {
  InstitutionPickupGroup,
  InstitutionPickupItem,
  InstitutionPickupList,
  ItemStatus,
} from "../types";

const pickups = ref<InstitutionPickupList | null>(null);
const loading = ref(true);
const error = ref("");
const notice = ref("");
const actionError = ref("");
const transferringId = ref<string | null>(null);
const confirmId = ref<string | null>(null);

const groups = computed<InstitutionPickupGroup[]>(() => pickups.value?.groups ?? []);
const total = computed(() => pickups.value?.total ?? 0);
const sensitiveCount = computed(() =>
  groups.value.reduce(
    (count, group) =>
      count + group.items.filter((item) => item.sensitive).length,
    0,
  ),
);
const overdueCount = computed(() =>
  groups.value.reduce(
    (count, group) => count + group.items.filter((item) => item.daysOverdue > 0).length,
    0,
  ),
);

onMounted(() => {
  void load();
});

async function load(showSpinner = true) {
  if (showSpinner) {
    loading.value = true;
  }
  error.value = "";
  try {
    pickups.value = await listInstitutionPickups();
  } catch (cause) {
    error.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a caricare la coda di ritiro. Riprova.";
  } finally {
    loading.value = false;
  }
}

async function transfer(item: InstitutionPickupItem) {
  if (transferringId.value) {
    return;
  }
  transferringId.value = item.id;
  actionError.value = "";
  notice.value = "";
  try {
    const transferred = await transferToInstitution(item.id);
    notice.value = `${transferred.publicCode} è ora in custodia della pubblica amministrazione.`;
    confirmId.value = null;
    await load(false);
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 409) {
      actionError.value =
        "L'oggetto non è più in uno stato trasferibile. Aggiorna la coda.";
      await load(false);
    } else {
      actionError.value =
        cause instanceof ApiError
          ? cause.message
          : "Non riesco a trasferire l'oggetto. Riprova.";
    }
  } finally {
    transferringId.value = null;
  }
}
</script>

<template>
  <div>
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p class="eyebrow">Pubblica amministrazione</p>
        <h1 class="page-title mt-1">Ritiri dai punti di raccolta</h1>
        <p class="page-subtitle">
          Oggetti con il termine di custodia scaduto, pronti per il trasferimento
          alla PA. I casi sensibili sono in cima.
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="loading"
          @click="load()"
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            class="h-4 w-4"
            :class="{ 'animate-spin': loading }"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M16.023 9.348h4.992V4.356m-4.992 4.992 3.181-3.182a8.25 8.25 0 0 0-14.24 2.9M2.985 19.644v-4.992h4.992m-4.992 0 3.181 3.182a8.25 8.25 0 0 0 14.24-2.9"
            />
          </svg>
          Aggiorna
        </button>
        <RouterLink to="/institution/scan" class="btn btn-secondary">
          Scansiona un pass
        </RouterLink>
        <RouterLink to="/institution/intake" class="btn btn-primary">
          + Intake ufficiale
        </RouterLink>
      </div>
    </div>

    <div class="mt-4 space-y-2">
      <p v-if="notice" class="banner-success" role="status">{{ notice }}</p>
      <p v-if="error" class="banner-error" role="alert">{{ error }}</p>
      <p v-if="actionError" class="banner-error" role="alert">{{ actionError }}</p>
    </div>

    <dl class="mt-5 grid grid-cols-3 gap-2.5 sm:gap-4">
      <div class="card-tight">
        <dt class="data-label">Da ritirare</dt>
        <dd class="mt-1 font-display text-3xl leading-none text-ink">{{ total }}</dd>
      </div>
      <div class="card-tight">
        <dt class="data-label">Sensibili</dt>
        <dd class="mt-1 font-display text-3xl leading-none text-rust">{{ sensitiveCount }}</dd>
      </div>
      <div class="card-tight">
        <dt class="data-label">In ritardo</dt>
        <dd class="mt-1 font-display text-3xl leading-none text-ochre">{{ overdueCount }}</dd>
      </div>
    </dl>

    <div v-if="loading" class="mt-6 space-y-3">
      <div v-for="index in 2" :key="index" class="card animate-pulse">
        <div class="h-4 w-1/3 rounded bg-paper-deep"></div>
        <div class="mt-3 h-3 w-2/3 rounded bg-paper-deep"></div>
        <div class="mt-2 h-3 w-1/2 rounded bg-paper-deep"></div>
      </div>
    </div>

    <div v-else-if="groups.length === 0" class="empty-state mt-6">
      <p class="text-sm font-medium text-ink-soft">Nessun ritiro in attesa</p>
      <p class="mt-1 text-xs text-ink-muted">
        Quando scade il termine di custodia di un oggetto, comparirà qui.
      </p>
    </div>

    <div v-else class="mt-6 space-y-4">
      <section
        v-for="group in groups"
        :key="`${group.intermediaryId}-${group.zone}`"
        class="card"
        :class="group.sensitive ? 'border-rust/30' : ''"
      >
        <div class="flex flex-wrap items-start justify-between gap-2">
          <div>
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="section-title">{{ organisationLabel(group.intermediaryId) }}</h2>
              <span
                v-if="group.sensitive"
                class="badge bg-rust-soft text-rust ring-rust/30"
              >
                Sensibile
              </span>
            </div>
            <p class="mt-0.5 text-xs text-ink-muted">
              {{ group.zone || "Zona non indicata" }} · {{ group.count }}
              {{ group.count === 1 ? "oggetto" : "oggetti" }}
            </p>
          </div>
          <p class="font-mono text-[11px] text-ink-muted">{{ group.intermediaryId }}</p>
        </div>

        <div class="mt-3 space-y-2">
          <article
            v-for="item in group.items"
            :key="item.id"
            class="rounded-md border border-line bg-paper-card px-3 py-3"
            :class="item.sensitive ? 'border-rust/30 bg-rust-soft/30' : ''"
          >
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <p class="truncate text-sm font-semibold text-ink">
                    {{ displayItemName(item) }}
                  </p>
                  <span class="font-mono text-[11px] text-ink-muted">{{ item.publicCode }}</span>
                </div>
                <div class="mt-1 flex flex-wrap items-center gap-1.5">
                  <StatusBadge
                    :status="item.status as ItemStatus"
                    :sensitive="item.sensitive"
                  />
                  <span
                    v-if="item.daysOverdue > 0"
                    class="badge bg-ochre-soft text-ochre ring-ochre/30"
                  >
                    {{ item.daysOverdue }}
                    {{ item.daysOverdue === 1 ? "giorno" : "giorni" }} di ritardo
                  </span>
                  <span v-else class="text-xs text-ink-muted">In scadenza oggi</span>
                </div>
                <p class="mt-1 text-xs text-ink-soft">
                  Termine: {{ formatDateTime(item.dueAt) }} ·
                  {{ item.foundZone || "zona non indicata" }}
                </p>
              </div>

              <div class="flex shrink-0 flex-wrap items-center gap-2">
                <RouterLink
                  :to="`/institution/items/${item.id}`"
                  class="btn btn-ghost px-3 py-2 text-xs"
                >
                  Dettagli
                </RouterLink>
                <template v-if="confirmId === item.id">
                  <button
                    type="button"
                    class="btn btn-primary px-3 py-2 text-xs"
                    :disabled="transferringId === item.id"
                    @click="transfer(item)"
                  >
                    {{ transferringId === item.id ? "Trasferisco…" : "Conferma" }}
                  </button>
                  <button
                    type="button"
                    class="btn btn-ghost px-2 py-2 text-xs"
                    :disabled="transferringId === item.id"
                    @click="confirmId = null"
                  >
                    Annulla
                  </button>
                </template>
                <button
                  v-else
                  type="button"
                  class="btn btn-secondary px-3 py-2 text-xs"
                  :disabled="transferringId !== null"
                  @click="confirmId = item.id"
                >
                  Trasferisci alla PA
                </button>
              </div>
            </div>
          </article>
        </div>
      </section>
    </div>
  </div>
</template>
