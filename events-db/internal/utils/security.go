package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// PasswordStrengthError describes exactly which password rules were violated.
type PasswordStrengthError struct {
	NeedsUpper   bool
	NeedsDigit   bool
	NeedsSpecial bool
	TooShort     bool
}

func (e *PasswordStrengthError) Error() string {
	return "password does not meet strength requirements"
}

// Any reports whether any rule was violated (i.e. the password is invalid).
func (e *PasswordStrengthError) Any() bool {
	return e.NeedsUpper || e.NeedsDigit || e.NeedsSpecial || e.TooShort
}

// ValidatePassword checks that the password:
//   - is at least 8 characters long
//   - contains at least one uppercase letter
//   - contains at least one digit
//   - contains at least one special character
//
// Returns a *PasswordStrengthError describing every rule that failed,
// or nil if the password is valid.
func ValidatePassword(password string) *PasswordStrengthError {
	result := &PasswordStrengthError{
		TooShort:     len([]rune(password)) < 8,
		NeedsUpper:   true,
		NeedsDigit:   true,
		NeedsSpecial: true,
	}

	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			result.NeedsUpper = false
		case unicode.IsDigit(ch):
			result.NeedsDigit = false
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			result.NeedsSpecial = false
		}
	}

	if !result.Any() {
		return nil
	}
	return result
}

func GenerateResetToken() (string, string, error) {
	rawToken := uuid.NewString()

	hash := sha256.Sum256([]byte(rawToken))
	hashedToken := hex.EncodeToString(hash[:])

	return rawToken, hashedToken, nil
}

func HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost, // cost = 10 (secure & fast enough)
	)
	if err != nil {
		return "", err
	}

	return string(hashedBytes), nil
}

func HashResetToken(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}