-- +goose Up
CREATE TABLE directory_connectors (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    name_key VARCHAR(255) NOT NULL,
    owner_user_id BIGINT NOT NULL,
    provider VARCHAR(32) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    client_id VARCHAR(64) NOT NULL,
    client_secret TEXT NOT NULL,
    remote_group_id VARCHAR(64) NOT NULL,
    target_group_id BIGINT NULL,
    email_domains TEXT NOT NULL DEFAULT '[]',
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    sync_interval_minutes INTEGER NOT NULL DEFAULT 0,
    last_sync_at TIMESTAMPTZ NULL,
    next_sync_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL,
    modified_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX idx_directory_connectors_owner_name ON directory_connectors(owner_user_id, name_key);
CREATE INDEX idx_directory_connectors_owner ON directory_connectors(owner_user_id);
CREATE INDEX idx_directory_connectors_due ON directory_connectors(enabled, next_sync_at);

CREATE TABLE directory_sync_runs (
    id BIGSERIAL PRIMARY KEY,
    connector_id BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NULL,
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
