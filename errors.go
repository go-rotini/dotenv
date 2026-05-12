package dotenv

import (
	"errors"
	"fmt"
)

// ParseError is returned by [Parse] / [Load] / [NewSource] when the
// `.env` source is malformed. It carries the file path (when available)
// and the line/column position of the failure.
type ParseError struct {
	Pos     Position
	Message string
}

func (e *ParseError) Error() string {
	if e == nil {
		return "dotenv: parse error"
	}
	if e.Pos.Line > 0 {
		if e.Pos.File != "" {
			return fmt.Sprintf("dotenv: %s:%d:%d: %s", e.Pos.File, e.Pos.Line, e.Pos.Column, e.Message)
		}
		return fmt.Sprintf("dotenv: line %d, column %d: %s", e.Pos.Line, e.Pos.Column, e.Message)
	}
	return fmt.Sprintf("dotenv: %s", e.Message)
}

func (*ParseError) Is(target error) bool {
	_, ok := target.(*ParseError)
	return ok
}

// ExpansionError is returned when variable expansion fails: an unset
// variable under [WithStrictExpansion], a `${VAR:?msg}` assertion
// failure, or the [WithMaxExpansionDepth] bound being exceeded by a
// deeply nested or self-referential `${VAR:-default}` / `${VAR:?msg}`
// construct. [ExpansionError.Cause] (and hence [errors.Is]) is
// [ErrUnsetVariable], [ErrAssertionFailed], or [ErrMaxExpansionDepth]
// respectively; every ExpansionError also matches [ErrExpansion].
type ExpansionError struct {
	Pos      Position
	Variable string
	Reason   string
	Cause    error
}

func (e *ExpansionError) Error() string {
	if e == nil {
		return "dotenv: expansion error"
	}
	switch {
	case e.Pos.Line > 0:
		return fmt.Sprintf("dotenv: line %d: expansion of %q: %s", e.Pos.Line, e.Variable, e.Reason)
	case e.Variable != "":
		return fmt.Sprintf("dotenv: expansion of %q: %s", e.Variable, e.Reason)
	default:
		return fmt.Sprintf("dotenv: expansion: %s", e.Reason)
	}
}

func (*ExpansionError) Is(target error) bool {
	_, ok := target.(*ExpansionError)
	return ok
}

func (e *ExpansionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Sentinel errors. The struct-pointer sentinels match by type via the
// corresponding Is method; the others match by value identity.
var (
	// ErrParse matches any [*ParseError].
	ErrParse = &ParseError{}

	// ErrExpansion matches any [*ExpansionError].
	ErrExpansion = &ExpansionError{}

	// ErrFileTooLarge is returned when an input exceeds [WithMaxFileSize].
	ErrFileTooLarge = errors.New("dotenv: file size exceeds limit")

	// ErrLineTooLong is returned when a single line exceeds [WithMaxLineLength].
	ErrLineTooLong = errors.New("dotenv: line length exceeds limit")

	// ErrUnsetVariable is the [ExpansionError.Cause] when expansion under
	// [WithStrictExpansion] references a variable that is not set.
	ErrUnsetVariable = errors.New("dotenv: variable referenced in expansion is unset")

	// ErrAssertionFailed is the [ExpansionError.Cause] when a `${VAR:?msg}`
	// assertion fails during expansion.
	ErrAssertionFailed = errors.New("dotenv: ${VAR:?...} assertion failed")

	// ErrMaxExpansionDepth is the [ExpansionError.Cause] when expansion
	// recursion exceeds the [WithMaxExpansionDepth] bound.
	ErrMaxExpansionDepth = errors.New("dotenv: variable-expansion depth limit exceeded")

	// ErrInvalidKey reports a key that is not a valid `.env` identifier (see
	// [IsValidKey]). [Marshal] / [Write] return an error wrapping it; [File.Set]
	// panics with an error wrapping it.
	ErrInvalidKey = errors.New("dotenv: invalid key")
)
