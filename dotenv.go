package dotenv

import (
	"fmt"
	"io"
	"os"
)

// Parse parses data as a `.env` byte stream and returns the resulting
// AST. Use [File.Get] / [File.Keys] / [File.ToMap] for access, or
// [File.Marshal] for round-trip emission. Malformed input yields a
// [*ParseError]; see [ParseOption] for the size limits and dialect options.
func Parse(data []byte, opts ...ParseOption) (*File, error) {
	o := defaultParseOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return parse(data, o)
}

// Load reads a `.env` file from path and returns its key/value map. When a
// key is assigned more than once, the last assignment wins.
func Load(path string, opts ...ParseOption) (map[string]string, error) {
	o := defaultParseOptions()
	for _, opt := range opts {
		opt(&o)
	}
	o.filePath = path

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("dotenv: read %s: %w", path, err)
	}
	f, err := parse(data, o)
	if err != nil {
		return nil, err
	}
	return f.ToMap(), nil
}

// NewSource reads a `.env` file from path and returns a [*Source] snapshot of
// its parsed map. The result can be used wherever a github.com/go-rotini/env
// Source is expected.
func NewSource(path string, opts ...ParseOption) (*Source, error) {
	m, err := Load(path, opts...)
	if err != nil {
		return nil, err
	}
	return &Source{m: m}, nil
}

// NewReaderSource reads a `.env` byte stream from r and returns a [*Source]
// over its parsed map.
func NewReaderSource(r io.Reader, opts ...ParseOption) (*Source, error) {
	o := defaultParseOptions()
	for _, opt := range opts {
		opt(&o)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("dotenv: read: %w", err)
	}
	f, err := parse(data, o)
	if err != nil {
		return nil, err
	}
	return &Source{m: f.ToMap()}, nil
}
