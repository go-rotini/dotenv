// Package dotenv parses `.env` files and exposes them as
// [github.com/go-rotini/env.Source] values.
//
// The package targets a conservative portable subset of the `.env`
// dialects in the wild. Every dialect-specific extension (the
// `export ` prefix, YAML-style `KEY: value`, backtick quoting, variable
// expansion, multi-line double-quoted strings) is supported but gated
// behind explicit ParseOptions or, for non-portable constructs that
// nonetheless parse, surfaced as [LintIssue] SeverityWarning entries
// from [Lint].
//
// # API summary
//
//   - [Parse] / [Load]                — bytes / file → AST or key/value map.
//   - [NewSource] / [NewReaderSource] — file / reader → [env.Source].
//   - [Lint]                          — flag non-portable constructs.
//   - [File].Get/Set/Delete/Keys/ToMap — AST navigation and mutation.
//   - [File].Marshal                  — byte-exact round-trip emission.
//
// # Dependency direction
//
// dotenv imports github.com/go-rotini/env for the [env.Source] and
// [env.Position] types. The reverse import would create a cycle and is
// not permitted. This split keeps the env package free of `.env`
// parsing code so consumers who only need the OS-env source pay no
// transitive cost. dotenv has no third-party dependencies.
//
// # File watching
//
// This package does not watch files. It reads the source once at
// construction ([NewSource] / [NewReaderSource] / [Load]); subsequent
// on-disk changes do not propagate. Live reload of a `.env` file is the
// job of a higher-level package that composes github.com/go-rotini/fs's
// file watcher with [Parse] and [env.Source] — the same arrangement
// used for yaml/toml/jsonc/jsonschema config files — so the watching
// dependency lives in exactly one place.
package dotenv
