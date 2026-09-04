package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is the minimum number of characters required in a password.
// Darkphish intentionally favors length over composition rules.
const MinPasswordLength = 12

// APIKeyLength is the number of random bytes used for Darkphish API keys.
const APIKeyLength = 32

// ErrAccountLocked indicates that an authenticated account is disabled.
var ErrAccountLocked = errors.New("account is locked")

// ErrPasswordChangeRequired indicates that an authenticated account must
// finish the local password-reset flow before using privileged application
// functionality.
var ErrPasswordChangeRequired = errors.New("password change required")

// ErrInvalidPassword is thrown when a user provides an incorrect password.
var ErrInvalidPassword = errors.New("Invalid Password")

// ErrPasswordMismatch is thrown when a user provides a mismatching password
// and confirmation password.
var ErrPasswordMismatch = errors.New("Passwords do not match")

// ErrReusedPassword is thrown when a user attempts to change their password to
// the existing password
var ErrReusedPassword = errors.New("Cannot reuse existing password")

// ErrEmptyPassword is thrown when a user provides a blank password to the register
// or change password functions
var ErrEmptyPassword = errors.New("No password provided")

// ErrPasswordTooShort is thrown when a user provides a password that is less
// than MinPasswordLength
var ErrPasswordTooShort = fmt.Errorf("Password must be at least %d characters", MinPasswordLength)

// GenerateSecureKey returns the hex representation of key generated from n
// random bytes
func GenerateSecureKey(n int) string {
	k := make([]byte, n)
	io.ReadFull(rand.Reader, k)
	return fmt.Sprintf("%x", k)
}

const (
	argon2Time        = uint32(2)
	argon2Memory      = uint32(19 * 1024)
	argon2Parallelism = uint8(1)
	argon2SaltLength  = 16
	argon2KeyLength   = uint32(32)
)

// GeneratePasswordHash returns an Argon2id hash using the centrally managed
// password-hashing policy.
func GeneratePasswordHash(password string) (string, error) {
	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Parallelism, argon2KeyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory, argon2Time, argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

// CheckPasswordPolicy ensures the provided password is valid according to our
// password policy.
//
// The current password policy is simply a minimum of 12 characters, though this
// may change in the future (see #1538).
func CheckPasswordPolicy(password string) error {
	switch {
	// Admittedly, empty passwords are a subset of too short passwords, but it
	// helps to provide a more specific error message
	case password == "":
		return ErrEmptyPassword
	case !utf8.ValidString(password) || utf8.RuneCountInString(password) < MinPasswordLength:
		return ErrPasswordTooShort
	}
	return nil
}

// ValidatePassword validates that the provided password matches the stored
// Argon2id or legacy bcrypt hash.
func ValidatePassword(password string, hash string) error {
	_, err := ValidatePasswordWithUpgrade(password, hash)
	return err
}

// ValidatePasswordWithUpgrade validates current Argon2id hashes and legacy
// bcrypt hashes. A successful legacy or stale-policy check returns a fresh
// Argon2id hash for the caller to persist.
func ValidatePasswordWithUpgrade(password, encoded string) (string, error) {
	if strings.HasPrefix(encoded, "$2") {
		if err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password)); err != nil {
			return "", ErrInvalidPassword
		}
		return GeneratePasswordHash(password)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		return "", ErrInvalidPassword
	}
	params, salt, expected, err := parseArgon2id(encoded)
	if err != nil {
		return "", ErrInvalidPassword
	}
	actual := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.parallelism, uint32(len(expected)))
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return "", ErrInvalidPassword
	}
	if params.time != argon2Time || params.memory != argon2Memory || params.parallelism != argon2Parallelism || len(expected) != int(argon2KeyLength) {
		return GeneratePasswordHash(password)
	}
	return "", nil
}

type argon2Parameters struct {
	time        uint32
	memory      uint32
	parallelism uint8
}

func parseArgon2id(encoded string) (argon2Parameters, []byte, []byte, error) {
	var params argon2Parameters
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return params, nil, nil, ErrInvalidPassword
	}
	settings := strings.Split(parts[3], ",")
	if len(settings) != 3 || !strings.HasPrefix(settings[0], "m=") || !strings.HasPrefix(settings[1], "t=") || !strings.HasPrefix(settings[2], "p=") {
		return params, nil, nil, ErrInvalidPassword
	}
	memory, memoryErr := strconv.ParseUint(strings.TrimPrefix(settings[0], "m="), 10, 32)
	timeCost, timeErr := strconv.ParseUint(strings.TrimPrefix(settings[1], "t="), 10, 32)
	parallelism, parallelismErr := strconv.ParseUint(strings.TrimPrefix(settings[2], "p="), 10, 8)
	if memoryErr != nil || timeErr != nil || parallelismErr != nil || memory < 8 || memory > 256*1024 || timeCost == 0 || timeCost > 10 || parallelism == 0 || parallelism > 16 {
		return params, nil, nil, ErrInvalidPassword
	}
	params.memory = uint32(memory)
	params.time = uint32(timeCost)
	params.parallelism = uint8(parallelism)
	if len(parts[4]) > 128 || len(parts[5]) > 128 {
		return params, nil, nil, ErrInvalidPassword
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return params, nil, nil, ErrInvalidPassword
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) < 16 || len(hash) > 64 {
		return params, nil, nil, ErrInvalidPassword
	}
	return params, salt, hash, nil
}

// ValidatePasswordChange validates that the new password matches the
// configured password policy, that the new password and confirmation
// password match.
//
// Note that this assumes the current password has been confirmed by the
// caller.
//
// If all of the provided data is valid, then the hash of the new password is
// returned.
func ValidatePasswordChange(currentHash, newPassword, confirmPassword string) (string, error) {
	// Ensure the new password passes our password policy
	if err := CheckPasswordPolicy(newPassword); err != nil {
		return "", err
	}
	// Check that new passwords match
	if newPassword != confirmPassword {
		return "", ErrPasswordMismatch
	}
	// Make sure that the new password isn't the same as the old one
	err := ValidatePassword(newPassword, currentHash)
	if err == nil {
		return "", ErrReusedPassword
	}
	// Generate the new hash
	return GeneratePasswordHash(newPassword)
}

// CheckAccountState centralizes lifecycle checks shared by session and token
// authentication. allowPasswordChange is only true for the browser password
// reset endpoint; bearer tokens never bypass a required password change.
func CheckAccountState(accountLocked, passwordChangeRequired, allowPasswordChange bool) error {
	if accountLocked {
		return ErrAccountLocked
	}
	if passwordChangeRequired && !allowPasswordChange {
		return ErrPasswordChangeRequired
	}
	return nil
}
