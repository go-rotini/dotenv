# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly by emailing **matthewcgetz@gmail.com**. Do not open a public issue.

You should receive a response within 72 hours. If accepted, a fix will be developed privately and released as a patch version.

## Resource Limits

The parser defaults to safe behavior to mitigate denial-of-service and accidental misuse:

- **Parser DoS guards**: configurable via `WithMaxFileSize`, `WithMaxLineLength`, and `WithMaxExpansionDepth`. Defaults are conservative (10 MiB file size, 1 MiB line length, expansion depth 16; the depth bound caps variable-expansion recursion so a pathological `${A:-${A:-${A}}}`-style input cannot overflow the stack — exceeding it yields an `*ExpansionError`).
- **No subshell execution.** The package never invokes `$(command)` or any shell substitution form, regardless of dialect.
- **No process-environment writes.** Parsing a `.env` file never calls `os.Setenv`; results are returned as in-memory maps / `Source` values. (`WithExpandFromOSEnv` *reads* the process environment via `os.LookupEnv` to resolve `$VAR` references — a read, opt-in, never a write.) Whether and how parsed values are applied to the process environment is the caller's decision.
