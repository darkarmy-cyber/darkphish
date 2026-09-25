-- +goose Up
CREATE TABLE `browser_sessions` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `session_hash` VARCHAR(64) NOT NULL UNIQUE,
    `user_id` BIGINT NOT NULL,
    `created_at` DATETIME(6) NOT NULL,
    `expires_at` DATETIME(6) NOT NULL,
    INDEX `idx_browser_sessions_user_expiry` (`user_id`, `expires_at`)
);

-- +goose Down
DROP TABLE IF EXISTS `browser_sessions`;
