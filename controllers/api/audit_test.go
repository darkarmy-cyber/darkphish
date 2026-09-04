package api

import "testing"

func TestSafeCSVRejectsFormulaPrefixesAfterWhitespace(t *testing.T) {
	for _, value := range []string{"=cmd", "+cmd", "-cmd", "@cmd", "\t=cmd", "  +cmd"} {
		if got := safeCSV(value); got == value {
			t.Fatalf("unsafe CSV cell %q was not neutralized", value)
		}
	}
	if got := safeCSV("ordinary"); got != "ordinary" {
		t.Fatalf("ordinary CSV cell changed to %q", got)
	}
}
