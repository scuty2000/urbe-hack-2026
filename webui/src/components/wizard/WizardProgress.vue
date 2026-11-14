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
            ? 'bg-vermilion text-ink ring-vermilion'
            : step > index + 1
              ? 'bg-vermilion-soft text-vermilion-dark ring-vermilion/30'
              : 'bg-paper-card text-ink-muted ring-line'
        "
      >
        {{ step > index + 1 ? "✓" : index + 1 }}
      </span>
      <span
        class="hidden truncate text-xs font-medium sm:block"
        :class="step === index + 1 ? 'text-ink' : 'text-ink-muted'"
      >
        {{ label }}
      </span>
      <span
        v-if="index < steps.length - 1"
        class="h-px min-w-2 flex-1 bg-line"
        aria-hidden="true"
      ></span>
    </li>
  </ol>
</template>
