package databasetest

import (
	"database/sql"
	"testing"
)

func rejectCiphertext(t *testing.T, connection *sql.DB, backend string) func() {
	t.Helper()
	var create, drop []string
	switch backend {
	case "sqlite3":
		create = []string{"CREATE TRIGGER reject_test_ciphertext BEFORE INSERT ON encrypted_credentials BEGIN SELECT RAISE(ABORT, 'injected ciphertext failure'); END"}
		drop = []string{"DROP TRIGGER IF EXISTS reject_test_ciphertext"}
	case "mysql":
		create = []string{"CREATE TRIGGER reject_test_ciphertext BEFORE INSERT ON encrypted_credentials FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected ciphertext failure'"}
		drop = []string{"DROP TRIGGER IF EXISTS reject_test_ciphertext"}
	case "postgres":
		create = []string{"CREATE FUNCTION reject_test_ciphertext() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected ciphertext failure'; END; $$", "CREATE TRIGGER reject_test_ciphertext BEFORE INSERT ON encrypted_credentials FOR EACH ROW EXECUTE FUNCTION reject_test_ciphertext()"}
		drop = []string{"DROP TRIGGER IF EXISTS reject_test_ciphertext ON encrypted_credentials", "DROP FUNCTION IF EXISTS reject_test_ciphertext()"}
	default:
		t.Fatal("unknown failure-injection backend")
	}
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		for _, statement := range drop {
			if _, err := connection.Exec(statement); err != nil {
				t.Error(err)
			}
		}
		cleaned = true
	}
	t.Cleanup(cleanup)
	for _, statement := range create {
		if _, err := connection.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return cleanup
}
