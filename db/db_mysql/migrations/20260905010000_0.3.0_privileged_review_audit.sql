-- +goose Up
CREATE TABLE `privileged_sessions` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `session_hash` VARCHAR(64) NOT NULL UNIQUE,
    `user_id` BIGINT NOT NULL,
    `reauthenticated_at` DATETIME(6) NOT NULL,
    `expires_at` DATETIME(6) NOT NULL,
    `created_at` DATETIME(6) NOT NULL,
    INDEX `idx_privileged_sessions_user_expiry` (`user_id`, `expires_at`)
);

CREATE TABLE `campaign_reviewers` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `campaign_id` BIGINT NOT NULL,
    `user_id` BIGINT NOT NULL,
    `assigned_by` BIGINT NOT NULL,
    `assigned_at` DATETIME(6) NOT NULL,
    `expires_at` DATETIME(6) NULL,
    `expired_audited_at` DATETIME(6) NULL,
    UNIQUE KEY `uq_campaign_reviewers_campaign_user` (`campaign_id`, `user_id`),
    INDEX `idx_campaign_reviewers_campaign` (`campaign_id`, `expires_at`),
    INDEX `idx_campaign_reviewers_user` (`user_id`, `expires_at`)
);
CREATE INDEX `idx_results_campaign_rid` ON `results` (`campaign_id`, `r_id`);

CREATE TABLE `audit_outbox` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `event_json` TEXT NOT NULL,
    `created_at` DATETIME(6) NOT NULL,
    `dispatched_at` DATETIME(6) NULL,
    `attempts` INTEGER NOT NULL DEFAULT 0,
    `last_error` VARCHAR(512) NOT NULL DEFAULT '',
    INDEX `idx_audit_outbox_pending` (`dispatched_at`, `id`)
);

ALTER TABLE `audit_events` ADD COLUMN `chain_id` VARCHAR(32) NOT NULL DEFAULT 'instance';
ALTER TABLE `audit_events` ADD COLUMN `chain_sequence` BIGINT NOT NULL DEFAULT 0;
ALTER TABLE `audit_events` ADD COLUMN `previous_hash` VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE `audit_events` ADD COLUMN `event_hash` VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE `audit_events` ADD COLUMN `audit_outbox_id` BIGINT NULL;
CREATE INDEX `idx_audit_events_chain_hash` ON `audit_events` (`chain_id`, `event_hash`);
CREATE INDEX `idx_audit_events_chain_sequence` ON `audit_events` (`chain_id`, `chain_sequence`);
CREATE UNIQUE INDEX `idx_audit_events_outbox` ON `audit_events` (`audit_outbox_id`);

CREATE TABLE `audit_checkpoints` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `format_version` INTEGER NOT NULL,
    `chain_id` VARCHAR(32) NOT NULL,
    `first_event_id` BIGINT NOT NULL,
    `last_event_id` BIGINT NOT NULL,
    `first_sequence` BIGINT NOT NULL,
    `last_sequence` BIGINT NOT NULL,
    `final_hash` VARCHAR(64) NOT NULL,
    `created_at` DATETIME(6) NOT NULL,
    `key_id` VARCHAR(64) NOT NULL,
    `signature` TEXT NOT NULL,
    UNIQUE KEY `idx_audit_checkpoints_chain_range` (`chain_id`, `last_event_id`)
);

INSERT INTO `roles` (`slug`, `name`, `description`)
VALUES ('security_reviewer', 'Security Reviewer', 'Campaign-scoped credential and security finding reviewer');

INSERT INTO `permissions` (`slug`, `name`, `description`) VALUES
    ('campaigns:read', 'Read Campaigns', 'Read assigned campaign metadata'),
    ('reports:read', 'Read Reports', 'Read assigned campaign reports'),
    ('credential-policy:read', 'Read Credential Policy', 'Read credential policy findings'),
    ('audit:self-sensitive-read', 'Read Own Sensitive Audit', 'Read audit events for the reviewer own sensitive actions');

INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT r.id, p.id FROM `roles` r, `permissions` p
WHERE r.slug='security_reviewer' AND p.slug IN ('view_objects', 'campaigns:read', 'reports:read', 'credentials:view', 'credential-policy:read', 'audit:self-sensitive-read');

INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT r.id, p.id FROM `roles` r, `permissions` p
WHERE r.slug='admin' AND p.slug IN ('campaigns:read', 'reports:read', 'credential-policy:read', 'audit:self-sensitive-read');

-- +goose Down
DELETE rp FROM `role_permissions` rp JOIN `roles` r ON r.`id`=rp.`role_id` WHERE r.`slug`='security_reviewer';
DELETE rp FROM `role_permissions` rp JOIN `permissions` p ON p.`id`=rp.`permission_id` WHERE p.`slug` IN ('campaigns:read', 'reports:read', 'credential-policy:read', 'audit:self-sensitive-read');
DELETE FROM `permissions` WHERE `slug` IN ('campaigns:read', 'reports:read', 'credential-policy:read', 'audit:self-sensitive-read');
DELETE FROM `roles` WHERE `slug`='security_reviewer';
DROP TABLE `audit_checkpoints`;
DROP INDEX `idx_audit_events_outbox` ON `audit_events`;
DROP INDEX `idx_audit_events_chain_sequence` ON `audit_events`;
DROP INDEX `idx_audit_events_chain_hash` ON `audit_events`;
ALTER TABLE `audit_events` DROP COLUMN `audit_outbox_id`, DROP COLUMN `event_hash`, DROP COLUMN `previous_hash`, DROP COLUMN `chain_sequence`, DROP COLUMN `chain_id`;
DROP TABLE `audit_outbox`;
DROP TABLE `campaign_reviewers`;
DROP INDEX `idx_results_campaign_rid` ON `results`;
DROP TABLE `privileged_sessions`;
