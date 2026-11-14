<script setup lang="ts">
import { categoryLabel, formatDate } from "../format";
import type { ChatCandidate } from "../types";

defineProps<{
  candidate: ChatCandidate;
  index: number;
}>();
</script>

<template>
  <article class="card-tight">
    <div class="flex items-start justify-between gap-2">
      <div class="min-w-0">
        <p class="text-[11px] font-semibold tracking-wide text-stone-500 uppercase">
          Candidato {{ index + 1 }}
        </p>
        <p class="mt-0.5 truncate text-sm font-semibold text-stone-900">
          {{ categoryLabel(candidate.category) }}
        </p>
      </div>
      <span
        class="badge shrink-0"
        :class="
          candidate.custodianType === 'institution'
            ? 'bg-indigo-50 text-indigo-800 ring-indigo-200'
            : 'bg-emerald-50 text-emerald-800 ring-emerald-200'
        "
      >
        {{ candidate.custodianType === "institution" ? "Presso la PA" : "Punto di raccolta" }}
      </span>
    </div>

    <dl class="mt-3 grid grid-cols-2 gap-2 text-xs">
      <div>
        <dt class="data-label">Zona</dt>
        <dd class="mt-0.5 text-stone-800">{{ candidate.foundZone || "Non indicata" }}</dd>
      </div>
      <div>
        <dt class="data-label">Ritrovato il</dt>
        <dd class="mt-0.5 text-stone-800">{{ formatDate(candidate.foundAt) }}</dd>
      </div>
    </dl>

    <p class="mt-2 text-[11px] leading-5 text-stone-500">
      Solo dati pubblici: nessun dettaglio privato, coordinata esatta o valore atteso.
    </p>
  </article>
</template>
