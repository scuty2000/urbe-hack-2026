<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref } from "vue";
import { RouterLink } from "vue-router";
import { Html5Qrcode } from "html5-qrcode";
import {
  ApiError,
  redeemPass,
  validatePass,
} from "../api";
import {
  formatDateTime,
  passReasonLabels,
} from "../format";
import type { PassRedemption, PassValidation } from "../types";

const token = ref("");
const validating = ref(false);
const redeeming = ref(false);
const validation = ref<PassValidation | null>(null);
const redemption = ref<PassRedemption | null>(null);
const formError = ref("");
const redeemError = ref("");
const cameraError = ref("");
const scanning = ref(false);
const starting = ref(false);

let scanner: Html5Qrcode | null = null;

async function startCamera() {
  if (scanning.value || starting.value) {
    return;
  }
  cameraError.value = "";
  formError.value = "";
  starting.value = true;
  await nextTick();

  try {
    scanner = new Html5Qrcode("qr-reader", { verbose: false });
    await scanner.start(
      { facingMode: "environment" },
      { fps: 10, qrbox: { width: 240, height: 240 } },
      (decodedText) => {
        void onDecoded(decodedText);
      },
      () => {
        // Per-frame decode failures are normal while the camera is pointing
        // away from a QR code; only the success callback matters here.
      },
    );
    scanning.value = true;
  } catch (cause) {
    await stopCamera();
    cameraError.value = cameraMessage(cause);
  } finally {
    starting.value = false;
  }
}

function cameraMessage(cause: unknown): string {
  const name = cause instanceof DOMException ? cause.name : "";
  switch (name) {
    case "NotAllowedError":
      return "Permesso fotocamera negato. Abilitalo nelle impostazioni del browser oppure usa il campo manuale.";
    case "NotFoundError":
      return "Nessuna fotocamera disponibile su questo dispositivo. Usa il campo manuale.";
    case "NotReadableError":
      return "La fotocamera è occupata da un'altra app. Chiudila e riprova.";
    case "OverconstrainedError":
      return "La fotocamera posteriore non è disponibile. Usa il campo manuale.";
    default:
      return "Non riesco ad avviare la fotocamera. Puoi sempre incollare il token manualmente.";
  }
}

async function stopCamera() {
  const active = scanner;
  scanner = null;
  scanning.value = false;
  if (!active) {
    return;
  }
  try {
    if (active.isScanning) {
      await active.stop();
    }
  } catch {
    // The camera may already be stopped by the browser.
  }
  try {
    active.clear();
  } catch {
    // Clearing an unmounted element is harmless.
  }
}

async function onDecoded(decodedText: string) {
  if (validating.value || redeeming.value) {
    return;
  }
  await stopCamera();
  token.value = decodedText.trim();
  await runValidate();
}

function resetResult() {
  validation.value = null;
  redemption.value = null;
  formError.value = "";
  redeemError.value = "";
}

async function runValidate() {
  const value = token.value.trim();
  resetResult();
  if (!value) {
    formError.value = "Inserisci il token del pass o scansiona il QR.";
    return;
  }

  if (scanning.value) {
    await stopCamera();
  }

  validating.value = true;
  try {
    validation.value = await validatePass(value);
  } catch (cause) {
    formError.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a verificare il pass. Riprova.";
  } finally {
    validating.value = false;
  }
}

async function redeem() {
  if (!validation.value?.valid) {
    return;
  }
  redeeming.value = true;
  redeemError.value = "";
  try {
    redemption.value = await redeemPass(token.value.trim());
    validation.value = null;
    token.value = "";
  } catch (cause) {
    redeemError.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a completare la riconsegna. Riprova.";
  } finally {
    redeeming.value = false;
  }
}

async function newScan() {
  resetResult();
  token.value = "";
  await stopCamera();
}

onBeforeUnmount(() => {
  void stopCamera();
});
</script>

<template>
  <div class="mx-auto w-full max-w-4xl">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p class="eyebrow">Intermediario</p>
        <h1 class="page-title mt-1">Scansiona un pass</h1>
        <p class="page-subtitle">
          Verifica il QR di riconsegna del proprietario e registra l'uscita
          dell'oggetto dalla custodia.
        </p>
      </div>
      <RouterLink to="/intermediary" class="btn btn-secondary">Dashboard</RouterLink>
    </div>

    <div class="mt-5 grid gap-4 lg:grid-cols-2">
      <section class="card">
        <h2 class="section-title">Token manuale</h2>
        <p class="mt-1 text-xs leading-5 text-stone-500">
          Se la fotocamera non è disponibile, incolla il token contenuto nel QR
          mostrato dal proprietario.
        </p>
        <form class="mt-3 space-y-3" @submit.prevent="runValidate">
          <label class="sr-only" for="pass-token">Token del pass</label>
          <textarea
            id="pass-token"
            v-model="token"
            class="input min-h-24 resize-y font-mono text-xs"
            rows="4"
            placeholder="eyJhbGciOi…"
            autocomplete="off"
            autocapitalize="off"
            spellcheck="false"
            :disabled="validating || redeeming"
          ></textarea>
          <div class="flex flex-wrap gap-2">
            <button
              type="submit"
              class="btn btn-primary"
              :disabled="validating || redeeming || !token.trim()"
            >
              <span
                v-if="validating"
                class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"
                aria-hidden="true"
              ></span>
              {{ validating ? "Verifico…" : "Verifica pass" }}
            </button>
            <button
              v-if="validation || redemption"
              type="button"
              class="btn btn-ghost"
              @click="newScan"
            >
              Nuova scansione
            </button>
          </div>
        </form>
        <p v-if="formError" class="mt-3 text-sm text-rose-700" role="alert">
          {{ formError }}
        </p>
        <p class="hint">
          Il token viene inviato solo al server e non viene salvato nel browser.
        </p>
      </section>

      <section class="card">
        <h2 class="section-title">Fotocamera</h2>
        <p class="mt-1 text-xs leading-5 text-stone-500">
          Inquadra il QR del pass. La verifica parte appena il codice viene
          letto.
        </p>

        <div class="mt-3">
          <div
            v-show="scanning || starting"
            id="qr-reader"
            class="min-h-56 overflow-hidden rounded-xl border border-stone-800 bg-stone-900"
          ></div>

          <div
            v-if="!scanning && !starting"
            class="flex min-h-56 flex-col items-center justify-center rounded-xl border border-dashed border-stone-300 bg-stone-50 px-4 text-center"
          >
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.5"
              class="h-9 w-9 text-stone-400"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M3.75 4.5v4.5m0-4.5h4.5m-4.5 0 4.5 4.5m12-4.5v4.5m0-4.5h-4.5m4.5 0-4.5 4.5m-12 10.5v-4.5m0 4.5h4.5m-4.5 0 4.5-4.5m12 4.5v-4.5m0 4.5h-4.5m4.5 0-4.5-4.5"
              />
            </svg>
            <p class="mt-2 text-xs text-stone-500">
              La fotocamera si attiva solo su tua richiesta.
            </p>
          </div>
        </div>

        <div class="mt-3 flex flex-wrap gap-2">
          <button
            v-if="!scanning"
            type="button"
            class="btn btn-primary"
            :disabled="starting || validating || redeeming"
            @click="startCamera"
          >
            {{ starting ? "Avvio…" : "Attiva fotocamera" }}
          </button>
          <button
            v-else
            type="button"
            class="btn btn-secondary"
            @click="stopCamera"
          >
            Ferma fotocamera
          </button>
        </div>
        <p v-if="cameraError" class="mt-3 text-sm text-rose-700" role="alert">
          {{ cameraError }}
        </p>
      </section>
    </div>

    <p v-if="redeemError" class="banner-error mt-4" role="alert">{{ redeemError }}</p>

    <section v-if="redemption" class="card mt-4 border-emerald-200 bg-emerald-50/60">
      <div class="flex items-start gap-3">
        <span
          class="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-emerald-700 text-white"
          aria-hidden="true"
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            class="h-6 w-6"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="m4.5 12.75 6 6 9-13.5" />
          </svg>
        </span>
        <div class="min-w-0">
          <p class="text-lg font-semibold text-emerald-950">Riconsegna registrata</p>
          <p class="mt-1 text-sm leading-6 text-emerald-900">
            L'oggetto <span class="font-mono font-semibold">{{ redemption.item.publicCode }}</span>
            è uscito dalla custodia di
            <span class="font-medium">{{ redemption.item.custodianId }}</span>.
          </p>
          <p class="mt-1 text-xs text-emerald-800">
            Riconsegnato il {{ formatDateTime(redemption.returnedAt) }}
          </p>
          <div class="mt-4 flex flex-wrap gap-2">
            <button type="button" class="btn btn-primary" @click="newScan">
              Nuova scansione
            </button>
            <RouterLink to="/intermediary" class="btn btn-secondary">
              Torna alla dashboard
            </RouterLink>
          </div>
        </div>
      </div>
    </section>

    <section
      v-else-if="validation"
      class="card mt-4"
      :class="validation.valid ? 'border-emerald-200' : 'border-amber-200'"
    >
      <div class="flex items-start gap-3">
        <span
          class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full"
          :class="
            validation.valid
              ? 'bg-emerald-100 text-emerald-800'
              : 'bg-amber-100 text-amber-800'
          "
          aria-hidden="true"
        >
          <svg
            v-if="validation.valid"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            class="h-5 w-5"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="m4.5 12.75 6 6 9-13.5" />
          </svg>
          <svg
            v-else
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            class="h-5 w-5"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m0 3h.008v.008H12v-.008ZM12 3l9 18H3L12 3Z" />
          </svg>
        </span>

        <div class="min-w-0 flex-1">
          <p class="text-base font-semibold" :class="validation.valid ? 'text-emerald-950' : 'text-amber-950'">
            {{ validation.valid ? "Pass valido" : "Pass non valido" }}
          </p>

          <template v-if="validation.valid && validation.item">
            <p class="mt-1 text-sm text-stone-700">
              Oggetto
              <span class="font-mono font-semibold">{{ validation.item.publicCode }}</span>
              · {{ validation.item.category }}
              <template v-if="validation.item.brand || validation.item.model">
                · {{ [validation.item.brand, validation.item.model].filter(Boolean).join(" ") }}
              </template>
            </p>
            <p v-if="validation.expiresAt" class="mt-1 text-xs text-stone-500">
              Pass valido fino al {{ formatDateTime(validation.expiresAt) }}.
            </p>
            <p class="mt-2 text-xs leading-5 text-stone-500">
              Verifica l'identità del proprietario prima di consegnare l'oggetto.
              La riconsegna è definitiva e non può essere annullata.
            </p>
            <div class="mt-4 flex flex-wrap gap-2">
              <button
                type="button"
                class="btn btn-primary"
                :disabled="redeeming"
                @click="redeem"
              >
                <span
                  v-if="redeeming"
                  class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"
                  aria-hidden="true"
                ></span>
                {{ redeeming ? "Registro…" : "Conferma riconsegna" }}
              </button>
              <button type="button" class="btn btn-ghost" @click="newScan">
                Annulla
              </button>
            </div>
          </template>

          <template v-else>
            <p class="mt-1 text-sm text-amber-900">
              {{ validation.reason ? passReasonLabels[validation.reason] : "Il pass non può essere utilizzato." }}
            </p>
            <button type="button" class="btn btn-secondary mt-4" @click="newScan">
              Prova un altro pass
            </button>
          </template>
        </div>
      </div>
    </section>

    <p class="mt-6 text-center text-xs leading-5 text-stone-400">
      La fotocamera richiede HTTPS o localhost. In demo il token può essere
      incollato manualmente.
    </p>
  </div>
</template>
