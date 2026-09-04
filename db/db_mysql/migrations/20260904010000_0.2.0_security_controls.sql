-- +goose Up
ALTER TABLE `campaigns` ADD COLUMN `credential_capture_mode` VARCHAR(32) NOT NULL DEFAULT 'disabled';
ALTER TABLE `campaigns` ADD COLUMN `credential_retention_hours` INTEGER NOT NULL DEFAULT 24;

CREATE TABLE `campaign_credential_policies` (
    `campaign_id` BIGINT PRIMARY KEY,
    `min_length` INTEGER NOT NULL DEFAULT 12,
    `max_length` INTEGER NOT NULL DEFAULT 128,
    `min_uppercase` INTEGER NOT NULL DEFAULT 0,
    `min_lowercase` INTEGER NOT NULL DEFAULT 0,
    `min_digits` INTEGER NOT NULL DEFAULT 0,
    `min_symbols` INTEGER NOT NULL DEFAULT 0,
    `disallowed_patterns` TEXT NOT NULL
);

CREATE TABLE `credential_policy_results` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `campaign_id` BIGINT NOT NULL,
    `result_id` BIGINT NOT NULL UNIQUE,
    `evaluated_at` DATETIME(6) NOT NULL,
    `length` INTEGER NOT NULL,
    `uppercase_count` INTEGER NOT NULL,
    `lowercase_count` INTEGER NOT NULL,
    `digit_count` INTEGER NOT NULL,
    `symbol_count` INTEGER NOT NULL,
    `strength_score` INTEGER NOT NULL,
    `policy_passed` BOOLEAN NOT NULL,
    `failures` TEXT NOT NULL,
    INDEX `idx_credential_policy_campaign` (`campaign_id`)
);

CREATE TABLE `encrypted_credentials` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `campaign_id` BIGINT NOT NULL,
    `result_id` BIGINT NOT NULL UNIQUE,
    `encrypted_value` TEXT NOT NULL,
    `created_at` DATETIME(6) NOT NULL,
    `expires_at` DATETIME(6) NOT NULL,
    `purged_at` DATETIME(6) NULL,
    INDEX `idx_encrypted_credentials_expiry` (`expires_at`, `purged_at`)
);

CREATE TABLE `personal_access_tokens` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `name` VARCHAR(128) NOT NULL,
    `prefix` VARCHAR(32) NOT NULL UNIQUE,
    `token_hash` VARCHAR(128) NOT NULL,
    `scopes` TEXT NOT NULL,
    `created_at` DATETIME(6) NOT NULL,
    `expires_at` DATETIME(6) NOT NULL,
    `last_used_at` DATETIME(6) NULL,
    `revoked_at` DATETIME(6) NULL,
    INDEX `idx_personal_access_tokens_user` (`user_id`, `revoked_at`, `expires_at`)
);
UPDATE `users` SET `api_key`=CONCAT('disabled-', `id`);

CREATE TABLE `audit_events` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `timestamp` DATETIME(6) NOT NULL,
    `actor` VARCHAR(255) NOT NULL,
    `actor_id` BIGINT NULL,
    `actor_type` VARCHAR(32) NOT NULL,
    `action` VARCHAR(128) NOT NULL,
    `target_type` VARCHAR(64) NOT NULL,
    `target_id` VARCHAR(255) NOT NULL,
    `result` VARCHAR(32) NOT NULL,
    `request_id` VARCHAR(64) NOT NULL,
    `source_ip` VARCHAR(64),
    `user_agent` VARCHAR(512),
    `auth_method` VARCHAR(32),
    `metadata` TEXT NOT NULL,
    INDEX `idx_audit_events_timestamp` (`timestamp`),
    INDEX `idx_audit_events_actor` (`actor_id`, `timestamp`),
    INDEX `idx_audit_events_action` (`action`, `timestamp`),
    INDEX `idx_audit_events_target` (`target_type`, `target_id`, `timestamp`)
);

INSERT INTO `permissions` (`slug`, `name`, `description`)
VALUES ('credentials:view', 'View Retained Credentials', 'Reveal individually retained encrypted campaign credentials');
INSERT INTO `role_permissions` (`role_id`, `permission_id`)
SELECT r.id, p.id FROM `roles` r, `permissions` p
WHERE r.slug='admin' AND p.slug='credentials:view';

-- +goose Down
DELETE rp FROM `role_permissions` rp
JOIN `permissions` p ON p.`id`=rp.`permission_id`
WHERE p.`slug`='credentials:view';
DELETE FROM `permissions` WHERE `slug`='credentials:view';
DROP TABLE `audit_events`;
DROP TABLE `personal_access_tokens`;
DROP TABLE `encrypted_credentials`;
DROP TABLE `credential_policy_results`;
DROP TABLE `campaign_credential_policies`;
ALTER TABLE `campaigns` DROP COLUMN `credential_retention_hours`;
ALTER TABLE `campaigns` DROP COLUMN `credential_capture_mode`;
