package dotenv

// ParseOption configures [Parse], [Load], [NewSource],
// [NewReaderSource], and [Lint].
type ParseOption func(*parseOptions)

type parseOptions struct {
	expand            bool
	strictExpansion   bool
	maxExpansionDepth int
	expansionParent   VarLookup
	backticks         bool
	strictDotenv      bool
	maxFileSize       int64
	maxLineLength     int
	relaxedNames      bool
	filePath          string
}

func defaultParseOptions() parseOptions {
	return parseOptions{
		maxExpansionDepth: 16,
		maxFileSize:       10 * 1024 * 1024, // 10 MiB
		maxLineLength:     1024 * 1024,      // 1 MiB
	}
}

// WithExpand enables Bash-style variable expansion in unquoted and
// double-quoted values: `$VAR`, `${VAR}`, `${VAR:-default}`,
// `${VAR-default}`, `${VAR:?error}`. Backslash-dollar (`\$`) suppresses
// expansion. Lookups walk the file's already-parsed entries first, then
// any parent set via [WithExpansionParent].
func WithExpand() ParseOption {
	return func(o *parseOptions) { o.expand = true }
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
// using only the standard library.
func WithExpandFromOSEnv() ParseOption {
	return func(o *parseOptions) {
		o.expand = true
		o.expansionParent = osEnvLookup{}
	}
}

// WithBackticks accepts backtick-quoted values (motdotla/dotenv dialect).
// Treated like single-quoted: literal, no escapes, no expansion. Default
// is to reject as a syntax error.
func WithBackticks() ParseOption {
	return func(o *parseOptions) { o.backticks = true }
}

// WithStrictDotenv rejects non-portable constructs: YAML-style
// `KEY: value` separators, the `export ` prefix, identifiers outside
// the POSIX class. Useful for asserting cross-dialect portability.
func WithStrictDotenv() ParseOption {
	return func(o *parseOptions) { o.strictDotenv = true }
}

// WithMaxFileSize caps the total input size. Default 10 MiB. Zero or
// negative means unlimited.
func WithMaxFileSize(n int64) ParseOption {
	return func(o *parseOptions) { o.maxFileSize = n }
}

// WithMaxLineLength caps a single physical line's length. Default 1 MiB.
// Zero or negative means unlimited. Multi-line double-quoted values
// count toward the line limit per physical line, not in aggregate.
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
// be interpolated via `${...}`; reference it through a parent ([WithExpansionParent])
// keyed on the hyphenated name, or avoid hyphens in keys you interpolate. (`$NAME`
// without braces only ever consumes POSIX identifier bytes, so `$MY-KEY`
// is `<value of MY>` followed by the literal `-KEY` regardless of this option.)
// Dots in names do not collide with any expansion operator.
func WithRelaxedNames() ParseOption {
	return func(o *parseOptions) { o.relaxedNames = true }
}
