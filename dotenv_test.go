package dotenv

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Load (file → map) ---

func TestLoad_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	contents := "FOO=bar\nBAZ=qux\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["FOO"] != "bar" || got["BAZ"] != "qux" {
		t.Errorf("got %+v", got)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/no/such/file.env")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoad_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("123FOO=bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if !errors.Is(err, ErrParse) {
		t.Errorf("err = %v, want ErrParse", err)
	}
}

// --- Parse (bytes → File) ---

func TestParse_HappyPath(t *testing.T) {
	f, err := Parse([]byte("FOO=bar\n"))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := f.Get("FOO"); v != "bar" {
		t.Errorf("got %q", v)
	}
}

// --- NewSource (file → *Source) ---

func TestNewSource_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=from-file\nEMPTY=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := NewSource(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := src.Lookup("FOO"); !ok || v != "from-file" {
		t.Errorf("got (%q, %v)", v, ok)
	}
	if v, ok := src.Lookup("EMPTY"); !ok || v != "" {
		t.Errorf("EMPTY = (%q, %v); want (\"\", true) — set-but-empty", v, ok)
	}
	if _, ok := src.Lookup("MISSING"); ok {
		t.Error("missing key returned ok=true")
	}
	if got := src.ToMap()["FOO"]; got != "from-file" {
		t.Errorf("ToMap()[FOO] = %q", got)
	}
}

// TestNewSource_LookupShape pins down that *Source's method set matches what
// an env loader's Source interface expects, so a parsed `.env` file plugs in
// with no adapter and no import edge to github.com/go-rotini/env.
func TestNewSource_LookupShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := NewSource(path)
	if err != nil {
		t.Fatal(err)
	}
	// Structural assertion: the same shape env.Source requires.
	var _ interface {
		Lookup(key string) (value string, ok bool)
	} = src
	var _ VarLookup = src
}

func TestNewSource_FileNotFound(t *testing.T) {
	_, err := NewSource("/no/such/file.env")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewSource_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("123FOO=bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewSource(path)
	if !errors.Is(err, ErrParse) {
		t.Errorf("err = %v, want ErrParse", err)
	}
}

func TestSource_NilSafety(t *testing.T) {
	var s *Source
	if _, ok := s.Lookup("FOO"); ok {
		t.Error("nil *Source Lookup returned ok=true")
	}
	if s.ToMap() != nil {
		t.Error("nil *Source ToMap returned non-nil")
	}
}

// --- NewReaderSource ---

func TestNewReaderSource_HappyPath(t *testing.T) {
	r := strings.NewReader("FOO=from-reader\n")
	src, err := NewReaderSource(r)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := src.Lookup("FOO"); v != "from-reader" {
		t.Errorf("got %q", v)
	}
}

func TestNewReaderSource_ParseError(t *testing.T) {
	r := strings.NewReader("123FOO=bar\n")
	_, err := NewReaderSource(r)
	if !errors.Is(err, ErrParse) {
		t.Errorf("err = %v, want ErrParse", err)
	}
}

// Note: end-to-end "a dotenv source flows through a loader, OSEnv wins over
// the file" coverage lives in github.com/go-rotini/env (and the higher-level
// rotini CLI package), which can import this package; dotenv intentionally
// does not depend on env.
