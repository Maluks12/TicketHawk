package models

import (
	"strings"
	"time"
)

type UserRole string

const (
	RoleAttendee UserRole = "attendee"
	RoleOrganizer UserRole = "organizer"
	RoleAdmin 	  UserRole = "admin"
)

type User struct {
	ID        string    `json:"id" db:"id"`
	Email     string    `json:"email" db:"email"`
	Password  string    `json:"-" db:"password"`
	FirstName *string	`json:"first_name,omitempty" db:"first_name"`
	LastName  *string	`json:"last_name,omitempty" db:"last_name"`
	PhoneNumber	*string `json:"phone_number" db:"phone_number"`
	DateOfBirth *time.Time `json:"date_of_birth,omitempty" db:"date_of_birth"`
	Role      UserRole    `json:"role" db:"role"`
	EmailVerified bool 	`json:"email_verified" db:"email_verified"`
	EmailVerificationToken *string `json:"-" db:"email_verification_token"`
	EmailVerificationExpires *time.Time `json:"-" db:"email_verification_expires"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type UserProfile struct {
	ID string `json:"id"`
	FirstName string `json:"first_name"`
	LastName string `json:"last_name"`
	Role UserRole `json:"role"`
}

type RegisterUserDTO struct {
	Email string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=8,max=128"`
	FirstName string `json:"first_name" binding:"required,min=1,max=100"`
	LastName string `json:"last_name" binding:"required,min=1,max=100"`
	PhoneNumber string `json:"phone_number" binding:"required,min=10,max=20"`
}

type UpdateUserDTO struct {
	FirstName *string `json:"first_name" binding:"required,min=1,max=100"`
	LastName *string `json:"last_name" binding:"required,min=1,max=100"`
	PhoneNumber *string `json:"phone_number" binding:"required,min=10,max=20"`
}

func (u *User)GetFullName() string {
	if u.FirstName != nil && u.LastName != nil {
		return strings.TrimSpace(*u.FirstName + " " + *u.LastName)
	}
	return u.Email
}

// UserIdentity is a linked social sign-in provider, surfaced on the profile
// screen so a user can see (and remove) what is connected to their account.
type UserIdentity struct {
	Provider    string     `json:"provider"`
	Email       *string    `json:"email,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// SocialSignInDTO is the request body for POST /v1/auth/social.
//
// The client does the interactive OAuth dance and sends the resulting ID token.
// There is no client_secret and no authorization code on this path — the server
// only ever verifies a signed assertion.
type SocialSignInDTO struct {
	Provider string `json:"provider" binding:"required,oneof=google apple"`
	IDToken  string `json:"id_token" binding:"required,max=8192"`
	// Nonce the client generated before starting the flow. Replay defence.
	Nonce string `json:"nonce" binding:"max=256"`
	// Apple sends the display name ONCE, in the authorization response rather
	// than in the ID token, and never again. The client forwards it on first
	// sign-in; it is treated as a hint, never as identity.
	FirstName string `json:"first_name" binding:"max=100"`
	LastName  string `json:"last_name"  binding:"max=100"`
}