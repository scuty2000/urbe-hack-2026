export type Role = "finder" | "searcher" | "intermediary" | "institution" | "admin";

export interface User {
  id: string;
  role: Role;
  name: string;
  email: string;
  createdAt?: string;
}

export type ItemStatus =
  | "enrolled"
  | "pending_custody"
  | "custody_rejected"
  | "in_custody"
  | "pickup_due"
  | "sensitive_pickup"
  | "transferred"
  | "returned"
  | "unclaimed"
  | "offered_to_finder"
  | "finder_collected"
  | "disposed";

export interface Item {
  id: string;
  publicCode: string;
  source?: string;
  custodyMode?: string;
  status: ItemStatus;
  legalFinderId?: string;
  custodianId?: string;
  category?: string;
  brand?: string;
  model?: string;
  color?: string;
  condition?: string;
  photos: string[];
  foundAt?: string;
  foundZone?: string;
  foundNotes?: string;
  riskTier?: "low" | "medium" | "high" | "sensitive";
  sensitive: boolean;
  rightsAssigned: boolean;
  rightsAssignedAt?: string;
  transferredAt?: string;
  legalTermStart?: string;
  offeredToFinderAt?: string;
  disposedAt?: string;
  /** Deadline of the open custody row; absent while custody is pending. */
  dueAt?: string;
  /** Server-computed days remaining, present only with dueAt. */
  daysLeft?: number;
  createdAt: string;
}

export interface ItemListResponse {
  items: Item[];
}

export interface TimelineEvent {
  id: string;
  type: string;
  actor: string;
  actorRole?: Role;
  timestamp: string;
  notes?: string;
  evidenceRef?: string;
}

export interface ItemTimeline {
  itemId: string;
  events: TimelineEvent[];
}

export interface CustodySummary {
  itemId: string;
  status: "in_custody";
  custodianType?: "intermediary" | "institution";
  custodianId?: string;
  legalFinderId?: string;
  maxCustodyDays: number;
  acceptedAt?: string;
  dueAt: string;
  daysLeft: number;
}

export interface PassItem {
  id: string;
  publicCode: string;
  category?: string;
  brand?: string;
  model?: string;
  color?: string;
  status: "in_custody" | "transferred" | "returned";
  custodianId: string;
}

export type PassReason =
  | "invalid_token"
  | "expired"
  | "wrong_custodian"
  | "unpaid"
  | "revoked"
  | "already_redeemed"
  | "item_not_in_custody"
  | "claim_not_approved"
  | "sensitive_requires_pa"
  | "custody_expired";

export interface PassValidation {
  valid: boolean;
  reason?: PassReason;
  expiresAt?: string;
  item?: PassItem;
}

export interface PassRedemption {
  item: PassItem;
  returnedAt: string;
}

/** JSON payload sent inside the multipart `payload` part of POST /api/items. */
export interface CreateItemPayload {
  intermediaryId?: string;
  category: string;
  brand?: string;
  model?: string;
  color?: string;
  condition?: string;
  privateProfile?: Record<string, unknown>;
  foundAt?: string;
  foundLat?: number;
  foundLng?: number;
  foundZone?: string;
  foundNotes?: string;
  rightsAssigned: boolean;
}

export interface GeoPosition {
  lat: number | null;
  lng: number | null;
  zone: string;
}

// ─── Searcher chat ──────────────────────────────────────────────────────────

export type ChatRole = "user" | "assistant";

export interface ChatMessage {
  role: ChatRole;
  content: string;
}

/** Public, role-safe candidate summary returned by the agent. */
export interface ChatCandidate {
  itemId: string;
  score: number;
  category: string;
  foundZone: string;
  foundAt: string;
  custodianType: "intermediary" | "institution";
}

export interface ChatReply {
  message: string;
  candidates: ChatCandidate[];
  /** Empty string when this turn did not create a claim. */
  claimId: string;
  done: boolean;
}

// ─── Claims ─────────────────────────────────────────────────────────────────

export type ClaimStatus =
  | "submitted"
  | "under_review"
  | "approved"
  | "rejected"
  | "more_info"
  | "handed_over";

export type ClaimReturnChannel = "intermediary_direct" | "pa";

/** Searcher-safe claim view: no raw confidence or verification report. */
export interface ClaimOwner {
  id: string;
  itemId: string;
  status: ClaimStatus;
  confidenceBand: "low" | "medium" | "high";
  returnChannel: ClaimReturnChannel;
  createdAt: string;
}

/** Custodian-safe claim summary: owner fields plus evidence and flags only. */
export interface ClaimSummary extends ClaimOwner {
  evidence: string[];
  flags: string[];
}

export interface VerificationQuestion {
  type: string;
  criteria?: Record<string, string> | null;
}

export interface VerificationAnswer {
  type?: string;
  choice?: string;
  noul?: number;
  score?: number;
  probabilities?: Record<string, number>;
  confidence?: number;
}

/** Full verification report for the custodian/institution. */
export interface VerificationReportData {
  questions: Record<string, VerificationQuestion>;
  answers: Record<string, string>;
  probabilities: Record<string, VerificationAnswer>;
  confidence: number;
  rationale: string;
  modelVersion: string;
  evidence: string[];
  flags: string[];
  decision: string;
  reasons: string[];
}

export interface ClaimReport extends ClaimOwner {
  confidence: number;
  report?: VerificationReportData;
}

// ─── Passes, fees, rewards ──────────────────────────────────────────────────

export type PassStatus =
  | "pending_payment"
  | "issued"
  | "validated"
  | "redeemed"
  | "expired"
  | "revoked";

export interface PassItemSummary {
  id: string;
  publicCode: string;
  category: string;
  brand?: string;
  model?: string;
  color?: string;
  status: "in_custody" | "transferred" | "returned";
  custodianId: string;
}

export interface PassResponse {
  passId: string;
  claimId: string;
  itemId: string;
  token: string;
  status: PassStatus;
  returnChannel: ClaimReturnChannel;
  expiresAt: string;
  instructions: string[];
  item: PassItemSummary;
}

export type FeeApplies = "none_direct_return" | "pa_tariff";
export type FeeStatus = "estimated" | "final" | "paid" | "waived";

export interface Payment {
  id: string;
  feeId: string;
  provider: string;
  providerRef: string;
  status: string;
  paidAt?: string;
  receiptUrl: string;
}

export interface Fee {
  id: string;
  itemId: string;
  applies: FeeApplies;
  custodyDays: number;
  amountCents: number;
  currency: string;
  status: FeeStatus;
  paid: boolean;
  payment?: Payment;
}

export interface RewardNotice {
  id: string;
  itemId: string;
  legalFinderId: string;
  requested: boolean;
  amountCents: number;
  shownToSearcherAt?: string;
  paymentStatus: string;
}

// ─── Institution ────────────────────────────────────────────────────────────

export interface InstitutionPickupItem {
  id: string;
  publicCode: string;
  category: string;
  brand?: string;
  model?: string;
  color?: string;
  status: string;
  foundZone: string;
  intermediaryId: string;
  sensitive: boolean;
  dueAt?: string;
  daysOverdue: number;
}

export interface InstitutionPickupGroup {
  intermediaryId: string;
  zone: string;
  sensitive: boolean;
  count: number;
  items: InstitutionPickupItem[];
}

export interface InstitutionPickupList {
  groups: InstitutionPickupGroup[];
  total: number;
}

/** JSON payload of the institution official-intake multipart request. */
export interface OfficialItemPayload {
  category: string;
  brand?: string;
  model?: string;
  color?: string;
  condition?: string;
  privateProfile?: Record<string, unknown>;
  foundAt?: string;
  foundLat?: number;
  foundLng?: number;
  foundZone?: string;
  foundNotes?: string;
}

// ─── Notifications and health ───────────────────────────────────────────────

export interface AppNotification {
  id: string;
  type: string;
  payload: Record<string, unknown>;
  read: boolean;
  readAt?: string;
  createdAt: string;
}

export interface HealthStatus {
  ok: boolean;
  db: boolean;
  qdrant: boolean;
  embeddings: boolean;
  /** True when the D3 decision engine is enabled and configured. */
  d3?: boolean;
  /** "deepseek" when the conversational provider is active, "mock" otherwise. */
  agentMode?: "deepseek" | "mock" | string;
}

// ─── Finder wizard ──────────────────────────────────────────────────────────

export interface WizardPhotoSlot {
  key: string;
  label: string;
  instruction: string;
  required: boolean;
}

export interface WizardPrefill {
  category: string;
  brand: string;
  model: string;
  color: string;
  condition: string;
}

export interface WizardAnalysis {
  objectType: string;
  confidence: number;
  photoPlan: WizardPhotoSlot[];
  prefill: WizardPrefill;
  privatePrefill: string;
  privateAttributes?: Record<string, unknown>;
  warnings: string[];
  source: "d3+deepseek" | "d3+fallback" | "d3" | "manual" | string;
  isDocument: boolean;
  sensitive: boolean;
}

export interface ReverseGeocode {
  zone: string;
  placeName: string;
  source: "mapbox" | "unavailable" | string;
}

export interface DropPoint {
  id: string;
  name: string;
  address: string;
  hours: string;
  distanceMeters: number;
  durationSeconds: number;
  walkingMinutes: number;
  approximate: boolean;
  source: "mapbox" | "haversine" | string;
}

export interface DropPointsResponse {
  points: DropPoint[];
  source: string;
  approximate: boolean;
}
