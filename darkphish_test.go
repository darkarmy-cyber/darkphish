package main

import (
	"strings"
	"testing"
)

func TestDevelopmentVersionSummary(t *testing.T) {
	previous := releaseVersion
	releaseVersion = ""
	t.Cleanup(func() { releaseVersion = previous })
	wantDisplay := strings.TrimSuffix(semanticVersion(), ".0") + "-dev"
	if !strings.Contains(versionSummary(), "Darkphish "+wantDisplay) || !strings.Contains(versionSummary(), "commit ") || !strings.Contains(versionSummary(), "built ") {
		t.Fatalf("unexpected development version: %q", versionSummary())
	}
}
