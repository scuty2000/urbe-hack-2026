<script setup lang="ts">
import { computed } from "vue";
import { RouterView, useRoute } from "vue-router";
import RoleNav from "./components/RoleNav.vue";

const route = useRoute();
const chromeless = computed(() => route.meta.chromeless === true);
</script>

<template>
  <div class="flex min-h-dvh flex-col">
    <RoleNav v-if="!chromeless" />
    <main
      :class="
        chromeless
          ? 'flex flex-1 flex-col'
          : 'mx-auto w-full max-w-6xl flex-1 px-4 py-6 sm:px-6 lg:px-8'
      "
    >
      <RouterView />
    </main>
    <footer
      v-if="!chromeless"
      class="mx-auto w-full max-w-6xl px-4 pb-8 text-center text-xs leading-5 text-stone-400 sm:px-6 lg:px-8"
    >
      Ritrova · Demo Oggetti Smarriti. I dati mostrati sono di esempio.
    </footer>
  </div>
</template>
