package dotenv

import (
	"errors"
	"strings"
	"testing"
)

func TestLint_PortableSubsetCleanly(t *testing.T) {
	src := []byte(`# this file is portable
FOO=bar
BAZ="quoted value"
EMPTY=
`)
	issues, err := Lint(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no warnings, got %v", issues)
	}
}

func TestLint_ExportPrefixWarns(t *testing.T) {
	src := []byte("export FOO=bar\n")
	issues, err := Lint(src)
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(issues, "export-prefix") {
		t.Errorf("expected export-prefix warning, got %v", issues)
	}
}

func TestLint_YAMLSeparatorWarns(t *testing.T) {
	src := []byte("FOO: bar\n")
	issues, err := Lint(src)
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(issues, "yaml-separator") {
		t.Errorf("expected yaml-separator warning, got %v", issues)
	}
}

func TestLint_NonPOSIXIdentifierWarns(t *testing.T) {
	src := []byte("FOO-BAR=1\n")
	issues, err := Lint(src, WithRelaxedNames())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(issues, "non-posix-identifier") {
		t.Errorf("expected non-posix-identifier warning, got %v", issues)
	}
}

func TestLint_BacktickValueWarns(t *testing.T) {
	src := []byte("FOO=`literal`\n")
	issues, err := Lint(src, WithBackticks())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(issues, "backtick-value") {
		t.Errorf("expected backtick-value warning, got %v", issues)
	}
}

func TestLint_SyntaxErrorReportedAsErrorSeverity(t *testing.T) {
	src := []byte("123FOO=bar\n")
	issues, err := Lint(src)
	if err == nil {
		t.Fatal("expected parse error")
	}
	// The syntax error is also surfaced as a SeverityError lint issue.
	gotSyntax := false
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			gotSyntax = true
			break
		}
	}
	if !gotSyntax {
		t.Errorf("expected SeverityError lint issue, got: %v", issues)
	}
	// And the underlying error type is ErrParse.
	if !errors.Is(err, ErrParse) {
		t.Errorf("err = %v, want ErrParse", err)
	}
}

func TestLint_EmptyInput(t *testing.T) {
	issues, err := Lint(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("got %v, want empty", issues)
	}
}

func TestSeverity_String(t *testing.T) {
	cases := map[Severity]string{
		SeverityError:   "error",
		SeverityWarning: "warning",
		Severity(99):    "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Severity(%d).String() = %q, want %q", s, got, want)
		}
	}
}

func hasIssue(issues []LintIssue, rule string) bool {
	for _, iss := range issues {
		if iss.Rule == rule {
			return true
		}
	}
	return false
}

func TestLint_MultiLineValueNotLinted(t *testing.T) {
	src := []byte("SECRET=\"line1\nhunter2-pass.word=zzz\nB: `x`\n\"\n")
	issues, err := Lint(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range issues {
		if strings.Contains(iss.Message, "hunter2") {
			t.Errorf("value fragment echoed: %v", iss)
		}
	}
	if len(issues) != 0 {
		t.Errorf("issues inside a quoted value: %v", issues)
	}
}

func TestLint_NonASCIIIdentifierWarns(t *testing.T) {
	issues, err := Lint([]byte("KÉY=1\n"), WithRelaxedNames())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(issues, "non-posix-identifier") {
		t.Errorf("got %v", issues)
	}
}

func TestLint_LimitErrorHasPosition(t *testing.T) {
	issues, err := Lint([]byte("A=1\nB=0123456789\n"), WithMaxLineLength(5))
	if err == nil {
		t.Fatal("want error")
	}
	if len(issues) != 1 || issues[0].Rule != "syntax" || issues[0].Pos.Line != 2 {
		t.Errorf("issues = %+v", issues)
	}
}

func TestLint_ExpansionIssueNamesVariable(t *testing.T) {
	issues, err := Lint([]byte("X=1\nA=x${B:?}\n"), WithExpand())
	if !errors.Is(err, ErrAssertionFailed) {
		t.Fatalf("err = %v", err)
	}
	want := LintIssue{
		Pos:      Position{Line: 2, Column: 4, Offset: 7},
		Rule:     "expansion",
		Message:  `"B": parameter null or not set`,
		Severity: SeverityError,
	}
	if len(issues) != 1 || issues[0] != want {
		t.Errorf("issues = %+v, want %+v", issues, want)
	}
	issues, _ = Lint([]byte("A=${a b}\n"), WithExpand())
	if len(issues) != 1 || issues[0].Rule != "expansion" || issues[0].Message != "bad substitution" {
		t.Errorf("bad substitution issues = %+v", issues)
	}
}
