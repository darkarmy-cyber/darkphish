package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderedTargetUsesExpectedPorts(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{"https://example.test/login", "example.test:443"},
		{"http://example.test/login", "example.test:80"},
		{"https://example.test:8443/login", "example.test:8443"},
	} {
		got, err := renderedTarget(tc.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.raw, got, tc.want)
		}
	}
}

func TestRenderedTargetRejectsInvalidURL(t *testing.T) {
	if _, err := renderedTarget("file:///tmp/page"); err == nil {
		t.Fatal("non-http import target accepted")
	}
}

func TestBoundedBufferRejectsOverflow(t *testing.T) {
	var b boundedBuffer
	b.limit = 4
	if _, err := b.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("5")); err == nil {
		t.Fatal("overflow accepted")
	}
}

func TestChromiumExecutableHonorsConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "browser")
	if err := os.WriteFile(path, []byte("placeholder"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DARKPHISH_CHROMIUM_PATH", path)
	if got := chromiumExecutable(); got != path {
		t.Fatalf("got %q want %q", got, path)
	}
}

func TestRenderedImportWarningDoesNotExposeDiagnostics(t *testing.T) {
	message := renderedImportWarning(errRenderedImportUnavailable)
	if message == "" || strings.Contains(message, "/") {
		t.Fatal("fallback warning is missing or exposes implementation details")
	}
}
