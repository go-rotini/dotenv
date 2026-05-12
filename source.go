package dotenv

import (
	"maps"
	"os"
)

// VarLookup resolves a variable name to its value. It is the contract
// [WithExpansionParent] uses to resolve `$VAR` / `${VAR}` references that are
// not defined in the file itself.
//
// The single method matches the Source interface in github.com/go-rotini/env,
// so env.OSEnv(), env.Map(...), and any other env.Source satisfy VarLookup —
// dotenv does not import env to make that work. (For the common "resolve
// against the process environment" case, prefer [WithExpandFromOSEnv], which
// needs no argument.)
type VarLookup interface {
	// Lookup returns the value associated with key and whether the key is
	// set. ok=true with an empty value means "set but empty".
	Lookup(key string) (value string, ok bool)
}

// Source is an in-memory key/value view of a parsed `.env` file, returned by
// [NewSource] and [NewReaderSource]. It is a snapshot taken at construction;
// later changes to the underlying file do not propagate (for live reload,
// layer a file watcher — e.g. from github.com/go-rotini/fs — over [Parse] and
// re-derive the source on change).
//
// (*Source).Lookup makes *Source structurally compatible with the Source
// interface in github.com/go-rotini/env, so a parsed `.env` file drops
// straight into an env loader's source chain without dotenv depending on env.
type Source struct {
	m map[string]string
}

// Lookup returns the value associated with key and whether the key is set.
// An ok=true result with an empty value distinguishes "set but empty" from
// "unset". Safe for concurrent use; *Source is immutable after construction.
func (s *Source) Lookup(key string) (value string, ok bool) {
	if s == nil {
		return "", false
	}
	v, ok := s.m[key]
	return v, ok
}

// ToMap returns a copy of the source's key/value pairs (last-write-wins per
// Bash convention, as resolved at parse time).
func (s *Source) ToMap() map[string]string {
	if s == nil {
		return nil
	}
	return maps.Clone(s.m)
}

// osEnvLookup is the [VarLookup] used by [WithExpandFromOSEnv]; it reads the
// live process environment via [os.LookupEnv] on every call.
type osEnvLookup struct{}

func (osEnvLookup) Lookup(key string) (string, bool) { return os.LookupEnv(key) }

var (
	_ VarLookup = (*Source)(nil)
	_ VarLookup = osEnvLookup{}
)
