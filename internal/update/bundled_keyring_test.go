package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Do not derive this fixture from runtimeEntries: that would hide a mismatch
// between the native release packaging and the updater's allowlist again.
func packagedRuntimeFixture(t *testing.T) map[string][]byte {
	t.Helper()
	keyring, err := os.ReadFile("../../license-public-keys.json")
	if err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{
		"darkphish": []byte("binary fixture"), "VERSION": []byte("0.15.0\n"),
		"LICENSE": []byte("license"), "NOTICE.md": []byte("notice"),
		"README.md": []byte("readme"), "CHANGELOG.md": []byte("changes"),
		"config.json": []byte("{}"), "license-public-keys.json": keyring,
		"db/fixture": []byte("migration"), "templates/fixture": []byte("template"),
		"static/fixture": []byte("asset"),
	}
}

func TestLayoutAcceptsBundledLicenseKeyringOnlyAsRegularFile(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "unmanaged sibling"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			for name, data := range packagedRuntimeFixture(t) {
				if name == "license-public-keys.json" && kind != "file" && kind != "unmanaged sibling" {
					continue
				}
				p := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			keyring := filepath.Join(root, "license-public-keys.json")
			switch kind {
			case "directory":
				if err := os.Mkdir(keyring, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if runtime.GOOS != "linux" {
					t.Skip("native updater symlink checks run on Linux")
				}
				if err := os.Symlink(filepath.Join(root, "README.md"), keyring); err != nil {
					t.Fatal(err)
				}
			case "unmanaged sibling":
				if err := os.WriteFile(keyring+".backup", []byte("unmanaged"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := (Layout{Root: root, Config: "config.json", Database: "darkphish.db"}).Validate()
			if kind == "file" && err != nil {
				t.Fatalf("official bundled public keyring blocked updates: %v", err)
			}
			if kind != "file" && err == nil {
				t.Fatalf("accepted unsafe layout: %s", kind)
			}
		})
	}
}

func TestExtractNativePackageWithBundledLicenseKeyring(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("successful native extraction requires Linux directory sync")
	}
	for _, variant := range []string{"complete", "missing keyring", "keyring directory", "keyring symlink", "extra file"} {
		t.Run(variant, func(t *testing.T) {
			files := packagedRuntimeFixture(t)
			if variant == "missing keyring" {
				delete(files, "license-public-keys.json")
			}
			if variant == "extra file" {
				files["license-public-keys.json.backup"] = []byte("unexpected")
			}
			var data bytes.Buffer
			gz := gzip.NewWriter(&data)
			tarWriter := tar.NewWriter(gz)
			for name, content := range files {
				header := &tar.Header{Name: "darkphish-v0.15.0-linux-amd64/" + name, Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(content))}
				if name == "license-public-keys.json" && strings.HasPrefix(variant, "keyring ") {
					header.Size = 0
					content = nil
					if variant == "keyring directory" {
						header.Typeflag = tar.TypeDir
					} else {
						header.Typeflag = tar.TypeSymlink
						header.Linkname = "README.md"
					}
				}
				if err := tarWriter.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
				if _, err := tarWriter.Write(content); err != nil {
					t.Fatal(err)
				}
			}
			if err := tarWriter.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			dest := t.TempDir()
			err := Extract(data.Bytes(), dest, "0.15.0", "amd64")
			if variant != "complete" {
				if err == nil {
					t.Fatalf("accepted invalid archive: %s", variant)
				}
				return
			}
			if err != nil {
				t.Fatal("official native package was rejected:", err)
			}
			keyring, err := os.ReadFile(filepath.Join(dest, "license-public-keys.json"))
			if err != nil || !bytes.Equal(keyring, files["license-public-keys.json"]) {
				t.Fatal("extraction changed or dropped the bundled keyring")
			}
		})
	}
}
