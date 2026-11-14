<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from "vue";
import { useRouter } from "vue-router";
import {
  ApiError,
  analyzeWizardPhoto,
  createItem,
  nearestDropPoints,
  reverseGeocode,
} from "../api";
import {
  itemCategories,
  itemColors,
  itemConditions,
  localDateTimeValue as nowForInput,
} from "../catalog";
import { formatDateTime, resizeToJpeg } from "../format";
import { saveReceipt } from "../receipt";
import type {
  CreateItemPayload,
  DropPoint,
  WizardAnalysis,
  WizardPhotoSlot,
} from "../types";
import GeoPicker from "../components/GeoPicker.vue";
import RightsAssignment from "../components/RightsAssignment.vue";
import PhotoPlanSlot from "../components/wizard/PhotoPlanSlot.vue";
import WizardProgress from "../components/wizard/WizardProgress.vue";

const router = useRouter();
const steps = ["Foto", "Dove", "Dettagli", "Punto", "Conferma"];
const step = ref(1);

const firstPhoto = ref<File | null>(null);
const firstPreview = ref<string | null>(null);
const firstInput = ref<HTMLInputElement | null>(null);
const processingFirst = ref(false);

const analysis = ref<WizardAnalysis | null>(null);
const analyzing = ref(false);
const analysisError = ref("");
const slotFiles = reactive<Record<string, File | null>>({});

const category = ref("");
const customCategory = ref("");
const brand = ref("");
const model = ref("");
const color = ref("");
const condition = ref("good");
const privateDetails = ref("");
const touched = reactive({
  category: false,
  color: false,
  condition: false,
  private: false,
  zone: false,
});

const foundAt = ref(nowForInput());
const lat = ref<number | null>(null);
const lng = ref<number | null>(null);
const zone = ref("");
const notes = ref("");
const reverseLoading = ref(false);
const reverseSource = ref("");

const dropPoints = ref<DropPoint[]>([]);
const dropPoint = ref("");
const dropLoading = ref(false);
const dropError = ref("");
const dropSource = ref("");

const rights = ref(false);
const submitting = ref(false);
const submitError = ref("");
const fieldErrors = ref<Record<string, string>>({});

let reverseTimer: ReturnType<typeof setTimeout> | null = null;

const isDocument = computed(() => analysis.value?.isDocument ?? false);
const effectiveCategory = computed(() =>
  category.value === "altro" ? customCategory.value.trim() : category.value,
);
const plan = computed<WizardPhotoSlot[]>(() => analysis.value?.photoPlan ?? []);
const selectedDropPoint = computed(() =>
  dropPoints.value.find((point) => point.id === dropPoint.value),
);
const sourceLabel = computed(() => {
  switch (analysis.value?.source) {
    case "d3+deepseek":
      return "Analisi D3 + DeepSeek";
    case "d3+fallback":
      return "Analisi D3 + regole locali";
    case "d3":
      return "Analisi D3";
    case "manual":
      return "Compilazione manuale";
    default:
      return "";
  }
});
const allPhotos = computed<File[]>(() => {
  const files: File[] = [];
  if (firstPhoto.value) {
    files.push(firstPhoto.value);
  }
  for (const slot of plan.value) {
    const file = slotFiles[slot.key];
    if (file) {
      files.push(file);
    }
  }
  return files;
});

function clearFieldErrors() {
  fieldErrors.value = {};
  analysisError.value = "";
  submitError.value = "";
}

function setFirstPhoto(file: File) {
  if (firstPreview.value) {
    URL.revokeObjectURL(firstPreview.value);
  }
  firstPhoto.value = file;
  firstPreview.value = URL.createObjectURL(file);
  resetAnalysis();
  void analyze();
}

function resetAnalysis() {
  analysis.value = null;
  analysisError.value = "";
  for (const key of Object.keys(slotFiles)) {
    delete slotFiles[key];
  }
  if (!touched.category) {
    category.value = "";
  }
  if (!touched.color) {
    color.value = "";
  }
  if (!touched.condition) {
    condition.value = "good";
  }
  if (!touched.private) {
    privateDetails.value = "";
  }
}

async function onFirstSelect(event: Event) {
  const target = event.target as HTMLInputElement;
  const file = target.files?.[0];
  target.value = "";
  if (!file) {
    return;
  }
  processingFirst.value = true;
  try {
    setFirstPhoto(await resizeToJpeg(file));
  } catch {
    analysisError.value = "Non riesco a elaborare questa foto. Provane un'altra.";
  } finally {
    processingFirst.value = false;
  }
}

async function analyze() {
  if (!firstPhoto.value) {
    analysisError.value = "Aggiungi prima una foto dell'oggetto.";
    return;
  }
  analyzing.value = true;
  analysisError.value = "";
  try {
    const result = await analyzeWizardPhoto({ slot: "hero" }, [firstPhoto.value]);
    analysis.value = result;
    applyAnalysis(result);
  } catch (cause) {
    analysisError.value =
      cause instanceof ApiError
        ? cause.message
        : "Analisi non disponibile: puoi compilare i campi manualmente.";
    const manual: WizardAnalysis = {
      objectType: "altro",
      confidence: 0,
      photoPlan: [
        {
          key: "detail",
          label: "Dettaglio",
          instruction: "Fotografa un dettaglio o un segno particolare (facoltativo).",
          required: false,
        },
      ],
      prefill: { category: "", color: "", condition: "good" },
      privatePrefill: "",
      warnings: ["Analisi automatica non disponibile: compila i campi manualmente."],
      source: "manual",
      isDocument: false,
      sensitive: false,
    };
    analysis.value = manual;
    applyAnalysis(manual);
  } finally {
    analyzing.value = false;
  }
}

function applyAnalysis(result: WizardAnalysis) {
  if (!touched.category) {
    category.value = result.prefill.category || "";
  }
  if (!touched.color) {
    color.value = result.prefill.color || "";
  }
  if (!touched.condition) {
    condition.value = result.prefill.condition || "good";
  }
  if (!touched.private) {
    privateDetails.value = result.privatePrefill || "";
  }
  if (result.isDocument) {
    category.value = "documenti";
  }
  for (const slot of result.photoPlan) {
    if (!(slot.key in slotFiles)) {
      slotFiles[slot.key] = null;
    }
  }
}

function onCategoryChange() {
  touched.category = true;
}
function onColorChange() {
  touched.color = true;
}
function onConditionChange() {
  touched.condition = true;
}
function onPrivateInput(event: Event) {
  touched.private = true;
  privateDetails.value = (event.target as HTMLTextAreaElement).value;
}
function appendPrivate(text: string) {
  touched.private = true;
  const current = privateDetails.value.trim();
  privateDetails.value = current ? `${current}, ${text}` : text;
}

watch(
  () => [lat.value, lng.value] as const,
  ([nextLat, nextLng]) => {
    if (reverseTimer) {
      clearTimeout(reverseTimer);
      reverseTimer = null;
    }
    if (step.value === 4) {
      void loadDropPoints();
    }
    if (nextLat === null || nextLng === null || touched.zone) {
      return;
    }
    reverseTimer = setTimeout(async () => {
      reverseLoading.value = true;
      try {
        const place = await reverseGeocode(nextLat, nextLng);
        reverseSource.value = place.source;
        if (place.zone && !touched.zone) {
          zone.value = place.zone;
        }
      } catch {
        reverseSource.value = "unavailable";
      } finally {
        reverseLoading.value = false;
      }
    }, 450);
  },
);

function onZoneInput(value: string) {
  touched.zone = true;
  zone.value = value;
}

watch(step, (value) => {
  if (value === 4) {
    void loadDropPoints();
  }
});

async function loadDropPoints() {
  dropError.value = "";
  dropLoading.value = true;
  try {
    const response = await nearestDropPoints(
      lat.value ?? undefined,
      lng.value ?? undefined,
      5,
    );
    dropPoints.value = response.points;
    dropSource.value = response.source;
    if (!dropPoints.value.some((point) => point.id === dropPoint.value)) {
      dropPoint.value = dropPoints.value[0]?.id ?? "";
    }
    if (dropPoints.value.length === 0) {
      dropError.value = "Nessun punto di raccolta attivo. Riprova tra poco.";
    }
  } catch (cause) {
    dropError.value =
      cause instanceof ApiError ? cause.message : "Non riesco a calcolare i punti di raccolta.";
    dropPoints.value = [];
    dropPoint.value = "";
    dropSource.value = "";
  } finally {
    dropLoading.value = false;
  }
}

function validateStep1(): boolean {
  const errors: Record<string, string> = {};
  if (!firstPhoto.value) {
    errors.photos = "Aggiungi la prima foto dell'oggetto.";
  }
  if (!effectiveCategory.value) {
    errors.category = "Scegli una categoria o descrivila in “Altro”.";
  }
  if (isDocument.value && category.value !== "documenti") {
    errors.category = "Un documento resta nella categoria Documenti.";
  }
  for (const slot of plan.value) {
    if (slot.required && !slotFiles[slot.key]) {
      errors[`slot:${slot.key}`] = `Aggiungi la foto “${slot.label}”.`;
    }
  }
  fieldErrors.value = errors;
  return Object.keys(errors).length === 0;
}

function validateStep2(): boolean {
  const errors: Record<string, string> = {};
  if (!zone.value.trim()) {
    errors.zone = "Indica la zona del ritrovamento.";
  }
  if (!foundAt.value || Number.isNaN(new Date(foundAt.value).getTime())) {
    errors.foundAt = "La data del ritrovamento non è valida.";
  }
  if ((lat.value === null) !== (lng.value === null)) {
    errors.geo = "Inserisci sia la latitudine sia la longitudine, oppure nessuna.";
  }
  fieldErrors.value = errors;
  return Object.keys(errors).length === 0;
}

function validateStep4(): boolean {
  const errors: Record<string, string> = {};
  if (!dropPoint.value) {
    errors.dropPoint = "Scegli il punto di raccolta.";
  }
  fieldErrors.value = errors;
  return Object.keys(errors).length === 0;
}

function next() {
  clearFieldErrors();
  if (step.value === 1 && !validateStep1()) {
    return;
  }
  if (step.value === 2 && !validateStep2()) {
    return;
  }
  if (step.value === 3 && !validateStep1()) {
    return;
  }
  if (step.value === 4 && !validateStep4()) {
    return;
  }
  if (step.value < 5) {
    step.value += 1;
    window.scrollTo({ top: 0, behavior: "smooth" });
  }
}

function back() {
  clearFieldErrors();
  if (step.value > 1) {
    step.value -= 1;
    window.scrollTo({ top: 0, behavior: "smooth" });
  }
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
    intermediaryId: dropPoint.value || undefined,
  };
  if (lat.value !== null && lng.value !== null) {
    payload.foundLat = lat.value;
    payload.foundLng = lng.value;
  }
  const privateText = privateDetails.value.trim();
  const privateProfile: Record<string, unknown> = {};
  if (privateText) {
    privateProfile.details = privateText;
  }
  // The structured D3 observations are the only private attributes the claim
  // engine can verify; free-text details are never used as questions.
  if (analysis.value?.privateAttributes) {
    Object.assign(privateProfile, analysis.value.privateAttributes);
  }
  if (Object.keys(privateProfile).length > 0) {
    payload.privateProfile = privateProfile;
  }
  return payload;
}

async function submit() {
  clearFieldErrors();
  if (!validateStep1()) {
    step.value = 1;
    return;
  }
  if (!validateStep2()) {
    step.value = 2;
    return;
  }
  if (!validateStep4()) {
    step.value = 4;
    return;
  }
  if (!rights.value) {
    fieldErrors.value = {
      rights: "Devi accettare la cessione della qualità di ritrovatore e il disclaimer.",
    };
    return;
  }

  submitting.value = true;
  try {
    const item = await createItem(buildPayload(), allPhotos.value);
    saveReceipt(item);
    await router.push({ name: "finder-receipt", params: { id: item.id } });
  } catch (cause) {
    submitError.value =
      cause instanceof ApiError
        ? cause.message
        : "Non sono riuscito a registrare l'oggetto. Riprova.";
  } finally {
    submitting.value = false;
  }
}

onBeforeUnmount(() => {
  if (reverseTimer) {
    clearTimeout(reverseTimer);
  }
  if (firstPreview.value) {
    URL.revokeObjectURL(firstPreview.value);
  }
});
</script>

<template>
  <div class="mx-auto w-full max-w-3xl">
    <div class="mb-5 flex items-start justify-between gap-4">
      <div>
        <p class="eyebrow">Finder</p>
        <h1 class="page-title mt-1">Ho trovato un oggetto</h1>
        <p class="page-subtitle">
          Una foto, poi ti guidiamo noi. Puoi controllare e correggere ogni campo.
        </p>
      </div>
      <span
        class="hidden h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-emerald-50 text-emerald-700 sm:flex"
        aria-hidden="true"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" class="h-6 w-6">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6.827 6.175A2.31 2.31 0 0 1 9.13 4.5h5.74a2.31 2.31 0 0 1 2.303 1.675l.128.45a2.25 2.25 0 0 1 1.928 1.325l.708 1.624a2.25 2.25 0 0 0 2.075 1.386H3a2.25 2.25 0 0 0 2.075-1.386l.708-1.624a2.25 2.25 0 0 1 1.928-1.325l.116-.45Z" />
          <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 12h16.5v6.75A2.25 2.25 0 0 1 18 21H6a2.25 2.25 0 0 1-2.25-2.25V12Z" />
        </svg>
      </span>
    </div>

    <WizardProgress :step="step" :steps="steps" />

    <div v-if="analysisError" class="banner-error mt-4" role="alert">{{ analysisError }}</div>
    <div v-if="submitError" class="banner-error mt-4" role="alert">{{ submitError }}</div>

    <form class="mt-5 space-y-5" novalidate @submit.prevent="submit">
      <!-- Step 1 -->
      <section v-if="step === 1" class="card space-y-4">
        <div class="flex items-center gap-3">
          <span class="step-dot">1</span>
          <div>
            <h2 class="section-title">Foto e oggetto</h2>
            <p class="text-xs text-stone-500">La prima foto è il punto di partenza: da lì capiamo cosa chiederti.</p>
          </div>
        </div>

        <div class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-950">
          Non fotografare documenti, notifiche o dati personali. Per un documento
          fotografa solo l'esterno e consegnalo al punto di raccolta.
        </div>

        <div>
          <input
            ref="firstInput"
            class="sr-only"
            type="file"
            accept="image/*"
            capture="environment"
            :disabled="processingFirst || analyzing"
            @change="onFirstSelect"
          />
          <div v-if="firstPreview" class="flex flex-wrap items-center gap-3">
            <img :src="firstPreview" alt="Prima foto dell'oggetto" class="h-28 w-28 rounded-xl object-cover ring-1 ring-stone-200" />
            <div class="flex flex-col gap-2">
              <button type="button" class="btn btn-secondary text-xs" :disabled="processingFirst || analyzing" @click="firstInput?.click()">
                Sostituisci foto
              </button>
              <button type="button" class="btn btn-ghost text-xs" :disabled="analyzing" @click="analyze">
                {{ analyzing ? "Analizzo…" : "Rianalizza" }}
              </button>
            </div>
          </div>
          <button v-else type="button" class="btn btn-primary" :disabled="processingFirst" @click="firstInput?.click()">
            {{ processingFirst ? "Elaboro la foto…" : "Fai una foto" }}
          </button>
          <p v-if="fieldErrors.photos" class="mt-2 text-xs font-medium text-rose-700">{{ fieldErrors.photos }}</p>
        </div>

        <div v-if="analyzing" class="flex items-center gap-2 text-sm text-stone-600" role="status">
          <span class="h-4 w-4 animate-spin rounded-full border-2 border-emerald-700 border-t-transparent" aria-hidden="true"></span>
          Analisi della foto e preparazione delle prossime indicazioni…
        </div>

        <div v-if="analysis" class="space-y-3">
          <div class="flex flex-wrap items-center gap-2">
            <span class="badge-soft">{{ sourceLabel }}</span>
            <span v-if="analysis.sensitive" class="badge-danger">Oggetto sensibile</span>
            <span v-if="analysis.confidence" class="text-xs text-stone-500">Confidenza {{ Math.round(analysis.confidence * 100) }}%</span>
          </div>
          <ul v-if="analysis.warnings.length" class="space-y-1 text-xs text-amber-900">
            <li v-for="warning in analysis.warnings" :key="warning">⚠ {{ warning }}</li>
          </ul>
          <div v-if="analysis.isDocument" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-950">
            Documento: non aggiungere foto del contenuto. La scheda resta sensibile e la consegna finale è gestita dalla PA.
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="label" for="wiz-category">Categoria</label>
            <select id="wiz-category" v-model="category" class="input" :disabled="isDocument" @change="onCategoryChange">
              <option value="" disabled>Scegli una categoria</option>
              <option v-for="option in itemCategories" :key="option.value" :value="option.value">{{ option.label }}</option>
            </select>
            <p v-if="fieldErrors.category" class="mt-1 text-xs font-medium text-rose-700">{{ fieldErrors.category }}</p>
          </div>
          <div v-if="category === 'altro'">
            <label class="label" for="wiz-custom-category">Descrivi la categoria</label>
            <input id="wiz-custom-category" v-model="customCategory" class="input" maxlength="80" type="text" placeholder="Es. strumento musicale" @input="onCategoryChange" />
          </div>
          <div>
            <label class="label" for="wiz-brand">Marca <span class="font-normal text-stone-500">(facoltativo)</span></label>
            <input id="wiz-brand" v-model="brand" class="input" maxlength="80" type="text" placeholder="Es. Apple" />
            <p class="hint">D3 non legge la marca: confermala o compilala.</p>
          </div>
          <div>
            <label class="label" for="wiz-model">Modello <span class="font-normal text-stone-500">(facoltativo)</span></label>
            <input id="wiz-model" v-model="model" class="input" maxlength="80" type="text" placeholder="Es. iPhone 15" />
          </div>
          <div>
            <label class="label" for="wiz-color">Colore</label>
            <select id="wiz-color" v-model="color" class="input" @change="onColorChange">
              <option value="">Non indicato</option>
              <option v-for="value in itemColors" :key="value" :value="value">{{ value.charAt(0).toUpperCase() + value.slice(1) }}</option>
            </select>
          </div>
          <div>
            <label class="label" for="wiz-condition">Condizioni</label>
            <select id="wiz-condition" v-model="condition" class="input" @change="onConditionChange">
              <option v-for="option in itemConditions" :key="option.value" :value="option.value">{{ option.label }}</option>
            </select>
          </div>
        </div>

        <div v-if="plan.length" class="space-y-3">
          <div>
            <p class="text-sm font-semibold text-stone-900">Foto consigliate</p>
            <p class="text-xs text-stone-500">Aggiungi quelle che riesci: le obbligatorie sono indicate.</p>
          </div>
          <PhotoPlanSlot
            v-for="slot in plan"
            :key="slot.key"
            :slot="slot"
            :file="slotFiles[slot.key] ?? null"
            :disabled="submitting"
            @update:file="(file) => (slotFiles[slot.key] = file)"
          />
          <p v-for="slot in plan" v-show="fieldErrors[`slot:${slot.key}`]" :key="`err-${slot.key}`" class="text-xs font-medium text-rose-700">
            {{ fieldErrors[`slot:${slot.key}`] }}
          </p>
        </div>
      </section>

      <!-- Step 2 -->
      <section v-if="step === 2" class="card space-y-4">
        <div class="flex items-center gap-3">
          <span class="step-dot">2</span>
          <div>
            <h2 class="section-title">Dove e quando</h2>
            <p class="text-xs text-stone-500">La posizione esatta è privata; la zona aiuta chi cerca.</p>
          </div>
        </div>

        <div>
          <label class="label" for="wiz-found-at">Data e ora del ritrovamento</label>
          <input id="wiz-found-at" v-model="foundAt" class="input" type="datetime-local" />
          <p v-if="fieldErrors.foundAt" class="mt-1 text-xs font-medium text-rose-700">{{ fieldErrors.foundAt }}</p>
        </div>

        <GeoPicker v-model:lat="lat" v-model:lng="lng" :zone="zone" @update:zone="onZoneInput" />
        <p v-if="reverseLoading" class="text-xs text-stone-500" role="status">Riconosco la zona dalla posizione…</p>
        <p v-else-if="reverseSource === 'unavailable' && zone" class="text-xs text-amber-800">Zona inserita manualmente: il servizio di geocoding non è disponibile.</p>
        <p v-else-if="reverseSource === 'mapbox' && zone" class="text-xs text-emerald-700">Zona compilata dalla posizione: controllala e correggila se serve.</p>
        <p v-if="fieldErrors.zone" class="text-xs font-medium text-rose-700">{{ fieldErrors.zone }}</p>
        <p v-if="fieldErrors.geo" class="text-xs font-medium text-rose-700">{{ fieldErrors.geo }}</p>

        <div>
          <label class="label" for="wiz-notes">Note contestuali <span class="font-normal text-stone-500">(facoltative)</span></label>
          <textarea id="wiz-notes" v-model="notes" class="input min-h-20 resize-y" maxlength="500" rows="3" placeholder="Es. trovato su una panchina davanti alla stazione"></textarea>
        </div>
      </section>

      <!-- Step 3 -->
      <section v-if="step === 3" class="card space-y-4">
        <div class="flex items-center gap-3">
          <span class="step-dot">3</span>
          <div>
            <h2 class="section-title">Dettagli riservati</h2>
            <p class="text-xs text-stone-500">Visibili solo agli operatori autorizzati, mai pubblicati.</p>
          </div>
        </div>
        <div v-if="analysis?.isDocument" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-950">
          Non inserire dati del documento (numero, intestatario, data di nascita). La PA gestirà la parte sensibile.
        </div>
        <div>
          <label class="label" for="wiz-private">Segni particolari o contenuto <span class="font-normal text-stone-500">(facoltativo)</span></label>
          <textarea id="wiz-private" :value="privateDetails" class="input min-h-24 resize-y" maxlength="500" rows="4" placeholder="Es. graffio sull'angolo, nome inciso sul retro, colore della custodia" @input="onPrivateInput"></textarea>
          <p class="hint">Questi dettagli non compaiono nella ricevuta e non vengono mostrati a chi cerca l'oggetto.</p>
          <div v-if="!analysis?.isDocument" class="mt-2 flex flex-wrap gap-2">
            <button type="button" class="badge-soft" @click="appendPrivate('crepa o segni visibili')">+ crepa</button>
            <button type="button" class="badge-soft" @click="appendPrivate('custodia')">+ custodia</button>
            <button type="button" class="badge-soft" @click="appendPrivate('adesivo')">+ adesivo</button>
          </div>
        </div>
      </section>

      <!-- Step 4 -->
      <section v-if="step === 4" class="card space-y-4">
        <div class="flex items-center gap-3">
          <span class="step-dot">4</span>
          <div>
            <h2 class="section-title">Punto di raccolta</h2>
            <p class="text-xs text-stone-500">I cinque punti attivi più vicini a piedi dalla segnalazione.</p>
          </div>
        </div>

        <div v-if="dropLoading" class="flex items-center gap-2 text-sm text-stone-600" role="status">
          <span class="h-4 w-4 animate-spin rounded-full border-2 border-emerald-700 border-t-transparent" aria-hidden="true"></span>
          Calcolo le distanze a piedi…
        </div>
        <div v-else-if="dropError" class="banner-error" role="alert">
          {{ dropError }}
          <button type="button" class="btn btn-secondary mt-3 text-xs" @click="loadDropPoints">Riprova</button>
        </div>
        <div v-else-if="dropPoints.length === 0" class="banner-info">Nessun punto disponibile.</div>
        <div v-else class="space-y-2">
          <div v-if="dropSource === 'none'" class="banner-info">
            Posizione non indicata: mostro i punti in ordine alfabetico. Puoi sceglierne uno
            in base all'indirizzo e agli orari.
          </div>
          <label
            v-for="point in dropPoints"
            :key="point.id"
            class="flex cursor-pointer gap-3 rounded-xl border p-3 transition"
            :class="dropPoint === point.id ? 'border-emerald-500 bg-emerald-50/60 ring-2 ring-emerald-500/20' : 'border-stone-200 bg-white hover:border-stone-300'"
          >
            <input v-model="dropPoint" type="radio" name="wizard-drop-point" :value="point.id" class="mt-1 h-4 w-4 shrink-0 text-emerald-700" />
            <span class="min-w-0 flex-1">
              <span class="flex flex-wrap items-center gap-2">
                <span class="block text-sm font-semibold text-stone-900">{{ point.name }}</span>
                <span v-if="point.approximate" class="badge-soft">stima</span>
              </span>
              <span class="block text-xs text-stone-600">{{ point.address }} · {{ point.hours }}</span>
              <span
                v-if="point.source !== 'none' && (point.walkingMinutes > 0 || point.distanceMeters > 0)"
                class="mt-1 block text-xs font-medium text-emerald-800"
              >
                {{ point.walkingMinutes }} min a piedi · {{ Math.round(point.distanceMeters) }} m
              </span>
              <span v-else class="mt-1 block text-xs text-stone-500">
                Distanza non disponibile: scegli in base all'indirizzo.
              </span>
            </span>
          </label>
        </div>
        <p v-if="fieldErrors.dropPoint" class="text-xs font-medium text-rose-700">{{ fieldErrors.dropPoint }}</p>
      </section>

      <!-- Step 5 -->
      <section v-if="step === 5" class="card space-y-4">
        <div class="flex items-center gap-3">
          <span class="step-dot">5</span>
          <div>
            <h2 class="section-title">Riepilogo e conferma</h2>
            <p class="text-xs text-stone-500">Controlla i dati e accetta il disclaimer per inviare.</p>
          </div>
        </div>

        <dl class="grid grid-cols-2 gap-4 text-sm">
          <div>
            <dt class="data-label">Oggetto</dt>
            <dd class="data-value">{{ effectiveCategory || "—" }} {{ brand }} {{ model }}</dd>
          </div>
          <div>
            <dt class="data-label">Foto</dt>
            <dd class="data-value">{{ allPhotos.length }}</dd>
          </div>
          <div>
            <dt class="data-label">Zona</dt>
            <dd class="data-value">{{ zone || "—" }}</dd>
          </div>
          <div>
            <dt class="data-label">Quando</dt>
            <dd class="data-value">{{ formatDateTime(foundAt) }}</dd>
          </div>
          <div>
            <dt class="data-label">Punto di raccolta</dt>
            <dd class="data-value">{{ selectedDropPoint?.name ?? "—" }}</dd>
          </div>
          <div>
            <dt class="data-label">A piedi</dt>
            <dd class="data-value">
              {{
                selectedDropPoint && selectedDropPoint.source !== "none"
                  ? `${selectedDropPoint.walkingMinutes} min`
                  : "—"
              }}
            </dd>
          </div>
          <div class="col-span-2">
            <dt class="data-label">Dettagli riservati</dt>
            <dd class="data-value">{{ privateDetails.trim() ? "Sì, salvati in modo privato" : "Nessuno" }}</dd>
          </div>
        </dl>

        <div class="rounded-xl border border-stone-200 bg-stone-50 px-4 py-3 text-sm leading-6 text-stone-700">
          <p class="font-semibold text-stone-900">Disclaimer</p>
          <p class="mt-1">
            Confermo di aver trovato l'oggetto e di consegnarlo al punto di raccolta scelto.
            Le foto non saranno pubbliche, la posizione esatta resta privata e i dettagli
            riservati saranno visibili solo agli operatori autorizzati. Non ho fotografato
            documenti, notifiche o dati personali oltre a quanto necessario.
          </p>
        </div>

        <RightsAssignment v-model="rights" mode="finder" :disabled="submitting" />
        <p v-if="fieldErrors.rights" class="text-xs font-medium text-rose-700">{{ fieldErrors.rights }}</p>

        <button type="submit" class="btn btn-primary w-full sm:w-auto" :disabled="submitting || !rights">
          <span v-if="submitting" class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent" aria-hidden="true"></span>
          {{ submitting ? "Invio in corso…" : "Registra l'oggetto" }}
        </button>
      </section>

      <div class="flex flex-col gap-3 sm:flex-row-reverse sm:justify-start">
        <button v-if="step < 5" type="button" class="btn btn-primary w-full sm:w-auto" @click="next">
          Avanti
        </button>
        <button v-if="step > 1" type="button" class="btn btn-secondary w-full sm:w-auto" :disabled="submitting" @click="back">
          Indietro
        </button>
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
