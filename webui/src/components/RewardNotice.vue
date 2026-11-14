<script setup lang="ts">
import { computed } from "vue";
import { formatCents, organisationLabel } from "../format";
import type { RewardNotice } from "../types";

const props = withDefaults(
  defineProps<{
    notice: RewardNotice | null;
    loading?: boolean;
    error?: string;
  }>(),
  { loading: false, error: "" },
);

const amount = computed(() => {
  if (!props.notice?.requested) {
    return "Nessun importo richiesto";
  }
  if (props.notice.amountCents <= 0) {
    return "Importo da concordare";
  }
  return formatCents(props.notice.amountCents);
});
</script>

<template>
  <section class="card">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h2 class="section-title">Ricompensa al finder</h2>
        <p class="mt-1 text-xs text-stone-500">
          Art. 930 c.c.: la ricompensa è facoltativa e si concorda fuori dalla
          piattaforma.
        </p>
      </div>
      <span
        class="badge shrink-0"
        :class="
          notice?.requested
            ? 'bg-sky-50 text-sky-800 ring-sky-200'
            : 'bg-stone-100 text-stone-700 ring-stone-200'
        "
      >
        {{ notice?.requested ? "Richiesta" : "Nessuna richiesta" }}
      </span>
    </div>

    <div v-if="loading" class="mt-4 animate-pulse space-y-2">
      <div class="h-4 w-1/2 rounded bg-stone-200"></div>
      <div class="h-3 w-2/3 rounded bg-stone-200"></div>
    </div>

    <p v-else-if="error" class="mt-4 text-sm text-stone-600">{{ error }}</p>

    <template v-else-if="notice">
      <p class="mt-4 text-lg font-semibold text-stone-900">{{ amount }}</p>
      <dl class="mt-3 space-y-2 text-xs">
        <div class="flex items-center justify-between gap-3">
          <dt class="text-stone-500">Finder legale</dt>
          <dd class="font-medium text-stone-800">
            {{ organisationLabel(notice.legalFinderId) }}
          </dd>
        </div>
        <div class="flex items-center justify-between gap-3">
          <dt class="text-stone-500">Stato</dt>
          <dd class="font-medium text-stone-800">
            {{ notice.paymentStatus === "outside_system" ? "Fuori piattaforma" : notice.paymentStatus }}
          </dd>
        </div>
        <div v-if="notice.shownToSearcherAt" class="flex items-center justify-between gap-3">
          <dt class="text-stone-500">Mostrata a chi cerca</dt>
          <dd class="font-medium text-stone-800">Sì</dd>
        </div>
      </dl>
      <p class="mt-3 text-[11px] leading-5 text-stone-500">
        La piattaforma non incassa né garantisce la ricompensa: nessun pagamento
        avviene tramite l'app.
      </p>
    </template>

    <p v-else class="mt-4 text-sm text-stone-600">
      Nessuna richiesta di ricompensa registrata per questo oggetto.
    </p>
  </section>
</template>
