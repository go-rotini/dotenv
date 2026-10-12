package dotenv

import "strconv"

// Position identifies a location in a `.env` source. File is the path the
// source was loaded from, or "" for an in-memory [Parse]. Line and Column are
// 1-based; Offset is the 0-based byte offset of the location within the
// source. The zero value (all fields zero) means "no position information".
//
// Position is the location type reported by [ParseError], [ExpansionError],
// [LintIssue], and [Entry].
type Position struct {
	File   string
	Line   int
	Column int
	Offset int
}

// String renders the position as "file:line:column", omitting the file when
// it is empty and the column when it is zero. A Position without a line
// (such as the zero value) renders as "<unknown position>".
func (p Position) String() string {
	if p.Line <= 0 {
		return "<unknown position>"
	}
	var b []byte
	if p.File != "" {
		b = append(b, p.File...)
		b = append(b, ':')
	}
	b = strconv.AppendInt(b, int64(p.Line), 10)
	if p.Column > 0 {
		b = append(b, ':')
		b = strconv.AppendInt(b, int64(p.Column), 10)
	}
	return string(b)
}
