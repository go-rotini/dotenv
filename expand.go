package dotenv

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// expander carries the state shared by every value expanded in one
// file (the already-resolved entries and the remaining substitution
// budget, [WithMaxExpandedSize]) and the value being expanded.
type expander struct {
	opts     parseOptions
	resolved map[string]string
	budget   int64 // remaining bytes references may add; ignored when !limited
	limited  bool

	// The value being expanded. Nested arguments are expanded as
	// sub-ranges of s, so literal and braces cover every level.
	entry    *Entry
	s        string
	literal  []int       // sorted offsets of escaped bytes in s; see Entry.literal
	unquoted bool        // enables the `\$` courtesy escape
	braces   []bracePair // built on the first `${`
	scanned  bool        // braces is built
}

// expandAll runs variable expansion over every assignment entry in
// source order. Single-quoted and backtick-quoted values are not
// expanded (their dialects suppress expansion) but are still visible to
// later references. Lookups consult the file's already-resolved entries
// first, then any [WithExpansionParent].
func (f *File) expandAll(opts parseOptions) error {
	x := &expander{
		opts:     opts,
		resolved: make(map[string]string),
		budget:   opts.expandedSizeLimit(),
		limited:  opts.expandedSizeLimit() > 0,
	}
	for i := range f.entries {
		e := &f.entries[i]
		if e.Kind != EntryAssignment {
			continue
		}
		if e.quote != '\'' && e.quote != '`' {
			x.entry, x.s, x.literal, x.unquoted = e, e.Value, e.literal, e.quote == 0
			x.scanned = false
			expanded, err := x.expand(0, len(e.Value), 0)
			if err != nil {
				return err
			}
			e.Value = expanded
			e.literal = nil
		}
		x.resolved[e.Key] = e.Value
	}
	return nil
}

// expand expands the references in x.s[lo:hi].
func (x *expander) expand(lo, hi, depth int) (string, error) {
	s := x.s
	var b strings.Builder
	b.Grow(hi - lo)
	next := sort.SearchInts(x.literal, lo) // index of the first literal offset >= i
	for i := lo; i < hi; {
		c := s[i]
		switch c {
		case '\\':
			if x.unquoted && i+1 < hi && s[i+1] == '$' {
				b.WriteByte('$')
				i += 2
				continue
			}
			b.WriteByte(c)
			i++
		case '$':
			for next < len(x.literal) && x.literal[next] < i {
				next++
			}
			if next < len(x.literal) && x.literal[next] == i {
				b.WriteByte('$')
				i++
				continue
			}
			ref, ok := x.parseVarRef(i, hi)
			if !ok {
				b.WriteByte('$')
				i++
				continue
			}
			out, err := x.resolve(i, ref, depth)
			if err != nil {
				return "", err
			}
			b.WriteString(out)
			i = ref.end
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), nil
}

// resolve returns the text that replaces ref, the reference at x.s[at].
func (x *expander) resolve(at int, ref varRef, depth int) (string, error) {
	if !IsValidKey(ref.name) {
		// The body is not a name and may be part of a secret value, so
		// the error carries only its position.
		return "", &ExpansionError{
			Pos:    x.pos(at),
			Reason: "bad substitution",
			Cause:  ErrBadSubstitution,
		}
	}
	value, isSet := lookupVar(ref.name, x.opts, x.resolved)
	empty := !isSet || value == ""
	arg := func() (string, error) {
		if depth+1 > x.opts.maxExpansionDepth {
			return "", &ExpansionError{
				Pos:    x.pos(at),
				Reason: fmt.Sprintf("max expansion depth %d exceeded", x.opts.maxExpansionDepth),
				Cause:  ErrMaxExpansionDepth,
			}
		}
		return x.expand(ref.argStart, ref.argEnd, depth+1)
	}
	switch ref.op {
	case ":-", "-":
		if !isSet || (ref.op == ":-" && empty) {
			return arg()
		}
	case ":?", "?":
		if !isSet || (ref.op == ":?" && empty) {
			msg, err := arg()
			if err != nil {
				return "", err
			}
			if msg == "" {
				msg = "parameter not set"
				if ref.op == ":?" {
					msg = "parameter null or not set"
				}
			}
			return "", &ExpansionError{Pos: x.pos(at), Variable: ref.name, Reason: msg, Cause: ErrAssertionFailed}
		}
	case ":+", "+":
		if !isSet || (ref.op == ":+" && empty) {
			return "", nil
		}
		return arg()
	default:
		if !isSet && x.opts.strictExpansion {
			return "", &ExpansionError{
				Pos:      x.pos(at),
				Variable: ref.name,
				Reason:   "unset variable referenced",
				Cause:    ErrUnsetVariable,
			}
		}
	}
	if x.limited {
		if int64(len(value)) > x.budget {
			return "", &ExpansionError{
				Pos:      x.pos(at),
				Variable: ref.name,
				Reason:   fmt.Sprintf("expanded values exceed %d bytes", x.opts.expandedSizeLimit()),
				Cause:    ErrExpansionTooLarge,
			}
		}
		x.budget -= int64(len(value))
	}
	return value, nil
}

// pos returns the source position of byte off of the value being
// expanded.
func (x *expander) pos(off int) Position {
	return x.entry.valuePos(off)
}

// varRef is one parsed `$NAME` or `${...}` reference. end is the index
// just past the reference; s[argStart:argEnd] is the operator's word.
type varRef struct {
	name     string
	op       string
	argStart int
	argEnd   int
	end      int
}

// parseVarRef parses the reference starting at x.s[i], which is `$`,
// within x.s[:hi]. ok=false when no reference starts there (lone `$`,
// `$<non-ident>`, unterminated `${`). Recognized shapes:
//
//	$NAME
//	${NAME}
//	${NAME:-WORD} ${NAME-WORD}   default
//	${NAME:?WORD} ${NAME?WORD}   assert
//	${NAME:+WORD} ${NAME+WORD}   alternative
//
// Any other body yields a name that fails [IsValidKey].
func (x *expander) parseVarRef(i, hi int) (varRef, bool) {
	s := x.s
	if i+1 >= hi {
		return varRef{}, false
	}
	if s[i+1] != '{' {
		end := i + 1
		for end < hi && isVarRefByte(s[end]) {
			end++
		}
		if end == i+1 {
			return varRef{}, false
		}
		return varRef{name: s[i+1 : end], end: end}, true
	}
	if !x.scanned {
		x.braces = matchBraces(s, x.literal, x.braces[:0])
		x.scanned = true
	}
	// The braces of an argument balance within it, so a match found
	// for the whole value never lies past hi.
	closeIdx := closingBrace(x.braces, i+1)
	if closeIdx < 0 {
		return varRef{}, false
	}
	bodyStart := i + 2
	ref := varRef{name: s[bodyStart:closeIdx], end: closeIdx + 1}
	if opIdx, opLen := firstTopLevelOperator(s, bodyStart, closeIdx, x.literal); opIdx >= 0 {
		ref.name = s[bodyStart:opIdx]
		ref.op = s[opIdx : opIdx+opLen]
		ref.argStart = opIdx + opLen
		ref.argEnd = closeIdx
	}
	return ref, true
}

// bracePair records the `{` of a `${` at open and its matching `}` at
// close, or -1 when it has none.
type bracePair struct{ open, close int }

// matchBraces finds the matching `}` for the `{` of every `${` in s,
// appended to pairs[:0] in order of open. A backslash skips the following byte
// unless its offset is in literal (a backslash written as `\\` in a
// double-quoted value). Every `{` opens a level, so the match for the
// `{` of a `${` is the first `}` that brings the depth back to that
// level.
func matchBraces(s string, literal []int, pairs []bracePair) []bracePair {
	n := strings.Count(s, "${")
	pairs = slices.Grow(pairs[:0], n)
	var buf [16]int
	stack := buf[:0] // indexes into pairs of unclosed opens
	if n > len(buf) {
		stack = make([]int, 0, n)
	}
	depth, next := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if !isLiteral(literal, &next, i) && i+1 < len(s) {
				i++
			}
		case '{':
			depth++
			if i > 0 && s[i-1] == '$' {
				// close holds the depth until the match is found.
				pairs = append(pairs, bracePair{open: i, close: -depth})
				stack = append(stack, len(pairs)-1)
			}
		case '}':
			if depth == 0 {
				continue
			}
			if n := len(stack); n > 0 && pairs[stack[n-1]].close == -depth {
				pairs[stack[n-1]].close = i
				stack = stack[:n-1]
			}
			depth--
		}
	}
	for _, k := range stack {
		pairs[k].close = -1
	}
	return pairs
}

// closingBrace returns the match recorded in pairs for the `{` at open,
// or -1.
func closingBrace(pairs []bracePair, open int) int {
	k, ok := slices.BinarySearchFunc(pairs, open, func(p bracePair, open int) int { return p.open - open })
	if !ok {
		return -1
	}
	return pairs[k].close
}

// isLiteral reports whether offset i is in the sorted literal offsets.
// *next is a cursor into literal that only moves forward, so calls
// must come in increasing order of i.
func isLiteral(literal []int, next *int, i int) bool {
	for *next < len(literal) && literal[*next] < i {
		*next++
	}
	return *next < len(literal) && literal[*next] == i
}

// firstTopLevelOperator finds the first operator (":-", ":?", ":+",
// "-", "?", "+") in s[lo:hi] outside any nested `{...}`. It returns the
// operator's index in s and its length, or (-1, 0) when none. Escapes
// follow [matchBraces].
func firstTopLevelOperator(s string, lo, hi int, literal []int) (int, int) {
	depth := 0
	next := sort.SearchInts(literal, lo)
	for i := lo; i < hi; i++ {
		switch s[i] {
		case '\\':
			if !isLiteral(literal, &next, i) && i+1 < hi {
				i++
			}
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 && i+1 < hi {
				switch s[i+1] {
				case '-', '?', '+':
					return i, 2
				}
			}
		case '-', '?', '+':
			if depth == 0 {
				return i, 1
			}
		}
	}
	return -1, 0
}

// isVarRefByte reports whether c can appear inside a `$NAME` (no
// braces) reference. POSIX identifier chars only — relaxed forms are
// too ambiguous to recognize without braces.
func isVarRefByte(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '_':
		return true
	}
	return false
}

// lookupVar resolves name against the file's already-parsed entries
// first, then any [WithExpansionParent] source. Only entries resolved
// *before* the current one are visible, so a plain self-reference
// (`A=${A}`) sees A as unset rather than recursing; pathological nested
// defaults (`${A:-${A:-${A}}}`) are bounded by [WithMaxExpansionDepth]
// and surface as an [*ExpansionError] when the cap is reached.
func lookupVar(name string, opts parseOptions, resolved map[string]string) (string, bool) {
	if v, ok := resolved[name]; ok {
		return v, true
	}
	if opts.expansionParent != nil {
		if v, ok := opts.expansionParent.Lookup(name); ok {
			return v, true
		}
	}
	return "", false
}

// valuePos returns the source position of byte off of e's decoded
// value, found by walking the entry's raw text. It is used only to
// report errors.
func (e *Entry) valuePos(off int) Position {
	raw := e.raw
	i := skipBlanks(raw, 0)
	if e.exported {
		i = skipBlanks(raw, i+len(exportKeyword))
	}
	i = skipBlanks(raw, i+len(e.Key))
	i = skipBlanks(raw, i+1) // the separator
	if e.quote != '"' {
		i += off
	} else {
		i++
		for d := 0; d < off && i < len(raw); {
			switch {
			case raw[i] == '\\' && i+1 < len(raw):
				switch raw[i+1] {
				case '\n':
				case '\r':
					if i+2 < len(raw) && raw[i+2] == '\n' {
						i++
					}
				case 'n', 'r', 't', 'b', 'f', '"', '\\', '$':
					d++
				default:
					d += 2
				}
				i += 2
			case raw[i] == '\r' && i+1 < len(raw) && raw[i+1] == '\n':
				d++
				i += 2
			default:
				d++
				i++
			}
		}
	}
	i = min(i, len(raw))
	p := e.Pos
	for j := range i {
		switch raw[j] {
		case '\n':
			p.Line, p.Column = p.Line+1, 1
		case '\r':
			if j+1 < len(raw) && raw[j+1] == '\n' {
				continue
			}
			p.Line, p.Column = p.Line+1, 1
		default:
			p.Column++
		}
	}
	p.Offset += i
	return p
}

// skipBlanks returns the index of the first byte of b at or after i
// that is not a space or tab.
func skipBlanks(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	return i
}
