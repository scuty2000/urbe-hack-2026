<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue";
import { resizeToJpeg } from "../../format";
import type { WizardPhotoSlot } from "../../types";

const props = defineProps<{
  slot: WizardPhotoSlot;
  file: File | null;
  disabled?: boolean;
}>();

const emit = defineEmits<{
  "update:file": [file: File | null];
}>();

const input = ref<HTMLInputElement | null>(null);
const processing = ref(false);
const error = ref("");
const preview = ref<string | null>(null);

watch(
  () => props.file,
  (file) => {
    if (preview.value) {
      URL.revokeObjectURL(preview.value);
    }
    preview.value = file ? URL.createObjectURL(file) : null;
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  if (preview.value) {
    URL.revokeObjectURL(preview.value);
  }
});

async function onSelect(event: Event) {
  const target = event.target as HTMLInputElement;
  const file = target.files?.[0];
  target.value = "";
  if (!file || props.disabled) {
    return;
  }
  processing.value = true;
  error.value = "";
  try {
    emit("update:file", await resizeToJpeg(file));
  } catch {
    error.value = "Non riesco a elaborare questa foto. Provane un'altra.";
  } finally {
    processing.value = false;
  }
}

function remove() {
  if (!props.disabled) {
    emit("update:file", null);
  }
}
</script>

<template>
  <div
    class="rounded-xl border p-3 transition"
    :class="file ? 'border-emerald-300 bg-emerald-50/40' : 'border-stone-200 bg-white'"
  >
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <p class="text-sm font-semibold text-stone-900">
          {{ slot.label }}
          <span v-if="slot.required" class="text-xs font-medium text-rose-700">obbligatoria</span>
          <span v-else class="text-xs font-normal text-stone-500">facoltativa</span>
        </p>
        <p class="mt-0.5 text-xs leading-5 text-stone-600">{{ slot.instruction }}</p>
      </div>
      <span
        v-if="file"
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-emerald-700 text-xs text-white"
        aria-label="Foto aggiunta"
      >
        ✓
      </span>
    </div>

    <div v-if="preview" class="mt-3 flex items-center gap-3">
      <img :src="preview" alt="" class="h-20 w-20 rounded-lg object-cover ring-1 ring-stone-200" />
      <button type="button" class="btn btn-ghost text-xs" :disabled="disabled" @click="remove">
        Rimuovi
      </button>
    </div>
    <div v-else class="mt-3">
      <input
        ref="input"
        class="sr-only"
        type="file"
        accept="image/*"
        capture="environment"
        :disabled="disabled || processing"
        @change="onSelect"
      />
      <button
        type="button"
        class="btn btn-secondary text-xs"
        :disabled="disabled || processing"
        @click="input?.click()"
      >
        {{ processing ? "Elaboro…" : "Aggiungi foto" }}
      </button>
    </div>
    <p v-if="error" class="mt-2 text-xs font-medium text-rose-700" role="alert">{{ error }}</p>
  </div>
</template>
