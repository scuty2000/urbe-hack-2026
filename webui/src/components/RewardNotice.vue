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
        <p class="mt-1 text-xs text-ink-muted">
          Art. 930 c.c.: la ricompensa è facoltativa e si concorda fuori dalla
          piattaforma.
        </p>
      </div>
      <span
        class="badge shrink-0"
        :class="
          notice?.requested
            ? 'bg-teal-soft text-teal ring-teal/25'
            : 'bg-paper text-ink-soft ring-line'
        "
      >
        {{ notice?.requested ? "Richiesta" : "Nessuna richiesta" }}
      </span>
    </div>

    <div v-if="loading" class="mt-4 animate-pulse space-y-2">
      <div class="h-4 w-1/2 rounded bg-paper-deep"></div>
      <div class="h-3 w-2/3 rounded bg-paper-deep"></div>
    </div>

    <p v-else-if="error" class="mt-4 text-sm text-ink-soft">{{ error }}</p>

    <template v-else-if="notice">
      <p class="mt-4 text-lg font-semibold text-ink">{{ amount }}</p>
      <dl class="mt-3 space-y-2 text-xs">
        <div class="flex items-center justify-between gap-3">
          <dt class="text-ink-muted">Finder legale</dt>
          <dd class="font-medium text-ink">
            {{ organisationLabel(notice.legalFinderId) }}
          </dd>
        </div>
        <div class="flex items-center justify-between gap-3">
          <dt class="text-ink-muted">Stato</dt>
          <dd class="font-medium text-ink">
            {{ notice.paymentStatus === "outside_system" ? "Fuori piattaforma" : notice.paymentStatus }}
          </dd>
        </div>
        <div v-if="notice.shownToSearcherAt" class="flex items-center justify-between gap-3">
          <dt class="text-ink-muted">Mostrata a chi cerca</dt>
          <dd class="font-medium text-ink">Sì</dd>
        </div>
      </dl>
      <p class="mt-3 text-[11px] leading-5 text-ink-muted">
        La piattaforma non incassa né garantisce la ricompensa: nessun pagamento
        avviene tramite l'app.
      </p>
    </template>

    <p v-else class="mt-4 text-sm text-ink-soft">
      Nessuna richiesta di ricompensa registrata per questo oggetto.
    </p>
  </section>
</template>
