package dotenv

import (
	"bytes"
	"strings"
	"testing"
)

// --- byte-exact round-trip on canonical input ---

func TestMarshal_RoundTripCanonical(t *testing.T) {
	cases := []string{
		"",
		"\n",
		"FOO=bar\n",
		"FOO=bar\nBAR=baz\n",
		"# top comment\nFOO=bar\n",
		"\nFOO=bar\n\nBAR=baz\n\n",
		"FOO='single quoted'\n",
		"FOO=\"double quoted\"\n",
		"export FOO=bar\n",
		"FOO=bar # trailing comment\n",
		"FOO=multi\nBAR=line\n# block of comments\n# more\nBAZ=qux\n",
	}
	for i, src := range cases {
		f, err := Parse([]byte(src))
		if err != nil {
			t.Errorf("case %d Parse: %v", i, err)
			continue
		}
		got, err := f.Marshal()
		if err != nil {
			t.Errorf("case %d Marshal: %v", i, err)
			continue
		}
		if !bytes.Equal(got, []byte(src)) {
			t.Errorf("case %d round-trip mismatch:\nwant %q\n got %q", i, src, got)
		}
	}
}

func TestMarshal_PreservesCRLF(t *testing.T) {
	src := "FOO=1\r\nBAR=2\r\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Errorf("CRLF not preserved:\nwant %q\n got %q", src, got)
	}
}

func TestMarshal_PreservesBareCR(t *testing.T) {
	src := "FOO=1\rBAR=2\r"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Errorf("bare CR not preserved:\nwant %q\n got %q", src, got)
	}
}

// --- mutation: Set replaces in place ---

func TestMarshal_SetExistingPreservesSurroundings(t *testing.T) {
	src := "# top comment\n\nA=1\nB=2\n# tail comment\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("A", "updated")
	got, err := f.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	gotStr := string(got)
	// The mutated key's line should now read A=updated; everything else
	// should be byte-identical to the original.
	if !strings.Contains(gotStr, "A=updated\n") {
		t.Errorf("mutated value not present:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "# top comment") {
		t.Errorf("top comment lost")
	}
	if !strings.Contains(gotStr, "# tail comment") {
		t.Errorf("tail comment lost")
	}
	if !strings.Contains(gotStr, "B=2\n") {
		t.Errorf("sibling key lost")
	}
}

func TestMarshal_SetNewAppends(t *testing.T) {
	src := "A=1\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("B", "2")
	got, err := f.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A=1\nB=2\n" {
		t.Errorf("got %q", got)
	}
}

func TestMarshal_SetAppendsToSourceWithoutTrailingNewline(t *testing.T) {
	// Source whose last line has no terminator — appending must not glue
	// the new entry onto it.
	f, err := Parse([]byte("A=1\nB=2"))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("C", "3")
	got, err := f.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A=1\nB=2\nC=3\n" {
		t.Fatalf("got %q, want %q", got, "A=1\nB=2\nC=3\n")
	}
	// And the output must re-parse to the expected map.
	f2, err := Parse(got)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	for k, want := range map[string]string{"A": "1", "B": "2", "C": "3"} {
		if v, ok := f2.Get(k); !ok || v != want {
			t.Errorf("re-parsed %s = (%q, %v), want (%q, true)", k, v, ok, want)
		}
	}
}

func TestMarshal_SetAppendsToEmptyFile(t *testing.T) {
	f, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	f.Set("A", "1")
	got, _ := f.Marshal()
	if string(got) != "A=1\n" {
		t.Errorf("got %q", got)
	}
}

func TestMarshal_SetUpdatePreservesLineEnding(t *testing.T) {
	f, err := Parse([]byte("A=1\r\nB=2\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("A", "updated")
	got, _ := f.Marshal()
	if string(got) != "A=updated\r\nB=2\r\n" {
		t.Errorf("got %q, want %q", got, "A=updated\r\nB=2\r\n")
	}
}

func TestMarshal_SetAppendMatchesLineEnding(t *testing.T) {
	f, err := Parse([]byte("A=1\r\nB=2\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("C", "3")
	got, _ := f.Marshal()
	if string(got) != "A=1\r\nB=2\r\nC=3\r\n" {
		t.Errorf("got %q, want %q", got, "A=1\r\nB=2\r\nC=3\r\n")
	}
}

func TestSet_PanicsOnInvalidKey(t *testing.T) {
	f, _ := Parse([]byte("A=1\n"))
	for _, bad := range []string{"", "1FOO", "HAS SPACE", "A=B", "A#B", "-LEAD", `A"B`, "A\nB"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Set(%q, ...) did not panic", bad)
				}
			}()
			f.Set(bad, "x")
		}()
	}
	// A relaxed-name key (hyphen/dot) is accepted by Set.
	for _, ok := range []string{"OK", "_x", "with-hyphen", "with.dot"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Set(%q, ...) panicked: %v", ok, r)
				}
			}()
			f.Set(ok, "v")
		}()
	}
}

func TestMarshal_SetQuotesValueWhenNeeded(t *testing.T) {
	src := "A=1\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("B", "value with spaces")
	got, _ := f.Marshal()
	if !strings.Contains(string(got), `B="value with spaces"`) {
		t.Errorf("got %q (should be double-quoted)", got)
	}
}

func TestMarshal_DeleteRemovesEntry(t *testing.T) {
	src := "A=1\nB=2\nC=3\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Delete("B") {
		t.Error("Delete returned false")
	}
	got, _ := f.Marshal()
	if string(got) != "A=1\nC=3\n" {
		t.Errorf("got %q", got)
	}
}

// --- nil safety ---

func TestMarshal_NilFile(t *testing.T) {
	var f *File
	got, err := f.Marshal()
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// --- helpers ---

func TestNeedsQuoting(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"plain", false},
		{"with space", true},
		{"with#hash", true},
		{"with$dollar", true},
		{`has"quote`, true},
		{`has'apos`, true},
		{`has\bs`, true},
		{"has`bt", true},
		{"has\nNL", true},
		{"\x01ctrl", true},
	}
	for _, c := range cases {
		if got := needsQuoting(c.in); got != c.want {
			t.Errorf("needsQuoting(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestQuoteValue(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", `"plain"`},
		{`with"quote`, `"with\"quote"`},
		{"with\nnewline", `"with\nnewline"`},
		{"with\\bs", `"with\\bs"`},
		{"$expand", `"\$expand"`},
		{"\b\f\r\t", `"\b\f\r\t"`},
	}
	for _, c := range cases {
		if got := quoteValue(c.in); got != c.want {
			t.Errorf("quoteValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatValue(t *testing.T) {
	if got := formatValue("plain"); got != "plain" {
		t.Errorf("got %q", got)
	}
	if got := formatValue("with space"); got != `"with space"` {
		t.Errorf("got %q", got)
	}
}
