-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
CREATE TABLE gonsole_records (
    id TEXT NOT NULL PRIMARY KEY,
    applied_at INTEGER NOT NULL,
    actor TEXT NOT NULL,
    account_id TEXT,
    command TEXT NOT NULL,
    args TEXT NOT NULL CHECK (json_valid(args)),
    flags TEXT NOT NULL CHECK (json_valid(flags))
);

CREATE INDEX gonsole_records_applied_at ON gonsole_records (applied_at DESC);

-- +goose Down
DROP TABLE gonsole_records;
