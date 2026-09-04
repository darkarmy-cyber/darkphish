package auth

import (
	"testing"
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
