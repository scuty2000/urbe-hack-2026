import type { PassResponse } from "./types";

/**
 * The raw pass token is returned exactly once by `POST /api/passes`. There is
 * no server-side read route for a searcher, so the issuing response is kept in
 * sessionStorage for the lifetime of the browser tab: enough for a reload of
 * the pass page, without persisting the token after the tab is closed.
 *
 * The entry is bound to the searcher who generated it. A stale entry can never
 * be rendered for a different signed-in user, even if the session changes in
 * the same tab (expiry, logout, demo profile switch).
 */
const PREFIX = "ritrova.pass.";

interface StoredPass {
  ownerId: string;
  pass: PassResponse;
}

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

function parseStored(raw: string): StoredPass | null {
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") {
      return null;
    }
    const stored = parsed as Partial<StoredPass>;
    if (typeof stored.ownerId !== "string" || !stored.ownerId) {
      return null;
    }
    return isPass(stored.pass) ? { ownerId: stored.ownerId, pass: stored.pass } : null;
  } catch {
    return null;
  }
}

/** Stores the pass for its owner; returns false when storage is unavailable. */
export function savePass(pass: PassResponse, ownerId: string): boolean {
  const store = storage();
  if (!store || !ownerId) {
    return false;
  }
  try {
    const stored: StoredPass = { ownerId, pass };
    store.setItem(`${PREFIX}${pass.passId}`, JSON.stringify(stored));
    return true;
  } catch {
    return false;
  }
}

/**
 * Loads a pass only when it belongs to the currently signed-in user. A missing
 * owner, a different owner or a malformed entry all resolve to null.
 */
export function loadPass(passId: string, ownerId?: string | null): PassResponse | null {
  const store = storage();
  if (!store || !passId || !ownerId) {
    return null;
  }
  try {
    const raw = store.getItem(`${PREFIX}${passId}`);
    if (!raw) {
      return null;
    }
    const stored = parseStored(raw);
    if (!stored || stored.ownerId !== ownerId) {
      // A foreign or legacy entry is useless to this user and must not linger.
      store.removeItem(`${PREFIX}${passId}`);
      return null;
    }
    return stored.pass;
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

/** Removes every pass token kept by this tab (used on auth transitions). */
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
