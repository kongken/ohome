// Package httpx contains HTTP-layer helpers shared by all domain handlers:
// the standard error envelope, cursor pagination, and small request/response
// utilities. Aligned with `common.v1.Error` from proto/common/v1/types.proto.
package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Error codes used across handlers. Keep stable — clients depend on these.
const (
	CodeInvalidQuery     = "invalid_query"
	CodeInvalidBody      = "invalid_body"
	CodeUnauthorized     = "unauthorized"
	CodeForbidden        = "forbidden"
	CodeNotFound         = "not_found"
	CodeConflict         = "conflict"
	CodeUnprocessable    = "unprocessable"
	CodeRateLimited      = "rate_limited"
	CodeInternal         = "internal"
	CodeUpstreamError    = "upstream_error"
	CodeAuthInvalidCreds = "auth_invalid_credentials"
	CodeAuthTokenInvalid = "auth_token_invalid"
	CodeAuthTokenExpired = "auth_token_expired"
)

// APIError is the wire format for `{code, message, details}`.
type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// Err is a typed error that carries an HTTP status and APIError payload. Use
// it from the handler layer when you want a specific code/status pair.
type Err struct {
	Status  int
	Payload APIError
}

func (e *Err) Error() string { return e.Payload.Code + ": " + e.Payload.Message }

// New builds an Err with no details.
func New(status int, code, message string) *Err {
	return &Err{Status: status, Payload: APIError{Code: code, Message: message}}
}

// WithDetails returns a copy of e with extra detail fields merged in.
func (e *Err) WithDetails(d map[string]string) *Err {
	out := *e
	if out.Payload.Details == nil {
		out.Payload.Details = map[string]string{}
	}
	for k, v := range d {
		out.Payload.Details[k] = v
	}
	return &out
}

// Common shortcuts.
func BadQuery(msg string) *Err   { return New(http.StatusBadRequest, CodeInvalidQuery, msg) }
func BadBody(msg string) *Err    { return New(http.StatusBadRequest, CodeInvalidBody, msg) }
func Unauthorized(msg string) *Err {
	if msg == "" {
		msg = "authentication required"
	}
	return New(http.StatusUnauthorized, CodeUnauthorized, msg)
}
func Forbidden(msg string) *Err  { return New(http.StatusForbidden, CodeForbidden, msg) }
func NotFound(msg string) *Err   { return New(http.StatusNotFound, CodeNotFound, msg) }
func Conflict(msg string) *Err   { return New(http.StatusConflict, CodeConflict, msg) }
func Unprocessable(msg string) *Err {
	return New(http.StatusUnprocessableEntity, CodeUnprocessable, msg)
}
func Internal(msg string) *Err {
	if msg == "" {
		msg = "internal server error"
	}
	return New(http.StatusInternalServerError, CodeInternal, msg)
}

// Abort writes the error to the gin context and aborts. Accepts any error;
// `*Err` keeps its status code, anything else becomes 500.
func Abort(c *gin.Context, err error) {
	var e *Err
	if errors.As(err, &e) {
		c.AbortWithStatusJSON(e.Status, e.Payload)
		return
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, APIError{
		Code:    CodeInternal,
		Message: err.Error(),
	})
}
