package dotenv

import (
	"bytes"
	"strings"
)

// Marshal returns the file's source representation. For unmutated
// inputs the result is byte-exact with the original; entries mutated
// or added via [File.Set] are freshly rendered with default quoting
// while surrounding entries retain their original bytes.
//
// Variable expansion performed at parse time (via [WithExpand] /
// [WithExpandFromOSEnv]) is NOT reflected here — Marshal emits the
// original source text; only [File.Set] re-renders a line. Use
// [File.ToMap] / [File.Get] for the expanded values.
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
