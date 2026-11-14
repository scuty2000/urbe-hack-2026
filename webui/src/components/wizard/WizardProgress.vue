<script setup lang="ts">
defineProps<{
  step: number;
  steps: string[];
}>();
</script>

<template>
  <ol class="flex items-center gap-2" aria-label="Avanzamento della segnalazione">
    <li
      v-for="(label, index) in steps"
      :key="label"
      class="flex min-w-0 flex-1 items-center gap-2"
      :aria-current="step === index + 1 ? 'step' : undefined"
    >
      <span
        class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold ring-1"
        :class="
          step === index + 1
            ? 'bg-emerald-700 text-white ring-emerald-700'
            : step > index + 1
              ? 'bg-emerald-50 text-emerald-800 ring-emerald-200'
              : 'bg-white text-stone-500 ring-stone-200'
        "
      >
        {{ step > index + 1 ? "✓" : index + 1 }}
      </span>
      <span
        class="hidden truncate text-xs font-medium sm:block"
        :class="step === index + 1 ? 'text-stone-900' : 'text-stone-500'"
      >
        {{ label }}
      </span>
      <span
        v-if="index < steps.length - 1"
        class="h-px min-w-2 flex-1 bg-stone-200"
        aria-hidden="true"
      ></span>
    </li>
  </ol>
</template>
