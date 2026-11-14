import type { PassResponse } from "./types";

/**
 * The raw pass token is returned exactly once by `POST /api/passes`. There is
 * no server-side read route for a searcher, so the issuing response is kept in
 * sessionStorage for the lifetime of the browser tab: enough for a reload of
 * the pass page, without persisting the token after the tab is closed.
 */
const PREFIX = "ritrova.pass.";

function storage(): Storage | null {
  try {
    return window.sessionStorage;
  } catch {
    // Private mode or a blocked storage policy: the pass page will show its
    // "not available in this session" state rather than break the app.
    return null;
  }
}

function isPass(value: unknown): value is PassResponse {
  if (!value || typeof value !== "object") {
    return false;
  }
  const pass = value as Partial<PassResponse>;
  return (
    typeof pass.passId === "string" &&
    pass.passId.length > 0 &&
    typeof pass.token === "string" &&
    pass.token.length > 0 &&
    typeof pass.claimId === "string" &&
    typeof pass.itemId === "string" &&
    typeof pass.expiresAt === "string" &&
    typeof pass.status === "string" &&
    !!pass.item &&
    typeof pass.item === "object"
  );
}

export function savePass(pass: PassResponse): void {
  const store = storage();
  if (!store) {
    return;
  }
  try {
    store.setItem(`${PREFIX}${pass.passId}`, JSON.stringify(pass));
  } catch {
    // Quota or policy failure: navigation still works, the page explains that
    // the QR code is only shown once.
  }
}

export function loadPass(passId: string): PassResponse | null {
  const store = storage();
  if (!store || !passId) {
    return null;
  }
  try {
    const raw = store.getItem(`${PREFIX}${passId}`);
    if (!raw) {
      return null;
    }
    const parsed: unknown = JSON.parse(raw);
    return isPass(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

export function clearPass(passId: string): void {
  const store = storage();
  if (!store) {
    return;
  }
  store.removeItem(`${PREFIX}${passId}`);
}

/** Removes every pass token kept by this tab (used on sign-out). */
export function clearAllPasses(): void {
  const store = storage();
  if (!store) {
    return;
  }
  try {
    const keys: string[] = [];
    for (let index = 0; index < store.length; index += 1) {
      const key = store.key(index);
      if (key && key.startsWith(PREFIX)) {
        keys.push(key);
      }
    }
    for (const key of keys) {
      store.removeItem(key);
    }
  } catch {
    // Storage access can fail under a restrictive policy; nothing to clear.
  }
}
