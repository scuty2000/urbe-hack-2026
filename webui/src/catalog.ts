/** Shared enrollment/intake option lists, so both forms stay consistent. */

export const itemCategories = [
  { value: "smartphone", label: "Smartphone" },
  { value: "laptop", label: "Laptop / computer" },
  { value: "portafoglio", label: "Portafoglio" },
  { value: "documenti", label: "Documenti / carta d'identità" },
  { value: "chiavi", label: "Chiavi" },
  { value: "gioielli", label: "Gioielli" },
  { value: "orologio", label: "Orologio" },
  { value: "libro", label: "Libro / quaderno" },
  { value: "bottiglia", label: "Bottiglia / borraccia" },
  { value: "ombrello", label: "Ombrello" },
  { value: "zaino", label: "Zaino / borsa" },
  { value: "occhiali", label: "Occhiali" },
  { value: "cuffie", label: "Cuffie / auricolari" },
  { value: "abbigliamento", label: "Abbigliamento" },
  { value: "bicicletta", label: "Bicicletta" },
  { value: "altro", label: "Altro" },
] as const;

export const itemColors = [
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
] as const;

export const itemConditions = [
  { value: "like_new", label: "Come nuovo" },
  { value: "good", label: "Buono" },
  { value: "fair", label: "Discreto" },
  { value: "poor", label: "Da riparare" },
] as const;

/** Value for a `datetime-local` input in the browser's local timezone. */
export function localDateTimeValue(date = new Date()): string {
  const offset = date.getTimezoneOffset();
  const local = new Date(date.getTime() - offset * 60_000);
  return local.toISOString().slice(0, 16);
}
