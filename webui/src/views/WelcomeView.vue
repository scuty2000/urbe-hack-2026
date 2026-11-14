<script setup lang="ts">
import { computed } from "vue";
import { RouterLink, useRouter } from "vue-router";
import { logout, user } from "../api";
import { roleLabels } from "../format";
import { clearAllPasses } from "../passStore";

const router = useRouter();
const roleLabel = computed(() => (user.value ? roleLabels[user.value.role] : "Ospite"));

async function signOut() {
  try {
    await logout();
  } finally {
    clearAllPasses();
    await router.push({ name: "login" });
  }
}
</script>

<template>
  <div class="mx-auto flex w-full max-w-2xl flex-1 flex-col justify-center py-10">
    <div class="card text-center">
      <span
        class="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-emerald-50 text-emerald-700"
        aria-hidden="true"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          class="h-6 w-6"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M12 6v6l4 2m6-2a10 10 0 1 1-20 0 10 10 0 0 1 20 0Z"
          />
        </svg>
      </span>
      <p class="eyebrow mt-4">{{ roleLabel }}</p>
      <h1 class="mt-2 text-2xl font-semibold tracking-tight text-stone-900">
        La tua area è in arrivo
      </h1>
      <p class="mx-auto mt-2 max-w-md text-sm leading-6 text-stone-600">
        In questa demo il percorso è completo per finder, intermediari, chi cerca
        e pubblica amministrazione. Le funzioni per
        <span class="font-medium">{{ roleLabel.toLowerCase() }}</span>
        saranno presentate in una prossima versione dell'app.
      </p>

      <div class="mt-6 flex flex-col justify-center gap-2 sm:flex-row">
        <RouterLink to="/login" class="btn btn-secondary">Torna all'accesso</RouterLink>
        <button type="button" class="btn btn-ghost" @click="signOut">Esci</button>
      </div>
    </div>
  </div>
</template>
