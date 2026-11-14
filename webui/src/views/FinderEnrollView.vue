<script setup lang="ts">
import { RouterLink } from "vue-router";
import ItemEnrollForm from "../components/ItemEnrollForm.vue";
</script>

<template>
  <div class="mx-auto w-full max-w-3xl">
    <div class="mb-5 flex items-start justify-between gap-4">
      <div>
        <p class="eyebrow">Finder</p>
        <h1 class="page-title mt-1">Ho trovato un oggetto</h1>
        <p class="page-subtitle">
          Compila la scheda e consegna l'oggetto al punto di raccolta scelto.
          Ci vorrà meno di un minuto.
        </p>
      </div>
      <span
        class="hidden h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-emerald-50 text-emerald-700 sm:flex"
        aria-hidden="true"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.7"
          class="h-6 w-6"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M6.827 6.175A2.31 2.31 0 0 1 9.13 4.5h5.74a2.31 2.31 0 0 1 2.303 1.675l.128.45a2.25 2.25 0 0 1 1.928 1.325l.708 1.624a2.25 2.25 0 0 0 2.075 1.386H3a2.25 2.25 0 0 0 2.075-1.386l.708-1.624a2.25 2.25 0 0 1 1.928-1.325l.116-.45Z"
          />
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M3.75 12h16.5v6.75A2.25 2.25 0 0 1 18 21H6a2.25 2.25 0 0 1-2.25-2.25V12Z"
          />
        </svg>
      </span>
    </div>

    <div class="banner-info mb-5">
      <p class="font-medium">Prima di iniziare</p>
      <p class="mt-1 leading-6">
        Tieni l'oggetto con te fino alla consegna. Non pubblicare foto di
        documenti o dati personali: la scheda serve solo a riconoscere l'oggetto.
      </p>
    </div>

    <ItemEnrollForm mode="finder" />

    <p class="mt-6 text-center text-xs text-stone-400">
      Hai già consegnato l'oggetto?
      <RouterLink to="/" class="link">Torna alla home</RouterLink>
    </p>
  </div>
</template>
