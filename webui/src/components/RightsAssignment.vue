<script setup lang="ts">
import { computed, useId } from "vue";

const props = defineProps<{
  mode: "finder" | "intermediary";
  disabled?: boolean;
}>();

const agreed = defineModel<boolean>({ default: false });
const checkboxId = useId();

const title = computed(() =>
  props.mode === "finder"
    ? "Cessione della qualità di ritrovatore legale"
    : "Cessione della qualità di ritrovatore e presa in carico",
);

const legalCopy = computed(() =>
  props.mode === "finder"
    ? "Confermo di aver trovato l'oggetto e di cedere al punto di raccolta selezionato la qualità di ritrovatore legale ai fini degli artt. 927 e seguenti del codice civile. La presa in carico fisica e il termine di custodia decorrono solo quando il punto conferma la custodia dalla propria dashboard. I miei dati personali e le coordinate esatte del ritrovamento non saranno pubblicati."
    : "Confermo di aver ricevuto materialmente l'oggetto dal finder e di cedere a me stesso, nell'esercizio del mio punto di raccolta, la qualità di ritrovatore legale ai fini degli artt. 927 e seguenti del codice civile. La registrazione crea la scheda in attesa di custodia: la presa in carico e il termine di conservazione decorrono solo quando confermo la custodia dalla dashboard. Mi impegno a conservare l'oggetto in modo sicuro e a riconsegnarlo solo al proprietario verificato.",
);

const checkboxCopy = computed(() =>
  props.mode === "finder"
    ? "Dichiaro di aver letto e di accettare la cessione della qualità di ritrovatore; la presa in carico sarà confermata dal punto di raccolta dalla dashboard."
    : "Dichiaro di aver letto e di accettare la cessione della qualità di ritrovatore a me stesso e di confermare la custodia dalla dashboard.",
);
</script>

<template>
  <div class="rounded-2xl border border-amber-200 bg-amber-50/70 p-4">
    <div class="flex gap-3">
      <span
        class="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-amber-200 text-amber-900"
        aria-hidden="true"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.9"
          class="h-4 w-4"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M12 9v3.75m0 3h.008v.008H12v-.008ZM10.29 3.86 2.82 17a2.25 2.25 0 0 0 1.95 3.38h14.46A2.25 2.25 0 0 0 21.18 17L13.71 3.86a2.25 2.25 0 0 0-3.42 0Z"
          />
        </svg>
      </span>
      <div class="min-w-0">
        <p class="text-sm font-semibold text-amber-950">{{ title }}</p>
        <p class="mt-1 text-xs leading-5 text-amber-900">{{ legalCopy }}</p>
      </div>
    </div>

    <label
      :for="checkboxId"
      class="mt-3 flex cursor-pointer items-start gap-2.5 rounded-xl bg-white/70 p-3 text-sm text-stone-800 ring-1 ring-amber-200"
    >
      <input
        :id="checkboxId"
        v-model="agreed"
        type="checkbox"
        class="mt-0.5 h-4 w-4 shrink-0 rounded border-stone-300 text-emerald-700 focus:ring-emerald-600"
        :disabled="disabled"
      />
      <span>
        {{ checkboxCopy }}
        <span class="font-medium text-rose-700">Obbligatorio per registrare l'oggetto.</span>
      </span>
    </label>
  </div>
</template>
