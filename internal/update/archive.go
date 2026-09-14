package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var runtimeEntries = []string{"darkphish", "VERSION", "LICENSE", "NOTICE.md", "README.md", "CHANGELOG.md", "db", "templates", "static"}

// Extract accepts only regular native release files. Symlinks, hardlinks,
// devices, ambiguous names, duplicates and archive bombs are rejected.
func Extract(data []byte, dest, version, arch string) error {
	if _, err := Compare(version, version); err != nil {
		return err
	}
	if arch != "amd64" && arch != "arm64" {
		return errors.New("unsupported architecture")
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	prefix := "darkphish-v" + version + "-linux-" + arch
	seen := map[string]bool{}
	var total int64
	for count := 0; ; count++ {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if count >= 30000 {
			return errors.New("too many archive entries")
		}
		name := strings.TrimSuffix(h.Name, "/")
		if strings.ContainsAny(name, "\\\x00:") || path.Clean(name) != name || strings.HasPrefix(name, "/") {
			return errors.New("unsafe archive path")
		}
		if name == prefix && h.Typeflag == tar.TypeDir {
			continue
		}
		if !strings.HasPrefix(name, prefix+"/") {
			return errors.New("unexpected archive root")
		}
		rel := strings.TrimPrefix(name, prefix+"/")
		if !filepath.IsLocal(rel) || seen[rel] {
			return errors.New("unsafe or duplicate archive entry")
		}
		seen[rel] = true
		root := strings.Split(rel, "/")[0]
		allowed := root == "config.json"
		for _, entry := range runtimeEntries {
			allowed = allowed || root == entry
		}
		if !allowed {
			return errors.New("unexpected runtime entry")
		}
		if h.Typeflag != tar.TypeDir && h.Typeflag != tar.TypeReg {
			return errors.New("archive links and special files are forbidden")
		}
		if h.Size < 0 || h.Size > 256<<20 {
			return errors.New("archive file exceeds limit")
		}
		total += h.Size
		if total > 1<<30 {
			return errors.New("archive exceeds expanded limit")
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if h.Typeflag == tar.TypeDir {
			if err = os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if rel == "darkphish" {
			mode = 0700
		}
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(f, tr, h.Size)
		syncErr := f.Sync()
		closeErr := f.Close()
		if err = errors.Join(copyErr, syncErr, closeErr); err != nil {
			return err
		}
	}
	for _, name := range runtimeEntries {
		info, err := os.Lstat(filepath.Join(dest, name))
		if err != nil {
			return errors.New("incomplete runtime archive")
		}
		if (name == "db" || name == "static" || name == "templates") != info.IsDir() {
			return errors.New("invalid runtime entry type")
		}
	}
	v, err := os.ReadFile(filepath.Join(dest, "VERSION"))
	if err != nil || strings.TrimSpace(string(v)) != version {
		return errors.New("archive version mismatch")
	}
	return nil
}
