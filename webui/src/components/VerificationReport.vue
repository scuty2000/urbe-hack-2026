<script setup lang="ts">
import { computed } from "vue";
import {
  percent,
  verificationDecisionLabels,
  verificationFlagLabels,
  verificationReasonLabel,
} from "../format";
import type { VerificationReportData } from "../types";

const props = defineProps<{
  report: VerificationReportData;
}>();

const questionLabels: Record<string, string> = {
  category: "Categoria",
  color: "Colore",
  case_color: "Colore della custodia",
  has_crack: "Presenza di crepe",
  has_sticker: "Presenza di adesivi",
  sticker: "Adesivo",
  contents: "Contenuto",
  description: "Descrizione",
  detail: "Dettaglio",
  document_type: "Tipo di documento",
  brand: "Marca",
  model: "Modello",
};

function questionLabel(key: string): string {
  if (questionLabels[key]) {
    return questionLabels[key];
  }
  const words = key.replace(/[_-]+/g, " ").trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

const decisionTone = computed(() => {
  switch (props.report.decision) {
    case "auto_approve":
      return "bg-emerald-50 text-emerald-800 ring-emerald-200";
    case "reject":
      return "bg-rose-50 text-rose-800 ring-rose-200";
    case "more_info":
      return "bg-amber-50 text-amber-800 ring-amber-200";
    default:
      return "bg-sky-50 text-sky-800 ring-sky-200";
  }
});

const questions = computed(() =>
  Object.entries(props.report.questions ?? {}).map(([key, question]) => ({
    key,
    label: questionLabel(key),
    type: question.type,
    answer: props.report.answers?.[key] ?? "—",
    probability: props.report.probabilities?.[key],
  })),
);
</script>

<template>
  <section class="card">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="section-title">Relazione di verifica</h2>
        <p class="mt-1 text-xs text-stone-500">
          Domande, risposte e confidenza del motore. Nessun valore privato del
          profilo viene mostrato.
        </p>
      </div>
      <span class="badge shrink-0" :class="decisionTone">
        {{ verificationDecisionLabels[report.decision] ?? report.decision }}
      </span>
    </div>

    <dl class="mt-4 grid grid-cols-2 gap-3 text-sm">
      <div>
        <dt class="data-label">Confidenza</dt>
        <dd class="data-value font-semibold">{{ percent(report.confidence) }}</dd>
      </div>
      <div>
        <dt class="data-label">Modello</dt>
        <dd class="data-value font-mono text-xs">{{ report.modelVersion || "—" }}</dd>
      </div>
    </dl>

    <p v-if="report.rationale" class="mt-3 rounded-xl bg-stone-50 px-3 py-2 text-xs leading-5 text-stone-700">
      {{ report.rationale }}
    </p>

    <div v-if="report.flags.length > 0" class="mt-3 flex flex-wrap gap-1.5">
      <span
        v-for="flag in report.flags"
        :key="flag"
        class="badge bg-amber-50 text-amber-800 ring-amber-200"
      >
        {{ verificationFlagLabels[flag] ?? flag }}
      </span>
    </div>

    <div v-if="questions.length > 0" class="mt-4 overflow-hidden rounded-xl border border-stone-200">
      <table class="min-w-full divide-y divide-stone-200 text-sm">
        <thead class="bg-stone-50">
          <tr>
            <th class="px-3 py-2 text-left text-xs font-semibold tracking-wide text-stone-500 uppercase">
              Domanda
            </th>
            <th class="px-3 py-2 text-left text-xs font-semibold tracking-wide text-stone-500 uppercase">
              Risposta
            </th>
            <th class="px-3 py-2 text-right text-xs font-semibold tracking-wide text-stone-500 uppercase">
              Confidenza
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-stone-100 bg-white">
          <tr v-for="question in questions" :key="question.key">
            <td class="px-3 py-2">
              <p class="font-medium text-stone-800">{{ question.label }}</p>
              <p class="font-mono text-[10px] text-stone-400">{{ question.type }}</p>
            </td>
            <td class="px-3 py-2 text-stone-700">{{ question.answer }}</td>
            <td class="px-3 py-2 text-right text-xs text-stone-600">
              {{ question.probability ? percent(question.probability.confidence ?? question.probability.score) : "—" }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="report.reasons.length > 0" class="mt-4">
      <h3 class="text-xs font-semibold tracking-wide text-stone-500 uppercase">Motivazioni</h3>
      <ul class="mt-1.5 space-y-1 text-xs leading-5 text-stone-700">
        <li v-for="reason in report.reasons" :key="reason" class="flex gap-2">
          <span class="text-stone-400" aria-hidden="true">•</span>
          <span>{{ verificationReasonLabel(reason) }}</span>
        </li>
      </ul>
    </div>

    <div v-if="report.evidence.length > 0" class="mt-4">
      <h3 class="text-xs font-semibold tracking-wide text-stone-500 uppercase">Prove</h3>
      <ul class="mt-1.5 space-y-1 text-xs text-stone-700">
        <li v-for="evidence in report.evidence" :key="evidence" class="font-mono">
          {{ evidence }}
        </li>
      </ul>
    </div>

    <p v-if="report.flags.length === 0 && report.reasons.length === 0" class="mt-4 text-xs text-stone-500">
      Nessun segnale o motivazione aggiuntiva.
    </p>
  </section>
</template>
