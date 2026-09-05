-- +goose Up
-- PostgreSQL support was introduced by Darkphish 0.3. This migration is a
-- consolidated, fresh-install equivalent of the historical upstream schema.
CREATE TABLE "users" (
    "id" BIGSERIAL PRIMARY KEY,
    "username" VARCHAR(255) NOT NULL UNIQUE,
    "hash" VARCHAR(255),
    "api_key" VARCHAR(255) NOT NULL UNIQUE,
    "role_id" BIGINT,
    "password_change_required" BOOLEAN,
    "last_login" TIMESTAMPTZ,
    "account_locked" BOOLEAN
);

CREATE TABLE "templates" (
    "id" BIGSERIAL PRIMARY KEY,
    "user_id" BIGINT,
    "name" VARCHAR(255),
    "subject" VARCHAR(255),
    "text" TEXT,
    "html" TEXT,
    "modified_date" TIMESTAMPTZ,
    "envelope_sender" VARCHAR(255)
);

CREATE TABLE "targets" (
    "id" BIGSERIAL PRIMARY KEY,
    "first_name" VARCHAR(255),
    "last_name" VARCHAR(255),
    "email" VARCHAR(255),
    "position" VARCHAR(255)
);

CREATE TABLE "smtp" (
    "id" BIGSERIAL PRIMARY KEY,
    "user_id" BIGINT,
    "interface_type" VARCHAR(255),
    "name" VARCHAR(255),
    "host" VARCHAR(255),
    "username" VARCHAR(255),
    "password" TEXT,
    "from_address" VARCHAR(255),
    "modified_date" TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    "ignore_cert_errors" BOOLEAN
);

CREATE TABLE "results" (
    "id" BIGSERIAL PRIMARY KEY,
    "campaign_id" BIGINT,
    "user_id" BIGINT,
    "r_id" VARCHAR(255),
    "email" VARCHAR(255),
    "first_name" VARCHAR(255),
    "last_name" VARCHAR(255),
    "status" VARCHAR(255) NOT NULL,
    "ip" VARCHAR(255),
    "latitude" REAL,
    "longitude" REAL,
    "position" VARCHAR(255),
    "send_date" TIMESTAMPTZ,
    "reported" BOOLEAN DEFAULT FALSE,
    "modified_date" TIMESTAMPTZ
);

CREATE TABLE "pages" (
    "id" BIGSERIAL PRIMARY KEY,
    "user_id" BIGINT,
    "name" VARCHAR(255),
    "html" TEXT,
    "modified_date" TIMESTAMPTZ,
    "capture_credentials" BOOLEAN,
    "capture_passwords" BOOLEAN,
    "redirect_url" VARCHAR(255)
);

CREATE TABLE "groups" (
    "id" BIGSERIAL PRIMARY KEY,
    "user_id" BIGINT,
    "name" VARCHAR(255),
    "modified_date" TIMESTAMPTZ
);

CREATE TABLE "group_targets" (
    "group_id" BIGINT,
    "target_id" BIGINT
);

CREATE TABLE "events" (
    "id" BIGSERIAL PRIMARY KEY,
    "campaign_id" BIGINT,
    "email" VARCHAR(255),
    "time" TIMESTAMPTZ,
    "message" VARCHAR(255),
    "details" TEXT
);

CREATE TABLE "campaigns" (
    "id" BIGSERIAL PRIMARY KEY,
    "user_id" BIGINT,
    "name" VARCHAR(255) NOT NULL,
    "created_date" TIMESTAMPTZ,
    "completed_date" TIMESTAMPTZ,
    "template_id" BIGINT,
    "page_id" BIGINT,
    "status" VARCHAR(255),
    "url" VARCHAR(255),
    "smtp_id" BIGINT,
    "launch_date" TIMESTAMPTZ,
    "send_by_date" TIMESTAMPTZ
);

CREATE TABLE "attachments" (
    "id" BIGSERIAL PRIMARY KEY,
    "template_id" BIGINT,
    "content" TEXT,
    "type" VARCHAR(255),
    "name" VARCHAR(255)
);

CREATE TABLE "headers" (
    "id" BIGSERIAL PRIMARY KEY,
    "key" VARCHAR(255),
    "value" VARCHAR(255),
    "smtp_id" BIGINT
);

CREATE TABLE "mail_logs" (
    "id" BIGSERIAL PRIMARY KEY,
    "campaign_id" BIGINT,
    "user_id" BIGINT,
    "send_date" TIMESTAMPTZ,
    "send_attempt" INTEGER,
    "r_id" VARCHAR(255),
    "processing" BOOLEAN
);

CREATE TABLE "email_requests" (
    "id" BIGSERIAL PRIMARY KEY,
    "user_id" BIGINT,
    "template_id" BIGINT,
    "page_id" BIGINT,
    "first_name" VARCHAR(255),
    "last_name" VARCHAR(255),
    "email" VARCHAR(255),
    "position" VARCHAR(255),
    "url" VARCHAR(255),
    "r_id" VARCHAR(255),
    "from_address" VARCHAR(255)
);

CREATE TABLE "roles" (
    "id" BIGSERIAL PRIMARY KEY,
    "slug" VARCHAR(255) NOT NULL UNIQUE,
    "name" VARCHAR(255) NOT NULL UNIQUE,
    "description" VARCHAR(255)
);

CREATE TABLE "permissions" (
    "id" BIGSERIAL PRIMARY KEY,
    "slug" VARCHAR(255) NOT NULL UNIQUE,
    "name" VARCHAR(255) NOT NULL UNIQUE,
    "description" VARCHAR(255)
);

CREATE TABLE "role_permissions" (
    "role_id" BIGINT NOT NULL,
    "permission_id" BIGINT NOT NULL
);

INSERT INTO "roles" ("slug", "name", "description") VALUES
    ('admin', 'Admin', 'System administrator with full permissions'),
    ('user', 'User', 'User role with edit access to objects and campaigns');

INSERT INTO "permissions" ("slug", "name", "description") VALUES
    ('view_objects', 'View Objects', 'View objects in Gophish'),
    ('modify_objects', 'Modify Objects', 'Create and edit objects in Gophish'),
    ('modify_system', 'Modify System', 'Manage system-wide configuration');

INSERT INTO "role_permissions" ("role_id", "permission_id")
SELECT r.id, p.id FROM "roles" r CROSS JOIN "permissions" p
WHERE r.slug IN ('admin', 'user') AND p.slug IN ('view_objects', 'modify_objects');

INSERT INTO "role_permissions" ("role_id", "permission_id")
SELECT r.id, p.id FROM "roles" r CROSS JOIN "permissions" p
WHERE r.slug='admin' AND p.slug='modify_system';

CREATE TABLE "webhooks" (
    "id" BIGSERIAL PRIMARY KEY,
    "name" VARCHAR(255),
    "url" VARCHAR(1000),
    "secret" TEXT,
    "is_active" BOOLEAN DEFAULT FALSE
);

CREATE TABLE "imap" (
    "user_id" BIGINT,
    "host" VARCHAR(255),
    "port" INTEGER,
    "username" VARCHAR(255),
    "password" TEXT,
    "modified_date" TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    "tls" BOOLEAN,
    "enabled" BOOLEAN,
    "folder" VARCHAR(255),
    "restrict_domain" VARCHAR(255),
    "delete_reported_campaign_email" BOOLEAN,
    "last_login" TIMESTAMPTZ,
    "imap_freq" INTEGER,
    "ignore_cert_errors" BOOLEAN
);

-- +goose Down
DROP TABLE "imap";
DROP TABLE "webhooks";
DROP TABLE "role_permissions";
DROP TABLE "permissions";
DROP TABLE "roles";
DROP TABLE "email_requests";
DROP TABLE "mail_logs";
DROP TABLE "headers";
DROP TABLE "attachments";
DROP TABLE "campaigns";
DROP TABLE "events";
DROP TABLE "group_targets";
DROP TABLE "groups";
DROP TABLE "pages";
DROP TABLE "results";
DROP TABLE "smtp";
DROP TABLE "targets";
DROP TABLE "templates";
DROP TABLE "users";
