<script setup lang="ts">
import { eventLabels, formatDateTime, roleLabels } from "../format";
import type { TimelineEvent } from "../types";

defineProps<{
  events: TimelineEvent[];
}>();
</script>

<template>
  <div v-if="events.length === 0" class="text-sm text-stone-500">
    Nessun evento disponibile.
  </div>
  <ol v-else class="relative space-y-5 border-l border-stone-200 pl-5">
    <li v-for="event in events" :key="event.id" class="relative">
      <span
        class="absolute top-1.5 -left-[1.4rem] h-2.5 w-2.5 rounded-full bg-emerald-600 ring-4 ring-emerald-50"
        aria-hidden="true"
      ></span>
      <div class="flex flex-col gap-0.5 sm:flex-row sm:items-baseline sm:justify-between">
        <p class="text-sm font-medium text-stone-900">
          {{ eventLabels[event.type] ?? event.type }}
        </p>
        <time class="text-xs text-stone-500" :datetime="event.timestamp">
          {{ formatDateTime(event.timestamp) }}
        </time>
      </div>
      <p class="mt-0.5 text-xs text-stone-600">
        {{ event.actorRole ? roleLabels[event.actorRole] : "Sistema" }}
        <span
          v-if="event.actor && event.actor !== event.actorRole"
          class="text-stone-400"
          >· {{ event.actor }}</span
        >
      </p>
      <p v-if="event.notes" class="mt-1 text-xs leading-5 text-stone-600">
        {{ event.notes }}
      </p>
      <p v-if="event.evidenceRef" class="mt-1 text-[11px] text-stone-400">
        Riferimento: {{ event.evidenceRef }}
      </p>
    </li>
  </ol>
</template>
