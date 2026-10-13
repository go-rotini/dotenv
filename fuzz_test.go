package dotenv_test

import (
	"bytes"
	"maps"
	"strings"
	"testing"

	"github.com/go-rotini/dotenv"
)

// Fuzz targets for the dotenv package. The contract is:
//
//  1. No panic for any input (Set is exempt — it panics on an invalid
//     key by contract; the edit fuzzer only ever passes valid keys).
//  2. No infinite loop.
//  3. For round-trip fuzzers: Parse → (edit) → Marshal → Parse
//     produces a consistent effective key/value map.
//
// Seeds are supplied in code via f.Add; `go test -fuzz` writes any
// minimized crashers it finds to testdata/fuzz/{TestName}/.

// scannerSeeds covers BOM/CRLF/quoting/escape/expansion forms plus
// known regression cases.
var scannerSeeds = [][]byte{
	[]byte(""),
	[]byte("\n"),
	[]byte("FOO=bar\n"),
	[]byte("export FOO=bar\n"),
	[]byte("FOO: bar\n"),
	[]byte("FOO=\"line1\\nline2\"\n"),
	[]byte("FOO='single $literal'\n"),
	[]byte("FOO=`backtick`\n"),
	[]byte("FOO=value # trailing comment\n"),
	[]byte("# top comment\nFOO=bar\n"),
	[]byte("FOO=\"multi\nline\"\n"),
	{0xEF, 0xBB, 0xBF, 'F', 'O', 'O', '=', 'b', 'a', 'r', '\n'}, // UTF-8 BOM
	[]byte("FOO=line1\r\nBAR=line2\r"),                          // mixed line endings
	[]byte("FOO=\\$NOT_EXPANDED\n"),
	[]byte("FOO=${BAR:-default}\n"),
	[]byte("FOO=${BAR:?required}\n"),
	[]byte("FOO=${A:-${B:-${C}}}\n"), // nested defaults
	[]byte("\xff\xfe\xfd"),           // garbage bytes
}

// FuzzScanner exercises the parser with default options. Any panic
// in the tokenizer or AST construction shows up here.
func FuzzScanner(f *testing.F) {
	for _, s := range scannerSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = dotenv.Parse(data)
	})
}

// FuzzParser exercises the parser with all dialect-relaxing options
// enabled simultaneously. Catches interaction bugs between options
// the default-options FuzzScanner won't reach.
func FuzzParser(f *testing.F) {
	for _, s := range scannerSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = dotenv.Parse(data,
			dotenv.WithBackticks(),
			dotenv.WithRelaxedNames(),
		)
	})
}

// FuzzExpand exercises the expansion pass. Inputs that hit the
// max-depth cap, nested defaults, assertion forms, and escape
// suppression all flow through here.
func FuzzExpand(f *testing.F) {
	for _, s := range [][]byte{
		[]byte("A=hello\nB=$A\n"),
		[]byte("A=hello\nB=${A}_world\n"),
		[]byte("A=hello\nB=\"prefix-${A}-suffix\"\n"),
		[]byte("A=\nB=${A:-fallback}\n"),
		[]byte("A=set\nB=${A:?required}\n"),
		[]byte("A=${MISSING:-${ALSO_MISSING:-final}}\n"),
		[]byte("A=${B:-${C:-${D:-${E:-final}}}}\n"),
		[]byte("A=\\${LITERAL}\n"),
		[]byte("A=$\nB=$.\nC=${UNCLOSED\n"),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = dotenv.Parse(data, dotenv.WithExpand())
		_, _ = dotenv.Parse(data, dotenv.WithExpand(), dotenv.WithStrictExpansion())
		_, _ = dotenv.Parse(data, dotenv.WithExpand(), dotenv.WithMaxExpansionDepth(4))

		// Literal text comes from the input, so the expanded values can
		// exceed the input by at most the substitution budget.
		const budget = 1 << 12
		file, err := dotenv.Parse(data, dotenv.WithExpand(), dotenv.WithMaxExpandedSize(budget))
		if err != nil {
			return
		}
		total := 0
		for _, e := range file.Entries() {
			total += len(e.Value)
		}
		if total > len(data)+budget {
			t.Fatalf("expanded to %d bytes from %d input bytes", total, len(data))
		}
	})
}

// FuzzMapRoundTrip checks that a parsed map survives Marshal and a
// reparse, with expansion off and on (Marshal escapes `$`).
func FuzzMapRoundTrip(f *testing.F) {
	for _, s := range []string{"A=1\n", "A='x y'\nB=\"a\\nb\"\n", "a.b-c=1\n", "A=`x`\n", "K: v\n", "A=\"\a$B\"\n"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		first, err := dotenv.Parse(data, dotenv.WithRelaxedNames(), dotenv.WithBackticks())
		if err != nil {
			return
		}
		m := first.ToMap()
		out, err := dotenv.Marshal(m)
		if err != nil {
			t.Fatalf("Marshal of parsed map: %v", err)
		}
		for _, opts := range [][]dotenv.ParseOption{
			{dotenv.WithRelaxedNames()},
			{dotenv.WithRelaxedNames(), dotenv.WithExpand()},
		} {
			second, err := dotenv.Parse(out, opts...)
			if err != nil {
				t.Fatalf("reparse: %v\n%q", err, out)
			}
			if got := second.ToMap(); !maps.Equal(got, m) {
				t.Fatalf("round trip %q -> %q -> %q", m, out, got)
			}
		}
	})
}

// FuzzLintAgrees checks that Lint and Parse agree on whether input
// parses.
func FuzzLintAgrees(f *testing.F) {
	for _, s := range scannerSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, perr := dotenv.Parse(data)
		_, lerr := dotenv.Lint(data)
		if (perr == nil) != (lerr == nil) {
			t.Fatalf("Parse err %v, Lint err %v", perr, lerr)
		}
	})
}

// FuzzRoundTrip is the round-trip property fuzzer. Parse → Marshal
// → Parse must produce the same effective map for any input that
// successfully parsed in the first place.
func FuzzRoundTrip(f *testing.F) {
	for _, s := range [][]byte{
		[]byte("FOO=bar\nBAZ=qux\n"),
		[]byte("# comment\nFOO=bar\n\n"),
		[]byte("export FOO=bar\nBAR='literal'\nBAZ=\"escaped\\n\"\n"),
		[]byte("FOO=\"line1\nline2\"\n"),
		[]byte("FOO=value # trailing\n"),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		first, err := dotenv.Parse(data)
		if err != nil {
			return // ill-formed inputs are out of scope for round-trip.
		}
		marshaled, err := first.Marshal()
		if err != nil {
			t.Fatalf("Marshal failed for previously-parsed input: %v", err)
		}
		second, err := dotenv.Parse(marshaled)
		if err != nil {
			t.Fatalf("re-parse of Marshal output failed: %v\n--- bytes ---\n%s",
				err, string(marshaled))
		}
		firstMap := first.ToMap()
		secondMap := second.ToMap()
		if !equalMaps(firstMap, secondMap) {
			t.Fatalf("round-trip diverged:\n  first=%v\n second=%v",
				firstMap, secondMap)
		}
		// Marshal output must itself be byte-stable for unmutated entries.
		if !bytes.Equal(data, marshaled) {
			// Not a hard failure — quoting normalization can legitimately
			// change byte representation — but the second parse must
			// re-marshal stably.
			again, err := second.Marshal()
			if err != nil {
				t.Fatalf("second Marshal failed: %v", err)
			}
			if !bytes.Equal(marshaled, again) {
				t.Fatalf("second round-trip not byte-stable:\n  once=%q\n twice=%q",
					string(marshaled), string(again))
			}
		}
	})
}

// FuzzMarshalEdit exercises the AST-mutation path the plain FuzzRoundTrip
// never reaches: Parse → Delete → Set → Marshal → re-Parse. The re-parsed
// file must (a) parse without error and (b) report the just-Set key with
// exactly the value it was given. This covers, in particular, appending to
// a source that lacks a trailing newline.
func FuzzMarshalEdit(f *testing.F) {
	for _, s := range [][]byte{
		[]byte("A=1\nB=2\n"),
		[]byte("A=1\nB=2"), // no trailing newline
		[]byte(""),         // empty file
		[]byte("# only a comment"),
		[]byte("A=1\r\nB=2\r\n"), // CRLF
		[]byte("FUZZ_INSERTED_KEY=already-here\nA=1\n"),
		[]byte("A='single'\nB=\"double\\n\"\nC=value # comment\n"),
	} {
		f.Add(s)
	}
	const key = "FUZZ_INSERTED_KEY"
	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := dotenv.Parse(data)
		if err != nil {
			return // ill-formed inputs are out of scope.
		}
		file.Delete(key) // guarantee a single occurrence after Set
		file.Set(key, string(data))

		marshaled, err := file.Marshal()
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		got, err := dotenv.Parse(marshaled)
		if err != nil {
			t.Fatalf("re-parse of Marshal output failed: %v\n--- bytes ---\n%q", err, string(marshaled))
		}
		v, ok := got.Get(key)
		if !ok {
			t.Fatalf("re-parsed file is missing the Set key %q\n--- marshaled ---\n%q", key, string(marshaled))
		}
		if v != string(data) {
			t.Fatalf("Set value did not round-trip:\n want %q\n  got %q\n--- marshaled ---\n%q",
				string(data), v, string(marshaled))
		}
	})
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || va != vb {
			return false
		}
	}
	return true
}

// FuzzEscapedBackslashIsPlain checks that, in a double-quoted value under
// expansion, `\\` behaves like any plain byte: the value with `\\` reads
// as the value with `~` in its place, with each `~` turned back into `\`.
func FuzzEscapedBackslashIsPlain(f *testing.F) {
	for _, s := range []string{`${A:-C:\Temp\}`, `\${A}`, `${A:-${B:-\}}`, `${A\}`, `${A:+x\}y}`, `${A:?\}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if strings.ContainsAny(s, "~\r") {
			return
		}
		enc := func(bs string) string {
			var b strings.Builder
			for i := 0; i < len(s); i++ {
				switch s[i] {
				case '\\':
					b.WriteString(bs)
				case '"':
					b.WriteString(`\"`)
				default:
					b.WriteByte(s[i])
				}
			}
			return "A=a\nB=\"" + b.String() + "\"\n"
		}
		f1, e1 := dotenv.Parse([]byte(enc(`\\`)), dotenv.WithExpand())
		f2, e2 := dotenv.Parse([]byte(enc(`~`)), dotenv.WithExpand())
		if (e1 == nil) != (e2 == nil) {
			t.Fatalf("%q: err %v vs %v", s, e1, e2)
		}
		if e1 != nil {
			return
		}
		v1, _ := f1.Get("B")
		v2, _ := f2.Get("B")
		if v1 != strings.ReplaceAll(v2, "~", `\`) {
			t.Fatalf("%q: %q vs %q", s, v1, v2)
		}
	})
}
