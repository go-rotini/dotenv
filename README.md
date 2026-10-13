# go-rotini/dotenv

A Go `.env` file parser: a conservative, portable subset of the `.env` dialects in
the wild, with opt-in extensions, a lossless AST for round-trip editing, a linter
that flags non-portable constructs, and a `Source` that drops straight into an
environment-variable loader.

This package is the `.env` parsing layer for [go-rotini/env](https://github.com/go-rotini/env)
and the [rotini](https://github.com/go-rotini/rotini) CLI framework — but it has
**no dependencies**: nothing outside the standard library, and no other go-rotini
module either. `dotenv.Source` is a small struct whose `Lookup` method is
structurally compatible with `go-rotini/env`'s `Source` interface, so a parsed
`.env` file plugs into an env loader's source chain without dotenv importing env;
likewise `WithExpansionParent` takes a one-method `VarLookup` interface that any
`env.Source` (e.g. `env.OSEnv()`) satisfies.

> **Watching / live reload is not in this package.** Re-reading a `.env` file when it
> changes on disk is handled one layer up, by a package that composes
> [`go-rotini/fs`](https://github.com/go-rotini/fs)'s file watcher with `dotenv.Parse` —
> the same mechanism used for yaml/toml/jsonc/jsonschema config files. `dotenv` itself
> reads the file once at construction; subsequent on-disk changes do not propagate.

## Features

- Accepts the common forms by default: `KEY=value`, the `export ` prefix, YAML-style
  `KEY: value`, single- and double-quoted values (both may span lines; a line break inside
  quotes reads as LF, even in a CRLF file), and trailing
  `# comments`. Backtick quoting, variable expansion and relaxed identifiers are
  opt-in via `ParseOption`s; `WithStrictDotenv()` rejects `export ` and `KEY: value`.
- Bash-style variable expansion (opt-in): `$VAR`, `${VAR}`, `${VAR:-default}`,
  `${VAR-default}`, `${VAR:?error}`, `${VAR?error}`, `${VAR:+alt}`, `${VAR+alt}`,
  `\$` to suppress; resolve references not defined in the file from the process
  environment with `WithExpandFromOSEnv()`, or from any `VarLookup` (e.g.
  `env.OSEnv()`, `env.Map(...)`) via `WithExpansionParent`. The file's own entries
  take precedence over those fallbacks, including single-quoted ones.
- Lossless `File` AST: entry order and per-entry source bytes are preserved, so
  `(*File).Marshal` is byte-exact on unmutated input; `Set`/`Delete` re-render only
  the affected line. (`Set` panics on a syntactically invalid key — see `IsValidKey`.
  Variable expansion done at parse time is not reflected by `Marshal`; use `ToMap`.)
- Write back out: `Marshal(map[string]string) ([]byte, error)` renders a map as `.env`
  bytes (sorted keys, minimal quoting); `Write(path, map)` does that plus a `0o600` write.
- `Lint` reports deviations from the portable subset (`export ` prefix, `KEY: value`,
  non-POSIX identifiers, backtick values) as warnings, and parse failures as errors.
- `Source` adapters: `NewSource` (path), `NewReaderSource` (`io.Reader`), `MapSource` (map),
  and `(*File).Source` — all return a `*Source` that any env loader accepts as a `Source`.
- DoS guards: max file size (default 10 MiB; files and readers are read only up to
  it), max line length (default 1 MiB), a cap on the bytes expansion may substitute
  (default: the file-size limit), and a bounded expansion-recursion depth (default 16)
  that catches deeply nested / self-referential `${VAR:-default}` chains.
- Zero dependencies — `go.mod` has no `require` line at all (only the dev `tool` block).

## Installation

```bash
go get github.com/go-rotini/dotenv
```

Requires Go 1.26 or later.

## Quick start

### Parse to a map

```go
import "github.com/go-rotini/dotenv"

m, err := dotenv.Load(".env", dotenv.WithExpand())
if err != nil { log.Fatal(err) }
fmt.Println(m["DATABASE_URL"])
```

`dotenv.Parse([]byte, ...ParseOption)` does the same from a byte slice and returns the
`*File` AST.

### Use as a loader source

`dotenv.NewSource` returns a `*dotenv.Source`, which satisfies `go-rotini/env`'s
`Source` interface structurally (no import edge needed):

```go
import (
	"github.com/go-rotini/env"
	"github.com/go-rotini/dotenv"
)

src, err := dotenv.NewSource(".env", dotenv.WithExpandFromOSEnv())
if err != nil { log.Fatal(err) }

// OSEnv() listed first wins over the .env file (the standard Compose / Heroku
// precedence model). Reverse the order to make the file authoritative.
loader := env.New(env.WithSource(env.OSEnv(), src))

var cfg Config
if err := loader.Load(&cfg); err != nil {
	log.Fatal(env.FormatError(err))
}
```

`dotenv.NewReaderSource(io.Reader, ...ParseOption)` builds a `*Source` from an already-open
stream; `dotenv.MapSource(map[string]string)` from an in-memory map (handy as a
`WithExpansionParent`). All take a snapshot at construction. `*Source` has `Lookup`,
`Keys`, and `ToMap`. If you already have a `*File` (from `Parse`) and also want a loader
source, call `f.Source()` — no re-parsing.

### Lossless editing

```go
data, _ := os.ReadFile(".env")
f, err := dotenv.Parse(data)
if err != nil { log.Fatal(err) }

f.Set("FEATURE_FLAGS", "feature-a,feature-b") // re-renders only this line
f.Delete("OBSOLETE_KEY")

out, _ := f.Marshal() // byte-exact for every untouched line
os.WriteFile(".env", out, 0o600)
```

`Set` panics if the key isn't a valid `.env` key (`dotenv.IsValidKey` is the predicate;
the panic value wraps `dotenv.ErrInvalidKey`). It updates the **first** assignment of the
key in place — preserving that line's original line ending — and removes any later
assignments of the same key, so `Get`/`ToMap` agree with the value just set; if the key is
absent it appends a new line (matching the source's line ending, and terminating the
previous last line first if the source lacked a trailing newline). `(*File).Marshal`
reflects `Set`/`Delete` mutations, not variable expansion performed at parse time (use
`ToMap`/`Get` for expanded values).

### Write a `.env` file from a map

```go
m := map[string]string{"PORT": "8080", "GREETING": "hello world"}

b, err := dotenv.Marshal(m)            // []byte: sorted keys, LF endings, minimal quoting
if err != nil { log.Fatal(err) }       // err (wrapping ErrInvalidKey) if a key is invalid
_ = b

if err := dotenv.Write(".env", m); err != nil { // Marshal + write with 0o600 (not atomic)
	log.Fatal(err)
}
```

### Lint

```go
issues, err := dotenv.Lint(data)
for _, iss := range issues {
	fmt.Printf("%s: [%s] %s: %s\n", iss.Pos, iss.Severity, iss.Rule, iss.Message)
}
// err != nil when the input failed to parse; warnings for the entries before
// the failure are still returned.
```

## Parse options

| Option | Effect |
| --- | --- |
| `WithExpand()` | Enable `$VAR` / `${VAR...}` expansion in unquoted and double-quoted values. |
| `WithExpandFromOSEnv()` | Like `WithExpand()`, and resolve references not defined in the file from the process environment (`os.LookupEnv`). |
| `WithExpansionParent(VarLookup)` | Resolve references not defined in the file from a `VarLookup` (e.g. `env.OSEnv()`, `env.Map(...)`). Requires `WithExpand()` or `WithExpandFromOSEnv()`. |
| `WithStrictExpansion()` | Unset variable references raise `*ExpansionError` (cause `ErrUnsetVariable`) instead of expanding to empty. |
| `WithMaxExpansionDepth(int)` | Bound expansion recursion (default 16). |
| `WithBackticks()` | Accept backtick-quoted values (motdotla/dotenv dialect); treated like single quotes. |
| `WithStrictDotenv()` | Reject the `export ` prefix and `KEY: value` separators (does not override `WithRelaxedNames()` or `WithBackticks()`). |
| `WithRelaxedNames()` | Accept identifiers with hyphens, dots, or non-ASCII characters. Under expansion `${MY-KEY}` reads as "MY, default `KEY`", so a hyphenated key cannot be referenced. |
| `WithMaxFileSize(int64)` | Cap total input size; files and readers are read only up to it (default 10 MiB; ≤0 or `math.MaxInt64` = unlimited). |
| `WithMaxExpandedSize(int64)` | Cap the total bytes expansion substitutes (default: the file-size limit, or 10 MiB if that is unlimited; ≤0 = unlimited). |
| `WithMaxLineLength(int)` | Cap a single entry, line ending excluded (default 1 MiB; ≤0 = unlimited). |

## Errors

- `*ParseError` (`errors.Is(err, dotenv.ErrParse)`) — malformed `.env` source; carries
  file path and line/column (`ParseError.Pos`, a `dotenv.Position`).
- `*ExpansionError` (`errors.Is(err, dotenv.ErrExpansion)`) — variable-expansion failure.
  Its `Cause` is `dotenv.ErrUnsetVariable` for an unset reference under strict expansion,
  `dotenv.ErrAssertionFailed` for a failed `${VAR:?msg}` assertion,
  `dotenv.ErrMaxExpansionDepth` when the `WithMaxExpansionDepth` bound was exceeded,
  `dotenv.ErrBadSubstitution` for a `${...}` body that is not a reference, or
  `dotenv.ErrExpansionTooLarge` when the `WithMaxExpandedSize` cap was exceeded.
- `dotenv.ErrFileTooLarge`, `dotenv.ErrLineTooLong` — resource-limit violations, the
  `Cause` of a `*ParseError` that carries the position.
- `dotenv.ErrInvalidKey` — wrapped by the error `Marshal`/`Write` return (and by the value
  `(*File).Set` panics with) when a key isn't a valid `.env` identifier.

## Documentation

Full API reference is available on [pkg.go.dev](https://pkg.go.dev/github.com/go-rotini/dotenv).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to contribute to this project.

## Code of Conduct

This project follows a code of conduct to ensure a welcoming community. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Security

To report a vulnerability, see [SECURITY.md](SECURITY.md).

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
