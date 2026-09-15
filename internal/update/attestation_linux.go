//go:build linux

package update

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"time"
)

// AttestationSupport requires an independently installed, administrator-owned
// verifier. Never discover it through PATH or download it with the update.
func AttestationSupport() error {
	for _, path := range []string{"/", "/usr", "/usr/bin", attestationVerifier} {
		info, err := os.Lstat(path)
		if err != nil {
			return errors.New("one-click update requires administrator-installed /usr/bin/gh >= 2.100.0")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("attestation verifier and its parent directories must be root-owned and not group/world writable")
		}
		if path == attestationVerifier && (!info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0) {
			return errors.New("invalid attestation verifier executable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := verifierCommand(ctx, "/", "version").Output()
	fields := strings.Fields(string(output))
	if err != nil || len(fields) < 3 || fields[0] != "gh" || fields[1] != "version" {
		return errors.New("cannot determine attestation verifier version")
	}
	cmp, err := Compare(fields[2], "2.100.0")
	if err != nil || cmp < 0 {
		return errors.New("one-click update requires GitHub CLI 2.100.0 or newer")
	}
	return nil
}
