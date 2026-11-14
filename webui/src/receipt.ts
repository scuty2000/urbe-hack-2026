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
    return JSON.parse(raw) as Item;
  } catch {
    return null;
  }
}
