package utils

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// RegisterCustomValidators registers all custom binding tags with gin's
// shared validator instance. Call this once at startup, before the first
// request is handled.
func RegisterCustomValidators() {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}

	// strong_password: at least 8 chars, one uppercase, one digit, one special char.
	v.RegisterValidation("strong_password", func(fl validator.FieldLevel) bool {
		return ValidatePassword(fl.Field().String()) == nil
	})
}

// FormatValidationErrors converts validator errors to user-friendly messages
func FormatValidationErrors(err error) interface{} {
	// Check if it's a validation error
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		// Not a validation error, return generic message
		return map[string]string{
			"message": "Invalid input data",
		}
	}

	// Create map of field -> error message
	errors := make(map[string]string)

	for _, e := range validationErrors {
		fieldName := strings.ToLower(e.Field())
		errors[fieldName] = getErrorMessage(e)
	}

	return errors
}

// getErrorMessage returns user-friendly error message for validation error
func getErrorMessage(e validator.FieldError) string {
	field := e.Field()

	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)

	case "email":
		return "Must be a valid email address"

	case "min":
		return fmt.Sprintf("%s must be at least %s characters", field, e.Param())

	case "max":
		return fmt.Sprintf("%s must not exceed %s characters", field, e.Param())

	case "strong_password":
		return "Password must be at least 8 characters and contain an uppercase letter, a number, and a special character"

	default:
		return fmt.Sprintf("%s is invalid", field)
	}
}