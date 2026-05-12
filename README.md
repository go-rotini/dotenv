# go-rotini/dotenv

A Go `.env` file parser: a conservative, portable subset of the `.env` dialects in
the wild, with opt-in extensions, a lossless AST for round-trip editing, a linter
that flags non-portable constructs, and adapters that surface a parsed file as a
[`env.Source`](https://pkg.go.dev/github.com/go-rotini/env#Source).

This package is the `.env` parsing layer for [go-rotini/env](https://github.com/go-rotini/env)
and the [rotini](https://github.com/go-rotini/rotini) CLI framework. It imports
`go-rotini/env` for the `Source` and `Position` types; the reverse import would be a
cycle and is not permitted, which keeps `go-rotini/env` free of `.env`-parsing code.

> **Watching / live reload is not in this package.** Re-reading a `.env` file when it
> changes on disk is handled one layer up, by a package that composes
> [`go-rotini/fs`](https://github.com/go-rotini/fs)'s file watcher with `dotenv.Parse`
> and `env.Source` — the same mechanism used for yaml/toml/jsonc/jsonschema config
> files. `dotenv` itself reads the file once at construction; subsequent on-disk
> changes do not propagate. This keeps `dotenv` a zero-third-party-dependency parser.

## Features

- Conservative portable `.env` subset by default; every dialect-specific extension
  (`export ` prefix, YAML-style `KEY: value`, backtick quoting, variable expansion,
  multi-line double-quoted strings, relaxed identifiers) is opt-in via `ParseOption`s.
- Bash-style variable expansion (opt-in): `$VAR`, `${VAR}`, `${VAR:-default}`,
  `${VAR-default}`, `${VAR:?error}`, `\$` to suppress; an optional parent `env.Source`
  (e.g. `env.OSEnv()`) for references not defined in the file.
- Lossless `File` AST: entry order and per-entry source bytes are preserved, so
  `(*File).Marshal` is byte-exact on unmutated input; `Set` re-renders only the
  mutated line.
- `Lint` reports deviations from the portable subset (`export ` prefix, `KEY: value`,
  non-POSIX identifiers, backtick values) as warnings, and parse failures as errors.
- `env.Source` adapters: `NewSource` (from a path) and `NewReaderSource` (from an
  `io.Reader`), so a `.env` file drops straight into an `env` loader's source chain.
- DoS guards: max file size (default 10 MiB), max line length (default 1 MiB),
  expansion depth (default 16), and expansion-cycle detection (`ErrExpansion`).
- Zero third-party dependencies (the only module dependency is `go-rotini/env`).

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

### Use as an `env.Source`

```go
import (
	"github.com/go-rotini/env"
	"github.com/go-rotini/dotenv"
)

src, err := dotenv.NewSource(".env", dotenv.WithExpand())
if err != nil { log.Fatal(err) }

// OSEnv() listed first wins over the .env file (the standard Compose / Heroku
// precedence model). Reverse the order to make the file authoritative.
loader := env.New(env.WithSource(env.OSEnv(), src))

var cfg Config
if err := loader.Load(&cfg); err != nil {
	log.Fatal(env.FormatError(err))
}
```

`dotenv.NewReaderSource(io.Reader, ...ParseOption)` builds a `Source` from an already-open
stream. Both adapters take a snapshot at construction.

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

### Lint

```go
issues, err := dotenv.Lint(data)
for _, iss := range issues {
	fmt.Printf("%s:%d: [%s] %s: %s\n", iss.Pos.File, iss.Pos.Line, iss.Severity, iss.Rule, iss.Message)
}
// err != nil when the input failed to parse; warnings collected before the
// failure are still returned.
```

## Parse options

| Option | Effect |
| --- | --- |
| `WithExpand()` | Enable `$VAR` / `${VAR...}` expansion in unquoted and double-quoted values. |
| `WithStrictExpansion()` | Unset variable references raise `*ExpansionError` instead of expanding to empty. |
| `WithExpansionParent(env.Source)` | Source consulted for references not defined in the file (e.g. `env.OSEnv()`). |
| `WithMaxExpansionDepth(int)` | Bound expansion recursion (default 16). |
| `WithBackticks()` | Accept backtick-quoted values (motdotla/dotenv dialect); treated like single quotes. |
| `WithStrictDotenv()` | Reject non-portable constructs (`KEY: value`, `export ` prefix, non-POSIX identifiers). |
| `WithRelaxedNames()` | Accept identifiers with hyphens, dots, or non-ASCII characters. |
| `WithMaxFileSize(int64)` | Cap total input size (default 10 MiB; ≤0 = unlimited). |
| `WithMaxLineLength(int)` | Cap a single physical line (default 1 MiB; ≤0 = unlimited). |

## Errors

- `*ParseError` (`errors.Is(err, dotenv.ErrParse)`) — malformed `.env` source; carries
  file path and line/column.
- `*ExpansionError` (`errors.Is(err, dotenv.ErrExpansion)`) — unset variable under
  strict expansion, a `${VAR:?msg}` assertion failure, depth-bound exceeded, or a cycle.
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
