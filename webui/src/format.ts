import type { ItemStatus, PassReason, Role } from "./types";

export const roleLabels: Record<Role, string> = {
  finder: "Finder",
  searcher: "Chi cerca",
  intermediary: "Intermediario",
  institution: "Pubblica amministrazione",
  admin: "Amministrazione",
};

export const statusLabels: Record<ItemStatus, string> = {
  enrolled: "Registrato",
  pending_custody: "In attesa di custodia",
  custody_rejected: "Custodia rifiutata",
  in_custody: "In custodia",
  pickup_due: "Ritiro dovuto",
  sensitive_pickup: "Ritiro urgente",
  transferred: "Trasferito alla PA",
  unclaimed: "Non reclamato",
  offered_to_finder: "Offerto al finder",
  finder_collected: "Ritirato dal finder",
  disposed: "Smaltito",
};

export const statusTones: Record<ItemStatus, string> = {
  enrolled: "bg-sky-50 text-sky-800 ring-sky-200",
  pending_custody: "bg-amber-50 text-amber-800 ring-amber-200",
  custody_rejected: "bg-rose-50 text-rose-800 ring-rose-200",
  in_custody: "bg-emerald-50 text-emerald-800 ring-emerald-200",
  pickup_due: "bg-orange-50 text-orange-800 ring-orange-200",
  sensitive_pickup: "bg-rose-50 text-rose-800 ring-rose-200",
  transferred: "bg-indigo-50 text-indigo-800 ring-indigo-200",
  unclaimed: "bg-stone-100 text-stone-700 ring-stone-200",
  offered_to_finder: "bg-sky-50 text-sky-800 ring-sky-200",
  finder_collected: "bg-emerald-50 text-emerald-800 ring-emerald-200",
  disposed: "bg-stone-100 text-stone-600 ring-stone-200",
};

export const riskLabels: Record<string, string> = {
  low: "Basso",
  medium: "Medio",
  high: "Alto",
  sensitive: "Sensibile",
};

export const conditionLabels: Record<string, string> = {
  like_new: "Come nuovo",
  good: "Buono",
  fair: "Discreto",
  poor: "Da riparare",
};

export const eventLabels: Record<string, string> = {
  rights_assigned: "Diritti assegnati",
  accepted: "Custodia accettata",
  custody_rejected: "Custodia rifiutata",
  transferred: "Trasferito alla PA",
  handover: "Riconsegna",
  pickup_due: "Ritiro dovuto",
  sensitive: "Percorso sensibile",
  offered_to_finder: "Offerto al finder",
  finder_collected: "Ritirato dal finder",
  disposed: "Smaltimento",
  fee_paid: "Tariffa pagata",
};

export const passReasonLabels: Record<PassReason, string> = {
  invalid_token: "Il codice non è valido o è stato manomesso.",
  expired: "Il pass è scaduto: chiedi al proprietario di emetterne uno nuovo.",
  wrong_custodian: "Il pass è stato emesso per un altro punto di raccolta.",
  unpaid: "Il pagamento della tariffa non risulta completato.",
  revoked: "Il pass è stato revocato.",
  already_redeemed: "Il pass è già stato utilizzato per una riconsegna.",
  item_not_in_custody: "L'oggetto non è più in custodia presso questo punto.",
  claim_not_approved: "Il reclamo collegato non è approvato.",
};

export function formatDateTime(value?: string | null): string {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "—";
  }
  return new Intl.DateTimeFormat("it-IT", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function formatDate(value?: string | null): string {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "—";
  }
  return new Intl.DateTimeFormat("it-IT", {
    day: "2-digit",
    month: "long",
    year: "numeric",
  }).format(date);
}

/** Converts a File to a data URL for the canvas resize pipeline. */
export function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(new Error("Impossibile leggere la foto."));
    reader.readAsDataURL(file);
  });
}

export function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error("Immagine non valida."));
    image.src = src;
  });
}

/**
 * Resizes a photo client-side so the largest side is at most 1200px and
 * re-encodes it as JPEG. Keeps uploads small on mobile connections.
 */
export async function resizeToJpeg(file: File, maxSide = 1200, quality = 0.82): Promise<File> {
  const dataUrl = await readFileAsDataURL(file);
  const image = await loadImage(dataUrl);
  const scale = Math.min(1, maxSide / Math.max(image.width, image.height));
  const width = Math.max(1, Math.round(image.width * scale));
  const height = Math.max(1, Math.round(image.height * scale));

  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("Impossibile elaborare l'immagine.");
  }
  context.drawImage(image, 0, 0, width, height);

  const blob = await new Promise<Blob | null>((resolve) =>
    canvas.toBlob(resolve, "image/jpeg", quality),
  );
  if (!blob) {
    throw new Error("Impossibile comprimere l'immagine.");
  }

  const baseName = file.name.replace(/\.[^.]+$/, "") || "foto";
  return new File([blob], `${baseName}.jpg`, {
    type: "image/jpeg",
    lastModified: Date.now(),
  });
}

export function displayItemName(item: {
  category?: string;
  brand?: string;
  model?: string;
}): string {
  const name =
    [item.brand, item.model, item.category].filter(Boolean).join(" · ") || "Oggetto";
  return name.charAt(0).toUpperCase() + name.slice(1);
}
