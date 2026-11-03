// Package clierr defines typed CLI errors and their process exit codes.
package clierr

import (
	"errors"
	"fmt"
	"strings"
)

// Code is the machine-readable error category.
type Code string

const (
	CodeError          Code = "error"
	CodeServer         Code = "server_error"
	CodeUsage          Code = "usage"
	CodeAuth           Code = "auth"
	CodeNotFound       Code = "not_found"
	CodeValidation     Code = "validation"
	CodeQuota          Code = "quota"
	CodeConflict       Code = "conflict"
	CodeConfirmation   Code = "confirmation_required"
	CodeReadOnly       Code = "read_only"
	CodeNetwork        Code = "network"
	CodePartial        Code = "partial"
	CodeOutcomeUnknown Code = "outcome_unknown"
)

// Error is a user-facing error with a stable code and exit status.
type Error struct {
	Code          Code
	Message, Hint string
	HTTPStatus    int
	CorrelationID string
	Cause         error
}

// New creates an Error.
func New(code Code, msg, hint string) *Error {
	return &Error{Code: code, Message: msg, Hint: hint}
}

// Error implements error.
func (e *Error) Error() string { return e.Message }

// Unwrap exposes the underlying cause.
func (e *Error) Unwrap() error { return e.Cause }

// Exit returns the process exit code for the error.
func (e *Error) Exit() int {
	switch e.Code {
	case CodeUsage:
		return 2
	case CodeAuth:
		return 3
	case CodeNotFound:
		return 4
	case CodeValidation:
		return 5
	case CodeQuota:
		return 6
	case CodeConflict:
		return 7
	case CodeConfirmation:
		return 8
	case CodeReadOnly:
		return 9
	case CodeNetwork:
		return 10
	case CodePartial:
		return 11
	case CodeOutcomeUnknown:
		return 12
	default: // CodeError, CodeServer and unknown codes
		return 1
	}
}

// cobraUsagePrefixes are the argument-validation messages produced by cobra
// itself (cobra.NoArgs, ExactArgs, MarkFlagRequired, ...).
var cobraUsagePrefixes = []string{
	"unknown command ", "unknown flag: ", "unknown shorthand flag: ",
	"flag needs an argument", "invalid argument ", "accepts ", "requires ",
	"required flag(s) ",
}

// From converts any error into an *Error. Typed errors are returned as is,
// cobra argument errors become usage errors, everything else CodeError.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var ce *Error
	if errors.As(err, &ce) {
		return ce
	}
	msg := err.Error()
	for _, p := range cobraUsagePrefixes {
		if strings.HasPrefix(msg, p) {
			return &Error{Code: CodeUsage, Message: msg, Cause: err}
		}
	}
	return &Error{Code: CodeError, Message: msg, Cause: err}
}

// Format renders the error as text for stderr.
func (e *Error) Format() string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Fehler: %s\n", e.Message)
	if e.Hint != "" {
		_, _ = fmt.Fprintf(&b, "Hinweis: %s\n", e.Hint)
	}
	if e.CorrelationID != "" {
		_, _ = fmt.Fprintf(&b, "Correlation-ID: %s\n", e.CorrelationID)
	}
	return b.String()
}
