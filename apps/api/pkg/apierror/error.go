package apierror

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Stable API error codes (master control-plane contract).
const (
	CodeValidationError    = "VALIDATION_ERROR"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeResourceNotFound   = "RESOURCE_NOT_FOUND"
	CodeConflict           = "CONFLICT"
	CodeInternalError      = "INTERNAL_ERROR"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeRateLimited        = "RATE_LIMITED"

	CodeOrganizationNotFound  = "ORGANIZATION_NOT_FOUND"
	CodeProjectNotFound       = "PROJECT_NOT_FOUND"
	CodeEnvironmentNotFound   = "ENVIRONMENT_NOT_FOUND"
	CodeServerNotFound        = "SERVER_NOT_FOUND"
	CodeServerOffline         = "SERVER_OFFLINE"
	CodeAgentUnavailable      = "AGENT_UNAVAILABLE"
	CodeApplicationNotFound   = "APPLICATION_NOT_FOUND"
	CodeDeploymentNotFound    = "DEPLOYMENT_NOT_FOUND"
	CodeRevisionNotFound      = "REVISION_NOT_FOUND"
	CodeRevisionNotReady      = "REVISION_NOT_READY"
	CodeDomainAlreadyAssigned = "DOMAIN_ALREADY_ASSIGNED"
	CodeSecretNotFound        = "SECRET_NOT_FOUND"
	CodeInsufficientResources = "INSUFFICIENT_RESOURCES"
	CodeBuildTimeout          = "BUILD_TIMEOUT"
	CodeHealthCheckFailed     = "HEALTH_CHECK_FAILED"
	CodeDatabaseNotFound      = "DATABASE_NOT_FOUND"
	CodeVolumeNotFound        = "VOLUME_NOT_FOUND"
	CodeVolumeInUse           = "VOLUME_IN_USE"
)

// Error is the standard API error envelope body.
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"requestId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type envelope struct {
	Error Error `json:"error"`
}

// APIError is returned by handlers/services for mapped HTTP responses.
type APIError struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
}

func (e *APIError) Error() string {
	return e.Message
}

func New(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

func Validation(message string, details map[string]any) *APIError {
	return &APIError{
		Status:  http.StatusBadRequest,
		Code:    CodeValidationError,
		Message: message,
		Details: details,
	}
}

func NotFound(message string) *APIError {
	return New(http.StatusNotFound, CodeResourceNotFound, message)
}

func NotFoundCode(code, message string) *APIError {
	return New(http.StatusNotFound, code, message)
}

func Unauthorized(message string) *APIError {
	return New(http.StatusUnauthorized, CodeUnauthorized, message)
}

func Forbidden(message string) *APIError {
	return New(http.StatusForbidden, CodeForbidden, message)
}

func Conflict(message string) *APIError {
	return New(http.StatusConflict, CodeConflict, message)
}

func ConflictCode(code, message string) *APIError {
	return New(http.StatusConflict, code, message)
}

func InsufficientResources(message string, details map[string]any) *APIError {
	return &APIError{
		Status:  http.StatusConflict,
		Code:    CodeInsufficientResources,
		Message: message,
		Details: details,
	}
}

func Internal(message string) *APIError {
	return New(http.StatusInternalServerError, CodeInternalError, message)
}

func Unavailable(message string) *APIError {
	return New(http.StatusServiceUnavailable, CodeServiceUnavailable, message)
}

// AsAPIError extracts an *APIError from err, if present.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

// WriteJSON writes the standard error envelope. Never includes stack traces or SQL.
func WriteJSON(w http.ResponseWriter, requestID string, err *APIError) {
	status := err.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Error: Error{
			Code:      err.Code,
			Message:   err.Message,
			RequestID: requestID,
			Details:   err.Details,
		},
	})
}

// WriteError maps known API errors; unknown errors become INTERNAL_ERROR without leaking internals.
func WriteError(w http.ResponseWriter, requestID string, err error) {
	if apiErr, ok := AsAPIError(err); ok {
		WriteJSON(w, requestID, apiErr)
		return
	}
	WriteJSON(w, requestID, Internal("internal server error"))
}
