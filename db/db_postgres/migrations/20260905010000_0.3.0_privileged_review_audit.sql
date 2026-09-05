-- +goose Up
CREATE TABLE "privileged_sessions" (
    "id" BIGSERIAL PRIMARY KEY,
    "session_hash" VARCHAR(64) NOT NULL UNIQUE,
    "user_id" BIGINT NOT NULL,
    "reauthenticated_at" TIMESTAMPTZ NOT NULL,
    "expires_at" TIMESTAMPTZ NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL
);
CREATE INDEX "idx_privileged_sessions_user_expiry" ON "privileged_sessions" ("user_id", "expires_at");

CREATE TABLE "campaign_reviewers" (
    "id" BIGSERIAL PRIMARY KEY,
    "campaign_id" BIGINT NOT NULL,
    "user_id" BIGINT NOT NULL,
    "assigned_by" BIGINT NOT NULL,
    "assigned_at" TIMESTAMPTZ NOT NULL,
    "expires_at" TIMESTAMPTZ,
    "expired_audited_at" TIMESTAMPTZ,
    UNIQUE ("campaign_id", "user_id")
);
CREATE INDEX "idx_campaign_reviewers_campaign" ON "campaign_reviewers" ("campaign_id", "expires_at");
CREATE INDEX "idx_campaign_reviewers_user" ON "campaign_reviewers" ("user_id", "expires_at");
CREATE INDEX "idx_results_campaign_rid" ON "results" ("campaign_id", "r_id");

CREATE TABLE "audit_outbox" (
    "id" BIGSERIAL PRIMARY KEY,
    "event_json" TEXT NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL,
    "dispatched_at" TIMESTAMPTZ,
    "attempts" INTEGER NOT NULL DEFAULT 0,
    "last_error" VARCHAR(512) NOT NULL DEFAULT ''
);
CREATE INDEX "idx_audit_outbox_pending" ON "audit_outbox" ("dispatched_at", "id");

ALTER TABLE "audit_events" ADD COLUMN "chain_id" VARCHAR(32) NOT NULL DEFAULT 'instance';
ALTER TABLE "audit_events" ADD COLUMN "chain_sequence" BIGINT NOT NULL DEFAULT 0;
ALTER TABLE "audit_events" ADD COLUMN "previous_hash" VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE "audit_events" ADD COLUMN "event_hash" VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE "audit_events" ADD COLUMN "audit_outbox_id" BIGINT;
CREATE UNIQUE INDEX "idx_audit_events_chain_hash" ON "audit_events" ("chain_id", "event_hash") WHERE "event_hash" <> '';
CREATE UNIQUE INDEX "idx_audit_events_chain_sequence" ON "audit_events" ("chain_id", "chain_sequence") WHERE "chain_sequence" <> 0;
CREATE UNIQUE INDEX "idx_audit_events_outbox" ON "audit_events" ("audit_outbox_id") WHERE "audit_outbox_id" IS NOT NULL;

CREATE TABLE "audit_checkpoints" (
    "id" BIGSERIAL PRIMARY KEY,
    "format_version" INTEGER NOT NULL,
    "chain_id" VARCHAR(32) NOT NULL,
    "first_event_id" BIGINT NOT NULL,
    "last_event_id" BIGINT NOT NULL,
    "first_sequence" BIGINT NOT NULL,
    "last_sequence" BIGINT NOT NULL,
    "final_hash" VARCHAR(64) NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL,
    "key_id" VARCHAR(64) NOT NULL,
    "signature" TEXT NOT NULL
);
CREATE UNIQUE INDEX "idx_audit_checkpoints_chain_range" ON "audit_checkpoints" ("chain_id", "last_event_id");

INSERT INTO "roles" ("slug", "name", "description")
VALUES ('security_reviewer', 'Security Reviewer', 'Campaign-scoped credential and security finding reviewer');

INSERT INTO "permissions" ("slug", "name", "description") VALUES
    ('campaigns:read', 'Read Campaigns', 'Read assigned campaign metadata'),
    ('reports:read', 'Read Reports', 'Read assigned campaign reports'),
    ('credential-policy:read', 'Read Credential Policy', 'Read credential policy findings'),
    ('audit:self-sensitive-read', 'Read Own Sensitive Audit', 'Read audit events for the reviewer own sensitive actions');

INSERT INTO "role_permissions" ("role_id", "permission_id")
SELECT r.id, p.id FROM "roles" r CROSS JOIN "permissions" p
WHERE r.slug='security_reviewer' AND p.slug IN ('view_objects', 'campaigns:read', 'reports:read', 'credentials:view', 'credential-policy:read', 'audit:self-sensitive-read');

INSERT INTO "role_permissions" ("role_id", "permission_id")
SELECT r.id, p.id FROM "roles" r CROSS JOIN "permissions" p
WHERE r.slug='admin' AND p.slug IN ('campaigns:read', 'reports:read', 'credential-policy:read', 'audit:self-sensitive-read');

-- +goose Down
DELETE FROM "role_permissions" WHERE "role_id"=(SELECT "id" FROM "roles" WHERE "slug"='security_reviewer');
DELETE FROM "role_permissions" WHERE "permission_id" IN (SELECT "id" FROM "permissions" WHERE "slug" IN ('campaigns:read', 'reports:read', 'credential-policy:read', 'audit:self-sensitive-read'));
DELETE FROM "permissions" WHERE "slug" IN ('campaigns:read', 'reports:read', 'credential-policy:read', 'audit:self-sensitive-read');
DELETE FROM "roles" WHERE "slug"='security_reviewer';
DROP TABLE "audit_checkpoints";
DROP INDEX "idx_audit_events_outbox";
DROP INDEX "idx_audit_events_chain_sequence";
DROP INDEX "idx_audit_events_chain_hash";
ALTER TABLE "audit_events" DROP COLUMN "event_hash";
ALTER TABLE "audit_events" DROP COLUMN "audit_outbox_id";
ALTER TABLE "audit_events" DROP COLUMN "previous_hash";
ALTER TABLE "audit_events" DROP COLUMN "chain_sequence";
ALTER TABLE "audit_events" DROP COLUMN "chain_id";
DROP TABLE "audit_outbox";
DROP TABLE "campaign_reviewers";
DROP INDEX "idx_results_campaign_rid";
DROP TABLE "privileged_sessions";
