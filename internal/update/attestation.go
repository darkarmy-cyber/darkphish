package update

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Public Sigstore Fulcio/CT/Rekor trust, distributed with the application,
// never taken from a release, environment, browser, or the attestation itself.
// Source: sigstore/root-signing targets/trusted_root.json, 2026-09-14.
// Rotation requires an application review; unknown keys fail closed.
//
//go:embed sigstore-trusted-root.jsonl
var sigstoreRoot []byte

const attestationVerifier = "/usr/bin/gh"
const releaseIdentity = "https://github.com/" + repository + "/.github/workflows/release.yml@refs/heads/main"

func verifierCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, attestationVerifier, args...)
	cmd.Dir = dir
	// No inherited GitHub credentials, Vault tokens, proxies, executable search
	// path, or CLI configuration. Bundle + custom root make verification offline.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "GH_CONFIG_DIR=" + dir, "GH_HOST=github.com", "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1"}
	return cmd
}

func attestationArgs(dir string, r Release) []string {
	return []string{"attestation", "verify", filepath.Join(dir, "SHA256SUMS"),
		"--bundle", filepath.Join(dir, "bundles.jsonl"),
		"--custom-trusted-root", filepath.Join(dir, "trusted-root.jsonl"),
		"--repo", repository, "--hostname", "github.com",
		"--cert-identity", releaseIdentity,
		"--cert-oidc-issuer", "https://token.actions.githubusercontent.com",
		"--source-ref", "refs/heads/main", "--source-digest", r.Source,
		"--signer-digest", r.Source, "--deny-self-hosted-runners",
		"--predicate-type", "https://slsa.dev/provenance/v1"}
}

func (c *Client) verifyAttestation(ctx context.Context, r Release, manifest []byte) error {
	if err := AttestationSupport(); err != nil {
		return err
	}
	h := sha256.Sum256(manifest)
	var response struct {
		Attestations []struct {
			Bundle json.RawMessage `json:"bundle"`
		} `json:"attestations"`
	}
	if err := c.json(ctx, "/attestations/sha256:"+hex.EncodeToString(h[:])+"?per_page=30", &response); err != nil {
		return errors.New("signed release attestations unavailable")
	}
	if len(response.Attestations) == 0 || len(response.Attestations) > 30 {
		return errors.New("missing or excessive release attestations")
	}
	var bundles strings.Builder
	for _, a := range response.Attestations {
		if len(a.Bundle) == 0 || string(a.Bundle) == "null" {
			return errors.New("missing inline attestation bundle")
		}
		compact, err := json.Marshal(a.Bundle)
		if err != nil {
			return errors.New("invalid attestation bundle")
		}
		bundles.Write(compact)
		bundles.WriteByte('\n')
	}
	dir, err := os.MkdirTemp("", "darkphish-attestation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for name, data := range map[string][]byte{"SHA256SUMS": manifest, "bundles.jsonl": []byte(bundles.String()), "trusted-root.jsonl": sigstoreRoot} {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := verifierCommand(verifyCtx, dir, attestationArgs(dir, r)...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		return errors.New("release signature or workflow provenance verification failed")
	}
	return nil
}
