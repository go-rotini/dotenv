// Package dotenv parses `.env` files into a lossless AST, a key/value map, or
// a [*Source] suitable for an environment-variable loader.
//
// The package targets a conservative portable subset of the `.env` dialects
// in the wild. Every dialect-specific extension (the `export ` prefix,
// YAML-style `KEY: value`, backtick quoting, variable expansion, multi-line
// double-quoted strings) is supported but gated behind explicit ParseOptions
// or, for non-portable constructs that nonetheless parse, surfaced as
// [LintIssue] SeverityWarning entries from [Lint].
//
// # API summary
//
//   - [Parse] / [Load]                — bytes / file → AST or key/value map.
//   - [NewSource] / [NewReaderSource] / [MapSource] — file / reader / map → [*Source].
//   - [Lint]                          — flag non-portable constructs.
//   - [File].Get/Set/Delete/Keys/ToMap — AST navigation and mutation.
//   - [File].Source                   — snapshot the AST's effective map as a [*Source].
//   - [File].Marshal                  — byte-exact round-trip emission of an AST.
//   - [Marshal] / [Write]             — map → `.env` bytes / file.
//   - [IsValidKey]                    — predicate for keys [File.Set] / [Marshal] accept.
//
// # Dependencies
//
// dotenv has no dependencies — not the standard library only in spirit, but
// literally: nothing outside the standard library, and no other go-rotini
// module either. Its [Source] is a struct whose single Lookup method is
// structurally compatible with the Source interface in
// github.com/go-rotini/env, so a parsed `.env` file drops straight into an
// env loader's source chain without dotenv importing env. Likewise
// [WithExpansionParent] takes the small [VarLookup] interface, which any
// env.Source (e.g. env.OSEnv()) satisfies; for the common case there is also
// [WithExpandFromOSEnv], which uses only [os.LookupEnv].
//
// # File watching
//
// This package does not watch files. It reads the source once at construction
// ([NewSource] / [NewReaderSource] / [Load]); subsequent on-disk changes do
// not propagate. Live reload of a `.env` file is the job of a higher-level
// package that composes github.com/go-rotini/fs's file watcher with [Parse] —
// the same arrangement used for yaml/toml/jsonc/jsonschema config files — so
// the watching dependency lives in exactly one place.
package dotenv
