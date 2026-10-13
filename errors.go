package dotenv

import (
	"errors"
	"fmt"
)

// ParseError reports malformed `.env` input. Pos locates the failure;
// Pos.File is set when the input was read from a path ([Load], [NewSource]).
// Cause is [ErrFileTooLarge] or [ErrLineTooLong] when a size limit was
// exceeded, and nil otherwise.
type ParseError struct {
	Pos     Position
	Message string
	Cause   error
}

// Error formats the error with its position when one is known.
func (e *ParseError) Error() string {
	if e == nil {
		return "dotenv: parse error"
	}
	switch {
	case e.Pos.Line > 0 && e.Pos.File != "":
		return fmt.Sprintf("dotenv: %s:%d:%d: %s", e.Pos.File, e.Pos.Line, e.Pos.Column, e.Message)
	case e.Pos.Line > 0:
		return fmt.Sprintf("dotenv: line %d, column %d: %s", e.Pos.Line, e.Pos.Column, e.Message)
	case e.Pos.File != "":
		return fmt.Sprintf("dotenv: %s: %s", e.Pos.File, e.Message)
	}
	return fmt.Sprintf("dotenv: %s", e.Message)
}

// Unwrap returns e.Cause.
func (e *ParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is reports whether target is a [*ParseError], so [errors.Is] with
// [ErrParse] matches any ParseError.
func (*ParseError) Is(target error) bool {
	_, ok := target.(*ParseError)
	return ok
}

// ExpansionError is returned when variable expansion fails: an unset
// variable under [WithStrictExpansion], a `${VAR:?msg}` assertion
// failure, the [WithMaxExpansionDepth] bound being exceeded by a deeply
// nested or self-referential `${VAR:-default}` / `${VAR:?msg}`
// construct, a `${...}` body that is not a variable reference, or the
// [WithMaxExpandedSize] budget being used up. [ExpansionError.Cause]
// (and hence [errors.Is]) is [ErrUnsetVariable], [ErrAssertionFailed],
// [ErrMaxExpansionDepth], [ErrBadSubstitution], or
// [ErrExpansionTooLarge] respectively; every ExpansionError also
// matches [ErrExpansion].
//
// Pos is the position of the `$` that starts the failing reference.
// Variable is the referenced name; it is empty for [ErrBadSubstitution],
// whose `${...}` body is not a name and is left out of the error.
type ExpansionError struct {
	Pos      Position
	Variable string
	Reason   string
	Cause    error
}

// Error formats the error with its position and variable name when known;
// like [ParseError.Error], the position is file:line:column when Pos.File
// is set.
func (e *ExpansionError) Error() string {
	if e == nil {
		return "dotenv: expansion error"
	}
	what := "expansion"
	if e.Variable != "" {
		what = fmt.Sprintf("expansion of %q", e.Variable)
	}
	switch {
	case e.Pos.Line > 0 && e.Pos.File != "":
		return fmt.Sprintf("dotenv: %s:%d:%d: %s: %s", e.Pos.File, e.Pos.Line, e.Pos.Column, what, e.Reason)
	case e.Pos.Line > 0:
		return fmt.Sprintf("dotenv: line %d, column %d: %s: %s", e.Pos.Line, e.Pos.Column, what, e.Reason)
	default:
		return fmt.Sprintf("dotenv: %s: %s", what, e.Reason)
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
// corresponding Is method; the others match by value identity. Never pass a
// struct-pointer sentinel's address to [errors.As] (as in &ErrParse): that
// overwrites the shared sentinel. Declare a variable of the error type for
// errors.As instead.
var (
	// ErrParse matches any [*ParseError].
	ErrParse = &ParseError{}

	// ErrExpansion matches any [*ExpansionError].
	ErrExpansion = &ExpansionError{}

	// ErrFileTooLarge is the [ParseError.Cause] when an input exceeds
	// [WithMaxFileSize].
	ErrFileTooLarge = errors.New("dotenv: file size exceeds limit")

	// ErrLineTooLong is the [ParseError.Cause] when a single entry exceeds
	// [WithMaxLineLength].
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

	// ErrBadSubstitution is the [ExpansionError.Cause] when a `${...}`
	// body is not a variable name with an optional supported operator,
	// such as `${}` or `${A B}`.
	ErrBadSubstitution = errors.New("dotenv: bad substitution")

	// ErrExpansionTooLarge is the [ExpansionError.Cause] when expansion
	// would substitute more bytes than [WithMaxExpandedSize] allows.
	ErrExpansionTooLarge = errors.New("dotenv: expanded values exceed limit")

	// ErrInvalidKey reports a key that is not a valid `.env` identifier (see
	// [IsValidKey]). [Marshal] / [Write] return an error wrapping it; [File.Set]
	// panics with an error wrapping it.
	ErrInvalidKey = errors.New("dotenv: invalid key")
)
