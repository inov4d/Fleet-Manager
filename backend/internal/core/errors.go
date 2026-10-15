package core

import "fmt"

// ErrorCode is the machine-readable error class exposed by the API.
type ErrorCode string

const (
	CodeNotFound     ErrorCode = "not_found"
	CodeValidation   ErrorCode = "validation"
	CodeConflict     ErrorCode = "conflict"
	CodeUnauthorized ErrorCode = "unauthorized"
	CodeForbidden    ErrorCode = "forbidden"
	CodeInternal     ErrorCode = "internal"
)

// Error is a domain error carrying an API error code.
type Error struct {
	Code ErrorCode
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func NotFound(what string, id string) error {
	return &Error{Code: CodeNotFound, Msg: fmt.Sprintf("%s %q not found", what, id)}
}
func Validation(format string, a ...any) error {
	return &Error{Code: CodeValidation, Msg: fmt.Sprintf(format, a...)}
}
func Conflict(format string, a ...any) error {
	return &Error{Code: CodeConflict, Msg: fmt.Sprintf(format, a...)}
}
func Forbidden(format string, a ...any) error {
	return &Error{Code: CodeForbidden, Msg: fmt.Sprintf(format, a...)}
}
func Unauthorized(msg string) error { return &Error{Code: CodeUnauthorized, Msg: msg} }
