package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// AppError represents an application error with HTTP context
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	StatusCode int    `json:"-"`
}

func (e *AppError) Error() string {
	return e.Message
}

// ErrorResponse is the JSON response format for errors
type ErrorResponse struct {
	Error *AppError `json:"error"`
}

// WriteJSON writes the error as JSON response
func (e *AppError) WriteJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.StatusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Error: e})
}

// ============================================================
// ERROR CONSTRUCTORS
// ============================================================

// Validation Errors (400)
func BadRequest(message string) *AppError {
	return &AppError{
		Code:       "BAD_REQUEST",
		Message:    message,
		StatusCode: http.StatusBadRequest,
	}
}

func InvalidURL(details string) *AppError {
	return &AppError{
		Code:       "INVALID_URL",
		Message:    "The provided URL is invalid",
		Details:    details,
		StatusCode: http.StatusBadRequest,
	}
}

func InvalidJSON(details string) *AppError {
	return &AppError{
		Code:       "INVALID_JSON",
		Message:    "Invalid JSON in request body",
		Details:    details,
		StatusCode: http.StatusBadRequest,
	}
}

func MissingField(field string) *AppError {
	return &AppError{
		Code:       "MISSING_FIELD",
		Message:    fmt.Sprintf("Required field '%s' is missing", field),
		StatusCode: http.StatusBadRequest,
	}
}

// Not Found Errors (404)
func NotFound(resource string) *AppError {
	return &AppError{
		Code:       "NOT_FOUND",
		Message:    fmt.Sprintf("%s not found", resource),
		StatusCode: http.StatusNotFound,
	}
}

func URLNotFound(code string) *AppError {
	return &AppError{
		Code:       "URL_NOT_FOUND",
		Message:    fmt.Sprintf("Short URL '%s' not found", code),
		StatusCode: http.StatusNotFound,
	}
}

// Conflict Errors (409)
func Conflict(message string) *AppError {
	return &AppError{
		Code:       "CONFLICT",
		Message:    message,
		StatusCode: http.StatusConflict,
	}
}

func URLExists(code string) *AppError {
	return &AppError{
		Code:       "URL_EXISTS",
		Message:    fmt.Sprintf("Short code '%s' already exists", code),
		StatusCode: http.StatusConflict,
	}
}

// Rate Limit Error (429)
func RateLimitExceeded() *AppError {
	return &AppError{
		Code:       "RATE_LIMIT_EXCEEDED",
		Message:    "Too many requests, please try again later",
		StatusCode: http.StatusTooManyRequests,
	}
}

// Server Errors (500)
func Internal(details string) *AppError {
	return &AppError{
		Code:       "INTERNAL_ERROR",
		Message:    "An internal server error occurred",
		Details:    details,
		StatusCode: http.StatusInternalServerError,
	}
}

func DatabaseError() *AppError {
	return &AppError{
		Code:       "DATABASE_ERROR",
		Message:    "A database error occurred",
		StatusCode: http.StatusInternalServerError,
	}
}
