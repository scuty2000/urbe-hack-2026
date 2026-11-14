<script setup lang="ts">
import { computed, ref } from "vue";
import { RouterLink, useRouter } from "vue-router";
import { ApiError, createItem, user } from "../api";
import { displayItemName, formatDateTime } from "../format";
import { saveReceipt } from "../receipt";
import type { CreateItemPayload, Item } from "../types";
import GeoPicker from "./GeoPicker.vue";
import PhotoUpload from "./PhotoUpload.vue";
import RightsAssignment from "./RightsAssignment.vue";
import StatusBadge from "./StatusBadge.vue";

const props = defineProps<{
  mode: "finder" | "intermediary";
}>();

const emit = defineEmits<{
  created: [item: Item];
}>();

const router = useRouter();

const dropPoints = [
  {
    id: "bar-x",
    name: "Bar X",
    area: "San Lorenzo",
    address: "Via dei Volsci 12, Roma",
    hours: "07:00–21:00",
  },
  {
    id: "portineria-sapienza",
    name: "Portineria Sapienza",
    area: "Città universitaria",
    address: "Piazzale Aldo Moro 5, Roma",
    hours: "08:00–20:00",
  },
] as const;

const categories = [
  { value: "smartphone", label: "Smartphone" },
  { value: "portafoglio", label: "Portafoglio" },
  { value: "documenti", label: "Documenti / carta d'identità" },
  { value: "chiavi", label: "Chiavi" },
  { value: "ombrello", label: "Ombrello" },
  { value: "zaino", label: "Zaino / borsa" },
  { value: "occhiali", label: "Occhiali" },
  { value: "cuffie", label: "Cuffie / auricolari" },
  { value: "abbigliamento", label: "Abbigliamento" },
  { value: "bicicletta", label: "Bicicletta" },
  { value: "altro", label: "Altro" },
];

const colors = [
  "nero",
  "bianco",
  "grigio",
  "blu",
  "rosso",
  "verde",
  "giallo",
  "marrone",
  "rosa",
  "multicolore",
];

const conditions = [
  { value: "like_new", label: "Come nuovo" },
  { value: "good", label: "Buono" },
  { value: "fair", label: "Discreto" },
  { value: "poor", label: "Da riparare" },
];

function nowForInput(): string {
  const now = new Date();
  const offset = now.getTimezoneOffset();
  const local = new Date(now.getTime() - offset * 60_000);
  return local.toISOString().slice(0, 16);
}

const photos = ref<File[]>([]);
const category = ref("");
const customCategory = ref("");
const brand = ref("");
const model = ref("");
const color = ref("");
const condition = ref("good");
const foundAt = ref(nowForInput());
const notes = ref("");
const privateDetails = ref("");
const dropPoint = ref<string>("bar-x");
const lat = ref<number | null>(null);
const lng = ref<number | null>(null);
const zone = ref("");
const rights = ref(false);

const submitting = ref(false);
const error = ref("");
const fieldErrors = ref<Record<string, string>>({});
const created = ref<Item | null>(null);

const isFinder = computed(() => props.mode === "finder");
const selectedDropPoint = computed(() =>
  dropPoints.find((point) => point.id === dropPoint.value),
);
const effectiveCategory = computed(() =>
  category.value === "altro" ? customCategory.value.trim() : category.value,
);
const intermediaryName = computed(() => user.value?.name ?? "il tuo punto di raccolta");

const createdDropPoint = computed(() => {
  if (!created.value) {
    return selectedDropPoint.value;
  }
  return dropPoints.find((point) => point.id === created.value?.custodianId);
});

function validate(): boolean {
  const errors: Record<string, string> = {};

  if (photos.value.length === 0) {
    errors.photos = "Aggiungi almeno una foto dell'oggetto.";
  }
  if (photos.value.length > 3) {
    errors.photos = "Puoi aggiungere al massimo 3 foto.";
  }
  if (!effectiveCategory.value) {
    errors.category = "Scegli una categoria o descrivila in “Altro”.";
  }
  if (isFinder.value && !dropPoint.value) {
    errors.dropPoint = "Scegli il punto di raccolta.";
  }
  if ((lat.value === null) !== (lng.value === null)) {
    errors.geo = "Inserisci sia la latitudine sia la longitudine, oppure nessuna.";
  }
  if (lat.value !== null && Math.abs(lat.value) > 90) {
    errors.geo = "La latitudine deve essere compresa tra -90 e 90.";
  }
  if (lng.value !== null && Math.abs(lng.value) > 180) {
    errors.geo = "La longitudine deve essere compresa tra -180 e 180.";
  }
  if (!rights.value) {
    errors.rights =
      "Devi accettare la cessione della qualità di ritrovatore e la conferma della custodia dalla dashboard per continuare.";
  }
  if (foundAt.value && Number.isNaN(new Date(foundAt.value).getTime())) {
    errors.foundAt = "La data del ritrovamento non è valida.";
  }

  fieldErrors.value = errors;
  if (Object.keys(errors).length > 0) {
    error.value = "Controlla i campi evidenziati prima di inviare.";
    return false;
  }
  return true;
}

function buildPayload(): CreateItemPayload {
  const payload: CreateItemPayload = {
    category: effectiveCategory.value,
    brand: brand.value.trim() || undefined,
    model: model.value.trim() || undefined,
    color: color.value || undefined,
    condition: condition.value || undefined,
    foundAt: foundAt.value
      ? new Date(foundAt.value).toISOString()
      : new Date().toISOString(),
    foundZone: zone.value.trim() || undefined,
    foundNotes: notes.value.trim() || undefined,
    rightsAssigned: rights.value,
  };

  if (isFinder.value) {
    payload.intermediaryId = dropPoint.value;
  }
  if (lat.value !== null && lng.value !== null) {
    payload.foundLat = lat.value;
    payload.foundLng = lng.value;
  }
  const privateText = privateDetails.value.trim();
  if (privateText) {
    payload.privateProfile = { details: privateText };
  }

  return payload;
}

async function submit() {
  error.value = "";

  if (!validate()) {
    window.scrollTo({ top: 0, behavior: "smooth" });
    return;
  }

  submitting.value = true;
  try {
    const item = await createItem(buildPayload(), photos.value);
    emit("created", item);

    if (isFinder.value) {
      saveReceipt(item);
      await router.push({ name: "finder-receipt", params: { id: item.id } });
      return;
    }

    created.value = item;
    window.scrollTo({ top: 0, behavior: "smooth" });
  } catch (cause) {
    error.value =
      cause instanceof ApiError
        ? cause.message
        : "Non sono riuscito a registrare l'oggetto. Riprova.";
  } finally {
    submitting.value = false;
  }
}

function resetForm() {
  created.value = null;
  photos.value = [];
  category.value = "";
  customCategory.value = "";
  brand.value = "";
  model.value = "";
  color.value = "";
  condition.value = "good";
  foundAt.value = nowForInput();
  notes.value = "";
  privateDetails.value = "";
  lat.value = null;
  lng.value = null;
  zone.value = "";
  rights.value = false;
  error.value = "";
  fieldErrors.value = {};
}
</script>

<template>
  <div>
    <!-- Intermediary success panel: the item stays pending until the dashboard
         performs the explicit custody acceptance. -->
    <section v-if="created" class="space-y-5">
      <div class="banner-success">
        <p class="font-semibold">Oggetto registrato correttamente.</p>
        <p class="mt-1">
          Il codice pubblico è <span class="font-mono font-semibold">{{ created.publicCode }}</span>.
          La scheda è in attesa di conferma: accetta la custodia dalla dashboard
          per avviare il termine di conservazione.
        </p>
      </div>

      <div class="card">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p class="eyebrow">Ricevuta di registrazione</p>
            <h2 class="mt-1 text-xl font-semibold text-stone-900">
              {{ displayItemName(created) }}
            </h2>
            <p class="mt-1 font-mono text-sm text-stone-600">
              {{ created.publicCode }}
            </p>
          </div>
          <StatusBadge :status="created.status" :sensitive="created.sensitive" />
        </div>

        <dl class="mt-5 grid grid-cols-2 gap-4 text-sm">
          <div>
            <dt class="data-label">Categoria</dt>
            <dd class="data-value">{{ created.category || "—" }}</dd>
          </div>
          <div>
            <dt class="data-label">Registrato il</dt>
            <dd class="data-value">{{ formatDateTime(created.createdAt) }}</dd>
          </div>
          <div>
            <dt class="data-label">Punto di raccolta</dt>
            <dd class="data-value">
              {{ createdDropPoint?.name ?? created.custodianId ?? intermediaryName }}
            </dd>
          </div>
          <div>
            <dt class="data-label">Zona</dt>
            <dd class="data-value">{{ created.foundZone || "—" }}</dd>
          </div>
        </dl>

        <div
          v-if="created.sensitive"
          class="mt-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900"
        >
          Oggetto sensibile: dopo l'accettazione della custodia il termine è di
          24 ore. Conservalo in un luogo sicuro e non condividerne i dettagli.
        </div>

        <div class="mt-5 flex flex-col gap-2 sm:flex-row">
          <button type="button" class="btn btn-primary" @click="router.push('/intermediary')">
            Vai alla dashboard e accetta
          </button>
          <button type="button" class="btn btn-secondary" @click="resetForm">
            Registra un altro oggetto
          </button>
        </div>
      </div>
    </section>

    <form v-else class="space-y-5" novalidate @submit.prevent="submit">
      <div v-if="error" class="banner-error" role="alert">{{ error }}</div>

      <section class="card space-y-4">
        <div class="flex items-center gap-3">
          <span class="step-dot">1</span>
          <div>
            <h2 class="section-title">Foto e descrizione</h2>
            <p class="text-xs text-stone-500">
              Bastano una o due foto nitide: aiutano la riconsegna.
            </p>
          </div>
        </div>

        <PhotoUpload v-model="photos" :max="3" :disabled="submitting" />
        <p v-if="fieldErrors.photos" class="text-xs font-medium text-rose-700">
          {{ fieldErrors.photos }}
        </p>

        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="label" for="category">Categoria</label>
            <select
              id="category"
              v-model="category"
              class="input"
              :disabled="submitting"
              :aria-invalid="Boolean(fieldErrors.category)"
            >
              <option value="" disabled>Scegli una categoria</option>
              <option v-for="option in categories" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
            <p v-if="fieldErrors.category" class="mt-1 text-xs font-medium text-rose-700">
              {{ fieldErrors.category }}
            </p>
          </div>
          <div v-if="category === 'altro'">
            <label class="label" for="custom-category">Descrivi la categoria</label>
            <input
              id="custom-category"
              v-model="customCategory"
              class="input"
              type="text"
              maxlength="80"
              placeholder="Es. strumento musicale"
              :disabled="submitting"
            />
          </div>
          <div>
            <label class="label" for="brand">Marca <span class="font-normal text-stone-400">(facoltativo)</span></label>
            <input
              id="brand"
              v-model="brand"
              class="input"
              type="text"
              maxlength="80"
              autocomplete="off"
              placeholder="Es. Apple"
              :disabled="submitting"
            />
          </div>
          <div>
            <label class="label" for="model">Modello <span class="font-normal text-stone-400">(facoltativo)</span></label>
            <input
              id="model"
              v-model="model"
              class="input"
              type="text"
              maxlength="80"
              autocomplete="off"
              placeholder="Es. iPhone 15"
              :disabled="submitting"
            />
          </div>
          <div>
            <label class="label" for="color">Colore</label>
            <select id="color" v-model="color" class="input" :disabled="submitting">
              <option value="">Non indicato</option>
              <option v-for="value in colors" :key="value" :value="value">
                {{ value.charAt(0).toUpperCase() + value.slice(1) }}
              </option>
            </select>
          </div>
          <div>
            <label class="label" for="condition">Condizioni</label>
            <select id="condition" v-model="condition" class="input" :disabled="submitting">
              <option v-for="option in conditions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </div>
        </div>
      </section>

      <section class="card space-y-4">
        <div class="flex items-center gap-3">
          <span class="step-dot">2</span>
          <div>
            <h2 class="section-title">Dove e quando</h2>
            <p class="text-xs text-stone-500">
              La posizione esatta è privata; la zona aiuta chi cerca.
            </p>
          </div>
        </div>

        <div>
          <label class="label" for="found-at">Data e ora del ritrovamento</label>
          <input
            id="found-at"
            v-model="foundAt"
            class="input"
            type="datetime-local"
            :disabled="submitting"
          />
          <p v-if="fieldErrors.foundAt" class="mt-1 text-xs font-medium text-rose-700">
            {{ fieldErrors.foundAt }}
          </p>
        </div>

        <GeoPicker
          v-model:lat="lat"
          v-model:lng="lng"
          v-model:zone="zone"
          :disabled="submitting"
        />
        <p v-if="fieldErrors.geo" class="text-xs font-medium text-rose-700">
          {{ fieldErrors.geo }}
        </p>

        <div>
          <label class="label" for="notes">Note contestuali <span class="font-normal text-stone-400">(facoltative)</span></label>
          <textarea
            id="notes"
            v-model="notes"
            class="input min-h-20 resize-y"
            maxlength="500"
            rows="3"
            placeholder="Es. trovato su una panchina davanti alla stazione"
            :disabled="submitting"
          ></textarea>
        </div>
      </section>

      <section class="card space-y-4">
        <div class="flex items-center gap-3">
          <span class="step-dot">3</span>
          <div>
            <h2 class="section-title">Dettagli riservati</h2>
            <p class="text-xs text-stone-500">
              Visibili solo agli operatori autorizzati, mai pubblicati.
            </p>
          </div>
        </div>
        <div>
          <label class="label" for="private-details">
            Segni particolari o contenuto
            <span class="font-normal text-stone-400">(facoltativo)</span>
          </label>
          <textarea
            id="private-details"
            v-model="privateDetails"
            class="input min-h-20 resize-y"
            maxlength="500"
            rows="3"
            placeholder="Es. graffio sull'angolo, nome inciso sul retro, colore della custodia"
            :disabled="submitting"
          ></textarea>
          <p class="hint">
            Questi dettagli non compaiono nella ricevuta e non vengono mostrati
            a chi cerca l'oggetto.
          </p>
        </div>
      </section>

      <section v-if="isFinder" class="card space-y-4">
        <div class="flex items-center gap-3">
          <span class="step-dot">4</span>
          <div>
            <h2 class="section-title">Punto di raccolta</h2>
            <p class="text-xs text-stone-500">
              Consegna l'oggetto al punto scelto: confermerà la presa in carico
              dalla dashboard.
            </p>
          </div>
        </div>

        <label
          v-for="point in dropPoints"
          :key="point.id"
          class="flex cursor-pointer gap-3 rounded-xl border p-3 transition"
          :class="
            dropPoint === point.id
              ? 'border-emerald-500 bg-emerald-50/60 ring-2 ring-emerald-500/20'
              : 'border-stone-200 bg-white hover:border-stone-300'
          "
        >
          <input
            v-model="dropPoint"
            type="radio"
            name="drop-point"
            :value="point.id"
            class="mt-1 h-4 w-4 shrink-0 text-emerald-700 focus:ring-emerald-600"
            :disabled="submitting"
          />
          <span class="min-w-0">
            <span class="block text-sm font-semibold text-stone-900">{{ point.name }}</span>
            <span class="block text-xs text-stone-600">
              {{ point.address }} · {{ point.area }}
            </span>
            <span class="mt-0.5 block text-xs text-stone-500">Aperto {{ point.hours }}</span>
          </span>
        </label>
        <p v-if="fieldErrors.dropPoint" class="text-xs font-medium text-rose-700">
          {{ fieldErrors.dropPoint }}
        </p>
      </section>

      <section class="card space-y-3">
        <div class="flex items-start gap-3 rounded-xl bg-stone-50 p-3 text-sm text-stone-700">
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            class="mt-0.5 h-5 w-5 shrink-0 text-emerald-700"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M9 12.75 11.25 15 15 9.75M21 12c0 1.268-.63 2.39-1.593 3.068a3.745 3.745 0 0 1-1.043 3.296 3.745 3.745 0 0 1-3.296 1.043A3.745 3.745 0 0 1 12 21c-1.268 0-2.39-.63-3.068-1.593a3.746 3.746 0 0 1-3.296-1.043 3.745 3.745 0 0 1-1.043-3.296A3.745 3.745 0 0 1 3 12c0-1.268.63-2.39 1.593-3.068a3.745 3.745 0 0 1 1.043-3.296 3.746 3.746 0 0 1 3.296-1.043A3.746 3.746 0 0 1 12 3c1.268 0 2.39.63 3.068 1.593a3.746 3.746 0 0 1 3.296 1.043 3.745 3.745 0 0 1 1.043 3.296A3.745 3.745 0 0 1 21 12Z"
            />
          </svg>
          <span v-if="isFinder">
            Dopo l'invio riceverai un codice oggetto. Conservalo: il finder non
            può più consultare la scheda dell'oggetto.
          </span>
          <span v-else>
            L'oggetto risulta già materialmente presso {{ intermediaryName }}.
            La registrazione crea la scheda in attesa di conferma: accetta la
            custodia dalla dashboard per avviare il termine di conservazione.
          </span>
        </div>

        <RightsAssignment v-model="rights" :mode="mode" :disabled="submitting" />
        <p v-if="fieldErrors.rights" class="text-xs font-medium text-rose-700">
          {{ fieldErrors.rights }}
        </p>
      </section>

      <div class="flex flex-col gap-3 sm:flex-row-reverse sm:justify-start">
        <button type="submit" class="btn btn-primary w-full sm:w-auto" :disabled="submitting">
          <span
            v-if="submitting"
            class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"
            aria-hidden="true"
          ></span>
          {{ submitting ? "Invio in corso…" : "Registra l'oggetto" }}
        </button>
        <RouterLink to="/" class="btn btn-ghost w-full sm:w-auto">Annulla</RouterLink>
      </div>
    </form>
  </div>
</template>

<style scoped>
.step-dot {
  display: inline-flex;
  height: 1.75rem;
  width: 1.75rem;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 9999px;
  background-color: #d1fae5;
  color: #065f46;
  font-size: 0.75rem;
  font-weight: 700;
}
</style>
