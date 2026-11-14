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
    class="group flex w-full items-center gap-3 rounded-2xl border border-stone-200 bg-white p-3.5 text-left shadow-sm transition hover:border-emerald-300 hover:shadow-md disabled:cursor-wait disabled:opacity-70"
    :disabled="busy"
    @click="$emit('select', demo)"
  >
    <span
      class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-stone-100 text-sm font-semibold text-stone-700 group-hover:bg-emerald-50 group-hover:text-emerald-800"
      aria-hidden="true"
    >
      {{ demo.initials }}
    </span>
    <span class="min-w-0 flex-1">
      <span class="flex flex-wrap items-center gap-1.5">
        <span class="truncate text-sm font-semibold text-stone-900">{{ demo.name }}</span>
        <span class="badge" :class="tone">{{ roleLabels[demo.role] }}</span>
      </span>
      <span class="mt-0.5 block text-xs leading-5 text-stone-500">{{ demo.description }}</span>
      <span class="mt-0.5 block font-mono text-[10px] text-stone-500">{{ demo.id }}</span>
    </span>
    <span
      class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-stone-500 transition group-hover:bg-emerald-700 group-hover:text-white"
      aria-hidden="true"
    >
      <span v-if="busy" class="h-4 w-4 animate-spin rounded-full border-2 border-emerald-700 border-t-transparent"></span>
      <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="h-4 w-4">
        <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
      </svg>
    </span>
  </button>
</template>
