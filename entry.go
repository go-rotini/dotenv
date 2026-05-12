package dotenv

import (
	"fmt"
	"slices"
)

// EntryKind classifies one logical line in a `.env` file.
type EntryKind int

const (
	// EntryBlank is a whitespace-only line.
	EntryBlank EntryKind = iota

	// EntryComment is a line whose first non-whitespace character is
	// `#`. Trailing comments on assignment lines are stored on the
	// assignment, not as separate EntryComment entries.
	EntryComment

	// EntryAssignment is a `KEY=VALUE` (or `KEY: VALUE`) line.
	EntryAssignment
)

// Entry is one logical line in a parsed `.env` file. Most callers use
// [File.Get] / [File.Set] / [File.Delete] / [File.Keys] instead of
// reading the entry list directly.
type Entry struct {
	Kind EntryKind

	// Key is the assignment key (without the `export ` prefix), empty
	// for non-assignment entries.
	Key string

	// Value is the decoded value: escapes interpreted, quoting
	// stripped, and (with [WithExpand]) variable references resolved.
	// Empty for non-assignment entries.
	Value string

	// Pos is the source position of the entry's first byte.
	Pos Position

	// quote is the value's source quoting form (0, '"', '\'', or '`')
	// — internal to the parse → expand pipeline.
	quote byte

	// raw is the verbatim source text including the line ending.
	// Marshal emits this slice unchanged for unmutated entries.
	raw []byte
}

// File is the AST for a parsed `.env` source. Entry order and per-
// entry source bytes are preserved so [File.Marshal] is byte-exact on
// unmutated inputs.
//
// A File returned by [Parse] / [Load] / [NewSource] is safe for
// concurrent reads ([File.Get], [File.Keys], [File.ToMap], [File.Entries],
// [File.Marshal]), but the mutating methods ([File.Set], [File.Delete])
// are not — guard a File you intend to mutate from concurrent goroutines.
type File struct {
	entries []Entry
	path    string
}

// Path returns the file path the AST was loaded from, or "" for an
// in-memory [Parse].
func (f *File) Path() string {
	if f == nil {
		return ""
	}
	return f.path
}

// Entries returns the entry list. The slice is not for caller
// mutation; use [File.Set] / [File.Delete].
func (f *File) Entries() []Entry {
	if f == nil {
		return nil
	}
	return f.entries
}

// Get returns the value associated with key. When the key appears in
// multiple assignments the LAST occurrence wins (matching `source
// <file>` in Bash).
func (f *File) Get(key string) (value string, ok bool) {
	if f == nil {
		return "", false
	}
	for _, e := range slices.Backward(f.entries) {
		if e.Kind == EntryAssignment && e.Key == key {
			return e.Value, true
		}
	}
	return "", false
}

// Set assigns value to key. An existing assignment is updated in place
// (re-rendered with sensible-default quoting, keeping the line's original
// line ending); otherwise a new assignment is appended on its own line.
// Surrounding entries keep their original bytes, so round-trip is
// byte-exact except for the mutated/added line.
//
// Set panics if key is not a valid `.env` key (see [IsValidKey]).
func (f *File) Set(key, value string) {
	if f == nil {
		return
	}
	if !IsValidKey(key) {
		panic(fmt.Sprintf("dotenv: Set: invalid key %q", key))
	}
	for i := range f.entries {
		if f.entries[i].Kind == EntryAssignment && f.entries[i].Key == key {
			f.entries[i].Value = value
			f.entries[i].raw = renderAssignmentLine(key, value, trailingLineEnding(f.entries[i].raw))
			return
		}
	}
	// Appending: terminate the current last line first so the new entry
	// doesn't get concatenated onto it (sources without a trailing newline).
	if n := len(f.entries); n > 0 {
		if last := &f.entries[n-1]; len(last.raw) > 0 && trailingLineEnding(last.raw) == "" {
			last.raw = append(last.raw, '\n')
		}
	}
	f.entries = append(f.entries, Entry{
		Kind:  EntryAssignment,
		Key:   key,
		Value: value,
		raw:   renderAssignmentLine(key, value, "\n"),
	})
}

// IsValidKey reports whether key is a syntactically valid `.env` key — one
// that round-trips through [Parse] when [WithRelaxedNames] is in effect. A
// valid key is non-empty, does not begin with a digit, and otherwise
// contains only identifier bytes: ASCII letters, digits and '_', plus —
// beyond the portable POSIX subset [A-Za-z_][A-Za-z0-9_]* — '-', '.', and
// non-ASCII bytes. [(*File).Set] panics on an invalid key. (Keys outside the
// POSIX subset only re-parse under [WithRelaxedNames]; the POSIX subset is
// always safe.)
func IsValidKey(key string) bool {
	if key == "" || !isIdentifierStart(key[0], true) {
		return false
	}
	for i := 1; i < len(key); i++ {
		if !isIdentifierByte(key[i], true) {
			return false
		}
	}
	return true
}

// trailingLineEnding returns the line terminator at the end of raw —
// "\r\n", "\r", "\n", or "" when raw does not end with a line ending.
func trailingLineEnding(raw []byte) string {
	n := len(raw)
	if n == 0 {
		return ""
	}
	switch raw[n-1] {
	case '\n':
		if n >= 2 && raw[n-2] == '\r' {
			return "\r\n"
		}
		return "\n"
	case '\r':
		return "\r"
	default:
		return ""
	}
}

// Delete removes every assignment for key. Returns true when at least
// one entry was removed. Comments and blank lines are unaffected.
func (f *File) Delete(key string) bool {
	if f == nil {
		return false
	}
	removed := false
	f.entries = slices.DeleteFunc(f.entries, func(e Entry) bool {
		if e.Kind == EntryAssignment && e.Key == key {
			removed = true
			return true
		}
		return false
	})
	return removed
}

// Keys returns assignment keys in insertion order, deduplicated to the
// first occurrence.
func (f *File) Keys() []string {
	if f == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(f.entries))
	out := make([]string, 0, len(f.entries))
	for _, e := range f.entries {
		if e.Kind != EntryAssignment {
			continue
		}
		if _, dup := seen[e.Key]; dup {
			continue
		}
		seen[e.Key] = struct{}{}
		out = append(out, e.Key)
	}
	return out
}

// ToMap returns the file's effective key/value mapping (last-write-
// wins per Bash convention).
func (f *File) ToMap() map[string]string {
	if f == nil {
		return nil
	}
	out := make(map[string]string, len(f.entries))
	for _, e := range f.entries {
		if e.Kind == EntryAssignment {
			out[e.Key] = e.Value
		}
	}
	return out
}
