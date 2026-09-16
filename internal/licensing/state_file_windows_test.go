package licensing

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsStateUsesOwnerOnlyDACLOnCreateAndReplacement(t *testing.T) {
	dir := t.TempDir()
	// Model a native archive extracted into a shared directory.
	public, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := public.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate := func(path string) {
		t.Helper()
		actual, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		if actual.String() != expected.String() {
			t.Fatalf("state DACL=%s want %s", actual.String(), expected.String())
		}
	}
	tmp, err := createPrivateStateTemp(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}
	assertPrivate(tmp.Name()) // Private even before a single credential byte is written.
	if err := os.Remove(tmp.Name()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("legacy shared state"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, credential := range []string{"test-refresh-one", "test-refresh-two"} {
		state := LocalState{InstallationID: "test-installation", RefreshToken: credential}
		if err := SaveState(path, state); err != nil {
			t.Fatal(err)
		}
		assertPrivate(path)
		stored, err := LoadState(path)
		if err != nil || stored.RefreshToken != credential {
			t.Fatal("private replacement did not preserve state")
		}
	}
}
