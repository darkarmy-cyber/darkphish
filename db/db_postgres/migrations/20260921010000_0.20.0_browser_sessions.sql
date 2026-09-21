-- +goose Up
CREATE TABLE "browser_sessions" (
    "id" BIGSERIAL PRIMARY KEY,
    "session_hash" VARCHAR(64) NOT NULL UNIQUE,
    "user_id" BIGINT NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL,
    "expires_at" TIMESTAMPTZ NOT NULL
);
CREATE INDEX "idx_browser_sessions_user_expiry" ON "browser_sessions" ("user_id", "expires_at");

-- +goose Down
DROP TABLE IF EXISTS "browser_sessions";
