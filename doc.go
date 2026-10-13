// Package dotenv parses `.env` files into a lossless AST, a key/value map, or
// a [*Source] suitable for an environment-variable loader.
//
// The default dialect accepts the common forms found in the wild: `KEY=value`,
// the `export ` prefix, YAML-style `KEY: value`, single- and double-quoted
// values (both may span lines; a line break inside quotes reads as LF
// whatever the file uses), and trailing `# comments`. Other extensions
// are opt-in [ParseOption] values: variable expansion ([WithExpand]),
// backtick quoting ([WithBackticks]) and non-POSIX key names
// ([WithRelaxedNames]). [WithStrictDotenv] rejects the `export ` prefix and
// YAML-style separators, and [Lint] reports non-portable constructs as
// [SeverityWarning] issues.
//
// # API summary
//
//   - [Parse] / [Load]                — bytes / file → AST or key/value map.
//   - [NewSource] / [NewReaderSource] / [MapSource] — file / reader / map → [*Source].
//   - [Lint]                          — flag non-portable constructs.
//   - [File].Get/Set/Delete/Keys/ToMap — AST navigation and mutation.
//   - [File.Source]                   — snapshot the AST's effective map as a [*Source].
//   - [File.Marshal]                  — byte-exact round-trip emission of an AST.
//   - [Marshal] / [Write]             — map → `.env` bytes / file.
//   - [IsValidKey]                    — predicate for keys [File.Set] / [Marshal] accept.
//
// # Dependencies
//
// dotenv imports nothing outside the standard library, including no other
// go-rotini module. [Source] has a single Lookup method that is structurally
// compatible with the Source interface in github.com/go-rotini/env, so a parsed
// `.env` file drops straight into an env loader's source chain. Likewise
// [WithExpansionParent] takes the small [VarLookup] interface, which any
// env.Source (e.g. env.OSEnv()) satisfies; [WithExpandFromOSEnv] covers the
// common case using only [os.LookupEnv].
//
// # File watching
//
// This package does not watch files. It reads the source once ([NewSource] /
// [NewReaderSource] / [Load]); later on-disk changes do not propagate. For
// live reload, compose a file watcher (such as the one in
// github.com/go-rotini/fs) with [Parse].
package dotenv
