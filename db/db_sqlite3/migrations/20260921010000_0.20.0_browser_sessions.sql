-- +goose Up
CREATE TABLE "browser_sessions" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "session_hash" TEXT NOT NULL UNIQUE,
    "user_id" INTEGER NOT NULL,
    "created_at" DATETIME NOT NULL,
    "expires_at" DATETIME NOT NULL
);
CREATE INDEX "idx_browser_sessions_user_expiry" ON "browser_sessions" ("user_id", "expires_at");

-- +goose Down
DROP TABLE IF EXISTS "browser_sessions";
