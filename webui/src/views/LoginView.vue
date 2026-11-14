<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { authError, login, user, type ApiError } from "../api";
import DemoUserButton from "../components/DemoUserButton.vue";
import { homeForRole } from "../router";
import type { Role } from "../types";
import logoUrl from "../assets/brand/o-rivojo-lockup.svg";

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
    description: "Cerca un oggetto perduto e richiede il pass di ritiro.",
    initials: "MB",
  },
  {
    id: "institution-demo",
    name: "Comune di Roma",
    role: "institution",
    description: "Gestisce ritiri, intake ufficiale e verifica delle richieste.",
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

const partnerUsers: DemoUser[] = [
  {
    id: "tabaccheria-termini",
    name: "Tabaccheria Termini",
    role: "intermediary",
    description: "Punto partner vicino alla stazione Termini.",
    initials: "TT",
  },
  {
    id: "caffe-trastevere",
    name: "Caffè Trastevere",
    role: "intermediary",
    description: "Punto partner a Trastevere.",
    initials: "CT",
  },
  {
    id: "farmacia-san-giovanni",
    name: "Farmacia San Giovanni",
    role: "intermediary",
    description: "Punto partner a San Giovanni.",
    initials: "SG",
  },
  {
    id: "punto-prati",
    name: "Punto Prati",
    role: "intermediary",
    description: "Punto partner a Prati.",
    initials: "PP",
  },
  {
    id: "mercato-ostiense",
    name: "Mercato Ostiense",
    role: "intermediary",
    description: "Punto partner a Ostiense.",
    initials: "MO",
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
      return "bg-ochre-soft text-ochre ring-ochre/30";
    case "searcher":
      return "bg-vermilion-soft text-vermilion-dark ring-vermilion/30";
    case "intermediary":
      return "bg-olive-soft text-olive ring-olive/30";
    case "institution":
      return "bg-teal-soft text-teal ring-teal/25";
    default:
      return "bg-paper text-ink-soft ring-line";
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
  <div class="grid flex-1 lg:grid-cols-[1.08fr_1fr]">
    <section
      class="relative flex flex-col justify-between overflow-hidden border-b border-ink/15 bg-paper px-6 py-10 sm:px-10 lg:border-r lg:border-b-0 lg:px-14 lg:py-14"
    >
      <div class="relative">
        <img
          :src="logoUrl"
          alt="O’ Rivojo — Oggetti smarriti"
          class="w-full max-w-[25rem]"
        />

        <p class="eyebrow mt-10">Registro unico · Roma</p>
        <h1
          class="mt-3 max-w-lg font-display text-3xl leading-[0.96] tracking-tight text-ink uppercase sm:text-4xl lg:text-[2.9rem]"
        >
          Un oggetto trovato merita di tornare a casa.
        </h1>
        <p class="mt-4 max-w-md text-sm leading-6 text-ink-soft">
          Segnala un ritrovamento in meno di un minuto, consegnalo a un punto di
          raccolta di quartiere e lascia che sia la rete a ritrovare il
          proprietario.
        </p>

        <ul class="mt-8 space-y-3 text-sm text-ink-soft">
          <li class="flex items-start gap-3">
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.4"
              class="mt-1 h-4 w-4 shrink-0 text-vermilion-dark"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M20 5v7a4 4 0 0 1-4 4H5m4-5-4 5 4 5"
              />
            </svg>
            Foto, posizione e categoria: una scheda chiara per chi cerca.
          </li>
          <li class="flex items-start gap-3">
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.4"
              class="mt-1 h-4 w-4 shrink-0 text-vermilion-dark"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M20 5v7a4 4 0 0 1-4 4H5m4-5-4 5 4 5"
              />
            </svg>
            La posizione esatta e i dettagli privati restano riservati.
          </li>
          <li class="flex items-start gap-3">
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.4"
              class="mt-1 h-4 w-4 shrink-0 text-vermilion-dark"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M20 5v7a4 4 0 0 1-4 4H5m4-5-4 5 4 5"
              />
            </svg>
            Bar e portinerie di quartiere come presidio di prossimità.
          </li>
        </ul>
      </div>

      <p
        class="relative mt-10 max-w-md border-t border-ink/15 pt-4 text-xs leading-5 text-ink-muted"
      >
        Demo per AgenticHack Rome. Nessuna password: scegli un profilo dimostrativo
        per esplorare i flussi.
      </p>
    </section>

    <section class="flex items-center justify-center bg-paper-soft px-4 py-10 sm:px-8">
      <div class="w-full max-w-md">
        <div>
          <p class="eyebrow">Accesso demo</p>
          <h2
            class="mt-2 font-display text-2xl leading-none tracking-tight text-ink uppercase"
          >
            Scegli un profilo
          </h2>
          <p class="mt-2 text-sm text-ink-soft">
            Ogni profilo mostra un pezzo del percorso di riconsegna.
          </p>
        </div>

        <p v-if="sessionWarning" class="banner-info mt-5" role="status">
          {{ sessionWarning }}
        </p>
        <p v-if="error" class="banner-error mt-5" role="alert">{{ error }}</p>

        <ul class="mt-6 space-y-2.5">
          <li v-for="demo in demoUsers" :key="demo.id">
            <DemoUserButton
              :demo="demo"
              :busy="busyId === demo.id"
              :tone="roleTone(demo.role)"
              @select="signIn"
            />
          </li>
        </ul>

        <details class="mt-3 rounded-lg border border-line bg-paper-card p-2">
          <summary
            class="cursor-pointer px-3 py-2 text-sm font-semibold text-ink-soft"
          >
            Altri punti di raccolta ({{ partnerUsers.length }})
          </summary>
          <ul class="mt-2 space-y-2.5">
            <li v-for="demo in partnerUsers" :key="demo.id">
              <DemoUserButton
                :demo="demo"
                :busy="busyId === demo.id"
                :tone="roleTone(demo.role)"
                @select="signIn"
              />
            </li>
          </ul>
        </details>

        <p class="mt-6 text-center text-xs leading-5 text-ink-muted">
          Sessione cookie <code class="font-mono">lf_session</code> · durata 12 ore.
          I profili “area in arrivo” mostrano solo il guscio dell'app.
        </p>
      </div>
    </section>
  </div>
</template>
