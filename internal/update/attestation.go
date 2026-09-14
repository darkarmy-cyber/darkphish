package update

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
const recoveryIdentity = "https://github.com/" + repository + "/.github/workflows/release-recover.yml@refs/heads/main"

func verifierCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, attestationVerifier, args...)
	cmd.Dir = dir
	// No inherited GitHub credentials, Vault tokens, proxies, executable search
	// path, or CLI configuration. Bundle + custom root make verification offline.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "GH_CONFIG_DIR=" + dir, "GH_HOST=github.com", "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1"}
	return cmd
}

func attestationArgs(dir string, r Release) []string {
	return publicationAttestationArgs(dir, releaseIdentity, r.Source)
}

func publicationAttestationArgs(dir, identity, execution string) []string {
	return []string{"attestation", "verify", filepath.Join(dir, "receipt.json"),
		"--bundle", filepath.Join(dir, "bundles.jsonl"),
		"--custom-trusted-root", filepath.Join(dir, "trusted-root.jsonl"),
		"--repo", repository, "--hostname", "github.com",
		"--cert-identity", identity,
		"--cert-oidc-issuer", "https://token.actions.githubusercontent.com",
		"--source-ref", "refs/heads/main", "--source-digest", execution,
		"--signer-digest", execution, "--deny-self-hosted-runners",
		"--predicate-type", "https://slsa.dev/provenance/v1"}
}

func (c *Client) verifyAttestation(ctx context.Context, r Release, receipt []byte) error {
	if err := AttestationSupport(); err != nil {
		return err
	}
	// The signed receipt binds the released source AND manifest hash. Recovery
	// runs sign on a later protected-main execution commit, not the source they
	// rebuild; these two commit identities must not be conflated.
	h := sha256.Sum256(receipt)
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
	for name, data := range map[string][]byte{"receipt.json": receipt, "bundles.jsonl": []byte(bundles.String()), "trusted-root.jsonl": sigstoreRoot} {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
	}
	verify := func(args []string) error {
		verifyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		cmd := verifierCommand(verifyCtx, dir, args...)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		return cmd.Run()
	}
	if verify(attestationArgs(dir, r)) == nil {
		return nil
	}
	executions, err := c.recoveryExecutions(ctx, r)
	if err != nil {
		return err
	}
	for _, execution := range executions {
		if verify(publicationAttestationArgs(dir, recoveryIdentity, execution)) == nil {
			return nil
		}
	}
	return errors.New("release signature or workflow provenance verification failed")
}

type recoveryRun struct {
	ID         int64     `json:"id"`
	HeadSHA    string    `json:"head_sha"`
	HeadBranch string    `json:"head_branch"`
	Path       string    `json:"path"`
	Event      string    `json:"event"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Created    time.Time `json:"created_at"`
}

type publicationStep struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Started    time.Time `json:"started_at"`
	Completed  time.Time `json:"completed_at"`
}

func recoveryPublicationMatches(run recoveryRun, steps []publicationStep, r Release) bool {
	if run.ID <= 0 || !sourceSHA.MatchString(run.HeadSHA) || run.HeadBranch != "main" || run.Path != ".github/workflows/release-recover.yml" || (run.Event != "schedule" && run.Event != "workflow_run") || run.Status != "completed" || run.Conclusion != "success" || run.Created.IsZero() || run.Created.After(r.Published) {
		return false
	}
	attested, published := false, false
	for _, step := range steps {
		if step.Status != "completed" || step.Conclusion != "success" || step.Started.IsZero() || step.Completed.Before(step.Started) {
			continue
		}
		if step.Name == "Attest rebuilt recovery artifacts" && !step.Completed.After(r.Published) {
			attested = true
		}
		if step.Name == "Publish rebuilt verified recovery assets" && !r.Published.Before(step.Started) && !r.Published.After(step.Completed) {
			published = true
		}
	}
	return attested && published
}

func (c *Client) recoveryExecutions(ctx context.Context, r Release) ([]string, error) {
	var response struct {
		Runs []recoveryRun `json:"workflow_runs"`
	}
	if err := c.json(ctx, "/actions/workflows/release-recover.yml/runs?branch=main&status=success&per_page=100&created=%3C%3D"+r.Published.Format(time.RFC3339), &response); err != nil {
		return nil, err
	}
	if len(response.Runs) > 100 {
		return nil, errors.New("excessive recovery publication history")
	}
	var executions []string
	for _, run := range response.Runs {
		if run.ID <= 0 || !sourceSHA.MatchString(run.HeadSHA) || run.Created.After(r.Published) {
			continue
		}
		var jobs struct {
			Jobs []struct {
				Name       string            `json:"name"`
				Conclusion string            `json:"conclusion"`
				Steps      []publicationStep `json:"steps"`
			} `json:"jobs"`
		}
		if err := c.json(ctx, fmt.Sprintf("/actions/runs/%d/jobs?per_page=100", run.ID), &jobs); err != nil {
			return nil, err
		}
		for _, job := range jobs.Jobs {
			if job.Name != "publish" || job.Conclusion != "success" || !recoveryPublicationMatches(run, job.Steps, r) {
				continue
			}
			var ancestry struct {
				Status string `json:"status"`
				Base   struct {
					SHA string `json:"sha"`
				} `json:"merge_base_commit"`
			}
			if err := c.json(ctx, "/compare/"+r.Source+"..."+run.HeadSHA, &ancestry); err != nil {
				return nil, err
			}
			if (ancestry.Status == "ahead" || ancestry.Status == "identical") && ancestry.Base.SHA == r.Source {
				executions = append(executions, run.HeadSHA)
			}
		}
	}
	return executions, nil
}
