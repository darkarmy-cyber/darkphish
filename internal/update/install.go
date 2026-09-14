package update

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Layout is derived exclusively from local configuration, never HTTP input.
// The first iteration requires DB/config beside the executable and external
// secrets outside managed runtime directories.
type Layout struct {
	Root     string
	Database string
	Config   string
	Excluded []string
}

func (l Layout) Validate() error {
	if l.Config != "config.json" {
		return errors.New("one-click update requires config.json in the native release directory")
	}
	for _, excluded := range l.Excluded {
		if filepath.Base(excluded) != excluded || excluded == l.Config || excluded == l.Database {
			return errors.New("invalid external-secret exclusion")
		}
		for _, entry := range runtimeEntries {
			if excluded == entry {
				return errors.New("external secret overlaps managed runtime")
			}
		}
	}
	for _, name := range []string{l.Database, l.Config} {
		if !filepath.IsLocal(name) || filepath.Base(name) != name {
			return errors.New("one-click update requires a local database and config beside the binary")
		}
	}
	if l.Database == l.Config {
		return errors.New("database and config overlap")
	}
	for _, entry := range runtimeEntries {
		if entry == l.Database || entry == l.Config {
			return errors.New("configuration overlaps runtime files")
		}
	}
	return filepath.WalkDir(l.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(l.Root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if rel == ".darkphish-updates" {
			return filepath.SkipDir
		}
		for _, exclude := range l.Excluded {
			if rel == exclude {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.Type()&os.ModeSymlink != 0 || (!d.IsDir() && !d.Type().IsRegular()) {
			return errors.New("runtime links or special files are unsupported")
		}
		root := strings.Split(rel, string(filepath.Separator))[0]
		allowed := root == l.Config || root == l.Database || root == l.Database+"-wal" || root == l.Database+"-shm" || root == "config.json"
		for _, entry := range runtimeEntries {
			allowed = allowed || root == entry
		}
		if !allowed {
			return errors.New("unmanaged runtime files: use a dedicated native release directory with external secrets stored outside it")
		}
		return nil
	})
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := dst
		if rel != "." {
			target = filepath.Join(dst, rel)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("backup refuses symbolic links")
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("backup refuses special files")
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()&0700)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		return errors.Join(err, out.Sync(), out.Close())
	})
}

func syncDir(p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func writeJSON(p string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return errors.Join(err, f.Sync(), f.Close())
}

// Backup must run after the supervised application has exited. VACUUM INTO
// creates an independent, consistent SQLite snapshot including committed WAL.
func Backup(ctx context.Context, l Layout, dest string) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if err := os.Mkdir(dest, 0700); err != nil {
		return err
	}
	for _, entry := range runtimeEntries {
		if err := copyTree(filepath.Join(l.Root, entry), filepath.Join(dest, entry)); err != nil {
			return err
		}
	}
	// Copy original bytes, never config after environment/secret resolution.
	b, err := os.ReadFile(filepath.Join(l.Root, l.Config))
	if err != nil {
		return err
	}
	var raw map[string]any
	if err = json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if secrets, ok := raw["secrets"].(map[string]any); ok {
		if vault, ok := secrets["vault"].(map[string]any); ok {
			delete(vault, "token")
		}
	}
	if err = writeJSON(filepath.Join(dest, l.Config), raw); err != nil {
		return err
	}
	u := url.URL{Scheme: "file", Path: filepath.Join(l.Root, l.Database)}
	db, err := sql.Open("sqlite3", u.String()+"?mode=rw&_busy_timeout=5000")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(ctx, "VACUUM INTO ?", filepath.Join(dest, l.Database))
	closeErr := db.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return err
	}
	snapshot, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(filepath.Join(dest, l.Database))+"?mode=ro")
	if err != nil {
		return err
	}
	var integrity string
	err = snapshot.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity)
	closeErr = snapshot.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New("SQLite snapshot integrity check failed")
	}
	hashes := map[string]string{}
	err = filepath.WalkDir(dest, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return syncDir(p)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		closeErr := f.Close()
		if err = errors.Join(err, closeErr); err != nil {
			return err
		}
		rel, _ := filepath.Rel(dest, p)
		hashes[filepath.ToSlash(rel)] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if err != nil {
		return err
	}
	if err = writeJSON(filepath.Join(dest, "backup-manifest.json"), map[string]any{"schema": "darkphish-pre-update-backup/v1", "created_at": time.Now().UTC(), "files": hashes, "excluded": l.Excluded, "external_secrets": "Environment secrets and Vault tokens are not exported. Restore external secrets from the authoritative secret store."}); err != nil {
		return err
	}
	return syncDir(dest)
}

// Transaction keeps originals by rename, on the same filesystem. The durable
// journal is written before any installation mutation and survives power loss.
type Transaction struct {
	Layout    Layout
	Directory string
}

func (t Transaction) Install(stage string) error {
	if err := verifyBackup(filepath.Join(t.Directory, "backup")); err != nil {
		return errors.New("completed backup required before install")
	}
	if err := syncTreeDirectories(stage); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(t.Directory, "original"), 0700); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(t.Directory, "install-started.json"), map[string]bool{"started": true}); err != nil {
		return err
	}
	if err := syncDir(t.Directory); err != nil {
		return err
	}
	// Persist the transaction's name in its parent before mutating the runtime.
	if err := syncDir(filepath.Dir(t.Directory)); err != nil {
		return err
	}
	for _, entry := range runtimeEntries {
		source := filepath.Join(t.Layout.Root, entry)
		original := filepath.Join(t.Directory, "original", entry)
		info, err := os.Lstat(source)
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			// Keep the executable present across a crash: link the original, then
			// atomically replace it. A missing executable cannot run recovery.
			err = os.Link(source, original)
		} else {
			err = os.Rename(source, original)
		}
		if err != nil {
			return err
		}
		if err := syncDir(filepath.Join(t.Directory, "original")); err != nil {
			return err
		}
		if err := syncDir(t.Layout.Root); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(stage, entry), filepath.Join(t.Layout.Root, entry)); err != nil {
			return err
		}
		if err := syncDir(t.Layout.Root); err != nil {
			return err
		}
	}
	return nil
}

// Rollback restores both the application and the pre-migration SQLite snapshot.
// The caller must stop and reap the replacement process before invoking it.
func (t Transaction) Rollback() error {
	if err := verifyBackup(filepath.Join(t.Directory, "backup")); err != nil {
		return err
	}
	for _, entry := range runtimeEntries {
		original := filepath.Join(t.Directory, "original", entry)
		info, err := os.Lstat(original)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		// All paths are fixed runtime roots inside the locally validated layout.
		if !info.Mode().IsRegular() {
			if err := os.RemoveAll(filepath.Join(t.Layout.Root, entry)); err != nil {
				return err
			}
		}
		if err := os.Rename(original, filepath.Join(t.Layout.Root, entry)); err != nil {
			return err
		}
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(filepath.Join(t.Layout.Root, t.Layout.Database+suffix)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := copyTree(filepath.Join(t.Directory, "backup", t.Layout.Database), filepath.Join(t.Layout.Root, t.Layout.Database)); err != nil {
		return err
	}
	return syncDir(t.Layout.Root)
}

func verifyBackup(directory string) error {
	b, err := os.ReadFile(filepath.Join(directory, "backup-manifest.json"))
	if err != nil {
		return err
	}
	var manifest struct {
		Schema string            `json:"schema"`
		Files  map[string]string `json:"files"`
	}
	if err = json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	if manifest.Schema != "darkphish-pre-update-backup/v1" || len(manifest.Files) == 0 {
		return errors.New("invalid backup manifest")
	}
	for name, want := range manifest.Files {
		if !filepath.IsLocal(name) || strings.Contains(name, "\\") {
			return errors.New("unsafe backup manifest path")
		}
		p := filepath.Join(directory, filepath.FromSlash(name))
		info, err := os.Lstat(p)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("invalid backup file")
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		closeErr := f.Close()
		if err = errors.Join(err, closeErr); err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != want {
			return errors.New("backup checksum mismatch")
		}
	}
	return nil
}
