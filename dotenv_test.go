package dotenv

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
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

func TestParse_MaxLineLengthDisabled(t *testing.T) {
	src := []byte("FOO=a-fairly-long-value\n") // 24 bytes
	if _, err := Parse(src, WithMaxLineLength(5)); !errors.Is(err, ErrLineTooLong) {
		t.Errorf("err = %v, want ErrLineTooLong with limit 5", err)
	}
	for _, n := range []int{0, -1} {
		if _, err := Parse(src, WithMaxLineLength(n)); err != nil {
			t.Errorf("WithMaxLineLength(%d) should disable the limit: %v", n, err)
		}
	}
}

func TestParse_MaxFileSizeDisabled(t *testing.T) {
	src := []byte("FOO=value\n") // 10 bytes
	if _, err := Parse(src, WithMaxFileSize(5)); !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("err = %v, want ErrFileTooLarge with limit 5", err)
	}
	for _, n := range []int64{0, -1} {
		if _, err := Parse(src, WithMaxFileSize(n)); err != nil {
			t.Errorf("WithMaxFileSize(%d) should disable the limit: %v", n, err)
		}
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

func TestNewReaderSource_ReadError(t *testing.T) {
	boom := errors.New("boom")
	_, err := NewReaderSource(iotest.ErrReader(boom))
	if err == nil {
		t.Fatal("expected an error from a failing reader")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the reader error", err)
	}
}

// Note: end-to-end "a dotenv source flows through a loader, OSEnv wins over
// the file" coverage lives in github.com/go-rotini/env (and the higher-level
// rotini CLI package), which can import this package; dotenv intentionally
// does not depend on env.

// countingReader yields an endless stream of "A=1\n" and counts bytes read.
type countingReader struct{ n int64 }

func (r *countingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = "A=1\n"[(r.n+int64(i))%4]
	}
	r.n += int64(len(p))
	return len(p), nil
}

func TestNewReaderSource_ReadsOnlyUpToLimit(t *testing.T) {
	r := &countingReader{}
	_, err := NewReaderSource(r, WithMaxFileSize(1024))
	if !errors.Is(err, ErrFileTooLarge) || !errors.Is(err, ErrParse) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
	if r.n > 64*1024 {
		t.Errorf("read %d bytes for a 1024-byte limit", r.n)
	}
}

func TestLoad_RegularFileOverLimitNotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.env")
	if err := os.WriteFile(path, []byte(strings.Repeat("A=1\n", 100)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path, WithMaxFileSize(10))
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error lacks the path: %v", err)
	}
	if _, err := Load(path, WithMaxFileSize(400)); err != nil {
		t.Errorf("at the limit: %v", err)
	}
}

func TestLimitErrorsCarryPosition(t *testing.T) {
	_, err := Load(writeTemp(t, "A=1\nB=0123456789\n"), WithMaxLineLength(5))
	pe, ok := errors.AsType[*ParseError](err)
	if !ok || !errors.Is(err, ErrLineTooLong) {
		t.Fatalf("err = %v, want *ParseError wrapping ErrLineTooLong", err)
	}
	if pe.Pos.Line != 2 || pe.Pos.Column != 1 || pe.Pos.File == "" {
		t.Errorf("Pos = %+v, want file, line 2, column 1", pe.Pos)
	}
	if strings.Count(err.Error(), "dotenv:") != 1 {
		t.Errorf("message = %q", err.Error())
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMaxInt64FileSizeIsUnlimited(t *testing.T) {
	src, err := NewReaderSource(strings.NewReader("A=1\n"), WithMaxFileSize(math.MaxInt64))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := src.Lookup("A"); !ok || v != "1" {
		t.Errorf("reader: A = %q, %v", v, ok)
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path, WithMaxFileSize(math.MaxInt64))
	if err != nil || m["A"] != "1" {
		t.Errorf("Load = %v, %v", m, err)
	}
}

func TestLoadReadErrorPrefixedOnce(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("want an error reading a directory")
	}
	if n := strings.Count(err.Error(), "dotenv:"); n != 1 {
		t.Errorf("err = %q, want one dotenv: prefix", err)
	}
	_, err = NewReaderSource(iotest.ErrReader(errors.New("boom")))
	if err == nil || err.Error() != "dotenv: read: boom" {
		t.Errorf("reader err = %v", err)
	}
}
