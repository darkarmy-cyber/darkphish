-- +goose Up
CREATE TABLE audit_chain_heads (
    chain_id VARCHAR(32) PRIMARY KEY,
    chain_sequence INTEGER NOT NULL DEFAULT 0,
    event_id INTEGER NOT NULL DEFAULT 0,
    event_hash VARCHAR(64) NOT NULL DEFAULT '',
    retired_sequence INTEGER NOT NULL DEFAULT 0,
    shared_signing_required INTEGER NOT NULL DEFAULT 0,
    initialized INTEGER NOT NULL DEFAULT 0
);
INSERT INTO audit_chain_heads (chain_id) VALUES ('instance');
CREATE TABLE audit_delivery_receipts (
    outbox_id INTEGER PRIMARY KEY,
    event_id INTEGER NOT NULL,
    chain_sequence INTEGER NOT NULL
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
