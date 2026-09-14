package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttestationCommandPinsIdentityAndDropsSecrets(t *testing.T) {
	r, _, _ := evidence(t)
	dir := t.TempDir()
	t.Setenv("GH_TOKEN", "must-not-inherit")
	t.Setenv("DARKPHISH_VAULT_TOKEN", "must-not-inherit")
	t.Setenv("GH_CONFIG_DIR", "/attacker-config")
	cmd := verifierCommand(context.Background(), dir, attestationArgs(dir, r)...)
	if cmd.Path != "/usr/bin/gh" || cmd.Dir != dir {
		t.Fatal("verifier must not use PATH or a release executable")
	}
	args := strings.Join(cmd.Args, "\n")
	for _, required := range []string{"--source-digest\n" + r.Source, "--signer-digest\n" + r.Source, "--source-ref\nrefs/heads/main", "--cert-identity\n" + releaseIdentity, "--cert-oidc-issuer\nhttps://token.actions.githubusercontent.com", "--deny-self-hosted-runners", "--custom-trusted-root\n" + filepath.Join(dir, "trusted-root.jsonl"), "--bundle\n" + filepath.Join(dir, "bundles.jsonl")} {
		if !strings.Contains(args, required) {
			t.Fatalf("missing attestation policy: %s", required)
		}
	}
	for _, env := range cmd.Env {
		if strings.Contains(env, "must-not-inherit") || strings.Contains(env, "attacker-config") {
			t.Fatal("verifier inherited credentials or configuration")
		}
	}
}

func TestConsistentForgedReleaseCannotReachArchiveDownload(t *testing.T) {
	r, manifest, receipt := evidence(t)
	for i := range r.Assets {
		var data []byte
		switch r.Assets[i].Name {
		case "SHA256SUMS":
			data = manifest
		case "darkphish-" + r.Tag + ".release.json":
			data = receipt
		default:
			continue
		}
		h := sha256.Sum256(data)
		r.Assets[i].Digest = "sha256:" + hex.EncodeToString(h[:])
		r.Assets[i].Size = int64(len(data))
	}
	if err := VerifyEvidence(r, manifest, receipt); err != nil {
		t.Fatal(err)
	}
	c := &Client{http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("attestation fetch sent credentials")
		}
		var data []byte
		switch {
		case strings.Contains(req.URL.Path, "/git/ref/tags/"):
			data = []byte(`{"object":{"type":"commit","sha":"` + r.Source + `"}}`)
		case strings.HasSuffix(req.URL.Path, "/SHA256SUMS"):
			data = manifest
		case strings.HasSuffix(req.URL.Path, ".release.json"):
			data = receipt
		case strings.Contains(req.URL.Path, "/attestations/"):
			data = []byte(`{"attestations":[{"bundle":{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}}]}`)
		default:
			t.Fatalf("unverified executable download reached: %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}}
	if data, err := c.VerifiedArchive(context.Background(), r, "amd64"); err == nil || data != nil {
		t.Fatal("self-consistent GitHub metadata bypassed independent attestation")
	}
}

func TestEmbeddedTrustRootIsIndependentPublicGood(t *testing.T) {
	var root struct {
		Tlogs  []json.RawMessage `json:"tlogs"`
		CTLogs []json.RawMessage `json:"ctlogs"`
		CAs    []json.RawMessage `json:"certificateAuthorities"`
	}
	if err := json.Unmarshal(sigstoreRoot, &root); err != nil || len(root.Tlogs) == 0 || len(root.CTLogs) == 0 || len(root.CAs) == 0 {
		t.Fatal("missing independent trust material")
	}
	if bytes.Count(sigstoreRoot, []byte{'\n'}) != 1 || sigstoreRoot[len(sigstoreRoot)-1] != '\n' {
		t.Fatal("offline verifier needs newline-terminated JSONL")
	}
}
