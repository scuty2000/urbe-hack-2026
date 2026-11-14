-- +goose Up
-- Lost-and-found registry schema.
--
-- Conventions:
--   * ids are application-generated UUIDs stored as TEXT;
--   * timestamps are ISO-8601 DATETIME values written by the Go layer (UTC);
--   * JSON payloads are TEXT columns serialised by the domain layer;
--   * booleans are INTEGER 0/1 and money is INTEGER cents.

create table if not exists users
(
	id         TEXT not null
		primary key,
	role       TEXT not null default '',
	name       TEXT not null default '',
	email      TEXT not null default '',
	created_at DATETIME
);

create table if not exists sessions
(
	token_hash TEXT not null
		primary key,
	user_id    TEXT not null,
	expires_at DATETIME,
	created_at DATETIME
);

create index if not exists idx_sessions_expires_at on sessions (expires_at);

create table if not exists intermediaries
(
	id            TEXT not null
		primary key,
	name          TEXT not null default '',
	address       TEXT not null default '',
	geo_lat       REAL not null default 0,
	geo_lng       REAL not null default 0,
	hours         TEXT not null default '',
	capacity      INTEGER not null default 0,
	status        TEXT not null default '',
	agreement_ref TEXT not null default '',
	contact       TEXT not null default ''
);

create table if not exists items
(
	id                    TEXT not null
		primary key,
	public_code           TEXT not null default '',
	source                TEXT not null default '',
	custody_mode          TEXT not null default '',
	status                TEXT not null default '',
	legal_finder_id       TEXT not null default '',
	rights_assigned_at    DATETIME,
	category              TEXT not null default '',
	brand                 TEXT not null default '',
	model                 TEXT not null default '',
	color                 TEXT not null default '',
	condition             TEXT not null default '',
	private_profile       TEXT not null default '{}',
	photos                TEXT not null default '[]',
	found_at              DATETIME,
	found_lat             REAL not null default 0,
	found_lng             REAL not null default 0,
	found_zone            TEXT not null default '',
	found_notes           TEXT not null default '',
	custodian_id          TEXT not null default '',
	risk_tier             TEXT not null default '',
	sensitive             INTEGER not null default 0,
	transferred_at        DATETIME,
	legal_term_start      DATETIME,
	offered_to_finder_at  DATETIME,
	disposed_at           DATETIME,
	created_at            DATETIME,
	updated_at            DATETIME
);

create index if not exists idx_items_status_category_zone on items (status, category, found_zone);
create index if not exists idx_items_custodian on items (custodian_id);
create index if not exists idx_items_legal_finder on items (legal_finder_id);

create table if not exists custody
(
	id                 TEXT not null
		primary key,
	item_id            TEXT not null,
	custodian_type     TEXT not null default '',
	custodian_id       TEXT not null default '',
	legal_finder_id    TEXT not null default '',
	rights_assigned_at DATETIME,
	approval_authority TEXT not null default '',
	max_custody_days   INTEGER not null default 0,
	accepted_at        DATETIME,
	due_at             DATETIME,
	closed_at          DATETIME
);

create index if not exists idx_custody_due_at on custody (due_at);
create index if not exists idx_custody_item_id on custody (item_id);

create table if not exists custody_events
(
	id           TEXT not null
		primary key,
	item_id      TEXT not null,
	type         TEXT not null default '',
	actor_id     TEXT not null default '',
	timestamp    DATETIME,
	notes        TEXT not null default '',
	evidence_ref TEXT not null default ''
);

create index if not exists idx_custody_events_item on custody_events (item_id, timestamp);

create table if not exists claims
(
	id               TEXT not null
		primary key,
	item_id          TEXT not null,
	searcher_id      TEXT not null default '',
	status           TEXT not null default '',
	return_channel   TEXT not null default '',
	confidence       REAL not null default 0,
	evidence         TEXT not null default '[]',
	approval_by      TEXT not null default '',
	approved_at      DATETIME,
	pickup_pass_id   TEXT not null default '',
	reward_notice_id TEXT not null default '',
	created_at       DATETIME
);

create index if not exists idx_claims_item_id on claims (item_id);
create index if not exists idx_claims_searcher on claims (searcher_id);

create table if not exists verification_sessions
(
	id            TEXT not null
		primary key,
	claim_id      TEXT not null,
	questions     TEXT not null default '{}',
	answers       TEXT not null default '{}',
	probabilities TEXT not null default '{}',
	rationale     TEXT not null default '',
	model_version TEXT not null default '',
	flags         TEXT not null default '[]',
	created_at    DATETIME
);

create index if not exists idx_verification_sessions_claim on verification_sessions (claim_id);

create table if not exists pickup_passes
(
	id                TEXT not null
		primary key,
	claim_id          TEXT not null,
	item_id           TEXT not null,
	searcher_id       TEXT not null default '',
	custodian_id      TEXT not null default '',
	token_hash        TEXT not null,
	issued_at         DATETIME,
	expires_at        DATETIME,
	status            TEXT not null default '',
	identity_required INTEGER not null default 0,
	return_channel    TEXT not null default '',
	payment_id        TEXT not null default '',
	approval_id       TEXT not null default '',
	fee_id            TEXT not null default '',
	reward_notice_id  TEXT not null default ''
);

create unique index if not exists idx_pickup_passes_token_hash on pickup_passes (token_hash);
create index if not exists idx_pickup_passes_item on pickup_passes (item_id);
create index if not exists idx_pickup_passes_claim on pickup_passes (claim_id);

create table if not exists fees
(
	id             TEXT not null
		primary key,
	item_id        TEXT not null,
	applies        TEXT not null default '',
	pa_tariff_id   TEXT not null default '',
	custody_days   INTEGER not null default 0,
	amount_cents   INTEGER not null default 0,
	currency       TEXT not null default 'EUR',
	status         TEXT not null default ''
);

create index if not exists idx_fees_item_id on fees (item_id);

create table if not exists payments
(
	id           TEXT not null
		primary key,
	fee_id       TEXT not null,
	provider     TEXT not null default '',
	provider_ref TEXT not null default '',
	status       TEXT not null default '',
	paid_at      DATETIME,
	receipt_url  TEXT not null default ''
);

create index if not exists idx_payments_fee_id on payments (fee_id);

create table if not exists reward_notices
(
	id                    TEXT not null
		primary key,
	item_id               TEXT not null,
	legal_finder_id       TEXT not null default '',
	requested             INTEGER not null default 0,
	amount_cents          INTEGER not null default 0,
	shown_to_searcher_at  DATETIME,
	payment_status        TEXT not null default ''
);

create index if not exists idx_reward_notices_item_id on reward_notices (item_id);

create table if not exists notifications
(
	id           TEXT not null
		primary key,
	recipient_id TEXT not null,
	type         TEXT not null default '',
	payload      TEXT not null default '{}',
	read_at      DATETIME,
	created_at   DATETIME
);

create index if not exists idx_notifications_recipient on notifications (recipient_id, created_at);

create table if not exists audit_logs
(
	id        TEXT not null
		primary key,
	actor_id  TEXT not null default '',
	action    TEXT not null default '',
	entity    TEXT not null default '',
	entity_id TEXT not null default '',
	metadata  TEXT not null default '{}',
	timestamp DATETIME
);

create index if not exists idx_audit_logs_entity on audit_logs (entity, entity_id);
create index if not exists idx_audit_logs_actor on audit_logs (actor_id);

create table if not exists lost_reports
(
	id          TEXT not null
		primary key,
	searcher_id TEXT not null default '',
	description TEXT not null default '',
	category    TEXT not null default '',
	location    TEXT not null default '',
	time_range  TEXT not null default '',
	embedding   TEXT not null default '[]',
	status      TEXT not null default ''
);

create index if not exists idx_lost_reports_searcher on lost_reports (searcher_id);

create table if not exists tariffs
(
	id               TEXT not null
		primary key,
	daily_rate_cents INTEGER not null default 0,
	fixed_fee_cents  INTEGER not null default 0,
	currency         TEXT not null default 'EUR',
	active           INTEGER not null default 0
);

-- +goose Down
drop table if exists tariffs;
drop table if exists lost_reports;
drop table if exists audit_logs;
drop table if exists notifications;
drop table if exists reward_notices;
drop table if exists payments;
drop table if exists fees;
drop table if exists pickup_passes;
drop table if exists verification_sessions;
drop table if exists claims;
drop table if exists custody_events;
drop table if exists custody;
drop table if exists items;
drop table if exists intermediaries;
drop table if exists sessions;
drop table if exists users;
