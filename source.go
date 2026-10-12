package dotenv

import (
	"maps"
	"os"
	"slices"
)

// VarLookup resolves a variable name to its value. It is the contract
// [WithExpansionParent] uses to resolve `$VAR` / `${VAR}` references that are
// not defined in the file itself.
//
// The single method matches the Source interface in github.com/go-rotini/env,
// so env.OSEnv(), env.Map(...), and any other env.Source satisfy VarLookup —
// dotenv does not import env to make that work. (For the common "resolve
// against the process environment" case, prefer [WithExpandFromOSEnv], which
// needs no argument; for an in-memory map, [MapSource] returns a VarLookup.)
type VarLookup interface {
	// Lookup returns the value associated with key and whether the key is
	// set. ok=true with an empty value means "set but empty".
	Lookup(key string) (value string, ok bool)
}

// Source is an immutable key/value snapshot of a parsed `.env` file, returned
// by [NewSource] / [NewReaderSource] / [MapSource] / [File.Source]. Later
// changes to the underlying file do not propagate. It is safe for concurrent
// use.
//
// [Source.Lookup] makes *Source structurally compatible with the Source
// interface in github.com/go-rotini/env, so it can be used directly in an env
// loader's source chain.
type Source struct {
	m map[string]string
}

// Lookup returns the value associated with key and whether the key is set.
// An ok=true result with an empty value distinguishes "set but empty" from
// "unset". A nil *Source has no keys.
func (s *Source) Lookup(key string) (value string, ok bool) {
	if s == nil {
		return "", false
	}
	v, ok := s.m[key]
	return v, ok
}

// Keys returns the source's keys in sorted order.
func (s *Source) Keys() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.m))
	for k := range s.m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// ToMap returns a copy of the source's key/value pairs.
func (s *Source) ToMap() map[string]string {
	if s == nil {
		return nil
	}
	return maps.Clone(s.m)
}

// MapSource returns a [*Source] over a copy of m, for example as an
// in-memory parent for [WithExpansionParent]. Later changes to m are not
// reflected.
func MapSource(m map[string]string) *Source {
	return &Source{m: maps.Clone(m)}
}

// osEnvLookup is the [VarLookup] used by [WithExpandFromOSEnv]; it reads the
// live process environment via [os.LookupEnv] on every call.
type osEnvLookup struct{}

func (osEnvLookup) Lookup(key string) (string, bool) { return os.LookupEnv(key) }

var (
	_ VarLookup = (*Source)(nil)
	_ VarLookup = osEnvLookup{}
)
