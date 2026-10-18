package dotenv

// ParseOption configures [Parse], [Load], [NewSource],
// [NewReaderSource], and [Lint].
//
// By default inputs are limited to 10 MiB ([WithMaxFileSize]) and entries
// to 1 MiB ([WithMaxLineLength]); exceeding either returns a [*ParseError]
// wrapping [ErrFileTooLarge] or [ErrLineTooLong]. Readers and files are
// read only up to the limit. Expansion may substitute at most as many
// bytes as the file-size limit ([WithMaxExpandedSize]).
type ParseOption func(*parseOptions)

type parseOptions struct {
	expand            func(*File, parseOptions) error // (*File).expandAll, set by WithExpand and WithExpandFromOSEnv, so a parse without them doesn't link expansion
	strictExpansion   bool
	maxExpansionDepth int
	expansionParent   VarLookup
	backticks         bool
	strictDotenv      bool
	maxFileSize       int64
	maxExpandedSize   int64
	expandedSizeSet   bool
	maxLineLength     int
	relaxedNames      bool
	filePath          string
}

// defaultMaxSize is the default input size and expansion budget (10 MiB).
const defaultMaxSize = 10 * 1024 * 1024

func defaultParseOptions() parseOptions {
	return parseOptions{
		maxExpansionDepth: 16,
		maxFileSize:       defaultMaxSize,
		maxLineLength:     1024 * 1024, // 1 MiB
	}
}

// WithExpand enables Bash-style variable expansion in unquoted and
// double-quoted values: `$VAR`, `${VAR}`, `${VAR:-word}`, `${VAR-word}`
// (default), `${VAR:?word}`, `${VAR?word}` (error), and `${VAR:+word}`,
// `${VAR+word}` (alternative). Any other `${...}` body fails with
// [ErrBadSubstitution]. Backslash-dollar (`\$`) suppresses expansion.
//
// In unquoted values a backslash is literal, except that `\$` is a
// literal `$` and, inside `${...}`, a backslash hides the next byte from
// brace matching, so `${A:-C:\Temp\}` is left unexpanded. In
// double-quoted values the escapes are decoded first, and a backslash
// written as `\\` is plain text: `"${A:-C:\\Temp\\}"` gives `C:\Temp\`.
// Every `{` inside `${...}` opens a level that needs its own `}`, so
// `${A:-{}` is left unexpanded.
//
// Single-quoted and backtick-quoted values are not expanded but can be
// referenced. Lookups walk the file's already-parsed entries first, then
// any parent set via [WithExpansionParent].
func WithExpand() ParseOption {
	return func(o *parseOptions) { o.expand = (*File).expandAll }
}

// WithStrictExpansion makes a reference to an unset variable raise an
// [*ExpansionError] (cause [ErrUnsetVariable]) rather than expand to empty.
// Has no effect unless [WithExpand] (or [WithExpandFromOSEnv]) is also set.
func WithStrictExpansion() ParseOption {
	return func(o *parseOptions) { o.strictExpansion = true }
}

// WithMaxExpansionDepth bounds variable-expansion recursion. Default 16; a
// non-positive value keeps the default. The bound is reached by deeply
// nested or self-referential `${VAR:-default}` / `${VAR:?msg}` constructs
// (e.g. `${A:-${A:-${A}}}`); exceeding it yields an [*ExpansionError] whose
// cause is [ErrMaxExpansionDepth]. Plain references like `${A}` (and even
// `A=${A}`) are resolved at most once against already-resolved entries and
// never recurse.
func WithMaxExpansionDepth(n int) ParseOption {
	return func(o *parseOptions) {
		if n > 0 {
			o.maxExpansionDepth = n
		}
	}
}

// WithExpansionParent sets the [VarLookup] consulted by variable expansion
// when a referenced variable is not present in the file's own entries. It
// has no effect unless [WithExpand] (or [WithExpandFromOSEnv]) is also set.
// The file's entries take precedence, as when the file is sourced by a
// shell: with `A=file` in the file and A=os in the parent, `$A` expands to
// "file" even where a loader gives the parent precedence for A itself.
//
// Any github.com/go-rotini/env Source satisfies [VarLookup], so passing
// env.OSEnv() or env.Map(...) works directly. For the common
// "fall back to the process environment" case, [WithExpandFromOSEnv] is
// shorter and pulls in no dependency.
func WithExpansionParent(parent VarLookup) ParseOption {
	return func(o *parseOptions) { o.expansionParent = parent }
}

// WithExpandFromOSEnv enables variable expansion (like [WithExpand]) and
// resolves references not defined in the file against the process
// environment via [os.LookupEnv]. It is equivalent to
// `WithExpand()` plus `WithExpansionParent(<an os.LookupEnv-backed VarLookup>)`,
// using only the standard library. As with [WithExpansionParent], the
// file's own entries take precedence over the environment.
func WithExpandFromOSEnv() ParseOption {
	return func(o *parseOptions) {
		o.expand = (*File).expandAll
		o.expansionParent = osEnvLookup{}
	}
}

// WithBackticks accepts backtick-quoted values (motdotla/dotenv dialect).
// Treated like single-quoted: literal, no escapes, no expansion. Default
// is to reject as a syntax error.
func WithBackticks() ParseOption {
	return func(o *parseOptions) { o.backticks = true }
}

// WithStrictDotenv rejects the `export ` prefix and YAML-style `KEY: value`
// separators, which otherwise parse. It does not override [WithRelaxedNames]
// or [WithBackticks]: without those options, non-POSIX key names and
// backtick-quoted values are already rejected.
func WithStrictDotenv() ParseOption {
	return func(o *parseOptions) { o.strictDotenv = true }
}

// WithMaxFileSize caps the total input size. Default 10 MiB. Zero,
// negative, or [math.MaxInt64] means unlimited. A reader or file is read
// only up to the cap.
func WithMaxFileSize(n int64) ParseOption {
	return func(o *parseOptions) { o.maxFileSize = n }
}

// WithMaxExpandedSize caps the total bytes variable expansion may
// substitute across a file, so a short file of nested references cannot
// expand to gigabytes. Exceeding it yields an [*ExpansionError] whose
// cause is [ErrExpansionTooLarge]. Default: the [WithMaxFileSize] limit,
// or 10 MiB when that is unlimited. Zero or negative means unlimited.
func WithMaxExpandedSize(n int64) ParseOption {
	return func(o *parseOptions) {
		o.maxExpandedSize = n
		o.expandedSizeSet = true
	}
}

// expandedSizeLimit is the effective [WithMaxExpandedSize] cap; zero
// means unlimited.
func (o parseOptions) expandedSizeLimit() int64 {
	switch {
	case o.expandedSizeSet:
		return max(o.maxExpandedSize, 0)
	case o.maxFileSize > 0:
		return o.maxFileSize
	}
	return defaultMaxSize
}

// WithMaxLineLength caps the length in bytes of a single entry, not
// counting its line ending. Default 1 MiB. Zero or negative means
// unlimited. An assignment whose quoted value spans several lines is
// measured as a whole, inner line endings included, not per physical line.
func WithMaxLineLength(n int) ParseOption {
	return func(o *parseOptions) { o.maxLineLength = n }
}

// WithRelaxedNames accepts identifiers containing characters outside the
// POSIX [A-Za-z_][A-Za-z0-9_]* class (hyphens, dots, non-ASCII).
//
// Interaction with [WithExpand]: a `${...}` reference is always parsed with
// the Bash `${VAR-default}` / `${VAR:-default}` rules, so a hyphen inside the
// braces is treated as the default-value operator, not as part of the name —
// `${MY-KEY}` means "value of MY, or the literal `KEY` if MY is unset", even
// if a key literally named `MY-KEY` exists. A hyphenated key therefore cannot
// be referenced in an expanded value: rename it, or read it from the parsed
// [File] after parsing. (`$NAME` without braces only ever consumes POSIX
// identifier bytes, so `$MY-KEY` is `<value of MY>` followed by the literal
// `-KEY` regardless of this option.)
// Dots in names do not collide with any expansion operator.
func WithRelaxedNames() ParseOption {
	return func(o *parseOptions) { o.relaxedNames = true }
}
