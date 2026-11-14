<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ApiError, listNotifications, markNotificationRead } from "../api";
import { formatDateTime, formatCents, notificationTypeLabels } from "../format";
import type { AppNotification } from "../types";

const open = ref(false);
const loading = ref(false);
const error = ref("");
const notifications = ref<AppNotification[]>([]);
const markingId = ref<string | null>(null);
let poll: number | undefined;

const unread = computed(
  () => notifications.value.filter((notification) => !notification.read).length,
);

async function load(silent = false) {
  if (!silent) {
    loading.value = true;
  }
  error.value = "";
  try {
    notifications.value = await listNotifications();
  } catch (cause) {
    if (!silent) {
      error.value =
        cause instanceof ApiError
          ? cause.message
          : "Non riesco a caricare le notifiche.";
    }
  } finally {
    loading.value = false;
  }
}

async function toggle() {
  open.value = !open.value;
  if (open.value) {
    await load();
  }
}

async function markRead(notification: AppNotification) {
  if (notification.read || markingId.value) {
    return;
  }
  markingId.value = notification.id;
  try {
    await markNotificationRead(notification.id);
    notification.read = true;
  } catch (cause) {
    error.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a segnare la notifica come letta.";
  } finally {
    markingId.value = null;
  }
}

function payloadSummary(notification: AppNotification): string {
  const payload = notification.payload ?? {};
  const parts: string[] = [];
  if (typeof payload.status === "string" && payload.status) {
    parts.push(payload.status.replace(/_/g, " "));
  }
  if (typeof payload.amountCents === "number") {
    parts.push(formatCents(payload.amountCents));
  }
  for (const key of ["itemId", "claimId", "passId"]) {
    const value = payload[key];
    if (typeof value === "string" && value) {
      parts.push(value);
    }
  }
  return parts.join(" · ");
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    open.value = false;
  }
}

onMounted(() => {
  void load(true);
  poll = window.setInterval(() => {
    void load(true);
  }, 60_000);
  window.addEventListener("keydown", onKeydown);
});

onBeforeUnmount(() => {
  if (poll) {
    window.clearInterval(poll);
  }
  window.removeEventListener("keydown", onKeydown);
});
</script>

<template>
  <div class="relative">
    <button
      type="button"
      class="relative flex h-9 w-9 items-center justify-center rounded-md text-ink-muted transition hover:bg-paper hover:text-ink"
      :aria-label="unread > 0 ? `Notifiche, ${unread} non lette` : 'Notifiche'"
      :aria-expanded="open"
      aria-haspopup="true"
      @click="toggle"
    >
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        class="h-5 w-5"
        aria-hidden="true"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M14.857 17.082a23.848 23.848 0 0 0 5.454-1.31A8.967 8.967 0 0 1 18 9.75V9A6 6 0 0 0 6 9v.75a8.967 8.967 0 0 1-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 0 1-5.714 0m5.714 0a3 3 0 1 1-5.714 0"
        />
      </svg>
      <span
        v-if="unread > 0"
        class="absolute -top-0.5 -right-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-rust px-1 text-[10px] font-semibold text-paper"
      >
        {{ unread > 9 ? "9+" : unread }}
      </span>
    </button>

    <template v-if="open">
      <div class="fixed inset-0 z-30" aria-hidden="true" @click="open = false"></div>
      <div
        class="absolute right-0 z-40 mt-2 w-80 max-w-[calc(100vw-2rem)] overflow-hidden rounded-lg border border-ink/20 bg-paper-card shadow-[4px_4px_0_#14110f]"
        role="dialog"
        aria-label="Notifiche"
      >
        <div class="flex items-center justify-between border-b border-line px-4 py-3">
          <p class="text-sm font-semibold text-ink">Notifiche</p>
          <button type="button" class="link text-xs" :disabled="loading" @click="load()">
            {{ loading ? "Aggiorno…" : "Aggiorna" }}
          </button>
        </div>

        <p v-if="error" class="border-b border-rust/20 bg-rust-soft px-4 py-2 text-xs text-rust">
          {{ error }}
        </p>

        <div v-if="loading && notifications.length === 0" class="space-y-2 p-4">
          <div v-for="index in 3" :key="index" class="h-10 animate-pulse rounded-lg bg-paper"></div>
        </div>

        <p v-else-if="notifications.length === 0" class="px-4 py-8 text-center text-sm text-ink-muted">
          Nessuna notifica.
        </p>

        <ul v-else class="max-h-96 divide-y divide-line overflow-y-auto">
          <li v-for="notification in notifications" :key="notification.id">
            <button
              type="button"
              class="flex w-full gap-2.5 px-4 py-3 text-left transition hover:bg-paper"
              :class="{ 'bg-vermilion-soft/40': !notification.read }"
              @click="markRead(notification)"
            >
              <span
                class="mt-1.5 h-2 w-2 shrink-0 rounded-full"
                :class="notification.read ? 'bg-paper-deep' : 'bg-vermilion'"
                aria-hidden="true"
              ></span>
              <span class="min-w-0 flex-1">
                <span class="flex items-baseline justify-between gap-2">
                  <span class="text-sm font-medium text-ink">
                    {{ notificationTypeLabels[notification.type] ?? notification.type }}
                  </span>
                  <span class="shrink-0 text-[10px] text-ink-muted">
                    {{ formatDateTime(notification.createdAt) }}
                  </span>
                </span>
                <span
                  v-if="payloadSummary(notification)"
                  class="mt-0.5 block truncate font-mono text-[10px] text-ink-muted"
                >
                  {{ payloadSummary(notification) }}
                </span>
                <span v-if="!notification.read" class="mt-1 block text-[10px] font-medium text-vermilion-dark">
                  {{ markingId === notification.id ? "Segno come letta…" : "Segna come letta" }}
                </span>
              </span>
            </button>
          </li>
        </ul>
      </div>
    </template>
  </div>
</template>
