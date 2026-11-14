<script setup lang="ts">
import { RouterLink } from "vue-router";
import ItemEnrollForm from "../components/ItemEnrollForm.vue";
import { user } from "../api";
</script>

<template>
  <div class="mx-auto w-full max-w-3xl">
    <div class="mb-5">
      <p class="eyebrow">Intermediario</p>
      <h1 class="page-title mt-1">Registra un oggetto ricevuto</h1>
      <p class="page-subtitle">
        Registra ciò che hai ricevuto al banco come
        <span class="font-medium text-ink">{{ user?.name ?? "il tuo punto di raccolta" }}</span>.
        L'oggetto viene registrato in attesa di conferma: accetta la custodia
        dalla dashboard per avviare il termine di legge.
      </p>
    </div>

    <div class="banner-info mb-5">
      <p class="font-medium">Percorso mediato</p>
      <p class="mt-1 leading-6">
        In questo flusso il punto di raccolta registra l'oggetto come finder e
        intermediario. La custodia legale si conferma con l'accettazione dalla
        dashboard, che avvia anche il termine di conservazione. Compila la
        scheda insieme alla persona che ha consegnato l'oggetto.
      </p>
    </div>

    <ItemEnrollForm mode="intermediary" />

    <p class="mt-6 text-center text-xs text-ink-muted">
      <RouterLink to="/intermediary" class="link">Torna alla dashboard</RouterLink>
    </p>
  </div>
</template>
