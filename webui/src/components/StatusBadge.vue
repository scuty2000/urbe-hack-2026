<script setup lang="ts">
import { computed } from "vue";
import { statusLabels, statusTones } from "../format";
import type { ItemStatus } from "../types";

const props = withDefaults(
  defineProps<{
    status: ItemStatus;
    sensitive?: boolean;
  }>(),
  { sensitive: false },
);

const label = computed(() => statusLabels[props.status] ?? props.status);
const tone = computed(
  () => statusTones[props.status] ?? "bg-paper text-ink-soft ring-line",
);
</script>

<template>
  <span class="inline-flex flex-wrap items-center gap-1.5">
    <span class="badge" :class="tone">{{ label }}</span>
    <span
      v-if="sensitive"
      class="badge bg-rust-soft text-rust ring-rust/30"
      title="Oggetto sensibile: termine di custodia di 24 ore"
    >
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        class="h-3.5 w-3.5"
        aria-hidden="true"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
        />
      </svg>
      Sensibile
    </span>
  </span>
</template>
