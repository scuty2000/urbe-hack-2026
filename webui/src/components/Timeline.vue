<script setup lang="ts">
import { eventLabels, formatDateTime, roleLabels } from "../format";
import type { TimelineEvent } from "../types";

defineProps<{
  events: TimelineEvent[];
}>();
</script>

<template>
  <div v-if="events.length === 0" class="text-sm text-ink-muted">
    Nessun evento disponibile.
  </div>
  <ol v-else class="relative space-y-5 border-l border-line pl-5">
    <li v-for="event in events" :key="event.id" class="relative">
      <span
        class="absolute top-1.5 -left-[1.4rem] h-2.5 w-2.5 rounded-full bg-vermilion ring-4 ring-vermilion/20"
        aria-hidden="true"
      ></span>
      <div class="flex flex-col gap-0.5 sm:flex-row sm:items-baseline sm:justify-between">
        <p class="text-sm font-medium text-ink">
          {{ eventLabels[event.type] ?? event.type }}
        </p>
        <time class="text-xs text-ink-muted" :datetime="event.timestamp">
          {{ formatDateTime(event.timestamp) }}
        </time>
      </div>
      <p class="mt-0.5 text-xs text-ink-soft">
        {{ event.actorRole ? roleLabels[event.actorRole] : "Sistema" }}
        <span
          v-if="event.actor && event.actor !== event.actorRole"
          class="text-ink-muted"
          >· {{ event.actor }}</span
        >
      </p>
      <p v-if="event.notes" class="mt-1 text-xs leading-5 text-ink-soft">
        {{ event.notes }}
      </p>
      <p v-if="event.evidenceRef" class="mt-1 text-[11px] text-ink-muted">
        Riferimento: {{ event.evidenceRef }}
      </p>
    </li>
  </ol>
</template>
