package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupManifestCoversEveryRestorableFile(t *testing.T) {
	for _, variant := range []string{"complete", "omit keyring", "omit and delete keyring", "omit binary", "omit config", "omit database", "omit nested", "extra file", "alias"} {
		t.Run(variant, func(t *testing.T) {
			transaction := t.TempDir()
			backup := filepath.Join(transaction, "backup")
			files := packagedRuntimeFixture(t)
			files["darkphish.db"] = []byte("snapshot fixture")
			hashes := map[string]string{}
			for name, data := range files {
				p := filepath.Join(backup, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, data, 0600); err != nil {
					t.Fatal(err)
				}
				hash := sha256.Sum256(data)
				hashes[name] = hex.EncodeToString(hash[:])
			}
			switch variant {
			case "omit keyring", "omit and delete keyring":
				delete(hashes, "license-public-keys.json")
				if variant == "omit and delete keyring" {
					if err := os.Remove(filepath.Join(backup, "license-public-keys.json")); err != nil {
						t.Fatal(err)
					}
				}
			case "omit binary":
				delete(hashes, "darkphish")
			case "omit config":
				delete(hashes, "config.json")
			case "omit database":
				delete(hashes, "darkphish.db")
			case "omit nested":
				delete(hashes, "templates/fixture")
			case "extra file":
				if err := os.WriteFile(filepath.Join(backup, "static", "unchecked"), []byte("unchecked"), 0600); err != nil {
					t.Fatal(err)
				}
			case "alias":
				hashes["./license-public-keys.json"] = hashes["license-public-keys.json"]
			}
			manifest, err := json.Marshal(map[string]any{"schema": "darkphish-pre-update-backup/v1", "files": hashes})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(backup, "backup-manifest.json"), manifest, 0600); err != nil {
				t.Fatal(err)
			}
			layout := Layout{Root: t.TempDir(), Config: "config.json", Database: "darkphish.db"}
			err = verifyBackup(backup, layout)
			if variant == "complete" {
				if err != nil {
					t.Fatal("complete backup rejected:", err)
				}
				return
			}
			if err == nil {
				t.Fatal("incomplete backup accepted:", variant)
			}
			// Both entry points must reject before mutation, including rollback's
			// cleanup of replaceable staging data.
			stage := filepath.Join(transaction, "stage")
			if err := os.Mkdir(stage, 0700); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(stage, "sentinel")
			if err := os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			tx := Transaction{Layout: layout, Directory: transaction}
			if err := tx.Install(stage); err == nil {
				t.Fatal("install accepted incomplete manifest")
			}
			if err := tx.Rollback(); err == nil {
				t.Fatal("rollback accepted incomplete manifest")
			}
			if _, err := os.Stat(filepath.Join(transaction, "install-started.json")); !os.IsNotExist(err) {
				t.Fatal("invalid backup began installation")
			}
			if data, err := os.ReadFile(sentinel); err != nil || string(data) != "unchanged" {
				t.Fatal("invalid backup allowed rollback cleanup")
			}
		})
	}
}
