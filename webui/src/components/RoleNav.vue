<script setup lang="ts">
import { computed, ref } from "vue";
import { RouterLink, useRouter } from "vue-router";
import { logout, user } from "../api";
import { roleLabels } from "../format";
import { homeForRole } from "../router";

const router = useRouter();
const busy = ref(false);

const links = computed(() => {
  switch (user.value?.role) {
    case "finder":
      return [{ to: "/finder", label: "Segnala un oggetto" }];
    case "intermediary":
      return [
        { to: "/intermediary", label: "Dashboard" },
        { to: "/intermediary/enroll", label: "Registra" },
        { to: "/intermediary/scan", label: "Scansiona" },
      ];
    default:
      return [];
  }
});

const home = computed(() => homeForRole(user.value?.role));

async function signOut() {
  busy.value = true;
  try {
    await logout();
    await router.push({ name: "login" });
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <header
    class="sticky top-0 z-30 border-b border-stone-200/80 bg-white/90 backdrop-blur"
  >
    <div
      class="mx-auto flex w-full max-w-6xl items-center gap-3 px-4 py-3 sm:px-6 lg:px-8"
    >
      <RouterLink :to="home" class="flex items-center gap-2.5">
        <span
          class="flex h-9 w-9 items-center justify-center rounded-xl bg-emerald-700 text-white shadow-sm"
          aria-hidden="true"
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            class="h-5 w-5"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="m21 21-4.35-4.35M17 10.5a6.5 6.5 0 1 1-13 0 6.5 6.5 0 0 1 13 0Z"
            />
          </svg>
        </span>
        <span class="leading-tight">
          <span class="block text-sm font-semibold tracking-tight text-stone-900"
            >Ritrova</span
          >
          <span class="block text-[11px] text-stone-500">Oggetti smarriti</span>
        </span>
      </RouterLink>

      <nav class="ml-4 hidden items-center gap-1 md:flex" aria-label="Navigazione principale">
        <RouterLink
          v-for="link in links"
          :key="link.to"
          :to="link.to"
          class="rounded-lg px-3 py-2 text-sm font-medium text-stone-600 hover:bg-stone-100 hover:text-stone-900"
          active-class="bg-emerald-50 text-emerald-800"
        >
          {{ link.label }}
        </RouterLink>
      </nav>

      <div class="ml-auto flex items-center gap-2">
        <div v-if="user" class="hidden text-right sm:block">
          <p class="max-w-40 truncate text-sm font-medium text-stone-900">
            {{ user.name }}
          </p>
          <p class="text-[11px] text-stone-500">{{ roleLabels[user.role] }}</p>
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
        class="shrink-0 rounded-lg px-3 py-1.5 text-sm font-medium text-stone-600 hover:bg-stone-100"
        active-class="bg-emerald-50 text-emerald-800"
      >
        {{ link.label }}
      </RouterLink>
    </nav>
  </header>
</template>
