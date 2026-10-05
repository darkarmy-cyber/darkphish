-- +goose Up
CREATE TABLE directory_connectors (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL UNIQUE,
    owner_user_id BIGINT NOT NULL,
    provider VARCHAR(32) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    client_id VARCHAR(64) NOT NULL,
    client_secret TEXT NOT NULL,
    remote_group_id VARCHAR(64) NOT NULL,
    target_group_id BIGINT NULL,
    email_domains TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    sync_interval_minutes INTEGER NOT NULL DEFAULT 0,
    last_sync_at DATETIME(6) NULL,
    next_sync_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    modified_at DATETIME(6) NOT NULL,
    INDEX idx_directory_connectors_due (enabled, next_sync_at)
);

CREATE TABLE directory_sync_runs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    connector_id BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL,
    started_at DATETIME(6) NOT NULL,
    finished_at DATETIME(6) NULL,
    discovered INTEGER NOT NULL DEFAULT 0,
    added INTEGER NOT NULL DEFAULT 0,
    updated INTEGER NOT NULL DEFAULT 0,
    removed INTEGER NOT NULL DEFAULT 0,
    skipped INTEGER NOT NULL DEFAULT 0,
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    INDEX idx_directory_sync_runs_connector (connector_id, started_at)
);

-- +goose Down
DROP TABLE directory_sync_runs;
DROP TABLE directory_connectors;
