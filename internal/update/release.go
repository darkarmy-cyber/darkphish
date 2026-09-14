package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const repository = "darkarmy-cyber/darkphish"
const apiRoot = "https://api.github.com/repos/" + repository
const downloadRoot = "https://github.com/" + repository + "/releases/download/"

var sourceSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)
var digestSHA = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var checksumLine = regexp.MustCompile(`^([a-f0-9]{64})  (darkphish-[A-Za-z0-9._-]+)$`)

type Actor struct {
	Login string `json:"login"`
	Type  string `json:"type"`
	ID    int64  `json:"id"`
}

func (a Actor) trusted() bool {
	return a.Login == "github-actions[bot]" && a.Type == "Bot" && a.ID == 41898282
}

type Asset struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Digest   string `json:"digest"`
	State    string `json:"state"`
	URL      string `json:"browser_download_url"`
	Uploader Actor  `json:"uploader"`
}

type Release struct {
	ID         int64     `json:"id"`
	Tag        string    `json:"tag_name"`
	Source     string    `json:"target_commitish"`
	Name       string    `json:"name"`
	Notes      string    `json:"body"`
	Published  time.Time `json:"published_at"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Author     Actor     `json:"author"`
	Assets     []Asset   `json:"assets"`
}

func (r *Release) UnmarshalJSON(data []byte) error {
	type plain Release
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"draft", "prerelease"} {
		value, ok := fields[key]
		if !ok || (string(value) != "true" && string(value) != "false") {
			return errors.New("missing release state")
		}
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = Release(decoded)
	return nil
}

func (r Release) Version() string { return strings.TrimPrefix(r.Tag, "v") }

// Client never accepts a repository, URL, credential, or path from a browser.
type Client struct{ http *http.Client }

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Port() != "" {
			return errors.New("untrusted release redirect")
		}
		switch req.URL.Host {
		case "github.com", "release-assets.githubusercontent.com":
			return nil
		}
		return errors.New("untrusted release redirect host")
	}}}
}

func (c *Client) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Darkphish-release-checker")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("release service unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release service returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("release response exceeds limit or is incomplete")
	}
	return data, nil
}

func (c *Client) json(ctx context.Context, path string, v any) error {
	b, err := c.get(ctx, apiRoot+path, 4<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func ValidateRelease(r Release) error {
	if r.ID <= 0 || r.Tag != "v"+r.Version() || r.Draft || r.Prerelease || r.Published.IsZero() || r.Published.After(time.Now().Add(time.Minute)) || !sourceSHA.MatchString(r.Source) || !r.Author.trusted() {
		return errors.New("untrusted release metadata")
	}
	if _, err := Compare(r.Version(), r.Version()); err != nil {
		return err
	}
	expected := map[string]bool{"SHA256SUMS": true, "darkphish-" + r.Tag + ".release.json": true, "darkphish-" + r.Tag + ".spdx.json": true}
	for _, target := range []string{"linux-amd64.tar.gz", "linux-arm64.tar.gz", "darwin-amd64.tar.gz", "darwin-arm64.tar.gz", "windows-amd64.zip"} {
		expected["darkphish-"+r.Tag+"-"+target] = true
	}
	if len(r.Assets) != len(expected) {
		return errors.New("incomplete release assets")
	}
	ids := map[int64]bool{}
	for _, a := range r.Assets {
		if !expected[a.Name] || ids[a.ID] || a.ID <= 0 || a.Size <= 0 || a.Size > 512<<20 || !digestSHA.MatchString(a.Digest) || a.State != "uploaded" || !a.Uploader.trusted() || a.URL != downloadRoot+r.Tag+"/"+a.Name {
			return errors.New("untrusted release asset")
		}
		delete(expected, a.Name)
		ids[a.ID] = true
	}
	return nil
}

func (c *Client) Latest(ctx context.Context) (Release, error) {
	var latest Release
	// Do not use /latest: published order need not equal SemVer order.
	for page := 1; page <= 20; page++ {
		var releases []Release
		if err := c.json(ctx, fmt.Sprintf("/releases?per_page=100&page=%d", page), &releases); err != nil {
			return Release{}, err
		}
		for _, r := range releases {
			if r.Draft || r.Prerelease {
				continue
			}
			if _, err := Compare(r.Version(), r.Version()); err != nil {
				continue
			}
			if r.Tag != "v"+r.Version() {
				continue
			}
			if latest.ID == 0 {
				latest = r
				continue
			}
			cmp, _ := Compare(r.Version(), latest.Version())
			if cmp > 0 {
				latest = r
			}
		}
		if len(releases) < 100 {
			if latest.ID == 0 {
				return Release{}, errors.New("no stable release available")
			}
			if err := ValidateRelease(latest); err != nil {
				return Release{}, err
			}
			return latest, nil
		}
	}
	return Release{}, errors.New("release listing exceeds limit")
}

func (c *Client) asset(ctx context.Context, r Release, name string, limit int64) ([]byte, error) {
	for _, a := range r.Assets {
		if a.Name == name {
			if a.Size > limit {
				return nil, errors.New("release asset exceeds limit")
			}
			b, err := c.get(ctx, downloadRoot+r.Tag+"/"+name, limit)
			if err != nil {
				return nil, err
			}
			h := sha256.Sum256(b)
			if int64(len(b)) != a.Size || "sha256:"+hex.EncodeToString(h[:]) != a.Digest {
				return nil, errors.New("release asset checksum mismatch")
			}
			return b, nil
		}
	}
	return nil, errors.New("missing release asset")
}

// VerifyEvidence binds all advertised payloads to the immutable source and receipt.
func VerifyEvidence(r Release, manifest, receipt []byte) error {
	if err := ValidateRelease(r); err != nil {
		return err
	}
	if len(manifest) > 65536 || len(receipt) > 4096 {
		return errors.New("release evidence exceeds limit")
	}
	h := sha256.Sum256(manifest)
	var proof map[string]string
	if err := json.Unmarshal(receipt, &proof); err != nil {
		return errors.New("invalid publication receipt")
	}
	if len(proof) != 4 || proof["schema"] != "darkphish-release-publication-receipt/v1" || proof["tag"] != r.Tag || proof["source_sha"] != r.Source || proof["checksums_sha256"] != hex.EncodeToString(h[:]) {
		return errors.New("publication receipt mismatch")
	}
	hashes := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		m := checksumLine.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
		if m == nil || hashes[m[2]] != "" {
			return errors.New("invalid checksum manifest")
		}
		hashes[m[2]] = m[1]
	}
	for _, a := range r.Assets {
		if a.Name == "SHA256SUMS" || a.Name == "darkphish-"+r.Tag+".release.json" {
			continue
		}
		if "sha256:"+hashes[a.Name] != a.Digest {
			return errors.New("checksum manifest mismatch")
		}
		delete(hashes, a.Name)
	}
	if len(hashes) != 0 {
		return errors.New("unexpected checksum entry")
	}
	return nil
}

// VerifiedArchive returns bytes only after all trust checks, including a second
// immutable release snapshot. The caller must still validate archive entries.
func (c *Client) VerifiedArchive(ctx context.Context, r Release, arch string) ([]byte, error) {
	if arch != "amd64" && arch != "arm64" {
		return nil, errors.New("unsupported architecture")
	}
	if err := ValidateRelease(r); err != nil {
		return nil, err
	}
	var ref struct {
		Object struct {
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"object"`
	}
	if err := c.json(ctx, "/git/ref/tags/"+r.Tag, &ref); err != nil {
		return nil, err
	}
	for depth := 0; ref.Object.Type == "tag" && depth < 8; depth++ {
		if !sourceSHA.MatchString(ref.Object.SHA) {
			return nil, errors.New("invalid tag object")
		}
		if err := c.json(ctx, "/git/tags/"+ref.Object.SHA, &ref); err != nil {
			return nil, err
		}
	}
	if ref.Object.Type != "commit" || ref.Object.SHA != r.Source {
		return nil, errors.New("immutable source mismatch")
	}
	manifest, err := c.asset(ctx, r, "SHA256SUMS", 65536)
	if err != nil {
		return nil, err
	}
	receipt, err := c.asset(ctx, r, "darkphish-"+r.Tag+".release.json", 4096)
	if err != nil {
		return nil, err
	}
	if err = VerifyEvidence(r, manifest, receipt); err != nil {
		return nil, err
	}
	// A receipt and GitHub asset digests alone are reproducible by a release
	// writer. Require independently signed workflow evidence before returning
	// any executable bytes to the supervisor (including its version probe).
	if err = c.verifyAttestation(ctx, r, manifest); err != nil {
		return nil, err
	}
	b, err := c.asset(ctx, r, "darkphish-"+r.Tag+"-linux-"+arch+".tar.gz", 512<<20)
	if err != nil {
		return nil, err
	}
	var after Release
	if err = c.json(ctx, fmt.Sprintf("/releases/%d", r.ID), &after); err != nil {
		return nil, err
	}
	x, _ := json.Marshal(r)
	y, _ := json.Marshal(after)
	if string(x) != string(y) {
		return nil, errors.New("release changed during verification")
	}
	var closing struct {
		Object struct {
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"object"`
	}
	if err = c.json(ctx, "/git/ref/tags/"+r.Tag, &closing); err != nil {
		return nil, err
	}
	for depth := 0; closing.Object.Type == "tag" && depth < 8; depth++ {
		if !sourceSHA.MatchString(closing.Object.SHA) {
			return nil, errors.New("invalid closing tag object")
		}
		if err = c.json(ctx, "/git/tags/"+closing.Object.SHA, &closing); err != nil {
			return nil, err
		}
	}
	if closing.Object.Type != "commit" || closing.Object.SHA != r.Source {
		return nil, errors.New("release tag changed during verification")
	}
	return b, nil
}
