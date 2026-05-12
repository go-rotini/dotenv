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

- Conservative portable `.env` subset by default; every dialect-specific extension
  (`export ` prefix, YAML-style `KEY: value`, backtick quoting, variable expansion,
  multi-line double-quoted strings, relaxed identifiers) is opt-in via `ParseOption`s.
- Bash-style variable expansion (opt-in): `$VAR`, `${VAR}`, `${VAR:-default}`,
  `${VAR-default}`, `${VAR:?error}`, `\$` to suppress; resolve references not defined
  in the file from the process environment with `WithExpandFromOSEnv()`, or from any
  `VarLookup` (e.g. `env.OSEnv()`, `env.Map(...)`) via `WithExpansionParent`.
- Lossless `File` AST: entry order and per-entry source bytes are preserved, so
  `(*File).Marshal` is byte-exact on unmutated input; `Set`/`Delete` re-render only
  the affected line. (`Set` panics on a syntactically invalid key — see `IsValidKey`.
  Variable expansion done at parse time is not reflected by `Marshal`; use `ToMap`.)
- `Lint` reports deviations from the portable subset (`export ` prefix, `KEY: value`,
  non-POSIX identifiers, backtick values) as warnings, and parse failures as errors.
- `Source` adapters: `NewSource` (from a path) and `NewReaderSource` (from an
  `io.Reader`) return a `*Source` that any env loader accepts as a `Source`.
- DoS guards: max file size (default 10 MiB), max line length (default 1 MiB), and a
  bounded expansion-recursion depth (default 16) that catches deeply nested /
  self-referential `${VAR:-default}` chains.
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
stream. Both take a snapshot at construction. `(*Source).Lookup` and `(*Source).ToMap`
are the only methods.

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

`Set` panics if the key isn't a valid `.env` key (`dotenv.IsValidKey` is the predicate).
It updates an existing assignment in place — preserving that line's original line ending —
or appends a new one (terminating the previous last line first if the source lacked a
trailing newline). `Marshal` reflects `Set`/`Delete` mutations, not variable expansion
performed at parse time (use `ToMap`/`Get` for expanded values).

### Lint

```go
issues, err := dotenv.Lint(data)
for _, iss := range issues {
	fmt.Printf("%s: [%s] %s: %s\n", iss.Pos, iss.Severity, iss.Rule, iss.Message)
}
// err != nil when the input failed to parse; warnings collected before the
// failure are still returned.
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
| `WithStrictDotenv()` | Reject non-portable constructs (`KEY: value`, `export ` prefix, non-POSIX identifiers). |
| `WithRelaxedNames()` | Accept identifiers with hyphens, dots, or non-ASCII characters. |
| `WithMaxFileSize(int64)` | Cap total input size (default 10 MiB; ≤0 = unlimited). |
| `WithMaxLineLength(int)` | Cap a single physical line (default 1 MiB; ≤0 = unlimited). |

## Errors

- `*ParseError` (`errors.Is(err, dotenv.ErrParse)`) — malformed `.env` source; carries
  file path and line/column (`ParseError.Pos`, a `dotenv.Position`).
- `*ExpansionError` (`errors.Is(err, dotenv.ErrExpansion)`) — variable-expansion failure.
  Its `Cause` is `dotenv.ErrUnsetVariable` for an unset reference under strict expansion,
  `dotenv.ErrAssertionFailed` for a failed `${VAR:?msg}` assertion, or `nil` when the
  `WithMaxExpansionDepth` bound was exceeded.
- `dotenv.ErrFileTooLarge`, `dotenv.ErrLineTooLong` — resource-limit violations.

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
