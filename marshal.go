package dotenv

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"
)

// Marshal returns the file's source representation. For unmutated
// inputs the result is byte-exact with the original, apart from a leading
// UTF-8 byte order mark, which is dropped. Entries mutated or added via
// [File.Set] are freshly rendered with default quoting while surrounding
// entries retain their original bytes. The error is always nil.
//
// Variable expansion is not reflected: Marshal emits the original source
// text. Use [File.ToMap] / [File.Get] for the expanded values.
func (f *File) Marshal() ([]byte, error) {
	if f == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	for _, e := range f.entries {
		buf.Write(e.raw)
	}
	return buf.Bytes(), nil
}

// Marshal renders m as a `.env`-format byte stream: one `KEY=VALUE` line per
// entry, keys in sorted (deterministic) order, LF line endings, values
// double-quoted only when a bare value would not round-trip. The output
// re-parses to an equivalent map via [Parse] / [Load].
//
// Marshal returns an error wrapping [ErrInvalidKey] if any key is not a valid
// `.env` key (see [IsValidKey]). Keys outside the POSIX subset re-parse only
// under [WithRelaxedNames]. To turn a parsed [*File] back into bytes
// (preserving comments and formatting), use [File.Marshal] instead.
func Marshal(m map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var buf bytes.Buffer
	for _, k := range keys {
		if !IsValidKey(k) {
			return nil, fmt.Errorf("dotenv: marshal: key %q: %w", k, ErrInvalidKey)
		}
		buf.Write(renderAssignmentLine(k, m[k], "\n"))
	}
	return buf.Bytes(), nil
}

// Write renders m with [Marshal] and writes it to path with 0o600
// permissions (creating the file, or truncating an existing one without
// changing its mode). Write is not atomic; for crash-safe replacement, use
// [Marshal] and write the bytes through your preferred mechanism (for
// example, github.com/go-rotini/fs's atomic WriteFile).
func Write(path string, m map[string]string) error {
	data, err := Marshal(m)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("dotenv: write %s: %w", path, err)
	}
	return nil
}

// renderAssignmentLine produces source bytes for a fresh assignment:
// bare value when safe, double-quoted otherwise. The line is
// terminated by ending (typically "\n").
func renderAssignmentLine(key, value, ending string) []byte {
	var b bytes.Buffer
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(formatValue(value))
	b.WriteString(ending)
	return b.Bytes()
}

func formatValue(value string) string {
	if !needsQuoting(value) {
		return value
	}
	return quoteValue(value)
}

// needsQuoting reports whether value must be double-quoted. Any byte
// that would alter line tokenization (whitespace, `#`, `$`, quotes,
// backslash, backtick) or any control byte forces quoting. Iteration is
// byte-wise — not rune-wise — so values containing invalid UTF-8 are
// classified by their actual bytes (and round-trip unchanged).
func needsQuoting(value string) bool {
	if value == "" {
		return false
	}
	for i := range len(value) {
		switch c := value[i]; c {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			return true
		case '#', '$', '"', '\'', '\\', '`':
			return true
		default:
			if c < 0x20 || c == 0x7F {
				return true
			}
		}
	}
	return false
}

// quoteValue double-quotes value with the standard escape table.
// Output is parseable by this package and by joho/godotenv. Iteration is
// byte-wise so every byte — including bytes that are part of invalid
// UTF-8 — is emitted verbatim (or escaped) rather than replaced.
func quoteValue(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for i := range len(value) {
		switch c := value[i]; c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '$':
			b.WriteString(`\$`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
