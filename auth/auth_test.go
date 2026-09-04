package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordPolicy(t *testing.T) {
	candidate := "short"
	got := CheckPasswordPolicy(candidate)
	if got != ErrPasswordTooShort {
		t.Fatalf("unexpected error received. expected %v got %v", ErrPasswordTooShort, got)
	}

	candidate = "valid password"
	got = CheckPasswordPolicy(candidate)
	if got != nil {
		t.Fatalf("unexpected error received. expected %v got %v", nil, got)
	}
}

func TestArgon2idRoundTripAndMalformedFailure(t *testing.T) {
	hash, err := GeneratePasswordHash("a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("expected Argon2id hash, got %q", hash)
	}
	if err := ValidatePassword("a sufficiently long password", hash); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	if err := ValidatePassword("wrong password", hash); err != ErrInvalidPassword {
		t.Fatalf("expected invalid password, got %v", err)
	}
	for _, malformed := range []string{
		"",
		"$argon2id$broken",
		"$argon2id$v=19$m=999999999,t=2,p=1$aaaa$bbbb",
		"$argon2id$v=19$m=19456,t=2,p=255$YWJjZGVmZ2g$YWJjZGVmZ2hpamtsbW5vcA",
		"$argon2id$v=19$m=19456,t=2,p=1,trailing$YWJjZGVmZ2g$YWJjZGVmZ2hpamtsbW5vcA",
	} {
		if err := ValidatePassword("password", malformed); err != ErrInvalidPassword {
			t.Fatalf("malformed hash %q did not fail closed: %v", malformed, err)
		}
	}
}

func TestPasswordPolicyCountsUnicodeCharacters(t *testing.T) {
	if err := CheckPasswordPolicy("pässwörd1234"); err != nil {
		t.Fatalf("valid Unicode password rejected: %v", err)
	}
	if err := CheckPasswordPolicy("áéíóúý"); err != ErrPasswordTooShort {
		t.Fatalf("short Unicode password returned %v", err)
	}
	if err := CheckPasswordPolicy(string([]byte{0xff, 0xfe}) + "long-password"); err != ErrPasswordTooShort {
		t.Fatalf("invalid UTF-8 password returned %v", err)
	}
}

func TestLegacyBcryptReturnsArgon2idUpgrade(t *testing.T) {
	legacy, err := bcrypt.GenerateFromPassword([]byte("legacy password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := ValidatePasswordWithUpgrade("legacy password", string(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(upgraded, "$argon2id$") {
		t.Fatalf("expected upgrade hash, got %q", upgraded)
	}
}

func TestValidatePasswordChange(t *testing.T) {
	newPassword := "valid password"
	confirmPassword := "invalid"
	currentPassword := "current password"
	currentHash, err := GeneratePasswordHash(currentPassword)
	if err != nil {
		t.Fatalf("unexpected error generating password hash: %v", err)
	}

	_, got := ValidatePasswordChange(currentHash, newPassword, confirmPassword)
	if got != ErrPasswordMismatch {
		t.Fatalf("unexpected error received. expected %v got %v", ErrPasswordMismatch, got)
	}

	newPassword = currentPassword
	confirmPassword = newPassword
	_, got = ValidatePasswordChange(currentHash, newPassword, confirmPassword)
	if got != ErrReusedPassword {
		t.Fatalf("unexpected error received. expected %v got %v", ErrReusedPassword, got)
	}
}

func TestCheckAccountState(t *testing.T) {
	tests := []struct {
		name                string
		locked              bool
		changeRequired      bool
		allowPasswordChange bool
		want                error
	}{
		{name: "active", want: nil},
		{name: "locked", locked: true, want: ErrAccountLocked},
		{name: "change required", changeRequired: true, want: ErrPasswordChangeRequired},
		{name: "password change route", changeRequired: true, allowPasswordChange: true, want: nil},
		{name: "locked password change route", locked: true, changeRequired: true, allowPasswordChange: true, want: ErrAccountLocked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckAccountState(tt.locked, tt.changeRequired, tt.allowPasswordChange); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
