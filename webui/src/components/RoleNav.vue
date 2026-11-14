<script setup lang="ts">
import { computed, ref } from "vue";
import { RouterLink, useRouter } from "vue-router";
import { logout, user } from "../api";
import { roleLabels } from "../format";
import { clearAllPasses } from "../passStore";
import { homeForRole } from "../router";
import NotificationBell from "./NotificationBell.vue";
import logoUrl from "../assets/brand/o-rivojo-lockup.svg";

const router = useRouter();
const busy = ref(false);

const links = computed(() => {
  switch (user.value?.role) {
    case "finder":
      return [{ to: "/finder", label: "Segnala un oggetto" }];
    case "searcher":
      return [{ to: "/searcher", label: "Cerca un oggetto" }];
    case "intermediary":
      return [
        { to: "/intermediary", label: "Dashboard" },
        { to: "/intermediary/enroll", label: "Registra" },
        { to: "/intermediary/scan", label: "Scansiona" },
      ];
    case "institution":
      return [
        { to: "/institution", label: "Ritiri" },
        { to: "/institution/intake", label: "Intake" },
        { to: "/institution/scan", label: "Scansiona" },
      ];
    default:
      return [];
  }
});

const showNotifications = computed(
  () => user.value?.role === "searcher" || user.value?.role === "institution",
);

const home = computed(() => homeForRole(user.value?.role));

async function signOut() {
  busy.value = true;
  try {
    await logout();
  } finally {
    // logout clears the local session even when the network call fails, so
    // always leave the authenticated shell. Cached pass tokens belong to the
    // signed-out session and must not survive it.
    clearAllPasses();
    await router.push({ name: "login" });
    busy.value = false;
  }
}
</script>

<template>
  <header
    class="sticky top-0 z-30 border-b border-ink/15 bg-paper/95 backdrop-blur"
  >
    <div
      class="mx-auto flex w-full max-w-6xl items-center gap-3 px-4 py-2.5 sm:px-6 lg:px-8"
    >
      <RouterLink :to="home" class="flex shrink-0 items-center gap-3">
        <img
          :src="logoUrl"
          alt="O’ Rivojo"
          class="h-8 w-auto sm:h-9"
        />
        <span
          class="hidden border-l border-ink/20 pl-3 text-[10px] leading-tight font-semibold tracking-[0.16em] text-ink-muted uppercase lg:block"
        >
          Oggetti smarriti<br />Roma
        </span>
      </RouterLink>

      <nav class="ml-4 hidden items-center gap-1 md:flex" aria-label="Navigazione principale">
        <RouterLink
          v-for="link in links"
          :key="link.to"
          :to="link.to"
          class="rounded-md px-3 py-2 text-sm font-semibold text-ink-soft transition hover:bg-paper-deep/60 hover:text-ink"
          active-class="bg-ink text-paper hover:bg-ink hover:text-paper"
        >
          {{ link.label }}
        </RouterLink>
      </nav>

      <div class="ml-auto flex items-center gap-2">
        <NotificationBell v-if="showNotifications" />
        <div v-if="user" class="hidden text-right sm:block">
          <p class="max-w-40 truncate text-sm font-semibold text-ink">
            {{ user.name }}
          </p>
          <p class="text-[11px] text-ink-muted">{{ roleLabels[user.role] }}</p>
        </div>
        <button
          type="button"
          class="btn btn-secondary px-3 py-2"
          :disabled="busy"
          @click="signOut"
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            class="h-4 w-4"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M15.75 9V5.25A2.25 2.25 0 0 0 13.5 3h-6A2.25 2.25 0 0 0 5.25 5.25v13.5A2.25 2.25 0 0 0 7.5 21h6a2.25 2.25 0 0 0 2.25-2.25V15M18 15l3-3m0 0-3-3m3 3H9"
            />
          </svg>
          <span class="hidden sm:inline">{{ busy ? "Esco…" : "Esci" }}</span>
          <span class="sr-only sm:hidden">Esci</span>
        </button>
      </div>
    </div>

    <nav
      v-if="links.length"
      class="mx-auto flex w-full max-w-6xl gap-1 overflow-x-auto px-4 pb-2 md:hidden"
      aria-label="Navigazione principale mobile"
    >
      <RouterLink
        v-for="link in links"
        :key="link.to"
        :to="link.to"
        class="shrink-0 rounded-md px-3 py-1.5 text-sm font-semibold text-ink-soft hover:bg-paper-deep/60"
        active-class="bg-ink text-paper hover:bg-ink hover:text-paper"
      >
        {{ link.label }}
      </RouterLink>
    </nav>
  </header>
</template>
