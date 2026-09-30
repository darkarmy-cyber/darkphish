package update

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStableUsesCanonicalSourceMarkerWithoutTagLookup(t *testing.T) {
	r, _, _ := evidence(t)
	source := r.Source
	r.Source = "main"
	r.Notes = "## 0.8.0\n\nSource commit: " + source + "\n\n<!-- darkphish-release-source:" + source + " -->\n"
	body, err := json.Marshal([]Release{r})
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	c := &Client{http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if !strings.Contains(req.URL.Path, "/releases") {
			t.Fatalf("catalogue performed unexpected request: %s", req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	got, err := c.Stable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Source != source {
		t.Fatalf("unexpected stable catalogue: %+v", got)
	}
	if requests != 1 {
		t.Fatalf("stable catalogue made %d requests, want 1", requests)
	}
}

func TestStableVersionFetchesOnlyRequestedRelease(t *testing.T) {
	r, _, _ := evidence(t)
	source := r.Source
	r.Source = "main"
	releaseBody, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	requests := []string{}
	c := &Client{http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.URL.Path)
		switch req.URL.Path {
		case "/repos/" + repository + "/releases/tags/v0.8.0":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(releaseBody)), Header: make(http.Header)}, nil
		case "/repos/" + repository + "/git/ref/tags/v0.8.0":
			data := []byte(`{"object":{"type":"commit","sha":"` + source + `"}}`)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected request: %s", req.URL.Path)
			return nil, nil
		}
	})}}
	got, err := c.StableVersion(context.Background(), "0.8.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version() != "0.8.0" || got.Source != source {
		t.Fatalf("unexpected selected release: %+v", got)
	}
	if len(requests) != 2 {
		t.Fatalf("selected release verification made %d requests, want 2: %v", len(requests), requests)
	}
	for _, path := range requests {
		if strings.Contains(path, "/releases?") || strings.HasSuffix(path, "/releases") {
			t.Fatalf("selected release verification enumerated the release catalogue: %v", requests)
		}
	}
}
