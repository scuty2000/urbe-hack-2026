-- +goose Up
create table if not exists example_table
(
	id   INTEGER not null
		primary key autoincrement,
	name TEXT
);
-- +goose Down
drop table if exists example_table;
