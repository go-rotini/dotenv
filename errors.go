package dotenv

import (
	"errors"
	"fmt"
)

// ParseError reports malformed `.env` input. Pos locates the failure;
// Pos.File is set when the input was read from a path ([Load], [NewSource]).
type ParseError struct {
	Pos     Position
	Message string
}

// Error formats the error with its position when one is known.
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

// Is reports whether target is a [*ParseError], so [errors.Is] with
// [ErrParse] matches any ParseError.
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

// Error formats the error with the line and variable name when known.
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

// Is reports whether target is an [*ExpansionError], so [errors.Is] with
// [ErrExpansion] matches any ExpansionError.
func (*ExpansionError) Is(target error) bool {
	_, ok := target.(*ExpansionError)
	return ok
}

// Unwrap returns e.Cause.
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

	// ErrLineTooLong is returned when a single entry exceeds [WithMaxLineLength].
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
