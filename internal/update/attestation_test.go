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
	"time"
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
		case strings.Contains(req.URL.Path, "/actions/workflows/release-recover.yml/runs"):
			data = []byte(`{"workflow_runs":[]}`)
		default:
			t.Fatalf("unverified executable download reached: %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}}
	if data, err := c.VerifiedArchive(context.Background(), r, "amd64"); err == nil || data != nil {
		t.Fatal("self-consistent GitHub metadata bypassed independent attestation")
	}
}

func TestRecoveryPublicationUsesExecutionCommitAndSignedReceipt(t *testing.T) {
	r, _, _ := evidence(t)
	execution := strings.Repeat("c", 40)
	run := recoveryRun{ID: 1, HeadSHA: execution, HeadBranch: "main", Path: ".github/workflows/release-recover.yml", Event: "schedule", Status: "completed", Conclusion: "success", Created: r.Published.Add(-time.Hour)}
	steps := []publicationStep{
		{Name: "Attest rebuilt recovery artifacts", Status: "completed", Conclusion: "success", Started: r.Published.Add(-2 * time.Minute), Completed: r.Published.Add(-time.Minute)},
		{Name: "Publish rebuilt verified recovery assets", Status: "completed", Conclusion: "success", Started: r.Published.Add(-time.Second), Completed: r.Published.Add(time.Second)},
	}
	if !recoveryPublicationMatches(run, steps, r) {
		t.Fatal("official recovery publication was rejected")
	}
	for name, mutate := range map[string]func(*recoveryRun){
		"branch":   func(run *recoveryRun) { run.HeadBranch = "feature" },
		"workflow": func(run *recoveryRun) { run.Path = ".github/workflows/untrusted.yml" },
		"failed":   func(run *recoveryRun) { run.Conclusion = "failure" },
		"trigger":  func(run *recoveryRun) { run.Event = "pull_request" },
		"later":    func(run *recoveryRun) { run.Created = r.Published.Add(time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := run
			mutate(&bad)
			if recoveryPublicationMatches(bad, steps, r) {
				t.Fatal("untrusted publication accepted")
			}
		})
	}
	args := strings.Join(publicationAttestationArgs("/private", recoveryIdentity, execution), "\n")
	if !strings.Contains(args, "--source-digest\n"+execution) || !strings.Contains(args, "--signer-digest\n"+execution) || !strings.Contains(args, recoveryIdentity) || !strings.Contains(args, "receipt.json") || strings.Contains(args, r.Source) {
		t.Fatal("recovery execution policy conflates build source and signing execution")
	}
	steps[1].Completed = r.Published.Add(-time.Second)
	if recoveryPublicationMatches(run, steps, r) {
		t.Fatal("unrelated publication time accepted")
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

func TestRecoveryStopsAfterVerifiedPublicationCandidate(t *testing.T) {
	r, _, _ := evidence(t)
	execution := strings.Repeat("c", 40)
	runs := make([]recoveryRun, 100)
	for i := range runs {
		runs[i] = recoveryRun{ID: int64(i + 1), HeadSHA: execution, HeadBranch: "main", Path: ".github/workflows/release-recover.yml", Event: "schedule", Status: "completed", Conclusion: "success", Created: r.Published.Add(-time.Hour)}
	}
	steps := []publicationStep{
		{Name: "Attest rebuilt recovery artifacts", Status: "completed", Conclusion: "success", Started: r.Published.Add(-2 * time.Minute), Completed: r.Published.Add(-time.Minute)},
		{Name: "Publish rebuilt verified recovery assets", Status: "completed", Conclusion: "success", Started: r.Published.Add(-time.Second), Completed: r.Published.Add(time.Second)},
	}
	requests := 0
	c := &Client{http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Header.Get("Authorization") != "" {
			t.Fatal("public recovery lookup sent credentials")
		}
		var value any
		switch {
		case strings.HasSuffix(req.URL.Path, "/workflows/release-recover.yml/runs"):
			value = map[string]any{"workflow_runs": runs}
		case strings.HasSuffix(req.URL.Path, "/runs/1/jobs"):
			value = map[string]any{"jobs": []map[string]any{{"name": "publish", "conclusion": "success", "steps": steps}}}
		case strings.Contains(req.URL.Path, "/compare/"):
			value = map[string]any{"status": "ahead", "merge_base_commit": map[string]string{"sha": r.Source}}
		default:
			t.Fatalf("unnecessary historical API request: %s", req.URL.Path)
		}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}}
	got, err := c.recoveryExecutions(context.Background(), r)
	if err != nil || len(got) != 1 || got[0] != execution || requests != 3 {
		t.Fatal("recovery lookup exhausted history after finding publisher", got, requests, err)
	}
}
