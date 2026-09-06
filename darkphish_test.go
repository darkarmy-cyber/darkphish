package main

import (
	"strings"
	"testing"

	"github.com/alecthomas/kingpin/v2"
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

func TestDefaultServerCommandAndAdministrativeCommands(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{nil, "serve"},
		{[]string{"--mode", "admin", "--disable-mailer"}, "serve"},
		{[]string{"serve"}, "serve"},
		{[]string{"version"}, "version"},
		{[]string{"migrate", "check"}, "migrate check"},
		{[]string{"secrets", "status"}, "secrets status"},
		{[]string{"audit", "verify"}, "audit verify"},
	} {
		context, err := kingpin.CommandLine.ParseContext(test.args)
		if err != nil || context.SelectedCommand == nil || context.SelectedCommand.FullCommand() != test.want {
			t.Fatalf("command dispatch failed for %q: %v", test.args, err)
		}
	}
}
