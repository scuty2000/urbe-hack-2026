import { readonly, ref } from "vue";
import { clearAllPasses } from "./passStore";
import type {
  AppNotification,
  ChatMessage,
  ChatReply,
  ClaimOwner,
  ClaimReport,
  ClaimSummary,
  CreateItemPayload,
  CustodySummary,
  Fee,
  HealthStatus,
  InstitutionPickupList,
  Item,
  ItemListResponse,
  ItemTimeline,
  OfficialItemPayload,
  PassRedemption,
  PassResponse,
  PassValidation,
  Payment,
  RewardNotice,
  User,
} from "./types";

/** Error returned by the API as `{ "error": "..." }`, or a network failure. */
export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status = 0) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

const currentUser = ref<User | null>(null);
const sessionError = ref<string | null>(null);
let sessionLoaded = false;
let sessionRequest: Promise<User | null> | null = null;

export const user = readonly(currentUser);
export const authError = readonly(sessionError);

interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  json?: unknown;
  body?: BodyInit;
  signal?: AbortSignal;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  let body = options.body;

  if (options.json !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(options.json);
  }

  let response: Response;
  try {
    response = await fetch(path, {
      method: options.method ?? "GET",
      body,
      headers,
      credentials: "include",
      signal: options.signal,
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") {
      throw error;
    }
    throw new ApiError("Impossibile raggiungere il server. Riprova.", 0);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const contentType = response.headers.get("content-type") ?? "";
  let payload: unknown = null;
  if (contentType.includes("application/json")) {
    payload = await response.json().catch(() => null);
  }

  if (!response.ok) {
    if (response.status === 401) {
      currentUser.value = null;
      sessionLoaded = true;
      // A session change invalidates every cached pass token: the next user in
      // this tab must never see the previous searcher's QR.
      clearAllPasses();
      // An expired session found while browsing should return to the login
      // page. The initial /api/auth/me probe is handled by the router guard.
      if (
        path !== "/api/auth/me" &&
        path !== "/api/auth/login" &&
        typeof window !== "undefined"
      ) {
        window.dispatchEvent(new CustomEvent("auth:unauthorized"));
      }
    }
    const message =
      payload && typeof payload === "object" && "error" in payload
        ? String((payload as { error: unknown }).error)
        : `Errore del server (${response.status}).`;
    throw new ApiError(message, response.status);
  }

  return payload as T;
}

/**
 * Loads the session once. Concurrent callers share the same request, which
 * keeps the router guard from firing /api/auth/me repeatedly.
 */
export function loadSession(force = false): Promise<User | null> {
  if (sessionLoaded && !force) {
    return Promise.resolve(currentUser.value);
  }
  if (sessionRequest) {
    return sessionRequest;
  }

  sessionRequest = (async () => {
    try {
      const me = await request<User>("/api/auth/me");
      currentUser.value = me;
      sessionError.value = null;
      return me;
    } catch (error) {
      currentUser.value = null;
      if (error instanceof ApiError && error.status === 401) {
        sessionError.value = null;
        return null;
      }
      sessionError.value =
        error instanceof ApiError ? error.message : "Sessione non disponibile.";
      return null;
    } finally {
      sessionLoaded = true;
      sessionRequest = null;
    }
  })();

  return sessionRequest;
}

export async function login(userId: string): Promise<User> {
  const me = await request<User>("/api/auth/login", {
    method: "POST",
    json: { userId },
  });
  // A new identity starts with a clean token cache.
  clearAllPasses();
  currentUser.value = me;
  sessionLoaded = true;
  sessionError.value = null;
  return me;
}

export async function logout(): Promise<void> {
  try {
    await request<void>("/api/auth/logout", { method: "POST" });
  } finally {
    currentUser.value = null;
    sessionLoaded = true;
    sessionError.value = null;
    clearAllPasses();
  }
}

export function clearSession(): void {
  currentUser.value = null;
  sessionLoaded = true;
  sessionError.value = null;
  clearAllPasses();
}

export async function createItem(
  payload: CreateItemPayload,
  photos: File[],
): Promise<Item> {
  const form = new FormData();
  form.append("payload", JSON.stringify(payload));
  for (const photo of photos) {
    form.append("photos", photo, photo.name);
  }
  return request<Item>("/api/items", { method: "POST", body: form });
}

export async function listMyItems(): Promise<Item[]> {
  const response = await request<ItemListResponse>("/api/items?custodian=me");
  return response.items;
}

export function getItem(id: string): Promise<Item> {
  return request<Item>(`/api/items/${encodeURIComponent(id)}`);
}

export async function getTimeline(id: string): Promise<ItemTimeline> {
  return request<ItemTimeline>(`/api/items/${encodeURIComponent(id)}/timeline`);
}

export function acceptCustody(id: string): Promise<CustodySummary> {
  return request<CustodySummary>(
    `/api/items/${encodeURIComponent(id)}/custody/accept`,
    { method: "POST" },
  );
}

export function refuseCustody(id: string, reason: string): Promise<Item> {
  return request<Item>(`/api/items/${encodeURIComponent(id)}/custody/refuse`, {
    method: "POST",
    json: { reason },
  });
}

export function validatePass(token: string): Promise<PassValidation> {
  return request<PassValidation>("/api/passes/validate", {
    method: "POST",
    json: { token },
  });
}

export function redeemPass(token: string): Promise<PassRedemption> {
  return request<PassRedemption>("/api/passes/redeem", {
    method: "POST",
    json: { token },
  });
}

// ─── Searcher chat, claims and passes ───────────────────────────────────────

/** Sends the conversation so far; the server builds the verification session. */
export function chat(messages: ChatMessage[]): Promise<ChatReply> {
  return request<ChatReply>("/api/chat", {
    method: "POST",
    json: { messages },
  });
}

/** Searcher-safe claim view (status, confidence band, pickup channel). */
export function getClaim(id: string): Promise<ClaimOwner> {
  return request<ClaimOwner>(`/api/claims/${encodeURIComponent(id)}`);
}

/** Custodian-safe claim summary (owner fields, evidence and flags). */
export function getClaimSummary(id: string): Promise<ClaimSummary> {
  return request<ClaimSummary>(`/api/claims/${encodeURIComponent(id)}`);
}

/** Records a custodian/institution decision and returns the safe summary. */
export function reviewClaim(
  id: string,
  decision: "approved" | "rejected",
): Promise<ClaimSummary> {
  return request<ClaimSummary>(`/api/claims/${encodeURIComponent(id)}/review`, {
    method: "POST",
    json: { decision },
  });
}

/** Issues the single-use pickup pass for an approved claim. */
export function createPass(claimId: string): Promise<PassResponse> {
  return request<PassResponse>("/api/passes", {
    method: "POST",
    json: { claimId },
  });
}

// ─── Fees and rewards ───────────────────────────────────────────────────────

export function getItemFee(itemId: string): Promise<Fee> {
  return request<Fee>(`/api/items/${encodeURIComponent(itemId)}/fee`);
}

/** Settles the fee through the mock provider; no real payment is involved. */
export function payFee(feeId: string): Promise<Payment> {
  return request<Payment>(`/api/fees/${encodeURIComponent(feeId)}/pay`, {
    method: "POST",
  });
}

export function getRewardNotice(itemId: string): Promise<RewardNotice> {
  return request<RewardNotice>(
    `/api/items/${encodeURIComponent(itemId)}/reward-notice`,
  );
}

// ─── Institution ────────────────────────────────────────────────────────────

export function listInstitutionPickups(): Promise<InstitutionPickupList> {
  return request<InstitutionPickupList>("/api/institution/pickups");
}

export function transferToInstitution(id: string): Promise<Item> {
  return request<Item>(
    `/api/institution/items/${encodeURIComponent(id)}/transfer`,
    { method: "POST" },
  );
}

export async function createInstitutionItem(
  payload: OfficialItemPayload,
  photos: File[],
): Promise<Item> {
  const form = new FormData();
  form.append("payload", JSON.stringify(payload));
  for (const photo of photos) {
    form.append("photos", photo, photo.name);
  }
  return request<Item>("/api/institution/items", {
    method: "POST",
    body: form,
  });
}

export function getInstitutionClaim(id: string): Promise<ClaimReport> {
  return request<ClaimReport>(
    `/api/institution/claims/${encodeURIComponent(id)}`,
  );
}

export function reviewInstitutionClaim(
  id: string,
  decision: "approved" | "rejected",
): Promise<ClaimReport> {
  return request<ClaimReport>(
    `/api/institution/claims/${encodeURIComponent(id)}/review`,
    { method: "POST", json: { decision } },
  );
}

export function offerToFinder(id: string): Promise<Item> {
  return request<Item>(`/api/institution/items/${encodeURIComponent(id)}/offer`, {
    method: "POST",
  });
}

export function collectFromFinder(id: string): Promise<Item> {
  return request<Item>(
    `/api/institution/items/${encodeURIComponent(id)}/collect`,
    { method: "POST" },
  );
}

export function disposeItem(id: string): Promise<Item> {
  return request<Item>(
    `/api/institution/items/${encodeURIComponent(id)}/dispose`,
    { method: "POST" },
  );
}

// ─── Notifications and health ───────────────────────────────────────────────

export async function listNotifications(): Promise<AppNotification[]> {
  const response = await request<{ notifications: AppNotification[] }>(
    "/api/notifications",
  );
  return response.notifications;
}

export function markNotificationRead(
  id: string,
): Promise<{ id: string; read: boolean }> {
  return request<{ id: string; read: boolean }>(
    `/api/notifications/${encodeURIComponent(id)}/read`,
    { method: "POST" },
  );
}

export function getHealth(): Promise<HealthStatus> {
  return request<HealthStatus>("/api/health");
}
