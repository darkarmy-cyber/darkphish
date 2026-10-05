-- +goose Up
CREATE TABLE directory_connectors (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    name_key CHAR(64) NOT NULL,
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
    UNIQUE INDEX idx_directory_connectors_owner_name (owner_user_id, name_key),
    INDEX idx_directory_connectors_owner (owner_user_id),
    INDEX idx_directory_connectors_due (enabled, next_sync_at),
    CONSTRAINT fk_directory_connectors_owner FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_directory_connectors_group FOREIGN KEY (target_group_id) REFERENCES `groups`(id) ON DELETE RESTRICT
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
    INDEX idx_directory_sync_runs_connector (connector_id, started_at),
    CONSTRAINT fk_directory_sync_runs_connector FOREIGN KEY (connector_id) REFERENCES directory_connectors(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE directory_sync_runs;
DROP TABLE directory_connectors;
