<script setup lang="ts">
import QRCode from "qrcode";
import { ref, watch } from "vue";

const props = withDefaults(
  defineProps<{
    value: string;
    size?: number;
    label?: string;
    muted?: boolean;
  }>(),
  { size: 220, label: "Codice QR del pass", muted: false },
);

const dataUrl = ref("");
const error = ref("");
const rendering = ref(false);
let renderId = 0;

watch(
  () => [props.value, props.size] as const,
  async ([value, size]) => {
    const current = ++renderId;
    error.value = "";
    if (!value) {
      dataUrl.value = "";
      return;
    }
    rendering.value = true;
    try {
      const rendered = await QRCode.toDataURL(value, {
        errorCorrectionLevel: "M",
        margin: 2,
        width: size,
        color: { dark: "#14110f", light: "#fffcf4" },
      });
      if (current === renderId) {
        dataUrl.value = rendered;
      }
    } catch {
      if (current === renderId) {
        dataUrl.value = "";
        error.value = "Non riesco a generare il codice QR.";
      }
    } finally {
      if (current === renderId) {
        rendering.value = false;
      }
    }
  },
  { immediate: true },
);
</script>

<template>
  <div class="inline-flex flex-col items-center">
    <div
      class="relative overflow-hidden rounded-lg border border-ink/25 bg-paper-card p-3"
      :style="{ width: `${size + 24}px`, height: `${size + 24}px` }"
    >
      <img
        v-if="dataUrl"
        :src="dataUrl"
        :alt="label"
        :width="size"
        :height="size"
        class="block"
        :class="{ 'opacity-40': muted }"
      />
      <div
        v-else
        class="flex h-full w-full items-center justify-center px-4 text-center text-xs text-ink-muted"
      >
        {{ rendering ? "Genero il codice…" : error || "Codice non disponibile" }}
      </div>

      <div
        v-if="dataUrl && muted"
        class="absolute inset-0 flex items-center justify-center"
      >
        <span
          class="rotate-[-12deg] rounded-lg bg-ink/80 px-3 py-1.5 text-xs font-semibold tracking-wide text-paper uppercase"
        >
          Non ancora valido
        </span>
      </div>
    </div>
    <p class="mt-2 max-w-60 text-center text-[11px] leading-4 text-ink-muted">
      Mostra il codice al punto di ritiro. Non condividerlo: chi lo possiede può
      ritirare l'oggetto.
    </p>
  </div>
</template>
