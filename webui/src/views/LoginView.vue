<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { authError, login, user, type ApiError } from "../api";
import { roleLabels } from "../format";
import { homeForRole } from "../router";
import type { Role } from "../types";

interface DemoUser {
  id: string;
  name: string;
  role: Role;
  description: string;
  initials: string;
}

const demoUsers: DemoUser[] = [
  {
    id: "finder-demo",
    name: "Giulia Ferrari",
    role: "finder",
    description: "Ha trovato un oggetto e vuole consegnarlo al punto di raccolta.",
    initials: "GF",
  },
  {
    id: "bar-x",
    name: "Bar X",
    role: "intermediary",
    description: "Punto di raccolta di San Lorenzo: riceve e custodisce gli oggetti.",
    initials: "BX",
  },
  {
    id: "portineria-sapienza",
    name: "Portineria Sapienza",
    role: "intermediary",
    description: "Punto di raccolta della città universitaria.",
    initials: "PS",
  },
  {
    id: "searcher-demo",
    name: "Marco Bianchi",
    role: "searcher",
    description: "Cerca un oggetto perduto (area in arrivo).",
    initials: "MB",
  },
  {
    id: "institution-demo",
    name: "Comune di Roma",
    role: "institution",
    description: "Reparto oggetti rinvenuti (area in arrivo).",
    initials: "CR",
  },
  {
    id: "admin-demo",
    name: "Admin Demo",
    role: "admin",
    description: "Amministrazione della piattaforma (area in arrivo).",
    initials: "AD",
  },
];

const router = useRouter();
const route = useRoute();
const busyId = ref<string | null>(null);
const error = ref("");
const sessionWarning = computed(() => {
  const value = authError.value;
  return value && !user.value ? value : "";
});

function roleTone(role: Role): string {
  switch (role) {
    case "finder":
      return "bg-sky-50 text-sky-800 ring-sky-200";
    case "intermediary":
      return "bg-emerald-50 text-emerald-800 ring-emerald-200";
    default:
      return "bg-stone-100 text-stone-600 ring-stone-200";
  }
}

async function signIn(demo: DemoUser) {
  if (busyId.value) {
    return;
  }
  busyId.value = demo.id;
  error.value = "";
  try {
    const me = await login(demo.id);
    const redirect =
      typeof route.query.redirect === "string" &&
      route.query.redirect.startsWith("/")
        ? route.query.redirect
        : homeForRole(me.role);
    await router.replace(redirect);
  } catch (cause) {
    const apiCause = cause as ApiError;
    error.value =
      apiCause?.message || "Accesso non riuscito. Controlla la connessione e riprova.";
  } finally {
    busyId.value = null;
  }
}
</script>

<template>
  <div class="grid flex-1 lg:grid-cols-[1.1fr_1fr]">
    <section
      class="relative flex flex-col justify-between overflow-hidden bg-emerald-900 px-6 py-10 text-emerald-50 sm:px-10 lg:px-14 lg:py-16"
    >
      <div
        class="pointer-events-none absolute -top-24 -right-24 h-72 w-72 rounded-full bg-emerald-700/60 blur-3xl"
        aria-hidden="true"
      ></div>
      <div
        class="pointer-events-none absolute -bottom-32 -left-20 h-80 w-80 rounded-full bg-lime-600/20 blur-3xl"
        aria-hidden="true"
      ></div>

      <div class="relative">
        <div class="flex items-center gap-3">
          <span
            class="flex h-11 w-11 items-center justify-center rounded-2xl bg-emerald-50 text-emerald-900 shadow-lg"
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
                d="m21 21-4.35-4.35M17 10.5a6.5 6.5 0 1 1-13 0 6.5 6.5 0 0 1 13 0Z"
              />
            </svg>
          </span>
          <div>
            <p class="text-lg font-semibold tracking-tight">Ritrova</p>
            <p class="text-xs text-emerald-200">Oggetti smarriti · Roma</p>
          </div>
        </div>

        <h1 class="mt-10 max-w-lg text-3xl font-semibold leading-tight tracking-tight sm:text-4xl">
          Un oggetto trovato merita di tornare a casa.
        </h1>
        <p class="mt-4 max-w-md text-sm leading-6 text-emerald-100/90">
          Segnala un ritrovamento in meno di un minuto, consegnalo a un punto di
          raccolta di quartiere e lascia che sia la rete a ritrovare il
          proprietario.
        </p>

        <ul class="mt-8 space-y-3 text-sm text-emerald-50/90">
          <li class="flex items-start gap-3">
            <span class="mt-0.5 text-emerald-300" aria-hidden="true">✓</span>
            Foto, posizione e categoria: una scheda chiara per chi cerca.
          </li>
          <li class="flex items-start gap-3">
            <span class="mt-0.5 text-emerald-300" aria-hidden="true">✓</span>
            La posizione esatta e i dettagli privati restano riservati.
          </li>
          <li class="flex items-start gap-3">
            <span class="mt-0.5 text-emerald-300" aria-hidden="true">✓</span>
            Bar e portinerie di quartiere come presidio di prossimità.
          </li>
        </ul>
      </div>

      <p class="relative mt-10 max-w-md text-xs leading-5 text-emerald-200/70">
        Demo per AgenticHack Rome. Nessuna password: scegli un profilo dimostrativo
        per esplorare i flussi.
      </p>
    </section>

    <section class="flex items-center justify-center px-4 py-10 sm:px-8">
      <div class="w-full max-w-md">
        <div>
          <p class="eyebrow">Accesso demo</p>
          <h2 class="mt-2 text-2xl font-semibold tracking-tight text-stone-900">
            Scegli un profilo
          </h2>
          <p class="mt-1 text-sm text-stone-600">
            Ogni profilo mostra un pezzo del percorso di riconsegna.
          </p>
        </div>

        <p v-if="sessionWarning" class="banner-info mt-5" role="status">
          {{ sessionWarning }}
        </p>
        <p v-if="error" class="banner-error mt-5" role="alert">{{ error }}</p>

        <ul class="mt-6 space-y-2.5">
          <li v-for="demo in demoUsers" :key="demo.id">
            <button
              type="button"
              class="group flex w-full items-center gap-3 rounded-2xl border border-stone-200 bg-white p-3.5 text-left shadow-sm transition hover:border-emerald-300 hover:shadow-md disabled:cursor-wait disabled:opacity-70"
              :disabled="busyId !== null"
              @click="signIn(demo)"
            >
              <span
                class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-stone-100 text-sm font-semibold text-stone-700 group-hover:bg-emerald-50 group-hover:text-emerald-800"
                aria-hidden="true"
              >
                {{ demo.initials }}
              </span>
              <span class="min-w-0 flex-1">
                <span class="flex flex-wrap items-center gap-1.5">
                  <span class="truncate text-sm font-semibold text-stone-900">
                    {{ demo.name }}
                  </span>
                  <span class="badge" :class="roleTone(demo.role)">
                    {{ roleLabels[demo.role] }}
                  </span>
                </span>
                <span class="mt-0.5 block text-xs leading-5 text-stone-500">
                  {{ demo.description }}
                </span>
                <span class="mt-0.5 block font-mono text-[10px] text-stone-400">
                  {{ demo.id }}
                </span>
              </span>
              <span
                class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-stone-400 transition group-hover:bg-emerald-700 group-hover:text-white"
                aria-hidden="true"
              >
                <span
                  v-if="busyId === demo.id"
                  class="h-4 w-4 animate-spin rounded-full border-2 border-emerald-700 border-t-transparent"
                ></span>
                <svg
                  v-else
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  class="h-4 w-4"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                </svg>
              </span>
            </button>
          </li>
        </ul>

        <p class="mt-6 text-center text-xs leading-5 text-stone-400">
          Sessione cookie <code class="font-mono">lf_session</code> · durata 12 ore.
          I profili “area in arrivo” mostrano solo il guscio dell'app.
        </p>
      </div>
    </section>
  </div>
</template>
