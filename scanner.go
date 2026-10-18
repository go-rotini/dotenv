package dotenv

import (
	"bytes"
	"fmt"
	"strings"
)

// scanner is a single-pass byte-level tokenizer over a `.env` source.
// It produces logical entries (blank / comment / assignment) in source
// order, preserving each entry's raw bytes for round-trip emission.
type scanner struct {
	src  []byte
	pos  int
	line int
	col  int
	opts parseOptions
}

func newScanner(src []byte, opts parseOptions) *scanner {
	return &scanner{
		src:  src,
		line: 1,
		col:  1,
		opts: opts,
	}
}

func (s *scanner) done() bool {
	return s.pos >= len(s.src)
}

// peek returns the byte at the current position, or 0 at EOF.
func (s *scanner) peek() byte {
	if s.pos >= len(s.src) {
		return 0
	}
	return s.src[s.pos]
}

func (s *scanner) peekAt(offset int) byte {
	if s.pos+offset >= len(s.src) {
		return 0
	}
	return s.src[s.pos+offset]
}

func (s *scanner) advance(n int) {
	for i := 0; i < n && s.pos < len(s.src); i++ {
		c := s.src[s.pos]
		s.pos++
		switch c {
		case '\n':
			s.line++
			s.col = 1
		case '\r':
			// Don't double-count CRLF: an LF following CR will bump
			// the line itself on the next iteration.
			if s.pos < len(s.src) && s.src[s.pos] == '\n' {
				s.col = 1
			} else {
				s.line++
				s.col = 1
			}
		default:
			s.col++
		}
	}
}

func (s *scanner) currentPos() Position {
	return Position{
		File:   s.opts.filePath,
		Line:   s.line,
		Column: s.col,
		Offset: s.pos,
	}
}

// scanFile drains src into a *File. Returns the first parse error.
func scanFile(src []byte, opts parseOptions) (*File, error) {
	entries, err := scanEntries(src, opts)
	if err != nil {
		return nil, err
	}
	return &File{path: opts.filePath, entries: entries}, nil
}

// scanEntries scans src (with any BOM stripped) into entries. On a
// parse error it also returns the entries before the failing one.
func scanEntries(src []byte, opts parseOptions) ([]Entry, error) {
	if err := checkFileSize(int64(len(src)), opts); err != nil {
		return nil, err
	}
	src = stripUTF8BOM(src)

	s := newScanner(src, opts)
	var entries []Entry
	for !s.done() {
		startPos := s.currentPos()
		startOff := s.pos
		entry, err := s.scanLogicalEntry()
		if err != nil {
			return entries, err
		}
		entry.Pos = startPos
		entry.raw = bytes.Clone(src[startOff:s.pos])
		entries = append(entries, entry)
	}
	return entries, nil
}

// checkFileSize enforces [WithMaxFileSize] for an input of n bytes.
func checkFileSize(n int64, opts parseOptions) error {
	if opts.maxFileSize > 0 && n > opts.maxFileSize {
		return &ParseError{
			Pos:     Position{File: opts.filePath},
			Message: fmt.Sprintf("input exceeds %d bytes", opts.maxFileSize),
			Cause:   ErrFileTooLarge,
		}
	}
	return nil
}

// stripUTF8BOM removes a leading UTF-8 BOM (EF BB BF) if present.
func stripUTF8BOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// scanLogicalEntry reads one logical entry from the current position.
// On return the scanner is positioned past the terminating line ending.
func (s *scanner) scanLogicalEntry() (Entry, error) {
	physStart := s.pos

	for !s.done() && (s.peek() == ' ' || s.peek() == '\t') {
		s.advance(1)
	}

	if s.done() || isLineEnding(s.peek()) {
		s.consumeLineEnding()
		if err := s.checkLineLength(physStart); err != nil {
			return Entry{}, err
		}
		return Entry{Kind: EntryBlank}, nil
	}

	if s.peek() == '#' {
		for !s.done() && !isLineEnding(s.peek()) {
			s.advance(1)
		}
		s.consumeLineEnding()
		if err := s.checkLineLength(physStart); err != nil {
			return Entry{}, err
		}
		return Entry{Kind: EntryComment}, nil
	}

	return s.scanAssignment(physStart)
}

func (s *scanner) scanAssignment(physStart int) (Entry, error) {
	startPos := s.currentPos()

	exported := hasExportPrefix(s.src[s.pos:], s.opts.relaxedNames)
	if exported {
		if s.opts.strictDotenv {
			return Entry{}, &ParseError{Pos: startPos, Message: "export prefix not allowed in strict mode"}
		}
		s.advance(len(exportKeyword))
		for !s.done() && (s.peek() == ' ' || s.peek() == '\t') {
			s.advance(1)
		}
	}

	keyStart := s.pos
	if !isIdentifierStart(s.peek(), s.opts.relaxedNames) {
		return Entry{}, &ParseError{Pos: s.currentPos(), Message: fmt.Sprintf("invalid identifier start byte %q", s.peek())}
	}
	s.advance(1)
	for !s.done() && isIdentifierByte(s.peek(), s.opts.relaxedNames) {
		s.advance(1)
	}
	key := string(s.src[keyStart:s.pos])

	for !s.done() && (s.peek() == ' ' || s.peek() == '\t') {
		s.advance(1)
	}

	if exported && (s.done() || isLineEnding(s.peek()) || s.peek() == '#') {
		return Entry{}, &ParseError{Pos: startPos, Message: fmt.Sprintf("`export %s` without an assignment is not supported", key)}
	}
	if s.done() {
		return Entry{}, &ParseError{Pos: s.currentPos(), Message: "expected '=' or ':' after key"}
	}
	sep := s.peek()
	switch sep {
	case '=':
		s.advance(1)
	case ':':
		if s.opts.strictDotenv {
			return Entry{}, &ParseError{Pos: s.currentPos(), Message: "YAML-style ':' separator not allowed in strict mode"}
		}
		s.advance(1)
	default:
		return Entry{}, &ParseError{Pos: s.currentPos(), Message: fmt.Sprintf("expected '=' or ':' after key, got %q", s.peek())}
	}

	for !s.done() && (s.peek() == ' ' || s.peek() == '\t') {
		s.advance(1)
	}

	value, literal, quote, err := s.scanValue()
	if err != nil {
		return Entry{}, err
	}

	if err := s.consumeTrailingAndLineEnding(); err != nil {
		return Entry{}, err
	}

	if err := s.checkLineLength(physStart); err != nil {
		return Entry{}, err
	}

	return Entry{
		Kind:     EntryAssignment,
		Key:      key,
		Value:    value,
		quote:    quote,
		literal:  literal,
		exported: exported,
		sep:      sep,
	}, nil
}

// scanValue reads an assignment's value, dispatching by quote kind.
// On return the scanner is positioned after the value (or its closing
// quote) but before any trailing whitespace / comment / line ending.
// Returns the decoded value, the offsets of escaped `$` and `\` bytes
// in a double-quoted value (see [scanner.scanDoubleQuoted]), the quote form (0, '"', '\”,
// '`'), and any parse error.
func (s *scanner) scanValue() (string, []int, byte, error) {
	if s.done() || isLineEnding(s.peek()) {
		return "", nil, 0, nil
	}
	switch s.peek() {
	case '"':
		v, literal, err := s.scanDoubleQuoted()
		return v, literal, '"', err
	case '\'':
		v, err := s.scanSingleQuoted()
		return v, nil, '\'', err
	case '`':
		if s.opts.backticks {
			v, err := s.scanBacktickQuoted()
			return v, nil, '`', err
		}
		return "", nil, 0, &ParseError{Pos: s.currentPos(), Message: "backtick-quoted value not allowed; use WithBackticks() to enable"}
	default:
		return s.scanUnquoted(), nil, 0, nil
	}
}

// scanUnquoted reads up to end-of-line or the start of a `# comment`,
// trimming trailing whitespace. `#` only introduces a comment when
// preceded by whitespace, so `foo#bar` is one value.
func (s *scanner) scanUnquoted() string {
	start := s.pos
	for !s.done() && !isLineEnding(s.peek()) {
		c := s.peek()
		if c == '#' {
			if s.pos > start {
				prev := s.src[s.pos-1]
				if prev == ' ' || prev == '\t' {
					break
				}
			} else {
				break
			}
		}
		s.advance(1)
	}
	raw := string(s.src[start:s.pos])
	return strings.TrimRight(raw, " \t")
}

// scanSingleQuoted reads a single-quoted value: bytes are literal, no
// escapes, multi-line allowed.
func (s *scanner) scanSingleQuoted() (string, error) {
	openPos := s.currentPos()
	s.advance(1)
	start := s.pos
	for !s.done() && s.peek() != '\'' {
		s.advance(1)
	}
	if s.done() {
		return "", &ParseError{Pos: openPos, Message: "unterminated single-quoted value"}
	}
	value := normalizeLineBreaks(string(s.src[start:s.pos]))
	s.advance(1)
	return value, nil
}

// scanBacktickQuoted is the backtick equivalent of [scanSingleQuoted];
// only reachable under [WithBackticks].
func (s *scanner) scanBacktickQuoted() (string, error) {
	openPos := s.currentPos()
	s.advance(1)
	start := s.pos
	for !s.done() && s.peek() != '`' {
		s.advance(1)
	}
	if s.done() {
		return "", &ParseError{Pos: openPos, Message: "unterminated backtick-quoted value"}
	}
	value := normalizeLineBreaks(string(s.src[start:s.pos]))
	s.advance(1)
	return value, nil
}

// scanDoubleQuoted reads a double-quoted value with the standard
// escape table. Literal newlines are permitted inside the quotes.
// `\<LF>` / `\<CRLF>` is line continuation. Under [WithExpand], the
// offsets of the bytes decoded from `\$` and `\\` are returned in
// literal, so expansion treats them as plain text. Unknown escapes pass
// through as `\X` (matching motdotla's tolerant behavior).
func (s *scanner) scanDoubleQuoted() (value string, literal []int, err error) {
	openPos := s.currentPos()
	s.advance(1)
	var buf strings.Builder
	for !s.done() && s.peek() != '"' {
		c := s.peek()
		if c == '\\' {
			if s.pos+1 >= len(s.src) {
				return "", nil, &ParseError{Pos: s.currentPos(), Message: "unterminated escape sequence"}
			}
			next := s.peekAt(1)
			switch next {
			case 'n':
				buf.WriteByte('\n')
				s.advance(2)
			case 'r':
				buf.WriteByte('\r')
				s.advance(2)
			case 't':
				buf.WriteByte('\t')
				s.advance(2)
			case 'b':
				buf.WriteByte('\b')
				s.advance(2)
			case 'f':
				buf.WriteByte('\f')
				s.advance(2)
			case '"':
				buf.WriteByte('"')
				s.advance(2)
			case '\\', '$':
				if s.opts.expand != nil {
					literal = append(literal, buf.Len())
				}
				buf.WriteByte(next)
				s.advance(2)
			case '\n':
				s.advance(2)
			case '\r':
				s.advance(2)
				if !s.done() && s.peek() == '\n' {
					s.advance(1)
				}
			default:
				buf.WriteByte('\\')
				buf.WriteByte(next)
				s.advance(2)
			}
			continue
		}
		if c == '\r' {
			buf.WriteByte('\n')
			s.advance(1)
			if !s.done() && s.peek() == '\n' {
				s.advance(1)
			}
			continue
		}
		buf.WriteByte(c)
		s.advance(1)
	}
	if s.done() {
		return "", nil, &ParseError{Pos: openPos, Message: "unterminated double-quoted value"}
	}
	s.advance(1)
	return buf.String(), literal, nil
}

// normalizeLineBreaks turns each CRLF or bare CR in a quoted value's
// literal text into LF.
func normalizeLineBreaks(v string) string {
	if !strings.Contains(v, "\r") {
		return v
	}
	return strings.ReplaceAll(strings.ReplaceAll(v, "\r\n", "\n"), "\r", "\n")
}

// consumeTrailingAndLineEnding skips whitespace + optional `# comment`
// + the line ending after a value. Errors when a non-whitespace,
// non-`#` byte appears between the value and end-of-line.
func (s *scanner) consumeTrailingAndLineEnding() error {
	for !s.done() && (s.peek() == ' ' || s.peek() == '\t') {
		s.advance(1)
	}
	if s.done() {
		return nil
	}
	if isLineEnding(s.peek()) {
		s.consumeLineEnding()
		return nil
	}
	if s.peek() == '#' {
		for !s.done() && !isLineEnding(s.peek()) {
			s.advance(1)
		}
		s.consumeLineEnding()
		return nil
	}
	return &ParseError{
		Pos:     s.currentPos(),
		Message: fmt.Sprintf("unexpected byte %q after value", s.peek()),
	}
}

// consumeLineEnding reads one line ending (LF, CRLF, or bare CR).
func (s *scanner) consumeLineEnding() {
	if s.done() {
		return
	}
	switch s.peek() {
	case '\n':
		s.advance(1)
	case '\r':
		s.advance(1)
		if !s.done() && s.peek() == '\n' {
			s.advance(1)
		}
	}
}

// checkLineLength enforces [WithMaxLineLength] against the logical
// entry starting at physStart, not counting its terminating line ending;
// a multi-line quoted value is measured as a whole.
func (s *scanner) checkLineLength(physStart int) error {
	if s.opts.maxLineLength <= 0 {
		return nil
	}
	end := s.pos
	if end > physStart && s.src[end-1] == '\n' {
		end--
	}
	if end > physStart && s.src[end-1] == '\r' {
		end--
	}
	if end-physStart > s.opts.maxLineLength {
		return &ParseError{
			Pos:     s.posAt(physStart),
			Message: fmt.Sprintf("entry exceeds %d bytes", s.opts.maxLineLength),
			Cause:   ErrLineTooLong,
		}
	}
	return nil
}

// posAt returns the position of offset off, which must not be after
// the scanner's position.
func (s *scanner) posAt(off int) Position {
	line, col := 1, 1
	for i := range off {
		switch s.src[i] {
		case '\n':
			line, col = line+1, 1
		case '\r':
			if i+1 < len(s.src) && s.src[i+1] == '\n' {
				continue
			}
			line, col = line+1, 1
		default:
			col++
		}
	}
	return Position{File: s.opts.filePath, Line: line, Column: col, Offset: off}
}

var exportKeyword = []byte("export")

// hasExportPrefix reports whether b begins with "export", one or more
// spaces or tabs, and an identifier-start byte.
func hasExportPrefix(b []byte, relaxed bool) bool {
	if !bytes.HasPrefix(b, exportKeyword) {
		return false
	}
	i := len(exportKeyword)
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	return i > len(exportKeyword) && i < len(b) && isIdentifierStart(b[i], relaxed)
}

// isLineEnding reports whether c is LF or CR.
func isLineEnding(c byte) bool {
	return c == '\n' || c == '\r'
}

// isIdentifierStart reports whether c may begin a POSIX identifier.
// With relaxed=true, non-ASCII bytes are also accepted; even in
// relaxed mode an identifier cannot start with a digit.
func isIdentifierStart(c byte, relaxed bool) bool {
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c == '_':
		return true
	}
	if relaxed && c >= 0x80 {
		return true
	}
	return false
}

// isIdentifierByte reports whether c may appear after the first byte
// of an identifier. relaxed=true also permits hyphens, dots, and
// non-ASCII bytes.
func isIdentifierByte(c byte, relaxed bool) bool {
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
	if relaxed {
		switch c {
		case '-', '.':
			return true
		}
		if c >= 0x80 {
			return true
		}
	}
	return false
}
