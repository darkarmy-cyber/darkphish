-- +goose Up
CREATE TABLE directory_connectors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    owner_user_id INTEGER NOT NULL,
    provider VARCHAR(32) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    client_id VARCHAR(64) NOT NULL,
    client_secret TEXT NOT NULL,
    remote_group_id VARCHAR(64) NOT NULL,
    target_group_id INTEGER,
    email_domains TEXT NOT NULL DEFAULT '[]',
    enabled BOOLEAN NOT NULL DEFAULT 0,
    sync_interval_minutes INTEGER NOT NULL DEFAULT 0,
    last_sync_at DATETIME,
    next_sync_at DATETIME,
    created_at DATETIME NOT NULL,
    modified_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX idx_directory_connectors_owner_name ON directory_connectors(owner_user_id, name);
CREATE INDEX idx_directory_connectors_owner ON directory_connectors(owner_user_id);
CREATE INDEX idx_directory_connectors_due ON directory_connectors(enabled, next_sync_at);

CREATE TABLE directory_sync_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    connector_id INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    discovered INTEGER NOT NULL DEFAULT 0,
    added INTEGER NOT NULL DEFAULT 0,
    updated INTEGER NOT NULL DEFAULT 0,
    removed INTEGER NOT NULL DEFAULT 0,
    skipped INTEGER NOT NULL DEFAULT 0,
    error_code VARCHAR(64) NOT NULL DEFAULT ''
);
CREATE INDEX idx_directory_sync_runs_connector ON directory_sync_runs(connector_id, started_at);

-- +goose Down
DROP TABLE directory_sync_runs;
DROP TABLE directory_connectors;
