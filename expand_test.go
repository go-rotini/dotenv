package dotenv

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"
)

// kvLookup is a tiny in-memory [VarLookup] for exercising
// [WithExpansionParent] without depending on github.com/go-rotini/env.
type kvLookup map[string]string

func (k kvLookup) Lookup(key string) (string, bool) {
	v, ok := k[key]
	return v, ok
}

// --- $VAR / ${VAR} basic forms ---

func TestExpand_BareDollar(t *testing.T) {
	src := "FIRST=hello\nSECOND=$FIRST\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("SECOND"); v != "hello" {
		t.Errorf("got %q", v)
	}
}

func TestExpand_BracedDollar(t *testing.T) {
	src := "FIRST=hello\nSECOND=${FIRST}_world\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("SECOND"); v != "hello_world" {
		t.Errorf("got %q", v)
	}
}

func TestExpand_DoubleQuoted(t *testing.T) {
	src := `FIRST=hello` + "\n" + `SECOND="prefix-${FIRST}-suffix"` + "\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("SECOND"); v != "prefix-hello-suffix" {
		t.Errorf("got %q", v)
	}
}

func TestExpand_SingleQuotedNotExpanded(t *testing.T) {
	src := "FIRST=hello\nSECOND='${FIRST}'\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("SECOND"); v != "${FIRST}" {
		t.Errorf("got %q (single-quoted should not expand)", v)
	}
}

func TestExpand_BackslashDollarSuppressed(t *testing.T) {
	src := `FIRST=hello` + "\n" + `SECOND="\${FIRST}"` + "\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("SECOND"); v != "${FIRST}" {
		t.Errorf("got %q (\\$ should suppress expansion)", v)
	}
}

func TestExpand_LoneDollar(t *testing.T) {
	src := "FOO=$\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("FOO"); v != "$" {
		t.Errorf("got %q", v)
	}
}

func TestExpand_DollarFollowedByNonIdent(t *testing.T) {
	src := "FOO=$.bar\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("FOO"); v != "$.bar" {
		t.Errorf("got %q", v)
	}
}

// --- :- and - default forms ---

func TestExpand_ColonDashUnsetUsesDefault(t *testing.T) {
	src := `FOO=${MISSING:-fallback}` + "\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("FOO"); v != "fallback" {
		t.Errorf("got %q", v)
	}
}

func TestExpand_ColonDashEmptyUsesDefault(t *testing.T) {
	src := "EMPTY=\nFOO=${EMPTY:-fallback}\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("FOO"); v != "fallback" {
		t.Errorf("got %q (:- should treat empty as unset)", v)
	}
}

func TestExpand_DashUnsetUsesDefault(t *testing.T) {
	src := `FOO=${MISSING-fallback}` + "\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("FOO"); v != "fallback" {
		t.Errorf("got %q", v)
	}
}

func TestExpand_DashEmptyKeepsEmpty(t *testing.T) {
	// POSIX `${VAR-default}`: empty stays empty (only unset triggers default).
	src := "EMPTY=\nFOO=${EMPTY-fallback}\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("FOO"); v != "" {
		t.Errorf("got %q (- with empty should NOT use default)", v)
	}
}

func TestExpand_DefaultIsExpanded(t *testing.T) {
	src := "INNER=resolved\nFOO=${MISSING:-${INNER}}\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("FOO"); v != "resolved" {
		t.Errorf("got %q (default should be recursively expanded)", v)
	}
}

// --- :? assertion form ---

func TestExpand_ColonQuestionUnsetErrors(t *testing.T) {
	src := `FOO=${MISSING:?required for foo}` + "\n"
	_, err := Parse([]byte(src), WithExpand())
	if !errors.Is(err, ErrAssertionFailed) {
		t.Errorf("err = %v, want ErrAssertionFailed", err)
	}
	if !errors.Is(err, ErrExpansion) {
		t.Errorf("err is not ExpansionError")
	}
}

func TestExpand_ColonQuestionSetSucceeds(t *testing.T) {
	src := "FOO=present\nBAR=${FOO:?required}\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("BAR"); v != "present" {
		t.Errorf("got %q", v)
	}
}

// --- WithStrictExpansion ---

func TestExpand_StrictExpansion(t *testing.T) {
	src := `FOO=${MISSING}` + "\n"
	_, err := Parse([]byte(src), WithExpand(), WithStrictExpansion())
	if !errors.Is(err, ErrUnsetVariable) {
		t.Errorf("err = %v, want ErrUnsetVariable", err)
	}
}

func TestExpand_LenientUnsetExpandsToEmpty(t *testing.T) {
	src := `FOO=${MISSING}` + "\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("FOO"); v != "" {
		t.Errorf("got %q", v)
	}
}

// --- WithExpansionParent ---

func TestExpand_ExpansionParent(t *testing.T) {
	parent := kvLookup(map[string]string{"FROM_PARENT": "the-value"})
	src := `FOO=${FROM_PARENT}` + "\n"
	f := mustParse(t, src, WithExpand(), WithExpansionParent(parent))
	if v, _ := f.Get("FOO"); v != "the-value" {
		t.Errorf("got %q (parent lookup failed)", v)
	}
}

func TestExpand_FilePrecedesParent(t *testing.T) {
	parent := kvLookup(map[string]string{"X": "from-parent"})
	src := "X=from-file\nFOO=${X}\n"
	f := mustParse(t, src, WithExpand(), WithExpansionParent(parent))
	if v, _ := f.Get("FOO"); v != "from-file" {
		t.Errorf("got %q (file should win over parent)", v)
	}
}

// --- WithExpandFromOSEnv ---

func TestExpand_FromOSEnv(t *testing.T) {
	t.Setenv("DOTENV_TEST_FROM_OSENV", "the-os-value")
	f := mustParse(t, "FOO=${DOTENV_TEST_FROM_OSENV}\nBAR=${DOTENV_TEST_DEFINITELY_NOT_SET}\n", WithExpandFromOSEnv())
	if v, _ := f.Get("FOO"); v != "the-os-value" {
		t.Errorf("FOO = %q, want the-os-value", v)
	}
	if v, _ := f.Get("BAR"); v != "" {
		t.Errorf("BAR = %q, want empty (unset OS var under lenient expand)", v)
	}
}

func TestExpand_FromOSEnvImpliesExpand(t *testing.T) {
	t.Setenv("DOTENV_TEST_X", "y")
	// WithExpandFromOSEnv enables expansion on its own — no separate WithExpand.
	f := mustParse(t, "FOO=$DOTENV_TEST_X\n", WithExpandFromOSEnv())
	if v, _ := f.Get("FOO"); v != "y" {
		t.Errorf("FOO = %q, want y", v)
	}
}

func TestExpand_FromOSEnvStrictUnsetErrors(t *testing.T) {
	_, err := Parse([]byte("FOO=${DOTENV_TEST_NOPE}\n"), WithExpandFromOSEnv(), WithStrictExpansion())
	if !errors.Is(err, ErrUnsetVariable) {
		t.Errorf("err = %v, want ErrUnsetVariable", err)
	}
}

func TestExpand_FileEntriesBeatOSEnv(t *testing.T) {
	t.Setenv("DOTENV_TEST_BOTH", "from-os")
	f := mustParse(t, "DOTENV_TEST_BOTH=from-file\nFOO=${DOTENV_TEST_BOTH}\n", WithExpandFromOSEnv())
	if v, _ := f.Get("FOO"); v != "from-file" {
		t.Errorf("FOO = %q, want from-file (file entry beats OS env)", v)
	}
}

// --- WithMaxExpansionDepth ---

func TestExpand_MaxDepth(t *testing.T) {
	src := "FOO=${MISSING:-${MISSING:-${MISSING:-${MISSING:-deep}}}}\n"
	_, err := Parse([]byte(src), WithExpand(), WithMaxExpansionDepth(2))
	if !errors.Is(err, ErrExpansion) {
		t.Errorf("err = %v, want ExpansionError", err)
	}
	if !errors.Is(err, ErrMaxExpansionDepth) {
		t.Errorf("err = %v, want cause ErrMaxExpansionDepth", err)
	}
	// Non-positive bound keeps the default (16), so this resolves fine.
	f := mustParse(t, src, WithExpand(), WithMaxExpansionDepth(0))
	if v, _ := f.Get("FOO"); v != "deep" {
		t.Errorf("FOO = %q with default depth, want deep", v)
	}
}

// --- expansion not running without WithExpand ---

func TestExpand_NotRunByDefault(t *testing.T) {
	src := `FOO=${MISSING}` + "\n"
	f := mustParse(t, src) // no WithExpand
	if v, _ := f.Get("FOO"); v != "${MISSING}" {
		t.Errorf("got %q (expansion ran without WithExpand)", v)
	}
}

// --- multiple variables in one value ---

func TestExpand_MultipleInOneValue(t *testing.T) {
	src := "A=1\nB=2\nC=$A-$B-${A}-${B}\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("C"); v != "1-2-1-2" {
		t.Errorf("got %q", v)
	}
}

// --- unterminated brace ---

func TestExpand_UnterminatedBraceLeftLiteral(t *testing.T) {
	src := "FOO=${UNCLOSED\n"
	f := mustParse(t, src, WithExpand())
	if v, _ := f.Get("FOO"); v != "${UNCLOSED" {
		t.Errorf("got %q (unterminated ${ should be literal)", v)
	}
}

// --- low-level helpers ---

func parseRef(s string) (varRef, bool) {
	return parseRefAt(s, 0)
}

func parseRefAt(s string, i int) (varRef, bool) {
	x := &expander{s: s}
	return x.parseVarRef(i, len(s))
}

func TestParseVarRef_BareName(t *testing.T) {
	r, ok := parseRef("$NAME rest")
	if !ok || r.name != "NAME" || r.end != 5 {
		t.Errorf("got %+v ok=%v", r, ok)
	}
}

func TestParseVarRef_BracedNoOp(t *testing.T) {
	r, ok := parseRef("${NAME} rest")
	if !ok || r.name != "NAME" || r.end != 7 || r.op != "" {
		t.Errorf("got %+v ok=%v", r, ok)
	}
}

func TestParseVarRef_Operator(t *testing.T) {
	s := "x${NAME:-de}f"
	r, ok := parseRefAt(s, 1)
	if !ok || r.name != "NAME" || r.op != ":-" || s[r.argStart:r.argEnd] != "de" || r.end != 12 {
		t.Errorf("got %+v ok=%v", r, ok)
	}
}

func TestParseVarRef_ShortInputRejected(t *testing.T) {
	if _, ok := parseRef("$"); ok {
		t.Error("lone $ should not parse")
	}
	if _, ok := parseRef("$."); ok {
		t.Error("$<non-ident> should not parse")
	}
}

func TestParseVarRef_UnterminatedBrace(t *testing.T) {
	if _, ok := parseRef("${UNCLOSED"); ok {
		t.Error("unterminated ${ should not parse")
	}
}

// braceScan is the per-reference scan that matchBraces replaces: the
// index in s of the `}` closing an implicit outer `{`, or -1.
func braceScan(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i++
			}
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

func TestMatchBraces(t *testing.T) {
	cases := []struct {
		in   string
		want int // match for the `{` at index 1
	}{
		{"${a}b", 3},
		{"${{nested}}rest", 10},
		{`${a\}b}`, 6}, // \} is escaped, doesn't count
		{"${unclosed", -1},
		{"${a{b ${c}", -1},
	}
	for _, c := range cases {
		if got := closingBrace(matchBraces(c.in, nil, nil), 1); got != c.want {
			t.Errorf("matchBraces(%q)[1] = %d, want %d", c.in, got, c.want)
		}
	}
	// A backslash at a literal offset (decoded from `\\`) escapes nothing.
	if got := closingBrace(matchBraces(`${a\}b}`, []int{3}, nil), 1); got != 4 {
		t.Errorf("literal backslash: match = %d, want 4", got)
	}
}

// FuzzMatchBraces checks matchBraces against the per-reference scan.
func FuzzMatchBraces(f *testing.F) {
	for _, s := range []string{"${A}", "${a{b ${c}", `${\}}`, "${${${", "x${A:-${B}}y}", `\${A}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		m := matchBraces(s, nil, nil)
		for i := 0; i+1 < len(s); i++ {
			if s[i] != '$' || s[i+1] != '{' {
				continue
			}
			want := braceScan(s[i+2:])
			if want >= 0 {
				want += i + 2
			}
			if got := closingBrace(m, i+1); got != want {
				t.Fatalf("%q at %d: matchBraces %d, scan %d", s, i, got, want)
			}
		}
	})
}

func TestFirstTopLevelOperator(t *testing.T) {
	cases := []struct {
		in    string
		idx   int
		oplen int
	}{
		{"NAME:-default", 4, 2},
		{"NAME:?required", 4, 2},
		{"NAME:+alt", 4, 2},
		{"NAME-default", 4, 1},
		{"NAME?msg", 4, 1},
		{"NAME+alt", 4, 1},
		{"NAME${INNER:-x}", -1, 0},         // operator inside nested ${}
		{`NAME\:-escaped`, 6, 1},           // escaped ':' does not count
		{"NAME", -1, 0},                    // no operator
		{"NAME{nested-thing}-real", 18, 1}, // skip nested braces
	}
	for _, c := range cases {
		gotIdx, gotLen := firstTopLevelOperator(c.in, 0, len(c.in), nil)
		if gotIdx != c.idx || gotLen != c.oplen {
			t.Errorf("firstTopLevelOperator(%q) = (%d, %d), want (%d, %d)",
				c.in, gotIdx, gotLen, c.idx, c.oplen)
		}
	}
}

func TestIsVarRefByte(t *testing.T) {
	for _, c := range "ABCabc012_" {
		if !isVarRefByte(byte(c)) {
			t.Errorf("isVarRefByte(%q) = false, want true", c)
		}
	}
	for _, c := range "-.=#$" {
		if isVarRefByte(byte(c)) {
			t.Errorf("isVarRefByte(%q) = true, want false", c)
		}
	}
}

func TestExpand_QuotedLiteralsAreReferenceable(t *testing.T) {
	t.Setenv("DOTENV_TEST_HOME", "/os/home")
	cases := []struct {
		src  string
		opts []ParseOption
		want string
	}{
		{"A='x'\nB=$A\n", []ParseOption{WithExpand()}, "x"},
		{"A='x'\nB=${A:?required}\n", []ParseOption{WithExpand()}, "x"},
		{"A=`x`\nB=$A\n", []ParseOption{WithExpand(), WithBackticks()}, "x"},
		{"A=1\nA='2'\nB=$A\n", []ParseOption{WithExpand()}, "2"},
		{"DOTENV_TEST_HOME='/file/home'\nB=$DOTENV_TEST_HOME\n", []ParseOption{WithExpandFromOSEnv()}, "/file/home"},
	}
	for _, c := range cases {
		f := mustParse(t, c.src, c.opts...)
		if v, _ := f.Get("B"); v != c.want {
			t.Errorf("%q: B = %q, want %q", c.src, v, c.want)
		}
	}
}

func TestExpand_OutputBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString("A0=xxxxxxxxxx\n")
	for i := 1; i <= 9; i++ {
		fmt.Fprintf(&b, "A%d=%s\n", i, strings.Repeat(fmt.Sprintf("$A%d", i-1), 10))
	}
	_, err := Parse([]byte(b.String()), WithExpand())
	if !errors.Is(err, ErrExpansionTooLarge) || !errors.Is(err, ErrExpansion) {
		t.Fatalf("err = %v, want ErrExpansionTooLarge", err)
	}
	if _, err := Parse([]byte("A=xx\nB=$A$A\n"), WithExpand(), WithMaxExpandedSize(3)); !errors.Is(err, ErrExpansionTooLarge) {
		t.Errorf("small budget: %v", err)
	}
	f := mustParse(t, "A=xx\nB=$A$A\n", WithExpand(), WithMaxExpandedSize(6))
	if v, _ := f.Get("B"); v != "xxxx" {
		t.Errorf("B = %q", v)
	}
	src := "A=xxxxxxxxxx\nB=$A$A$A\n" // 22 bytes that substitute 30
	if _, err := Parse([]byte(src), WithExpand(), WithMaxFileSize(int64(len(src)))); !errors.Is(err, ErrExpansionTooLarge) {
		t.Errorf("default budget should follow WithMaxFileSize: %v", err)
	}
}

func TestExpand_UnterminatedBracesLinear(t *testing.T) {
	grow := func(n int) string { return "A=" + strings.Repeat("${", n) + "\n" }
	f := mustParse(t, grow(20_000), WithExpand())
	if v, _ := f.Get("A"); len(v) != 40_000 {
		t.Errorf("len = %d", len(v))
	}
	checkLinear(t, grow, 1_000)
}

// checkLinear fails when expanding the value grow builds is not linear in
// its size. It first counts the bytes the expander's own scans visit at
// size 8m, which must stay within three times the input. That count sees
// only the scans that report to it, so it then times parsing at sizes m
// and 8m: linear code takes about 8 times as long at the larger size,
// while code that rescans the rest of the value per reference takes
// about 64 times as long. The best of several runs is compared, and a
// failure is retried, so machine load doesn't decide the result.
func checkLinear(t *testing.T, grow func(n int) string, m int) {
	t.Helper()
	small, large := grow(m), grow(8*m)
	o := defaultParseOptions()
	WithExpand()(&o)
	f, err := scanFile([]byte(large), o)
	if err != nil {
		t.Fatal(err)
	}
	x := newExpander(o)
	if err := x.expandFile(f); err != nil {
		t.Fatal(err)
	}
	if x.work > 3*len(large) {
		t.Errorf("expanding %d bytes scanned %d", len(large), x.work)
	}

	reps := 1
	for reps < 1<<16 && bestParseTime(t, small, reps) < 2*time.Millisecond {
		reps *= 2
	}
	var ratio float64
	for range 2 {
		ratio = float64(bestParseTime(t, large, reps)) / float64(bestParseTime(t, small, reps))
		if ratio <= 24 {
			return
		}
	}
	t.Errorf("parsing %d bytes took %.0f times as long as %d bytes; linear is about 8", len(large), ratio, len(small))
}

// bestParseTime returns the shortest of five timings of reps expanding
// parses of src.
func bestParseTime(t *testing.T, src string, reps int) time.Duration {
	t.Helper()
	best := time.Duration(math.MaxInt64)
	for range 5 {
		start := time.Now()
		for range reps {
			if _, err := Parse([]byte(src), WithExpand()); err != nil {
				t.Fatal(err)
			}
		}
		best = min(best, time.Since(start))
	}
	return best
}

func TestExpand_EscapedBackslashBeforeReference(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"A=x\nB=\"\\\\$A\"\n", `\x`},
		{"A=x\nB=\"\\$A\"\n", `$A`},
		{"A=x\nB=\\$A\n", `$A`},
	} {
		f := mustParse(t, c.src, WithExpand())
		if v, _ := f.Get("B"); v != c.want {
			t.Errorf("%q: B = %q, want %q", c.src, v, c.want)
		}
	}
}

func TestExpand_Operators(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"A=1\nB=${A:+alt}\n", "alt"},
		{"A=\nB=${A:+alt}\n", ""},
		{"A=\nB=${A+alt}\n", "alt"},
		{"B=${A+alt}\n", ""},
		{"A=\nB=${A?msg}\n", ""},
	} {
		f := mustParse(t, c.src, WithExpand())
		if v, _ := f.Get("B"); v != c.want {
			t.Errorf("%q: B = %q, want %q", c.src, v, c.want)
		}
	}
	if _, err := Parse([]byte("B=${A?gone}\n"), WithExpand()); !errors.Is(err, ErrAssertionFailed) || !strings.Contains(err.Error(), "gone") {
		t.Errorf("${A?msg}: %v", err)
	}
	for _, src := range []string{"B=${}\n", "B=${A B}\n", "B=${A:=d}\n"} {
		if _, err := Parse([]byte(src), WithExpand()); !errors.Is(err, ErrBadSubstitution) {
			t.Errorf("%q: err = %v, want ErrBadSubstitution", src, err)
		}
	}
}

func TestExpand_EmptyAssertMessage(t *testing.T) {
	_, err := Parse([]byte("B=${A:?}\n"), WithExpand())
	if err == nil || !strings.HasSuffix(err.Error(), "parameter null or not set") {
		t.Errorf("err = %v", err)
	}
}

func TestExpand_BELIsKept(t *testing.T) {
	for _, src := range []string{"B=\"a\ab\"\n", "B=a\ab\n", "A=1\nB=\"\a$A\"\n"} {
		plain := mustParse(t, src)
		want, _ := plain.Get("B")
		want = strings.ReplaceAll(want, "$A", "1")
		f := mustParse(t, src, WithExpand())
		if v, _ := f.Get("B"); v != want {
			t.Errorf("%q: B = %q, want %q", src, v, want)
		}
	}
}

func TestExpand_EscapesAndDefaultsLinear(t *testing.T) {
	grow := func(n int) string {
		return "A=\"" + strings.Repeat(`\$`, n) + strings.Repeat("${X:-}", n) + "\"\n"
	}
	n := 12_000 // 96 KB of `\$` escapes and `${X:-}` defaults in one value
	f := mustParse(t, grow(n), WithExpand())
	if v, _ := f.Get("A"); v != strings.Repeat("$", n) {
		t.Errorf("len = %d, want %d", len(v), n)
	}
	checkLinear(t, grow, 1_500)
}

func TestExpand_NestedDefaultsMemory(t *testing.T) {
	// 16 nested defaults around a 900 KB value: each level's argument
	// is copied once, so the parse allocates a small multiple of 16 × 900 KB.
	body := strings.Repeat("y", 900*1024)
	src := "A=\"" + strings.Repeat("${U:-", 15) + body + strings.Repeat("}", 15) + "\"\n"
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f := mustParse(t, src, WithExpand(), WithMaxLineLength(0))
	runtime.ReadMemStats(&after)
	if v, _ := f.Get("A"); v != body {
		t.Fatalf("len = %d, want %d", len(v), len(body))
	}
	if got, limit := after.TotalAlloc-before.TotalAlloc, uint64(40<<20); got > limit {
		t.Errorf("parse allocated %d bytes, want at most %d", got, limit)
	}
}

func TestExpand_BadSubstitutionHidesBody(t *testing.T) {
	for _, c := range []struct {
		src    string
		secret string
		line   int
		col    int
	}{
		{"DB_PASS=\"p@${x7!Qz hunter2}9\"\n", "hunter2", 1, 12},
		{"T=${{ secrets.TOKEN }}\n", "secrets", 1, 3},
		{"X=1\nexport  A = \"x\\n\\\\${s3cr3t!}\"\n", "s3cr3t", 2, 19},
		{"X=1\nA=\"a\nb${s3cr3t!}\"\n", "s3cr3t", 3, 2},
	} {
		_, err := Parse([]byte(c.src), WithExpand())
		var ee *ExpansionError
		if !errors.As(err, &ee) || !errors.Is(err, ErrBadSubstitution) {
			t.Fatalf("%q: err = %v, want ErrBadSubstitution", c.src, err)
		}
		if ee.Variable != "" || strings.Contains(err.Error(), c.secret) {
			t.Errorf("%q: error shows the body: Variable=%q, %v", c.src, ee.Variable, err)
		}
		if ee.Pos.Line != c.line || ee.Pos.Column != c.col {
			t.Errorf("%q: Pos = %d:%d, want %d:%d", c.src, ee.Pos.Line, ee.Pos.Column, c.line, c.col)
		}
	}
}

func TestExpand_ErrorPositionAfterContinuation(t *testing.T) {
	for _, c := range []struct {
		src       string
		line, col int
	}{
		{"B=\"a\\\n${U:?}\"\n", 2, 1},
		{"B=\"a\\\r\n${U:?}\"\n", 2, 1},
		{"B=\"\\\n\\\n${U:?}\"\n", 3, 1},
		{"B=\"a\\\nb${U:?}\"\n", 2, 2},
	} {
		_, err := Parse([]byte(c.src), WithExpand())
		var ee *ExpansionError
		if !errors.As(err, &ee) {
			t.Fatalf("%q: err = %v", c.src, err)
		}
		if ee.Pos.Line != c.line || ee.Pos.Column != c.col || c.src[ee.Pos.Offset] != '$' {
			t.Errorf("%q: Pos = %+v, want %d:%d at the $", c.src, ee.Pos, c.line, c.col)
		}
	}
}

func TestExpand_ErrorPositionIsReference(t *testing.T) {
	_, err := Parse([]byte("A=1\nB=xx${C:-${D:?}}\n"), WithExpand())
	var ee *ExpansionError
	if !errors.As(err, &ee) || ee.Variable != "D" {
		t.Fatalf("err = %v", err)
	}
	if ee.Pos.Line != 2 || ee.Pos.Column != 10 || ee.Pos.Offset != 13 {
		t.Errorf("Pos = %+v, want line 2, column 10, offset 13", ee.Pos)
	}
}

func TestExpand_BackslashBeforeClose(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"B=\"${DIR:-C:\\\\Temp\\\\}\"\n", `C:\Temp\`},
		{"DIR=D:\\\nB=\"${DIR:-C:\\\\Temp\\\\}\"\n", `D:\`},
		{"B=\"${DIR:-a\\}b}\"\n", `a\}b`},
		{"B=\"\\\\${DIR:-x}\"\n", `\x`},
		{"B=${DIR:-C:\\Temp\\}\n", `${DIR:-C:\Temp\}`},
		{"B=${A:-{}\n", `${A:-{}`},
	} {
		f := mustParse(t, c.src, WithExpand())
		if v, _ := f.Get("B"); v != c.want {
			t.Errorf("%q: B = %q, want %q", c.src, v, c.want)
		}
	}
}
