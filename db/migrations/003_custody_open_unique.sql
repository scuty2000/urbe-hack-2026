-- +goose Up
-- An item can have at most one open custody row at a time (§7.3 invariant).
-- The partial index is the storage-level guard: a transfer must close the old
-- row before inserting the new one, which is exactly what the transfer flow
-- does inside a single transaction.
create unique index if not exists idx_custody_open_item on custody (item_id) where closed_at is null;

-- +goose Down
drop index if exists idx_custody_open_item;
