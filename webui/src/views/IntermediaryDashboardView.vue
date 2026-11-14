<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { acceptCustody, ApiError, listMyItems, refuseCustody } from "../api";
import StatusBadge from "../components/StatusBadge.vue";
import { displayItemName, formatDateTime } from "../format";
import type { Item } from "../types";

const items = ref<Item[]>([]);
const loading = ref(true);
const error = ref("");
const notice = ref("");
const actionError = ref("");
const busyId = ref<string | null>(null);
const refusingId = ref<string | null>(null);
const refuseReason = ref("");
const now = ref(Date.now());

let clock: number | undefined;

const pending = computed(() =>
  items.value.filter((item) => item.status === "pending_custody"),
);
const inCustody = computed(() =>
  items.value.filter(
    (item) =>
      item.status === "in_custody" ||
      item.status === "pickup_due" ||
      item.status === "sensitive_pickup",
  ),
);
const sensitiveCount = computed(
  () => items.value.filter((item) => item.sensitive).length,
);
const missingDeadlines = computed(() =>
  inCustody.value.some((item) => !item.dueAt),
);

onMounted(() => {
  void loadItems();
  clock = window.setInterval(() => {
    now.value = Date.now();
  }, 60_000);
});

onBeforeUnmount(() => {
  if (clock) {
    window.clearInterval(clock);
  }
});

async function loadItems(showSpinner = true) {
  if (showSpinner) {
    loading.value = true;
  }
  error.value = "";
  try {
    items.value = await listMyItems();
  } catch (cause) {
    error.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a caricare gli oggetti. Riprova.";
  } finally {
    loading.value = false;
  }
}

function daysLeft(item: Item): number | null {
  if (!item.dueAt) {
    return item.daysLeft ?? null;
  }
  // The deadline comes from the server; the clock only keeps the display fresh
  // between dashboard refreshes. The custody policy is never duplicated here.
  return Math.max(
    0,
    Math.ceil((new Date(item.dueAt).getTime() - now.value) / 86_400_000),
  );
}

function deadlineLabel(item: Item): string {
  return item.dueAt ? formatDateTime(item.dueAt) : "Termine non disponibile";
}

function remainingLabel(item: Item): string {
  const days = daysLeft(item);
  if (days === null) {
    return "";
  }
  if (days === 0) {
    return "Scaduto";
  }
  return `${days} ${days === 1 ? "giorno" : "giorni"}`;
}

async function accept(item: Item) {
  busyId.value = item.id;
  actionError.value = "";
  notice.value = "";
  try {
    const summary = await acceptCustody(item.id);
    now.value = Date.now();
    item.status = summary.status;
    item.dueAt = summary.dueAt;
    item.daysLeft = summary.daysLeft;
    notice.value = `Custodia accettata per ${item.publicCode}. Termine: ${formatDateTime(
      summary.dueAt,
    )}.`;
    await loadItems(false);
  } catch (cause) {
    actionError.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco ad accettare la custodia. Riprova.";
  } finally {
    busyId.value = null;
  }
}

function openRefuse(item: Item) {
  refusingId.value = item.id;
  refuseReason.value = "";
  actionError.value = "";
}

function closeRefuse() {
  refusingId.value = null;
  refuseReason.value = "";
}

async function confirmRefuse(item: Item) {
  const reason = refuseReason.value.trim();
  if (!reason) {
    actionError.value = "Indica il motivo del rifiuto.";
    return;
  }
  busyId.value = item.id;
  actionError.value = "";
  notice.value = "";
  try {
    await refuseCustody(item.id, reason);
    notice.value = `Custodia rifiutata per ${item.publicCode}. L'oggetto è stato segnalato.`;
    closeRefuse();
    await loadItems(false);
  } catch (cause) {
    actionError.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a rifiutare la custodia. Riprova.";
  } finally {
    busyId.value = null;
  }
}
</script>

<template>
  <div>
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p class="eyebrow">Intermediario</p>
        <h1 class="page-title mt-1">Dashboard custodia</h1>
        <p class="page-subtitle">
          Accetta gli oggetti in arrivo, tieni sotto controllo i termini e
          registra ciò che ricevi al banco.
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="loading"
          @click="loadItems()"
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
        <RouterLink to="/intermediary/enroll" class="btn btn-primary">
          + Registra oggetto
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
        <dt class="data-label">In attesa</dt>
        <dd class="mt-1 text-2xl font-semibold text-amber-700">{{ pending.length }}</dd>
      </div>
      <div class="card-tight">
        <dt class="data-label">In custodia</dt>
        <dd class="mt-1 text-2xl font-semibold text-emerald-700">{{ inCustody.length }}</dd>
      </div>
      <div class="card-tight">
        <dt class="data-label">Sensibili</dt>
        <dd class="mt-1 text-2xl font-semibold text-rose-700">{{ sensitiveCount }}</dd>
      </div>
    </dl>

    <section class="mt-7">
      <div class="flex items-baseline justify-between gap-3">
        <h2 class="section-title">Da accettare</h2>
        <span class="text-xs text-stone-500">{{ pending.length }} oggetti</span>
      </div>

      <div v-if="loading" class="mt-3 grid gap-3 sm:grid-cols-2">
        <div v-for="index in 2" :key="index" class="card-tight animate-pulse">
          <div class="flex gap-3">
            <div class="h-16 w-16 rounded-lg bg-stone-200"></div>
            <div class="flex-1 space-y-2">
              <div class="h-3 w-2/3 rounded bg-stone-200"></div>
              <div class="h-3 w-1/3 rounded bg-stone-200"></div>
              <div class="h-3 w-1/2 rounded bg-stone-200"></div>
            </div>
          </div>
        </div>
      </div>

      <div v-else-if="pending.length === 0" class="empty-state mt-3">
        <p class="text-sm font-medium text-stone-700">Nessun oggetto in attesa</p>
        <p class="mt-1 text-xs text-stone-500">
          Le nuove segnalazioni dei finder compariranno qui.
        </p>
      </div>

      <div v-else class="mt-3 grid gap-3 sm:grid-cols-2">
        <article v-for="item in pending" :key="item.id" class="card-tight">
          <div class="flex gap-3">
            <div class="h-16 w-16 shrink-0 overflow-hidden rounded-lg bg-stone-100">
              <img
                v-if="item.photos.length"
                :src="item.photos[0]"
                :alt="displayItemName(item)"
                class="h-full w-full object-cover"
                loading="lazy"
              />
              <span
                v-else
                class="flex h-full w-full items-center justify-center text-xs text-stone-600"
                >Nessuna foto</span
              >
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-start justify-between gap-2">
                <p class="truncate text-sm font-semibold text-stone-900">
                  {{ displayItemName(item) }}
                </p>
                <span class="shrink-0 font-mono text-[11px] text-stone-500">
                  {{ item.publicCode }}
                </span>
              </div>
              <p class="mt-0.5 text-xs text-stone-600">
                {{ item.foundZone || "Zona non indicata" }} ·
                {{ formatDateTime(item.foundAt) }}
              </p>
              <div class="mt-1.5">
                <StatusBadge :status="item.status" :sensitive="item.sensitive" />
              </div>
            </div>
          </div>

          <div
            v-if="item.sensitive"
            class="mt-3 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-900"
          >
            Oggetto sensibile: dopo l'accettazione hai 24 ore di custodia.
          </div>

          <div v-if="refusingId === item.id" class="mt-3 space-y-2">
            <label class="label" :for="`reason-${item.id}`">Motivo del rifiuto</label>
            <textarea
              :id="`reason-${item.id}`"
              v-model="refuseReason"
              class="input min-h-16 resize-y"
              maxlength="300"
              rows="2"
              placeholder="Es. spazio in cassaforte non disponibile"
              :disabled="busyId === item.id"
            ></textarea>
            <div class="flex gap-2">
              <button
                type="button"
                class="btn btn-danger flex-1 py-2"
                :disabled="busyId === item.id || !refuseReason.trim()"
                @click="confirmRefuse(item)"
              >
                {{ busyId === item.id ? "Invio…" : "Conferma rifiuto" }}
              </button>
              <button
                type="button"
                class="btn btn-ghost py-2"
                :disabled="busyId === item.id"
                @click="closeRefuse"
              >
                Annulla
              </button>
            </div>
          </div>

          <div v-else class="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              class="btn btn-primary flex-1 py-2"
              :disabled="busyId !== null"
              @click="accept(item)"
            >
              <span
                v-if="busyId === item.id"
                class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"
                aria-hidden="true"
              ></span>
              {{ busyId === item.id ? "Accetto…" : "Accetta custodia" }}
            </button>
            <button
              type="button"
              class="btn btn-secondary py-2"
              :disabled="busyId !== null"
              @click="openRefuse(item)"
            >
              Rifiuta
            </button>
            <RouterLink
              :to="`/intermediary/items/${item.id}`"
              class="btn btn-ghost py-2"
            >
              Dettagli
            </RouterLink>
          </div>
        </article>
      </div>
    </section>

    <section class="mt-8">
      <div class="flex items-baseline justify-between gap-3">
        <h2 class="section-title">In custodia</h2>
        <span class="text-xs text-stone-500">{{ inCustody.length }} oggetti</span>
      </div>

      <div v-if="loading" class="card mt-3 animate-pulse">
        <div class="h-4 w-1/3 rounded bg-stone-200"></div>
        <div class="mt-3 h-3 w-2/3 rounded bg-stone-200"></div>
      </div>

      <div v-else-if="inCustody.length === 0" class="empty-state mt-3">
        <p class="text-sm font-medium text-stone-700">Nessun oggetto in custodia</p>
        <p class="mt-1 text-xs text-stone-500">
          Accetta una segnalazione per iniziare a custodire un oggetto.
        </p>
      </div>

      <!-- Mobile cards -->
      <div v-else class="mt-3 space-y-2.5 md:hidden">
        <RouterLink
          v-for="item in inCustody"
          :key="item.id"
          :to="`/intermediary/items/${item.id}`"
          class="card-tight block"
        >
          <div class="flex gap-3">
            <div class="h-14 w-14 shrink-0 overflow-hidden rounded-lg bg-stone-100">
              <img
                v-if="item.photos.length"
                :src="item.photos[0]"
                :alt="displayItemName(item)"
                class="h-full w-full object-cover"
                loading="lazy"
              />
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-start justify-between gap-2">
                <p class="truncate text-sm font-semibold text-stone-900">
                  {{ displayItemName(item) }}
                </p>
                <span class="shrink-0 font-mono text-[11px] text-stone-500">
                  {{ item.publicCode }}
                </span>
              </div>
              <p class="mt-0.5 text-xs text-stone-600">
                {{ item.foundZone || "Zona non indicata" }}
              </p>
              <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
                <StatusBadge :status="item.status" :sensitive="item.sensitive" />
                <span
                  v-if="daysLeft(item) !== null"
                  class="badge"
                  :class="
                    (daysLeft(item) ?? 0) <= 1
                      ? 'bg-rose-50 text-rose-800 ring-rose-200'
                      : 'bg-stone-100 text-stone-700 ring-stone-200'
                  "
                >
                  {{ remainingLabel(item) }}
                </span>
              </div>
            </div>
          </div>
        </RouterLink>
      </div>

      <!-- Desktop table -->
      <div v-if="!loading && inCustody.length > 0" class="card mt-3 hidden overflow-hidden p-0 md:block">
        <table class="min-w-full divide-y divide-stone-200 text-sm">
          <thead class="bg-stone-50">
            <tr>
              <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-stone-500">
                Oggetto
              </th>
              <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-stone-500">
                Codice
              </th>
              <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-stone-500">
                Stato
              </th>
              <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-stone-500">
                Termine
              </th>
              <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide text-stone-500">
                Azioni
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-stone-100 bg-white">
            <tr v-for="item in inCustody" :key="item.id" class="hover:bg-stone-50/60">
              <td class="px-4 py-3">
                <div class="flex items-center gap-3">
                  <div class="h-10 w-10 shrink-0 overflow-hidden rounded-lg bg-stone-100">
                    <img
                      v-if="item.photos.length"
                      :src="item.photos[0]"
                      :alt="displayItemName(item)"
                      class="h-full w-full object-cover"
                      loading="lazy"
                    />
                  </div>
                  <div class="min-w-0">
                    <p class="truncate font-medium text-stone-900">
                      {{ displayItemName(item) }}
                    </p>
                    <p class="text-xs text-stone-500">
                      {{ item.foundZone || "Zona non indicata" }}
                    </p>
                  </div>
                </div>
              </td>
              <td class="px-4 py-3 font-mono text-xs text-stone-600">{{ item.publicCode }}</td>
              <td class="px-4 py-3">
                <StatusBadge :status="item.status" :sensitive="item.sensitive" />
              </td>
              <td class="px-4 py-3">
                <p class="text-xs text-stone-700">{{ deadlineLabel(item) }}</p>
                <p
                  v-if="daysLeft(item) !== null"
                  class="mt-0.5 text-xs font-semibold"
                  :class="(daysLeft(item) ?? 0) <= 1 ? 'text-rose-700' : 'text-stone-500'"
                >
                  {{ remainingLabel(item) }}
                </p>
              </td>
              <td class="px-4 py-3 text-right">
                <RouterLink
                  :to="`/intermediary/items/${item.id}`"
                  class="link text-xs"
                >
                  Dettagli
                </RouterLink>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <p v-if="missingDeadlines" class="mt-2 text-xs text-amber-700">
        Per alcuni oggetti il server non ha restituito il termine di custodia:
        il valore è mostrato come “Termine non disponibile”. Aggiorna la
        dashboard dopo aver accettato la custodia.
      </p>
    </section>
  </div>
</template>
