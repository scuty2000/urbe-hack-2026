import type { Item } from "./types";

/**
 * A finder cannot GET an item after enrollment, so the POST response is the
 * only receipt available. It is kept per tab in sessionStorage.
 */
export function receiptKey(id: string): string {
  return `oggetti-smarriti:receipt:${id}`;
}

export function saveReceipt(item: Item): boolean {
  try {
    sessionStorage.setItem(receiptKey(item.id), JSON.stringify(item));
    return true;
  } catch {
    return false;
  }
}

export function loadReceipt(id: string): Item | null {
  try {
    const raw = sessionStorage.getItem(receiptKey(id));
    if (!raw) {
      return null;
    }
    const parsed: unknown = JSON.parse(raw);
    if (!isReceipt(parsed, id)) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

function isReceipt(value: unknown, id: string): value is Item {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as Partial<Item>;
  return (
    candidate.id === id &&
    typeof candidate.publicCode === "string" &&
    typeof candidate.status === "string" &&
    typeof candidate.createdAt === "string" &&
    typeof candidate.sensitive === "boolean" &&
    typeof candidate.rightsAssigned === "boolean" &&
    Array.isArray(candidate.photos)
  );
}
