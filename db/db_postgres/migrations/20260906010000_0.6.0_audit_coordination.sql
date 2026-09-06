-- +goose Up
CREATE TABLE audit_chain_heads (
    chain_id VARCHAR(32) PRIMARY KEY,
    chain_sequence BIGINT NOT NULL DEFAULT 0,
    event_id BIGINT NOT NULL DEFAULT 0,
    event_hash VARCHAR(64) NOT NULL DEFAULT '',
    retired_sequence BIGINT NOT NULL DEFAULT 0,
    shared_signing_required BOOLEAN NOT NULL DEFAULT FALSE,
    initialized BOOLEAN NOT NULL DEFAULT FALSE
);
INSERT INTO audit_chain_heads (chain_id) VALUES ('instance');
CREATE TABLE audit_delivery_receipts (
    outbox_id BIGINT PRIMARY KEY,
    event_id BIGINT NOT NULL,
    chain_sequence BIGINT NOT NULL
);
CREATE UNIQUE INDEX idx_audit_checkpoints_chain_sequence ON audit_checkpoints (chain_id, last_sequence);

CREATE TABLE audit_signing_identities (
    key_id VARCHAR(64) PRIMARY KEY,
    fingerprint VARCHAR(64) NOT NULL
);

-- +goose Down
DROP TABLE audit_signing_identities;
DROP INDEX idx_audit_checkpoints_chain_sequence;
DROP TABLE audit_delivery_receipts;
DROP TABLE audit_chain_heads;
