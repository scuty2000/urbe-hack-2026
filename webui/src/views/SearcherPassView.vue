<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink, useRoute } from "vue-router";
import {
  ApiError,
  getClaim,
  getItemFee,
  getRewardNotice,
  payFee,
  user,
} from "../api";
import FeeSummary from "../components/FeeSummary.vue";
import QRCode from "../components/QRCode.vue";
import RewardNoticePanel from "../components/RewardNotice.vue";
import {
  displayItemName,
  formatCents,
  formatDateTime,
  organisationLabel,
  passStatusLabels,
  returnChannelLabels,
} from "../format";
import { clearPass, loadPass } from "../passStore";
import type { Fee, PassResponse, RewardNotice } from "../types";

const route = useRoute();

const pass = ref<PassResponse | null>(null);
const fee = ref<Fee | null>(null);
const reward = ref<RewardNotice | null>(null);
const feeLoading = ref(false);
const rewardLoading = ref(false);
const feeError = ref("");
const rewardError = ref("");
const paying = ref(false);
const actionError = ref("");
const notice = ref("");
const showCode = ref(true);
const now = ref(Date.now());
const verifying = ref(true);
const accessError = ref("");
const claimWarning = ref("");

const passId = computed(() => String(route.params.id ?? ""));

const expired = computed(() =>
  pass.value ? new Date(pass.value.expiresAt).getTime() <= now.value : false,
);

const paid = computed(() => Boolean(fee.value?.paid));

const awaitingPayment = computed(
  () =>
    Boolean(pass.value) &&
    (pass.value?.status === "pending_payment" || fee.value?.applies === "pa_tariff") &&
    !paid.value,
);

const displayStatus = computed(() => {
  if (!pass.value) {
    return "";
  }
  if (expired.value) {
    return "expired";
  }
  if (pass.value.status === "pending_payment" && paid.value) {
    return "issued";
  }
  return pass.value.status;
});

const statusTone = computed(() => {
  switch (displayStatus.value) {
    case "issued":
    case "validated":
      return "bg-olive-soft text-olive ring-olive/30";
    case "pending_payment":
      return "bg-ochre-soft text-ochre ring-ochre/30";
    case "redeemed":
      return "bg-paper text-ink-soft ring-line";
    case "expired":
    case "revoked":
      return "bg-rust-soft text-rust ring-rust/30";
    default:
      return "bg-teal-soft text-teal ring-teal/25";
  }
});

const muted = computed(() => expired.value || awaitingPayment.value);

const identityRequired = computed(
  () =>
    pass.value?.instructions.some((line) =>
      line.toLowerCase().includes("identity document"),
    ) ?? false,
);

const instructions = computed(() => {
  if (!pass.value) {
    return [] as string[];
  }
  const passValue = pass.value;
  return passValue.instructions.map((line) => {
    const lower = line.toLowerCase();
    if (lower.includes("show this qr code")) {
      return `Mostra questo codice QR al punto di ritiro: ${organisationLabel(
        passValue.item.custodianId,
      )}.`;
    }
    if (lower.includes("single-use and expires")) {
      return `Il pass è monouso e scade il ${formatDateTime(passValue.expiresAt)}.`;
    }
    if (lower.includes("do not share")) {
      return "Non condividere il codice: chi lo possiede può ritirare l'oggetto.";
    }
    if (lower.includes("public administration fee")) {
      return "La tariffa della pubblica amministrazione deve essere pagata prima della consegna.";
    }
    if (lower.includes("identity document")) {
      return "Porta un documento d'identità valido per il controllo del custode.";
    }
    return line;
  });
});

onMounted(() => {
  void initialise();
});

async function initialise() {
  verifying.value = true;
  accessError.value = "";
  claimWarning.value = "";
  const owner = user.value?.id;
  const stored = loadPass(passId.value, owner);
  if (!stored) {
    pass.value = null;
    verifying.value = false;
    return;
  }
  pass.value = stored;
  const allowed = await verifyClaim(stored);
  verifying.value = false;
  if (allowed && pass.value) {
    await loadSummary(stored);
  }
}

/**
 * Defence in depth on top of the owner-bound sessionStorage entry: the searcher
 * must still be able to read the claim, and the claim must point at the same
 * item as the pass. A 403/404 clears the cached token.
 */
async function verifyClaim(current: PassResponse): Promise<boolean> {
  try {
    const claim = await getClaim(current.claimId);
    if (claim.itemId !== current.itemId) {
      accessError.value =
        "Il pass non corrisponde alla richiesta registrata. Generane uno nuovo dalla ricerca.";
      clearPass(current.passId);
      pass.value = null;
      return false;
    }
    return true;
  } catch (cause) {
    if (
      cause instanceof ApiError &&
      (cause.status === 403 || cause.status === 404)
    ) {
      accessError.value =
        "Questo pass non appartiene al tuo profilo. Torna alla ricerca e apri la tua richiesta.";
      clearPass(current.passId);
      pass.value = null;
      return false;
    }
    claimWarning.value =
      "Non riesco a verificare la richiesta in questo momento: il pass resta visibile solo in questa sessione.";
    return true;
  }
}

async function loadSummary(current: PassResponse) {
  await Promise.all([loadFee(current), loadReward(current)]);
  now.value = Date.now();
}

async function loadFee(current: PassResponse) {
  feeLoading.value = true;
  feeError.value = "";
  try {
    fee.value = await getItemFee(current.itemId);
  } catch (cause) {
    fee.value = null;
    feeError.value =
      cause instanceof ApiError
        ? cause.message
        : "Non riesco a leggere la tariffa.";
  } finally {
    feeLoading.value = false;
  }
}

async function loadReward(current: PassResponse) {
  rewardLoading.value = true;
  rewardError.value = "";
  try {
    reward.value = await getRewardNotice(current.itemId);
  } catch (cause) {
    reward.value = null;
    if (cause instanceof ApiError && cause.status === 404) {
      rewardError.value = "";
    } else {
      rewardError.value =
        cause instanceof ApiError
          ? cause.message
          : "Non riesco a leggere la richiesta di ricompensa.";
    }
  } finally {
    rewardLoading.value = false;
  }
}

async function pay() {
  if (!pass.value || !fee.value || paying.value) {
    return;
  }
  paying.value = true;
  actionError.value = "";
  notice.value = "";
  try {
    await payFee(fee.value.id);
    const current = pass.value;
    await loadFee(current);
    await loadReward(current);
    notice.value = paid.value
      ? "Tariffa pagata: il pass è ora valido per il ritiro."
      : "Pagamento registrato. Aggiorna tra poco se lo stato non è ancora aggiornato.";
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 409) {
      actionError.value =
        "La tariffa non può essere pagata in questo momento. Aggiorna la pagina o contatta la PA.";
    } else {
      actionError.value =
        cause instanceof ApiError
          ? cause.message
          : "Non riesco a completare il pagamento simulato. Riprova.";
    }
  } finally {
    paying.value = false;
  }
}
</script>

<template>
  <div class="mx-auto w-full max-w-4xl">
    <RouterLink :to="{ name: 'searcher' }" class="link text-sm">
      ← Torna alla ricerca
    </RouterLink>

    <div v-if="verifying" class="mt-4 card animate-pulse">
      <div class="h-6 w-1/2 rounded bg-paper-deep"></div>
      <div class="mt-3 h-4 w-1/3 rounded bg-paper-deep"></div>
      <div class="mt-6 h-40 rounded-md bg-paper"></div>
      <p class="mt-4 text-xs text-ink-muted">Verifico il pass e la richiesta…</p>
    </div>

    <div v-else-if="!pass" class="empty-state mt-4">
      <p class="text-sm font-medium text-ink-soft">
        {{ accessError || "Pass non disponibile" }}
      </p>
      <p class="mx-auto mt-2 max-w-md text-xs leading-5 text-ink-muted">
        Il codice QR viene mostrato una sola volta e resta solo nella sessione
        del browser in cui è stato generato, legato al profilo che lo ha
        richiesto. Torna alla ricerca e genera un nuovo pass dalla richiesta
        approvata.
      </p>
      <RouterLink :to="{ name: 'searcher' }" class="btn btn-primary mt-4">
        Torna alla ricerca
      </RouterLink>
    </div>

    <template v-else>
      <header class="mt-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <p class="eyebrow">Pass di ritiro</p>
          <h1 class="page-title mt-1">{{ displayItemName(pass.item) }}</h1>
          <p class="mt-1 font-mono text-sm text-ink-soft">{{ pass.item.publicCode }}</p>
        </div>
        <span class="badge" :class="statusTone">
          {{ passStatusLabels[displayStatus] ?? displayStatus }}
        </span>
      </header>

      <div class="mt-4 space-y-2">
        <p v-if="notice" class="banner-success" role="status">{{ notice }}</p>
        <p v-if="actionError" class="banner-error" role="alert">{{ actionError }}</p>
        <p v-if="claimWarning" class="banner-info" role="status">{{ claimWarning }}</p>
        <p
          v-if="expired"
          class="banner-error"
          role="alert"
        >
          Questo pass è scaduto il {{ formatDateTime(pass.expiresAt) }}. Torna
          alla ricerca e genera un nuovo pass.
        </p>
        <p v-else-if="awaitingPayment" class="banner-info" role="status">
          Il pass è in attesa del pagamento della tariffa PA: il codice QR non è
          ancora valido al banco.
        </p>
      </div>

      <div class="mt-5 grid gap-4 lg:grid-cols-[auto_1fr]">
        <section class="card flex flex-col items-center justify-center border-ink/30">
          <QRCode
            v-if="showCode"
            :value="pass.token"
            :size="230"
            :muted="muted"
            label="Codice QR del pass di ritiro"
          />
          <div v-else class="flex h-60 w-60 flex-col items-center justify-center rounded-lg border border-dashed border-line bg-paper px-6 text-center">
            <p class="text-sm font-medium text-ink-soft">Codice nascosto</p>
            <p class="mt-1 text-xs text-ink-muted">
              Mostralo solo al momento del ritiro.
            </p>
          </div>

          <div class="mt-3 flex flex-wrap justify-center gap-2">
            <button type="button" class="btn btn-secondary px-3 py-2 text-xs" @click="showCode = !showCode">
              {{ showCode ? "Nascondi codice" : "Mostra codice" }}
            </button>
          </div>

          <dl class="mt-4 grid w-full grid-cols-1 gap-2 border-t border-dashed border-ink/25 pt-3 text-center text-xs">
            <div>
              <dt class="data-label">Scade il</dt>
              <dd class="mt-0.5 font-medium text-ink">{{ formatDateTime(pass.expiresAt) }}</dd>
            </div>
            <div>
              <dt class="data-label">Richiesta</dt>
              <dd class="mt-0.5 font-mono text-[11px] text-ink-soft">{{ pass.claimId }}</dd>
            </div>
          </dl>
        </section>

        <div class="space-y-4">
          <section class="card">
            <h2 class="section-title">Dove e come ritirare</h2>
            <dl class="mt-3 grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
              <div>
                <dt class="data-label">Canale di riconsegna</dt>
                <dd class="data-value text-xs">
                  {{ returnChannelLabels[pass.returnChannel] ?? pass.returnChannel }}
                </dd>
              </div>
              <div>
                <dt class="data-label">Custode</dt>
                <dd class="data-value">{{ organisationLabel(pass.item.custodianId) }}</dd>
              </div>
              <div>
                <dt class="data-label">Oggetto</dt>
                <dd class="data-value">{{ displayItemName(pass.item) }}</dd>
              </div>
              <div>
                <dt class="data-label">Codice pubblico</dt>
                <dd class="data-value font-mono text-xs">{{ pass.item.publicCode }}</dd>
              </div>
            </dl>

            <div v-if="instructions.length > 0" class="mt-4">
              <h3 class="text-xs font-semibold tracking-wide text-ink-muted uppercase">
                Istruzioni
              </h3>
              <ul class="mt-1.5 space-y-1 text-xs leading-5 text-ink-soft">
                <li v-for="instruction in instructions" :key="instruction" class="flex gap-2">
                  <span class="text-vermilion-dark" aria-hidden="true">✓</span>
                  <span>{{ instruction }}</span>
                </li>
              </ul>
            </div>

            <div class="mt-4 rounded-md bg-paper px-3 py-2.5">
              <h3 class="text-xs font-semibold tracking-wide text-ink-muted uppercase">
                Cosa portare
              </h3>
              <ul class="mt-1.5 space-y-1 text-xs leading-5 text-ink-soft">
                <li class="flex gap-2">
                  <span class="text-ink-muted" aria-hidden="true">•</span>
                  <span>Il codice QR di questo pass (monouso).</span>
                </li>
                <li v-if="identityRequired" class="flex gap-2">
                  <span class="text-ink-muted" aria-hidden="true">•</span>
                  <span>Un documento d'identità valido.</span>
                </li>
                <li v-if="pass.returnChannel === 'pa'" class="flex gap-2">
                  <span class="text-ink-muted" aria-hidden="true">•</span>
                  <span>
                    Il pagamento della tariffa PA<template v-if="paid"> (già registrato)</template>.
                  </span>
                </li>
                <li v-if="reward?.requested" class="flex gap-2">
                  <span class="text-ink-muted" aria-hidden="true">•</span>
                  <span>
                    L'eventuale ricompensa concordata con il finder
                    ({{ reward.amountCents > 0 ? formatCents(reward.amountCents) : "importo da concordare" }}),
                    fuori dalla piattaforma.
                  </span>
                </li>
              </ul>
            </div>
          </section>

          <FeeSummary
            :fee="fee"
            :loading="feeLoading"
            :error="feeError"
            :payable="pass.returnChannel === 'pa'"
            :busy="paying"
            @pay="pay"
          />

          <RewardNoticePanel
            :notice="reward"
            :loading="rewardLoading"
            :error="rewardError"
          />
        </div>
      </div>
    </template>
  </div>
</template>
