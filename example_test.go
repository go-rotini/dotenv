package dotenv_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-rotini/dotenv"
)

// Parse a `.env` byte stream and read values out of the resulting AST.
func Example() {
	f, err := dotenv.Parse([]byte("# app config\nAPP_NAME=demo\nPORT=8080\n"))
	if err != nil {
		log.Fatal(err)
	}
	for _, k := range []string{"APP_NAME", "PORT"} {
		v, _ := f.Get(k)
		fmt.Printf("%s=%s\n", k, v)
	}
	// Output:
	// APP_NAME=demo
	// PORT=8080
}

// A *Source's Lookup method makes a parsed file usable wherever a
// github.com/go-rotini/env Source is expected — no import edge needed.
// (set==true with an empty value means "present but empty".)
func ExampleNewReaderSource() {
	src, err := dotenv.NewReaderSource(strings.NewReader("FOO=bar\nDEBUG=\n"))
	if err != nil {
		log.Fatal(err)
	}
	foo, ok := src.Lookup("FOO")
	fmt.Printf("FOO=%q (set=%t)\n", foo, ok)
	dbg, ok := src.Lookup("DEBUG")
	fmt.Printf("DEBUG=%q (set=%t)\n", dbg, ok)
	_, ok = src.Lookup("MISSING")
	fmt.Printf("MISSING set=%t\n", ok)
	// Output:
	// FOO="bar" (set=true)
	// DEBUG="" (set=true)
	// MISSING set=false
}

// WithExpand turns on Bash-style variable expansion. References resolve
// against the file's already-parsed entries first; `${VAR:-default}` supplies
// a fallback; nested references are expanded recursively.
func ExampleWithExpand() {
	src := "DOMAIN=example.com\n" +
		"API_URL=https://api.${DOMAIN}\n" +
		"ADMIN_EMAIL=${ADMIN_EMAIL:-admin@${DOMAIN}}\n"
	f, err := dotenv.Parse([]byte(src), dotenv.WithExpand())
	if err != nil {
		log.Fatal(err)
	}
	for _, k := range []string{"API_URL", "ADMIN_EMAIL"} {
		v, _ := f.Get(k)
		fmt.Println(k, "=", v)
	}
	// Output:
	// API_URL = https://api.example.com
	// ADMIN_EMAIL = admin@example.com
}

// Edit a `.env` file losslessly: Set updates an existing assignment in place
// (or appends a new one), Delete removes one, and Marshal emits the result —
// comments, quoting and untouched lines are preserved byte-for-byte.
func ExampleFile_Set() {
	f, err := dotenv.Parse([]byte("# generated config\nDB_HOST=localhost\nDB_PORT=5432\nDEBUG=true\n"))
	if err != nil {
		log.Fatal(err)
	}
	f.Set("DB_PORT", "5433") // update in place
	f.Set("DB_NAME", "app")  // append
	f.Delete("DEBUG")
	out, _ := f.Marshal()
	fmt.Print(string(out))
	// Output:
	// # generated config
	// DB_HOST=localhost
	// DB_PORT=5433
	// DB_NAME=app
}

// Lint flags constructs outside the portable `.env` subset (warnings) and any
// parse failure (an error). The input below parses, but uses two non-portable
// forms.
func ExampleLint() {
	data := []byte("export DATABASE_URL=postgres://localhost/app\nPORT: 8080\n")
	issues, err := dotenv.Lint(data)
	fmt.Println("err:", err)
	for _, iss := range issues {
		fmt.Printf("line %d: %s %s\n", iss.Pos.Line, iss.Severity, iss.Rule)
	}
	// Output:
	// err: <nil>
	// line 1: warning export-prefix
	// line 2: warning yaml-separator
}

// Marshal renders a map as `.env` bytes (sorted keys, LF endings, minimal
// quoting). The result re-parses via Parse / Load.
func ExampleMarshal() {
	out, err := dotenv.Marshal(map[string]string{
		"PORT":          "8080",
		"DATABASE_URL":  "postgres://localhost/app",
		"FEATURE_FLAGS": "a,b,c",
		"GREETING":      "hello world", // forces quoting
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(string(out))
	// Output:
	// DATABASE_URL=postgres://localhost/app
	// FEATURE_FLAGS=a,b,c
	// GREETING="hello world"
	// PORT=8080
}

// Write renders a map with Marshal and writes it to a file (0o600).
func ExampleWrite() {
	dir, err := os.MkdirTemp("", "dotenv-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, ".env")
	if err := dotenv.Write(path, map[string]string{"FOO": "bar", "BAZ": "qux"}); err != nil {
		log.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(string(data))
	// Output:
	// BAZ=qux
	// FOO=bar
}
