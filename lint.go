package dotenv

import (
	"errors"
	"fmt"
)

// Severity classifies a [LintIssue].
type Severity int

const (
	// SeverityError marks a [LintIssue] that prevented parsing.
	SeverityError Severity = iota

	// SeverityWarning marks a parseable but non-portable construct
	// (YAML-style `:`, `export` prefix, backtick quoting, hyphens or
	// dots in identifiers).
	SeverityWarning
)

// String returns "error", "warning", or "unknown".
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	}
	return "unknown"
}

// LintIssue is one finding from [Lint].
type LintIssue struct {
	// Pos is the start of the offending line, or the failure's position
	// for a "syntax" or "expansion" issue (zero when unknown).
	Pos Position

	// Rule names the check: "export-prefix", "yaml-separator",
	// "non-posix-identifier", "backtick-value", "syntax" for a parse
	// failure, or "expansion" for a [WithExpand] failure.
	Rule string

	// Message is a human-readable description.
	Message string

	// Severity is [SeverityError] for "syntax" and "expansion",
	// [SeverityWarning] otherwise.
	Severity Severity
}

// Lint reports constructs in data that fall outside the portable
// `.env` subset.
//
// Warnings ([SeverityWarning] entries) come from the parser's entries,
// one set per assignment at the assignment's start, so text inside a
// quoted value is never mistaken for a key. When parsing fails, the
// warnings cover the entries before the failure, and the failure is
// reported both as a [SeverityError] entry and as the returned error.
func Lint(data []byte, opts ...ParseOption) ([]LintIssue, error) {
	o := defaultParseOptions()
	for _, opt := range opts {
		opt(&o)
	}

	var issues []LintIssue
	collect := func(rule, msg string, pos Position, sev Severity) {
		issues = append(issues, LintIssue{
			Pos:      pos,
			Rule:     rule,
			Message:  msg,
			Severity: sev,
		})
	}

	entries, err := scanEntries(data, o)
	for _, e := range entries {
		lintEntry(e, collect)
	}
	if err == nil && o.expand != nil {
		f := &File{entries: entries}
		err = o.expand(f, o)
	}

	if err != nil {
		var pe *ParseError
		var ee *ExpansionError
		switch {
		case errors.As(err, &pe):
			collect("syntax", pe.Message, pe.Pos, SeverityError)
		case errors.As(err, &ee):
			msg := ee.Reason
			if ee.Variable != "" {
				msg = fmt.Sprintf("%q: %s", ee.Variable, ee.Reason)
			}
			collect("expansion", msg, ee.Pos, SeverityError)
		default:
			collect("syntax", err.Error(), Position{}, SeverityError)
		}
		return issues, err
	}
	return issues, nil
}

// lintEntry reports the non-portable constructs of one assignment.
func lintEntry(e Entry, collect func(rule, msg string, pos Position, sev Severity)) {
	if e.Kind != EntryAssignment {
		return
	}
	if e.exported {
		collect("export-prefix", "`export ` prefix is not portable to motdotla/dotenv",
			e.Pos, SeverityWarning)
	}
	if e.sep == ':' {
		collect("yaml-separator",
			"`KEY: VALUE` form is not portable; use `KEY=VALUE`",
			e.Pos, SeverityWarning)
	}
	for i := range len(e.Key) {
		if !isIdentifierByte(e.Key[i], false) {
			collect("non-posix-identifier",
				"identifier \""+e.Key+"\" contains non-POSIX character",
				e.Pos, SeverityWarning)
			break
		}
	}
	if e.quote == '`' {
		collect("backtick-value",
			"backtick-quoted value is not portable; use double quotes",
			e.Pos, SeverityWarning)
	}
}
