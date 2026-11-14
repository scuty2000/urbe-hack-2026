<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { resizeToJpeg } from "../format";

const props = withDefaults(
  defineProps<{
    modelValue: File[];
    max?: number;
    disabled?: boolean;
  }>(),
  { max: 3, disabled: false },
);

const emit = defineEmits<{
  "update:modelValue": [files: File[]];
}>();

const input = ref<HTMLInputElement | null>(null);
const processing = ref(false);
const error = ref("");
const dragOver = ref(false);
const urls = new Map<File, string>();

const previews = computed(() =>
  props.modelValue.map((file) => ({
    file,
    url: urlFor(file),
    name: file.name,
    size: Math.max(1, Math.round(file.size / 1024)),
  })),
);

const remaining = computed(() => Math.max(0, props.max - props.modelValue.length));

function urlFor(file: File): string {
  const existing = urls.get(file);
  if (existing) {
    return existing;
  }
  const url = URL.createObjectURL(file);
  urls.set(file, url);
  return url;
}

function openPicker() {
  if (!props.disabled && !processing.value) {
    input.value?.click();
  }
}

function resetInput() {
  if (input.value) {
    input.value.value = "";
  }
}

async function addFiles(files: FileList | null) {
  if (!files || files.length === 0) {
    return;
  }
  error.value = "";
  if (props.disabled) {
    return;
  }

  const incoming = Array.from(files);
  const images = incoming.filter((file) => file.type.startsWith("image/"));
  if (images.length !== incoming.length) {
    error.value = "Puoi caricare solo immagini JPEG, PNG o WebP.";
  }

  const room = props.max - props.modelValue.length;
  if (room <= 0) {
    error.value = `Puoi caricare al massimo ${props.max} foto.`;
    resetInput();
    return;
  }

  const accepted = images.slice(0, room);
  if (images.length > room) {
    error.value = `Hai aggiunto ${images.length} foto: ne conservo ${room} (massimo ${props.max}).`;
  }

  processing.value = true;
  const resized: File[] = [];
  for (const file of accepted) {
    try {
      resized.push(await resizeToJpeg(file));
    } catch {
      error.value = `Non riesco a elaborare "${file.name}". Prova con un'altra foto.`;
    }
  }
  processing.value = false;

  if (resized.length > 0) {
    emit("update:modelValue", [...props.modelValue, ...resized]);
  }
  resetInput();
}

function onSelect(event: Event) {
  const target = event.target as HTMLInputElement;
  void addFiles(target.files);
}

function onDrop(event: DragEvent) {
  dragOver.value = false;
  void addFiles(event.dataTransfer?.files ?? null);
}

function remove(file: File) {
  const url = urls.get(file);
  if (url) {
    URL.revokeObjectURL(url);
    urls.delete(file);
  }
  emit(
    "update:modelValue",
    props.modelValue.filter((candidate) => candidate !== file),
  );
}

watch(
  () => props.modelValue,
  (files) => {
    const live = new Set(files);
    for (const [file, url] of urls) {
      if (!live.has(file)) {
        URL.revokeObjectURL(url);
        urls.delete(file);
      }
    }
  },
);

onBeforeUnmount(() => {
  for (const url of urls.values()) {
    URL.revokeObjectURL(url);
  }
  urls.clear();
});
</script>

<template>
  <div>
    <div class="flex items-baseline justify-between gap-3">
      <span class="label mb-0">Foto dell'oggetto</span>
      <span class="text-xs text-stone-500">
        {{ modelValue.length }}/{{ max }} · almeno 1
      </span>
    </div>

    <div class="mt-2 grid grid-cols-3 gap-2.5">
      <div
        v-for="preview in previews"
        :key="preview.url"
        class="group relative aspect-square overflow-hidden rounded-xl border border-stone-200 bg-stone-100"
      >
        <img
          :src="preview.url"
          :alt="`Foto ${preview.name}`"
          class="h-full w-full object-cover"
        />
        <button
          type="button"
          class="absolute top-1.5 right-1.5 flex h-7 w-7 items-center justify-center rounded-full bg-stone-900/70 text-white transition hover:bg-rose-600"
          :disabled="disabled"
          :aria-label="`Rimuovi ${preview.name}`"
          @click="remove(preview.file)"
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
            class="h-3.5 w-3.5"
            aria-hidden="true"
          >
            <path stroke-linecap="round" d="M6 6l12 12M18 6 6 18" />
          </svg>
        </button>
        <span
          class="absolute bottom-1.5 left-1.5 rounded bg-stone-900/60 px-1.5 py-0.5 text-[10px] text-white"
          >{{ preview.size }} KB</span
        >
      </div>

      <button
        v-if="modelValue.length < max"
        type="button"
        class="flex aspect-square flex-col items-center justify-center gap-1 rounded-xl border-2 border-dashed px-2 text-center transition disabled:cursor-not-allowed disabled:opacity-60"
        :class="
          dragOver
            ? 'border-emerald-500 bg-emerald-50'
            : 'border-stone-300 bg-white hover:border-emerald-400 hover:bg-emerald-50/40'
        "
        :disabled="disabled || processing"
        @click="openPicker"
        @dragover.prevent="dragOver = true"
        @dragleave.prevent="dragOver = false"
        @drop.prevent="onDrop"
      >
        <svg
          v-if="!processing"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          class="h-6 w-6 text-stone-400"
          aria-hidden="true"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M12 4.5v15m7.5-7.5h-15"
          />
        </svg>
        <span
          v-else
          class="h-5 w-5 animate-spin rounded-full border-2 border-emerald-600 border-t-transparent"
          aria-hidden="true"
        ></span>
        <span class="text-xs font-medium text-stone-600">
          {{ processing ? "Elaboro…" : "Aggiungi foto" }}
        </span>
        <span v-if="remaining > 1" class="text-[10px] text-stone-400"
          >ancora {{ remaining }}</span
        >
      </button>
    </div>

    <input
      ref="input"
      type="file"
      accept="image/jpeg,image/png,image/webp"
      multiple
      class="hidden"
      @change="onSelect"
    />

    <p class="hint">
      Le foto vengono ridimensionate a 1200px e compresse sul tuo dispositivo
      prima dell'invio.
    </p>
    <p v-if="error" class="mt-2 text-xs font-medium text-rose-700" role="alert">
      {{ error }}
    </p>
  </div>
</template>
