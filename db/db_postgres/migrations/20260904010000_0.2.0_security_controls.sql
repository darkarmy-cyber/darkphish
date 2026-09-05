-- +goose Up
ALTER TABLE "campaigns" ADD COLUMN "credential_capture_mode" VARCHAR(32) NOT NULL DEFAULT 'disabled';
ALTER TABLE "campaigns" ADD COLUMN "credential_retention_hours" INTEGER NOT NULL DEFAULT 24;

CREATE TABLE "campaign_credential_policies" (
    "campaign_id" BIGINT PRIMARY KEY,
    "min_length" INTEGER NOT NULL DEFAULT 12,
    "max_length" INTEGER NOT NULL DEFAULT 128,
    "min_uppercase" INTEGER NOT NULL DEFAULT 0,
    "min_lowercase" INTEGER NOT NULL DEFAULT 0,
    "min_digits" INTEGER NOT NULL DEFAULT 0,
    "min_symbols" INTEGER NOT NULL DEFAULT 0,
    "disallowed_patterns" TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE "credential_policy_results" (
    "id" BIGSERIAL PRIMARY KEY,
    "campaign_id" BIGINT NOT NULL,
    "result_id" BIGINT NOT NULL UNIQUE,
    "evaluated_at" TIMESTAMPTZ NOT NULL,
    "length" INTEGER NOT NULL,
    "uppercase_count" INTEGER NOT NULL,
    "lowercase_count" INTEGER NOT NULL,
    "digit_count" INTEGER NOT NULL,
    "symbol_count" INTEGER NOT NULL,
    "strength_score" INTEGER NOT NULL,
    "policy_passed" BOOLEAN NOT NULL,
    "failures" TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX "idx_credential_policy_campaign" ON "credential_policy_results" ("campaign_id");

CREATE TABLE "encrypted_credentials" (
    "id" BIGSERIAL PRIMARY KEY,
    "campaign_id" BIGINT NOT NULL,
    "result_id" BIGINT NOT NULL UNIQUE,
    "encrypted_value" TEXT NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL,
    "expires_at" TIMESTAMPTZ NOT NULL,
    "purged_at" TIMESTAMPTZ
);
CREATE INDEX "idx_encrypted_credentials_expiry" ON "encrypted_credentials" ("expires_at", "purged_at");

CREATE TABLE "personal_access_tokens" (
    "id" BIGSERIAL PRIMARY KEY,
    "user_id" BIGINT NOT NULL,
    "name" VARCHAR(128) NOT NULL,
    "prefix" VARCHAR(32) NOT NULL UNIQUE,
    "token_hash" VARCHAR(128) NOT NULL,
    "scopes" TEXT NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL,
    "expires_at" TIMESTAMPTZ NOT NULL,
    "last_used_at" TIMESTAMPTZ,
    "revoked_at" TIMESTAMPTZ
);
CREATE INDEX "idx_personal_access_tokens_user" ON "personal_access_tokens" ("user_id", "revoked_at", "expires_at");
UPDATE "users" SET "api_key"='disabled-' || "id"::TEXT;

CREATE TABLE "audit_events" (
    "id" BIGSERIAL PRIMARY KEY,
    "timestamp" TIMESTAMPTZ NOT NULL,
    "actor" VARCHAR(255) NOT NULL,
    "actor_id" BIGINT,
    "actor_type" VARCHAR(32) NOT NULL,
    "action" VARCHAR(128) NOT NULL,
    "target_type" VARCHAR(64) NOT NULL,
    "target_id" VARCHAR(255) NOT NULL,
    "result" VARCHAR(32) NOT NULL,
    "request_id" VARCHAR(64) NOT NULL,
    "source_ip" VARCHAR(64),
    "user_agent" VARCHAR(512),
    "auth_method" VARCHAR(32),
    "metadata" TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX "idx_audit_events_timestamp" ON "audit_events" ("timestamp");
CREATE INDEX "idx_audit_events_actor" ON "audit_events" ("actor_id", "timestamp");
CREATE INDEX "idx_audit_events_action" ON "audit_events" ("action", "timestamp");
CREATE INDEX "idx_audit_events_target" ON "audit_events" ("target_type", "target_id", "timestamp");

INSERT INTO "permissions" ("slug", "name", "description")
VALUES ('credentials:view', 'View Retained Credentials', 'Reveal individually retained encrypted campaign credentials');
INSERT INTO "role_permissions" ("role_id", "permission_id")
SELECT r.id, p.id FROM "roles" r CROSS JOIN "permissions" p
WHERE r.slug='admin' AND p.slug='credentials:view';

-- +goose Down
DELETE FROM "role_permissions" WHERE "permission_id"=(SELECT "id" FROM "permissions" WHERE "slug"='credentials:view');
DELETE FROM "permissions" WHERE "slug"='credentials:view';
DROP TABLE "audit_events";
DROP TABLE "personal_access_tokens";
DROP TABLE "encrypted_credentials";
DROP TABLE "credential_policy_results";
DROP TABLE "campaign_credential_policies";
ALTER TABLE "campaigns" DROP COLUMN "credential_retention_hours";
ALTER TABLE "campaigns" DROP COLUMN "credential_capture_mode";
