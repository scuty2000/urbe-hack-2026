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
  | "claim_not_approved";

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
