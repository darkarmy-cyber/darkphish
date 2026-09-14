package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCheckerIgnoresDraftAndPrereleaseWithoutToken(t *testing.T) {
	r, _, _ := evidence(t)
	draft := r
	draft.ID = 2
	draft.Draft = true
	draft.Tag = "v99.0.0"
	pre := r
	pre.ID = 3
	pre.Prerelease = true
	pre.Tag = "v98.0.0"
	body, _ := json.Marshal([]Release{draft, pre, r})
	c := &Client{http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.github.com" || req.Header.Get("Authorization") != "" {
			t.Fatal("checker sent credentials or used an unexpected host")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	got, err := c.Latest(context.Background())
	if err != nil || got.Tag != r.Tag {
		t.Fatalf("latest: %s %v", got.Tag, err)
	}
	for _, raw := range []string{`{"draft":false}`, `{"draft":null,"prerelease":false}`, `{"prerelease":false}`} {
		var release Release
		if err = json.Unmarshal([]byte(raw), &release); err == nil {
			t.Fatal("accepted missing release state")
		}
	}
}

func TestUpdateOutcomeSurvivesReleaseChecks(t *testing.T) {
	r, _, _ := evidence(t)
	body, _ := json.Marshal([]Release{r})
	for _, result := range []string{"applied", "rollback", "backup_failed", "apply_failed"} {
		s := NewService("0.7.1", "", func(Release) error { return nil })
		s.SetResult(result)
		message := s.Status().Result
		if message == "" {
			t.Fatal("missing transaction outcome")
		}
		s.client.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
		})
		status, err := s.Check(context.Background(), true)
		if err != nil || status.Result != message {
			t.Fatal("release check erased transaction outcome")
		}
		if err = s.Apply(); err != nil || s.Status().Result != "" {
			t.Fatal("new accepted update retained previous outcome")
		}
	}
}

func TestSQLiteBackupAndRollback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native apply requires Linux rollback allocation")
	}
	root := t.TempDir()
	dir := t.TempDir()
	l := Layout{Root: root, Config: "config.json", Database: "darkphish.db"}
	for _, name := range runtimeEntries {
		p := filepath.Join(root, name)
		if name == "db" || name == "static" || name == "templates" {
			if err := os.Mkdir(p, 0700); err != nil {
				t.Fatal(err)
			}
			p = filepath.Join(p, "fixture")
		}
		if err := os.WriteFile(p, []byte("old"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DARKPHISH_VAULT_TOKEN", "environment-secret-not-for-backup")
	if err := os.WriteFile(filepath.Join(root, l.Config), []byte(`{"secrets":{"vault":{"token":"inline-vault-secret","token_file":"/run/secrets/vault"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(root, l.Database))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE fixture(value TEXT); INSERT INTO fixture VALUES ('before')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	backup := filepath.Join(dir, "backup")
	if err = Backup(context.Background(), l, backup); err != nil {
		t.Fatal(err)
	}
	configBytes, err := os.ReadFile(filepath.Join(backup, l.Config))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(configBytes, []byte("inline-vault-secret")) || bytes.Contains(configBytes, []byte("environment-secret")) {
		t.Fatal("backup exported a Vault token")
	}
	manifest, err := os.ReadFile(filepath.Join(backup, "backup-manifest.json"))
	if err != nil || !bytes.Contains(manifest, []byte("authoritative secret store")) {
		t.Fatal("missing external-secret recovery notice")
	}
	stage := t.TempDir()
	for _, entry := range runtimeEntries {
		if err = copyTree(filepath.Join(root, entry), filepath.Join(stage, entry)); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(stage, "darkphish"), []byte("replacement"), 0700); err != nil {
		t.Fatal(err)
	}
	tx := Transaction{Layout: l, Directory: dir}
	reserve := filepath.Join(dir, "rollback-reserve")
	if err = os.WriteFile(reserve, []byte("allocation unavailable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = tx.Install(stage); err == nil {
		t.Fatal("install ignored failed space reservation")
	}
	if _, err = os.Stat(filepath.Join(dir, "install-started.json")); !os.IsNotExist(err) {
		t.Fatal("installation started before rollback space was reserved")
	}
	if data, err := os.ReadFile(filepath.Join(root, "darkphish")); err != nil || string(data) != "old" {
		t.Fatal("space reservation failure changed runtime")
	}
	if err = os.Remove(reserve); err != nil {
		t.Fatal(err)
	}
	if err = tx.Install(stage); err != nil {
		t.Fatal("installation failed", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	// Emulate another interruption losing a restored directory entry. Recovery
	// must be repeatable without having consumed its backup source.
	if err = os.RemoveAll(filepath.Join(root, "templates")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal("repeated rollback failed:", err)
	}
	if _, err = os.Stat(filepath.Join(root, "templates", "fixture")); err != nil {
		t.Fatal("repeated rollback lost a runtime directory")
	}
	b, err := os.ReadFile(filepath.Join(root, "darkphish"))
	if err != nil || string(b) != "old" {
		t.Fatal("binary was not rolled back")
	}
	db, err = sql.Open("sqlite3", filepath.Join(root, l.Database))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err = db.QueryRow("SELECT value FROM fixture").Scan(&value); err != nil || value != "before" {
		t.Fatalf("database not restored: %s %v", value, err)
	}
}

func TestReplacementRejectsUnbundledRuntimeFiles(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	for _, base := range []string{root, stage} {
		for _, entry := range []string{"db", "templates", "static"} {
			if err := os.Mkdir(filepath.Join(base, entry), 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	custom := filepath.Join(root, "static", "custom-payload")
	if err := os.WriteFile(custom, []byte("campaign asset"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReplacement(context.Background(), root, stage); err == nil {
		t.Fatal("replacement would discard a campaign asset")
	}
	if data, err := os.ReadFile(custom); err != nil || string(data) != "campaign asset" {
		t.Fatal("rejected replacement changed custom file")
	}
}

func TestDatabaseCannotOverlapUpdateState(t *testing.T) {
	l := Layout{Root: t.TempDir(), Config: "config.json", Database: ".darkphish-updates"}
	if err := l.Validate(); err == nil {
		t.Fatal("update state accepted as database")
	}
	l.Database = "darkphish.db"
	if err := os.WriteFile(filepath.Join(l.Root, ".darkphish-updates"), []byte("unrelated file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := l.Validate(); err == nil {
		t.Fatal("regular file update-state collision accepted")
	}
}

func TestCancelledInstallDoesNotMutateRuntime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "darkphish")
	if err := os.WriteFile(path, []byte("old"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx := Transaction{Layout: Layout{Root: root}, Directory: t.TempDir()}
	if err := tx.InstallContext(ctx, t.TempDir()); err != context.Canceled {
		t.Fatal("cancelled install did not stop", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "old" {
		t.Fatal("cancelled install changed runtime")
	}
}

func TestExtractionAndCopyObserveCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := contextReader{ctx, strings.NewReader("archive contents")}
	if n, err := reader.Read(make([]byte, 1)); err != nil || n != 1 {
		t.Fatal("initial read failed", err)
	}
	cancel()
	if _, err := reader.Read(make([]byte, 1)); err != context.Canceled {
		t.Fatal("copy ignored cancellation", err)
	}
	if err := ExtractContext(ctx, nil, t.TempDir(), "0.8.0", "amd64"); err != context.Canceled {
		t.Fatal("extraction ignored cancellation", err)
	}
	if err := copyTreeContext(ctx, t.TempDir(), t.TempDir()); err != context.Canceled {
		t.Fatal("backup copy ignored cancellation", err)
	}
}

func TestStableSemVer(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"0.7.1", "0.7.0", 1}, {"0.8.0", "0.7.99", 1}, {"0.9.0", "0.10.0", -1}, {"0.8.10", "0.8.2", 1}, {"0.8.0", "0.8.0", 0}} {
		got, err := Compare(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("%s / %s: %d %v", tc.a, tc.b, got, err)
		}
	}
	for _, v := range []string{"0.7", "v0.7.1", "0.7.1-rc.1", "0.7.1+build", "00.7.1", "0.07.1", "0.7.01", "-1.0.0", "0.1.18446744073709551616", "0.7.1\n"} {
		if _, err := Compare(v, "0.7.0"); err == nil {
			t.Fatalf("accepted %q", v)
		}
	}
}

func evidence(t *testing.T) (Release, []byte, []byte) {
	t.Helper()
	r := Release{ID: 1, Tag: "v0.8.0", Source: strings.Repeat("a", 40), Published: time.Now().Add(-time.Hour), Author: Actor{Login: "github-actions[bot]", Type: "Bot", ID: 41898282}}
	var manifest strings.Builder
	names := []string{"darkphish-v0.8.0-darwin-amd64.tar.gz", "darkphish-v0.8.0-darwin-arm64.tar.gz", "darkphish-v0.8.0-linux-amd64.tar.gz", "darkphish-v0.8.0-linux-arm64.tar.gz", "darkphish-v0.8.0-windows-amd64.zip", "darkphish-v0.8.0.spdx.json", "SHA256SUMS", "darkphish-v0.8.0.release.json"}
	for i, name := range names {
		digest := strings.Repeat("b", 64)
		r.Assets = append(r.Assets, Asset{ID: int64(i + 1), Name: name, Size: 10, Digest: "sha256:" + digest, State: "uploaded", URL: downloadRoot + r.Tag + "/" + name, Uploader: r.Author})
		if i < 6 {
			manifest.WriteString(digest + "  " + name + "\n")
		}
	}
	m := []byte(manifest.String())
	h := sha256.Sum256(m)
	receipt, err := json.Marshal(map[string]string{"schema": "darkphish-release-publication-receipt/v1", "tag": r.Tag, "source_sha": r.Source, "checksums_sha256": hex.EncodeToString(h[:])})
	if err != nil {
		t.Fatal(err)
	}
	return r, m, receipt
}

func TestReleaseEvidence(t *testing.T) {
	r, m, p := evidence(t)
	if err := VerifyEvidence(r, m, p); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Release){"draft": func(r *Release) { r.Draft = true }, "prerelease": func(r *Release) { r.Prerelease = true }, "human": func(r *Release) { r.Author.ID = 1 }, "branch": func(r *Release) { r.Source = "main" }, "url": func(r *Release) { r.Assets[0].URL = "https://evil.example/binary" }, "duplicate": func(r *Release) { r.Assets[0] = r.Assets[1] }, "uploader": func(r *Release) { r.Assets[0].Uploader.ID = 1 }, "digest": func(r *Release) { r.Assets[0].Digest = "sha256:" + strings.Repeat("c", 64) }} {
		t.Run(name, func(t *testing.T) {
			r, m, p := evidence(t)
			change(&r)
			if err := VerifyEvidence(r, m, p); err == nil {
				t.Fatal("accepted untrusted evidence")
			}
		})
	}
	for _, bad := range [][]byte{[]byte("{}"), bytes.Replace(p, []byte(r.Source), []byte(strings.Repeat("c", 40)), 1), bytes.Replace(p, []byte("v0.8.0"), []byte("v0.8.1"), 1)} {
		if err := VerifyEvidence(r, m, bad); err == nil {
			t.Fatal("accepted receipt mismatch")
		}
	}
	if err := VerifyEvidence(r, append(m, 'x'), p); err == nil {
		t.Fatal("accepted checksum mismatch")
	}
}

func TestArchiveTraversalAndLinks(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind byte
	}{{"../escape", tar.TypeReg}, {"darkphish-v0.8.0-linux-amd64/../../escape", tar.TypeReg}, {"/absolute", tar.TypeReg}, {"darkphish-v0.8.0-linux-amd64/link", tar.TypeSymlink}, {"darkphish-v0.8.0-linux-amd64/link", tar.TypeLink}, {"darkphish-v0.8.0-linux-amd64/C:\\escape", tar.TypeReg}} {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{Name: tc.name, Typeflag: tc.kind, Linkname: "/etc/passwd", Mode: 0600}); err != nil {
			t.Fatal(err)
		}
		tw.Close()
		gz.Close()
		if err := Extract(b.Bytes(), t.TempDir(), "0.8.0", "amd64"); err == nil {
			t.Fatalf("accepted %s", tc.name)
		}
	}
}

func TestExtractCompleteNestedRuntime(t *testing.T) {
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	prefix := "darkphish-v0.8.0-linux-amd64/"
	for _, name := range runtimeEntries {
		content := []byte("fixture")
		if name == "VERSION" {
			content = []byte("0.8.0\n")
		}
		if name == "db" || name == "templates" || name == "static" {
			name += "/nested/file"
		}
		if err := tw.WriteHeader(&tar.Header{Name: prefix + name, Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := Extract(data.Bytes(), dest, "0.8.0", "amd64"); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"db", "templates", "static"} {
		if _, err := os.Stat(filepath.Join(dest, root, "nested", "file")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBackupBeforeInstall(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()
	old := []byte("original")
	if err := os.WriteFile(filepath.Join(root, "darkphish"), old, 0700); err != nil {
		t.Fatal(err)
	}
	tx := Transaction{Layout: Layout{Root: root, Config: "config.json", Database: "darkphish.db"}, Directory: dir}
	if err := tx.Install(t.TempDir()); err == nil {
		t.Fatal("installed without backup")
	}
	b, err := os.ReadFile(filepath.Join(root, "darkphish"))
	if err != nil || !bytes.Equal(b, old) {
		t.Fatal("original changed before backup")
	}
}

func TestUnsupportedServiceDoesNotApply(t *testing.T) {
	called := false
	s := NewService("0.8.0", "MySQL is unsupported", func(Release) error { called = true; return nil })
	s.status.Available = true
	s.status.Checked = time.Now()
	if err := s.Apply(); err == nil || called {
		t.Fatal("unsupported installation applied")
	}
	if _, err := s.Check(context.Background(), false); err != nil {
		t.Fatal(err)
	}
}
