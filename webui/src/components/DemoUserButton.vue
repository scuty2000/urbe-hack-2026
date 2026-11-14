<script setup lang="ts">
import { roleLabels } from "../format";
import type { Role } from "../types";

defineProps<{
  demo: {
    id: string;
    name: string;
    role: Role;
    description: string;
    initials: string;
  };
  busy: boolean;
  tone: string;
}>();

defineEmits<{
  select: [demo: {
    id: string;
    name: string;
    role: Role;
    description: string;
    initials: string;
  }];
}>();
</script>

<template>
  <button
    type="button"
    class="group flex w-full items-center gap-3 rounded-lg border border-line bg-paper-card p-3.5 text-left transition hover:border-ink/50 hover:shadow-[3px_3px_0_#e14b2a] disabled:cursor-wait disabled:opacity-70"
    :disabled="busy"
    @click="$emit('select', demo)"
  >
    <span
      class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-paper text-sm font-semibold text-ink-soft group-hover:bg-vermilion-soft group-hover:text-vermilion-dark"
      aria-hidden="true"
    >
      {{ demo.initials }}
    </span>
    <span class="min-w-0 flex-1">
      <span class="flex flex-wrap items-center gap-1.5">
        <span class="truncate text-sm font-semibold text-ink">{{ demo.name }}</span>
        <span class="badge" :class="tone">{{ roleLabels[demo.role] }}</span>
      </span>
      <span class="mt-0.5 block text-xs leading-5 text-ink-muted">{{ demo.description }}</span>
      <span class="mt-0.5 block font-mono text-[10px] text-ink-muted">{{ demo.id }}</span>
    </span>
    <span
      class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-ink-muted transition group-hover:bg-vermilion group-hover:text-ink"
      aria-hidden="true"
    >
      <span v-if="busy" class="h-4 w-4 animate-spin rounded-full border-2 border-vermilion border-t-transparent"></span>
      <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="h-4 w-4">
        <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
      </svg>
    </span>
  </button>
</template>
