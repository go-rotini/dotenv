package dotenv

import (
	"slices"
	"testing"
)

func TestFile_GetSetDelete(t *testing.T) {
	f := mustParse(t, "FOO=1\nBAR=2\n")

	if v, ok := f.Get("FOO"); !ok || v != "1" {
		t.Errorf("Get(FOO) = (%q, %v)", v, ok)
	}

	// Set existing key.
	f.Set("FOO", "updated")
	if v, _ := f.Get("FOO"); v != "updated" {
		t.Errorf("after Set: %q", v)
	}

	// Set new key (appended at end).
	f.Set("NEW", "added")
	if v, _ := f.Get("NEW"); v != "added" {
		t.Errorf("after new Set: %q", v)
	}

	// Delete existing.
	if !f.Delete("BAR") {
		t.Error("Delete returned false for existing key")
	}
	if _, ok := f.Get("BAR"); ok {
		t.Error("BAR still present after Delete")
	}

	// Delete non-existing.
	if f.Delete("NOPE") {
		t.Error("Delete returned true for non-existing key")
	}
}

func TestFile_LastWriteWins(t *testing.T) {
	f := mustParse(t, "FOO=first\nFOO=second\n")
	if v, _ := f.Get("FOO"); v != "second" {
		t.Errorf("last-write-wins: got %q", v)
	}
}

func TestFile_Keys(t *testing.T) {
	f := mustParse(t, "A=1\nB=2\nC=3\nA=4\n")
	got := f.Keys()
	want := []string{"A", "B", "C"} // first-occurrence order; no duplicates
	if !slices.Equal(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
}

func TestFile_ToMap(t *testing.T) {
	f := mustParse(t, "A=1\nB=2\nA=overridden\n")
	got := f.ToMap()
	want := map[string]string{"A": "overridden", "B": "2"}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ToMap()[%s] = %q, want %q", k, got[k], v)
		}
	}
}

func TestFile_NilSafety(t *testing.T) {
	var f *File
	if v, ok := f.Get("FOO"); ok || v != "" {
		t.Errorf("nil File Get = (%q, %v)", v, ok)
	}
	if f.Delete("FOO") {
		t.Error("nil File Delete returned true")
	}
	if k := f.Keys(); k != nil {
		t.Errorf("nil File Keys = %v", k)
	}
	if m := f.ToMap(); m != nil {
		t.Errorf("nil File ToMap = %v", m)
	}
	if e := f.Entries(); e != nil {
		t.Errorf("nil File Entries = %v", e)
	}
	if p := f.Path(); p != "" {
		t.Errorf("nil File Path = %q", p)
	}
	f.Set("FOO", "x") // should not panic
}

func TestFile_EntriesAndPath(t *testing.T) {
	f := mustParse(t, "FOO=1\n# comment\n")
	if len(f.Entries()) != 2 {
		t.Errorf("got %d entries", len(f.Entries()))
	}
	// Path is empty for in-memory parses.
	if f.Path() != "" {
		t.Errorf("Path = %q", f.Path())
	}
}

func TestIsValidKey(t *testing.T) {
	valid := []string{"A", "_", "_x", "FOO", "f00", "FOO_BAR", "with-hyphen", "with.dot", "naïve", "Ω"}
	invalid := []string{"", "1FOO", "0", "-LEAD", ".lead", "HAS SPACE", "HAS\tTAB", "A=B", "A:B", "A#B", `A"B`, "A'B", "A`B", "A$B", "A\\B", "A\nB", "A\x00B"}
	for _, k := range valid {
		if !IsValidKey(k) {
			t.Errorf("IsValidKey(%q) = false, want true", k)
		}
	}
	for _, k := range invalid {
		if IsValidKey(k) {
			t.Errorf("IsValidKey(%q) = true, want false", k)
		}
	}
	// Keys IsValidKey accepts must round-trip through Parse(WithRelaxedNames).
	for _, k := range valid {
		f, err := Parse([]byte(k+"=v\n"), WithRelaxedNames())
		if err != nil {
			t.Errorf("Parse(%q=v) with WithRelaxedNames: %v", k, err)
			continue
		}
		if got, ok := f.Get(k); !ok || got != "v" {
			t.Errorf("round-trip of key %q: Get = (%q, %v)", k, got, ok)
		}
	}
}

func TestTrailingLineEnding(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"x", ""},
		{"x\n", "\n"},
		{"x\r\n", "\r\n"},
		{"x\r", "\r"},
		{"\n", "\n"},
		{"\r\n", "\r\n"},
		{"\r", "\r"},
	}
	for _, c := range cases {
		if got := trailingLineEnding([]byte(c.in)); got != c.want {
			t.Errorf("trailingLineEnding(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFile_SetWithDuplicates(t *testing.T) {
	f := mustParse(t, "A=1\nB=x\nA=2\n# c\nA=3\n")
	f.Set("A", "final")
	// Get / ToMap must agree with the value just set.
	if v, _ := f.Get("A"); v != "final" {
		t.Errorf("Get(A) = %q after Set, want final", v)
	}
	if v := f.ToMap()["A"]; v != "final" {
		t.Errorf("ToMap()[A] = %q, want final", v)
	}
	// The first A line is updated in place; later A lines are removed; B and
	// the comment are kept.
	out, _ := f.Marshal()
	if string(out) != "A=final\nB=x\n# c\n" {
		t.Errorf("Marshal = %q, want %q", out, "A=final\nB=x\n# c\n")
	}
}

func TestFile_Source(t *testing.T) {
	f := mustParse(t, "A=hello\nB=${A}-world\n", WithExpand())
	src := f.Source()
	if v, ok := src.Lookup("B"); !ok || v != "hello-world" {
		t.Errorf("Source().Lookup(B) = (%q, %v), want (hello-world, true)", v, ok)
	}
	// Snapshot: a later Set on the File does not affect the returned Source.
	f.Set("B", "changed")
	if v, _ := src.Lookup("B"); v != "hello-world" {
		t.Errorf("Source is not a snapshot: B = %q after Set", v)
	}
	// A nil *File yields a usable (empty) Source.
	var nilF *File
	if _, ok := nilF.Source().Lookup("X"); ok {
		t.Error("nil File Source Lookup returned ok=true")
	}
}

func TestMapSource(t *testing.T) {
	m := map[string]string{"X": "1", "Y": "2"}
	src := MapSource(m)
	if v, ok := src.Lookup("X"); !ok || v != "1" {
		t.Errorf("MapSource Lookup(X) = (%q, %v)", v, ok)
	}
	// Independent copy: mutating m does not affect the Source.
	m["X"] = "mutated"
	if v, _ := src.Lookup("X"); v != "1" {
		t.Errorf("MapSource is not a copy: X = %q", v)
	}
	// Usable as a WithExpansionParent.
	f := mustParse(t, "FOO=${P}\n", WithExpand(), WithExpansionParent(MapSource(map[string]string{"P": "from-parent"})))
	if v, _ := f.Get("FOO"); v != "from-parent" {
		t.Errorf("FOO = %q via MapSource parent, want from-parent", v)
	}
	// nil map → empty Source.
	if _, ok := MapSource(nil).Lookup("X"); ok {
		t.Error("MapSource(nil) Lookup returned ok=true")
	}
}

func TestSource_Keys(t *testing.T) {
	src := MapSource(map[string]string{"C": "3", "A": "1", "B": "2"})
	if got := src.Keys(); !slices.Equal(got, []string{"A", "B", "C"}) {
		t.Errorf("Keys() = %v, want [A B C]", got)
	}
	var nilS *Source
	if k := nilS.Keys(); k != nil {
		t.Errorf("nil Source Keys = %v", k)
	}
}

func TestFile_EntriesIsACopy(t *testing.T) {
	f := mustParse(t, "A=1\n")
	f.Entries()[0].Value = "2"
	if v, _ := f.Get("A"); v != "1" {
		t.Errorf("Get = %q after mutating Entries()", v)
	}
}
