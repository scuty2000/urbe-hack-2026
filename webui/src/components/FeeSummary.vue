<script setup lang="ts">
import { computed } from "vue";
import { feeAppliesLabels, feeStatusLabels, formatCents, formatDateTime } from "../format";
import type { Fee } from "../types";

const props = withDefaults(
  defineProps<{
    fee: Fee | null;
    loading?: boolean;
    payable?: boolean;
    busy?: boolean;
    error?: string;
  }>(),
  { loading: false, payable: false, busy: false, error: "" },
);

const emit = defineEmits<{
  pay: [];
}>();

const amount = computed(() =>
  props.fee ? formatCents(props.fee.amountCents, props.fee.currency) : "—",
);

const freeDirectReturn = computed(
  () => props.fee?.applies === "none_direct_return" || (props.fee?.amountCents ?? 0) <= 0,
);

const displayStatus = computed(() => {
  if (!props.fee) {
    return "";
  }
  if (props.fee.paid && freeDirectReturn.value) {
    return "none";
  }
  if (props.fee.paid) {
    return "paid";
  }
  return props.fee.status;
});

const displayStatusLabel = computed(() => {
  if (displayStatus.value === "none") {
    return "Nessuna tariffa";
  }
  return feeStatusLabels[displayStatus.value] ?? displayStatus.value;
});

const statusTone = computed(() => {
  switch (displayStatus.value) {
    case "paid":
    case "waived":
    case "none":
      return "bg-olive-soft text-olive ring-olive/30";
    case "final":
      return "bg-ochre-soft text-ochre ring-ochre/30";
    default:
      return "bg-paper text-ink-soft ring-line";
  }
});
</script>

<template>
  <section class="card">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h2 class="section-title">Tariffa di custodia</h2>
        <p class="mt-1 text-xs text-ink-muted">
          Stima calcolata dalla pubblica amministrazione per i giorni di custodia.
        </p>
      </div>
      <span v-if="fee" class="badge shrink-0" :class="statusTone">
        {{ displayStatusLabel }}
      </span>
    </div>

    <div v-if="loading" class="mt-4 animate-pulse space-y-2">
      <div class="h-4 w-1/3 rounded bg-paper-deep"></div>
      <div class="h-3 w-2/3 rounded bg-paper-deep"></div>
    </div>

    <p v-else-if="error" class="mt-4 text-sm text-rust">{{ error }}</p>

    <div v-else-if="!fee" class="mt-4 text-sm text-ink-soft">
      Nessuna tariffa disponibile per questo oggetto.
    </div>

    <template v-else>
      <p class="mt-4 font-display text-3xl leading-none tracking-tight text-ink">
        {{ amount }}
      </p>
      <p class="mt-1 text-xs text-ink-muted">
        {{ feeAppliesLabels[fee.applies] ?? fee.applies }}
        <span v-if="!freeDirectReturn">
          · {{ fee.custodyDays }} {{ fee.custodyDays === 1 ? "giorno" : "giorni" }}
        </span>
      </p>

      <div
        v-if="freeDirectReturn"
        class="mt-3 rounded-md border border-olive/30 bg-olive-soft px-3 py-2 text-xs text-ink"
      >
        Riconsegna diretta al punto di raccolta: nessun importo dovuto.
      </div>

      <div v-if="fee.payment" class="mt-3 rounded-md bg-paper px-3 py-2 text-xs text-ink-soft">
        <p class="font-medium text-ink">Pagamento registrato</p>
        <p class="mt-0.5">
          {{ fee.payment.provider }} · {{ formatDateTime(fee.payment.paidAt) }}
        </p>
        <p class="mt-0.5 font-mono text-[11px] text-ink-muted">
          {{ fee.payment.receiptUrl }}
        </p>
      </div>

      <div v-if="payable && !fee.paid" class="mt-4">
        <button
          type="button"
          class="btn btn-primary w-full"
          :disabled="busy"
          @click="emit('pay')"
        >
          <span
            v-if="busy"
            class="h-4 w-4 animate-spin rounded-full border-2 border-paper border-t-transparent"
            aria-hidden="true"
          ></span>
          {{ busy ? "Pagamento in corso…" : "Paga la tariffa" }}
        </button>
        <p class="mt-2 text-[11px] leading-4 text-ink-muted">
          Pagamento simulato per la demo: nessun addebito reale. Il pass si sblocca
          appena la tariffa risulta pagata.
        </p>
      </div>
    </template>
  </section>
</template>
