package utils

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

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