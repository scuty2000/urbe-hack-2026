<script setup lang="ts">
import { computed, ref, watch } from "vue";

const props = defineProps<{
  lat: number | null;
  lng: number | null;
  zone: string;
  disabled?: boolean;
}>();

const emit = defineEmits<{
  "update:lat": [value: number | null];
  "update:lng": [value: number | null];
  "update:zone": [value: string];
}>();

const locating = ref(false);
const geoError = ref("");
const geoSuccess = ref(false);

const latInput = ref<HTMLInputElement | null>(null);
const lngInput = ref<HTMLInputElement | null>(null);
const latText = ref(props.lat === null ? "" : String(props.lat));
const lngText = ref(props.lng === null ? "" : String(props.lng));

// Keep the text fields in sync when the position arrives from the browser or
// the parent resets the form, but never fight the user while they are typing.
watch(
  () => props.lat,
  (value) => {
    if (document.activeElement !== latInput.value) {
      latText.value = value === null ? "" : String(value);
    }
  },
);
watch(
  () => props.lng,
  (value) => {
    if (document.activeElement !== lngInput.value) {
      lngText.value = value === null ? "" : String(value);
    }
  },
);

const latInvalid = computed(
  () => latText.value.trim() !== "" && parseCoordinate(latText.value, 90) === null,
);
const lngInvalid = computed(
  () => lngText.value.trim() !== "" && parseCoordinate(lngText.value, 180) === null,
);

function parseCoordinate(raw: string, limit: number): number | null {
  const normalized = raw.replace(",", ".").trim();
  if (normalized === "") {
    return null;
  }
  const value = Number(normalized);
  if (!Number.isFinite(value) || Math.abs(value) > limit) {
    return null;
  }
  return value;
}

function onLatInput(event: Event) {
  latText.value = (event.target as HTMLInputElement).value;
  emit("update:lat", parseCoordinate(latText.value, 90));
}

function onLngInput(event: Event) {
  lngText.value = (event.target as HTMLInputElement).value;
  emit("update:lng", parseCoordinate(lngText.value, 180));
}

function setZone(event: Event) {
  emit("update:zone", (event.target as HTMLInputElement).value);
}

function locate() {
  geoError.value = "";
  geoSuccess.value = false;

  if (!("geolocation" in navigator)) {
    geoError.value = "Il tuo browser non supporta la geolocalizzazione.";
    return;
  }

  locating.value = true;
  navigator.geolocation.getCurrentPosition(
    (position) => {
      locating.value = false;
      geoSuccess.value = true;
      emit("update:lat", Number(position.coords.latitude.toFixed(6)));
      emit("update:lng", Number(position.coords.longitude.toFixed(6)));
    },
    (error) => {
      locating.value = false;
      switch (error.code) {
        case error.PERMISSION_DENIED:
          geoError.value =
            "Permesso negato. Puoi inserire le coordinate manualmente.";
          break;
        case error.POSITION_UNAVAILABLE:
          geoError.value =
            "Posizione non disponibile. Prova a inserire latitudine e longitudine.";
          break;
        case error.TIMEOUT:
          geoError.value = "La posizione sta impiegando troppo tempo. Riprova.";
          break;
        default:
          geoError.value = "Non riesco a leggere la posizione. Inseriscila manualmente.";
      }
    },
    { enableHighAccuracy: true, timeout: 12000, maximumAge: 30000 },
  );
}
</script>

<template>
  <div>
    <button
      type="button"
      class="btn btn-secondary w-full sm:w-auto"
      :disabled="disabled || locating"
      @click="locate"
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
          d="M15 10.5a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z"
        />
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M19.5 10.5c0 7.142-7.5 11.25-7.5 11.25S4.5 17.642 4.5 10.5a7.5 7.5 0 1 1 15 0Z"
        />
      </svg>
      {{ locating ? "Rilevo la posizione…" : "Usa la mia posizione" }}
    </button>

    <p v-if="geoSuccess" class="mt-2 text-xs font-medium text-emerald-700">
      Posizione acquisita. Le coordinate esatte restano private.
    </p>
    <p v-if="geoError" class="mt-2 text-xs text-rose-700" role="alert">
      {{ geoError }}
    </p>

    <div class="mt-4 grid grid-cols-2 gap-3">
      <div>
        <label class="label" for="geo-lat">Latitudine</label>
        <input
          id="geo-lat"
          ref="latInput"
          v-model="latText"
          class="input"
          :class="{ 'border-rose-400': latInvalid }"
          type="text"
          inputmode="decimal"
          placeholder="41.9028"
          :disabled="disabled"
          autocomplete="off"
          @input="onLatInput"
        />
        <p v-if="latInvalid" class="mt-1 text-[11px] text-rose-700">
          Valore tra -90 e 90.
        </p>
      </div>
      <div>
        <label class="label" for="geo-lng">Longitudine</label>
        <input
          id="geo-lng"
          ref="lngInput"
          v-model="lngText"
          class="input"
          :class="{ 'border-rose-400': lngInvalid }"
          type="text"
          inputmode="decimal"
          placeholder="12.4964"
          :disabled="disabled"
          autocomplete="off"
          @input="onLngInput"
        />
        <p v-if="lngInvalid" class="mt-1 text-[11px] text-rose-700">
          Valore tra -180 e 180.
        </p>
      </div>
    </div>

    <div class="mt-3">
      <label class="label" for="geo-zone">Zona del ritrovamento</label>
      <input
        id="geo-zone"
        class="input"
        type="text"
        placeholder="Es. San Lorenzo, Roma"
        maxlength="120"
        :value="props.zone"
        :disabled="disabled"
        autocomplete="off"
        @input="setZone"
      />
    </div>

    <p class="hint">
      La posizione esatta serve solo per la custodia e non viene mai pubblicata
      né mostrata agli utenti che cercano.
    </p>
  </div>
</template>
