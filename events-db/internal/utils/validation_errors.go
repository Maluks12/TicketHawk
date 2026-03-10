package utils

import (
    "fmt"
    "strings"
    
    "github.com/go-playground/validator/v10"
)

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
    
    default:
        return fmt.Sprintf("%s is invalid", field)
    }
}