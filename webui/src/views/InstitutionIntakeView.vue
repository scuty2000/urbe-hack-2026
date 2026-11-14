<script setup lang="ts">
import { computed, ref } from "vue";
import { RouterLink } from "vue-router";
import { ApiError, createInstitutionItem } from "../api";
import GeoPicker from "../components/GeoPicker.vue";
import PhotoUpload from "../components/PhotoUpload.vue";
import StatusBadge from "../components/StatusBadge.vue";
import {
  itemCategories,
  itemColors,
  itemConditions,
  localDateTimeValue,
} from "../catalog";
import { displayItemName, formatDateTime, categoryLabel, organisationLabel } from "../format";
import type { Item, OfficialItemPayload } from "../types";

const photos = ref<File[]>([]);
const category = ref("");
const customCategory = ref("");
const brand = ref("");
const model = ref("");
const color = ref("");
const condition = ref("good");
const foundAt = ref(localDateTimeValue());
const notes = ref("");
const privateDetails = ref("");
const lat = ref<number | null>(null);
const lng = ref<number | null>(null);
const zone = ref("");

const submitting = ref(false);
const error = ref("");
const fieldErrors = ref<Record<string, string>>({});
const created = ref<Item | null>(null);

const effectiveCategory = computed(() =>
  category.value === "altro" ? customCategory.value.trim() : category.value,
);

function validate(): boolean {
  const errors: Record<string, string> = {};
  if (photos.value.length === 0) {
    errors.photos = "Aggiungi almeno una foto dell'oggetto.";
  }
  if (!effectiveCategory.value) {
    errors.category = "Scegli una categoria o descrivila in “Altro”.";
  }
  if ((lat.value === null) !== (lng.value === null)) {
    errors.geo = "Inserisci sia la latitudine sia la longitudine, oppure nessuna.";
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

function buildPayload(): OfficialItemPayload {
  const payload: OfficialItemPayload = {
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
  };
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
    created.value = await createInstitutionItem(buildPayload(), photos.value);
    window.scrollTo({ top: 0, behavior: "smooth" });
  } catch (cause) {
    error.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a registrare l'oggetto. Riprova.";
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
  foundAt.value = localDateTimeValue();
  notes.value = "";
  privateDetails.value = "";
  lat.value = null;
  lng.value = null;
  zone.value = "";
  error.value = "";
  fieldErrors.value = {};
}
</script>

<template>
  <div class="mx-auto w-full max-w-3xl">
    <RouterLink to="/institution" class="link text-sm">← Coda di ritiro</RouterLink>

    <section v-if="created" class="mt-4 space-y-4">
      <div class="banner-success">
        <p class="font-semibold">Oggetto acquisito dalla pubblica amministrazione.</p>
        <p class="mt-1">
          Codice pubblico
          <span class="font-mono font-semibold">{{ created.publicCode }}</span>.
          L'oggetto è in custodia della PA: il termine legale parte da ora.
        </p>
      </div>

      <div class="card">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p class="eyebrow">Intake ufficiale</p>
            <h1 class="mt-1 text-xl font-semibold text-stone-900">
              {{ displayItemName(created) }}
            </h1>
            <p class="mt-1 font-mono text-sm text-stone-600">{{ created.publicCode }}</p>
          </div>
          <StatusBadge :status="created.status" :sensitive="created.sensitive" />
        </div>

        <dl class="mt-5 grid grid-cols-2 gap-4 text-sm">
          <div>
            <dt class="data-label">Categoria</dt>
            <dd class="data-value">{{ categoryLabel(created.category) }}</dd>
          </div>
          <div>
            <dt class="data-label">Registrato il</dt>
            <dd class="data-value">{{ formatDateTime(created.createdAt) }}</dd>
          </div>
          <div>
            <dt class="data-label">Zona</dt>
            <dd class="data-value">{{ created.foundZone || "—" }}</dd>
          </div>
          <div>
            <dt class="data-label">Custode</dt>
            <dd class="data-value">{{ organisationLabel(created.custodianId) }}</dd>
          </div>
        </dl>

        <div v-if="created.sensitive" class="mt-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900">
          Oggetto sensibile: conservalo in un luogo sicuro e non condividerne i
          dettagli.
        </div>

        <div class="mt-5 flex flex-col gap-2 sm:flex-row">
          <RouterLink
            :to="`/institution/items/${created.id}`"
            class="btn btn-primary"
          >
            Apri la scheda
          </RouterLink>
          <button type="button" class="btn btn-secondary" @click="resetForm">
            Registra un altro oggetto
          </button>
        </div>
      </div>
    </section>

    <template v-else>
      <header class="mt-4">
        <p class="eyebrow">Pubblica amministrazione</p>
        <h1 class="page-title mt-1">Intake ufficiale</h1>
        <p class="page-subtitle">
          Registra un oggetto consegnato direttamente allo sportello. La PA è
          finder legale e custode dalla registrazione.
        </p>
      </header>

      <form class="mt-5 space-y-4" novalidate @submit.prevent="submit">
        <div v-if="error" class="banner-error" role="alert">{{ error }}</div>

        <section class="card space-y-4">
          <h2 class="section-title">Foto e descrizione</h2>
          <PhotoUpload v-model="photos" :max="3" :disabled="submitting" />
          <p v-if="fieldErrors.photos" class="text-xs font-medium text-rose-700">
            {{ fieldErrors.photos }}
          </p>

          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="intake-category">Categoria</label>
              <select
                id="intake-category"
                v-model="category"
                class="input"
                :disabled="submitting"
                :aria-invalid="Boolean(fieldErrors.category)"
              >
                <option value="" disabled>Scegli una categoria</option>
                <option
                  v-for="option in itemCategories"
                  :key="option.value"
                  :value="option.value"
                >
                  {{ option.label }}
                </option>
              </select>
              <p v-if="fieldErrors.category" class="mt-1 text-xs font-medium text-rose-700">
                {{ fieldErrors.category }}
              </p>
            </div>
            <div v-if="category === 'altro'">
              <label class="label" for="intake-custom-category">Descrivi la categoria</label>
              <input
                id="intake-custom-category"
                v-model="customCategory"
                class="input"
                type="text"
                maxlength="80"
                placeholder="Es. strumento musicale"
                :disabled="submitting"
              />
            </div>
            <div>
              <label class="label" for="intake-brand">Marca</label>
              <input
                id="intake-brand"
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
              <label class="label" for="intake-model">Modello</label>
              <input
                id="intake-model"
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
              <label class="label" for="intake-color">Colore</label>
              <select id="intake-color" v-model="color" class="input" :disabled="submitting">
                <option value="">Non indicato</option>
                <option v-for="value in itemColors" :key="value" :value="value">
                  {{ value.charAt(0).toUpperCase() + value.slice(1) }}
                </option>
              </select>
            </div>
            <div>
              <label class="label" for="intake-condition">Condizioni</label>
              <select id="intake-condition" v-model="condition" class="input" :disabled="submitting">
                <option
                  v-for="option in itemConditions"
                  :key="option.value"
                  :value="option.value"
                >
                  {{ option.label }}
                </option>
              </select>
            </div>
          </div>
        </section>

        <section class="card space-y-4">
          <h2 class="section-title">Dove e quando</h2>
          <div>
            <label class="label" for="intake-found-at">Data e ora della consegna</label>
            <input
              id="intake-found-at"
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
            <label class="label" for="intake-notes">Note contestuali</label>
            <textarea
              id="intake-notes"
              v-model="notes"
              class="input min-h-20 resize-y"
              maxlength="500"
              rows="3"
              placeholder="Es. consegnato allo sportello da un cittadino"
              :disabled="submitting"
            ></textarea>
          </div>
        </section>

        <section class="card space-y-4">
          <h2 class="section-title">Dettagli riservati</h2>
          <div>
            <label class="label" for="intake-private">
              Segni particolari o contenuto
              <span class="font-normal text-stone-400">(facoltativo)</span>
            </label>
            <textarea
              id="intake-private"
              v-model="privateDetails"
              class="input min-h-20 resize-y"
              maxlength="500"
              rows="3"
              placeholder="Es. graffio sull'angolo, nome inciso sul retro"
              :disabled="submitting"
            ></textarea>
            <p class="hint">
              Visibili solo agli operatori autorizzati. Non compaiono nella scheda
              pubblica né nelle risposte a chi cerca.
            </p>
          </div>
        </section>

        <div class="flex flex-col gap-3 sm:flex-row-reverse sm:justify-start">
          <button type="submit" class="btn btn-primary w-full sm:w-auto" :disabled="submitting">
            <span
              v-if="submitting"
              class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"
              aria-hidden="true"
            ></span>
            {{ submitting ? "Registro…" : "Registra l'oggetto" }}
          </button>
          <RouterLink to="/institution" class="btn btn-ghost w-full sm:w-auto">
            Annulla
          </RouterLink>
        </div>
      </form>
    </template>
  </div>
</template>
