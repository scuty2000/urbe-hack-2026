<script setup lang="ts">
import { computed } from "vue";
import { RouterLink, useRoute } from "vue-router";
import StatusBadge from "../components/StatusBadge.vue";
import { conditionLabels, displayItemName, formatDateTime } from "../format";
import { loadReceipt } from "../receipt";

const route = useRoute();
const item = computed(() => loadReceipt(String(route.params.id ?? "")));

const dropPointNames: Record<string, string> = {
  "bar-x": "Bar X · Via dei Volsci 12, Roma",
  "portineria-sapienza": "Portineria Sapienza · Piazzale Aldo Moro 5, Roma",
};

function printReceipt() {
  window.print();
}
</script>

<template>
  <div class="mx-auto w-full max-w-2xl">
    <div v-if="!item" class="empty-state">
      <span
        class="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-stone-100 text-stone-500"
        aria-hidden="true"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          class="h-6 w-6"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M9 12h6m-6 4h6m2 4H7a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h7l5 5v9a2 2 0 0 1-2 2Z"
          />
        </svg>
      </span>
      <h1 class="mt-4 text-lg font-semibold text-stone-900">
        Ricevuta non disponibile
      </h1>
      <p class="mx-auto mt-2 max-w-md text-sm leading-6 text-stone-600">
        Il finder non può riaprire la scheda di un oggetto già registrato. La
        ricevuta è conservata solo nella sessione del browser in cui è avvenuto
        l'invio: se hai cambiato scheda o dispositivo, contatta il punto di
        raccolta indicando il codice oggetto.
      </p>
      <RouterLink to="/finder" class="btn btn-primary mt-5">
        Registra un nuovo oggetto
      </RouterLink>
    </div>

    <template v-else>
      <div class="text-center">
        <span
          class="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-emerald-100 text-emerald-800"
          aria-hidden="true"
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            class="h-7 w-7"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="m4.5 12.75 6 6 9-13.5" />
          </svg>
        </span>
        <p class="eyebrow mt-4">Segnalazione completata</p>
        <h1 class="mt-1 text-2xl font-semibold tracking-tight text-stone-900">
          Grazie per il tuo aiuto
        </h1>
        <p class="mx-auto mt-2 max-w-lg text-sm leading-6 text-stone-600">
          L'oggetto è stato registrato. Conserva il codice qui sotto: è
          l'identificativo pubblico della segnalazione.
        </p>
      </div>

      <div class="card mt-6">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p class="data-label">Codice oggetto</p>
            <p class="mt-1 font-mono text-2xl font-semibold tracking-wide text-stone-900">
              {{ item.publicCode }}
            </p>
          </div>
          <StatusBadge :status="item.status" :sensitive="item.sensitive" />
        </div>

        <div class="mt-5 flex gap-3 rounded-xl bg-stone-50 p-3">
          <div
            v-if="item.photos.length > 0"
            class="h-20 w-20 shrink-0 overflow-hidden rounded-lg bg-stone-200"
          >
            <img
              :src="item.photos[0]"
              :alt="displayItemName(item)"
              class="h-full w-full object-cover"
            />
          </div>
          <div class="min-w-0">
            <p class="text-sm font-semibold text-stone-900">
              {{ displayItemName(item) }}
            </p>
            <p class="mt-0.5 text-xs text-stone-600">
              {{ item.foundZone || "Zona non indicata" }} ·
              {{ formatDateTime(item.foundAt) }}
            </p>
            <p class="mt-1 text-xs text-stone-500">
              {{ conditionLabels[item.condition ?? ""] ?? item.condition ?? "Condizioni non indicate" }}
            </p>
          </div>
        </div>

        <dl class="mt-5 grid gap-4 text-sm sm:grid-cols-2">
          <div>
            <dt class="data-label">Punto di raccolta</dt>
            <dd class="data-value">
              {{ dropPointNames[item.custodianId ?? ""] ?? item.custodianId ?? "—" }}
            </dd>
          </div>
          <div>
            <dt class="data-label">Registrato il</dt>
            <dd class="data-value">{{ formatDateTime(item.createdAt) }}</dd>
          </div>
          <div>
            <dt class="data-label">Stato</dt>
            <dd class="data-value">{{ item.status === "pending_custody" ? "In attesa che il punto confermi la custodia" : item.status }}</dd>
          </div>
          <div>
            <dt class="data-label">Qualità di ritrovatore</dt>
            <dd class="data-value">
              {{ item.rightsAssigned ? "Ceduta al punto di raccolta" : "In attesa di conferma" }}
              <span class="block text-xs text-stone-500">
                La presa in carico sarà confermata dal punto.
              </span>
            </dd>
          </div>
        </dl>

        <div
          v-if="item.sensitive"
          class="mt-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900"
        >
          Oggetto sensibile: quando il punto di raccolta conferma la custodia,
          il termine di conservazione è di 24 ore. Per documenti e carte, evita
          di condividerne foto con altre persone.
        </div>

        <div class="mt-5 flex flex-col gap-2 border-t border-stone-100 pt-5 sm:flex-row">
          <button type="button" class="btn btn-secondary" @click="printReceipt">
            Stampa ricevuta
          </button>
          <RouterLink to="/finder" class="btn btn-primary">
            Segnala un altro oggetto
          </RouterLink>
        </div>
      </div>

      <div class="card mt-4">
        <h2 class="section-title">Cosa succede ora</h2>
        <ol class="mt-3 space-y-3 text-sm leading-6 text-stone-600">
          <li class="flex gap-3">
            <span class="font-mono text-xs text-emerald-700">01</span>
            Il punto di raccolta conferma la presa in carico dalla dashboard:
            solo allora inizia il termine di custodia.
          </li>
          <li class="flex gap-3">
            <span class="font-mono text-xs text-emerald-700">02</span>
            Chi ha perso l'oggetto può ritrovarlo tramite la ricerca e aprire un
            reclamo con le prove di appartenenza.
          </li>
          <li class="flex gap-3">
            <span class="font-mono text-xs text-emerald-700">03</span>
            La riconsegna avviene al punto di raccolta, con verifica del
            proprietario. Non serve altro da parte tua.
          </li>
        </ol>
      </div>
    </template>
  </div>
</template>
