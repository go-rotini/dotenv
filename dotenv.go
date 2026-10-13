package dotenv

import (
	"fmt"
	"io"
	"math"
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
// key is assigned more than once, the last assignment wins. At most the
// [WithMaxFileSize] limit is read.
func Load(path string, opts ...ParseOption) (map[string]string, error) {
	o := defaultParseOptions()
	for _, opt := range opts {
		opt(&o)
	}
	o.filePath = path

	data, err := readFileLimited(path, o)
	if err != nil {
		return nil, err
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
// over its parsed map. At most one byte more than the [WithMaxFileSize]
// limit is read from r.
func NewReaderSource(r io.Reader, opts ...ParseOption) (*Source, error) {
	o := defaultParseOptions()
	for _, opt := range opts {
		opt(&o)
	}
	data, err := io.ReadAll(limitReader(r, o))
	if err != nil {
		return nil, fmt.Errorf("dotenv: read: %w", err)
	}
	f, err := parse(data, o)
	if err != nil {
		return nil, err
	}
	return &Source{m: f.ToMap()}, nil
}

// readFileLimited reads path, refusing a regular file larger than the
// [WithMaxFileSize] limit before reading it and reading anything else
// (a FIFO, a device) only up to the limit.
func readFileLimited(path string, o parseOptions) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("dotenv: read %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if fi, err := f.Stat(); err == nil && fi.Mode().IsRegular() {
		if err := checkFileSize(fi.Size(), o); err != nil {
			return nil, err
		}
	}
	data, err := io.ReadAll(limitReader(f, o))
	if err != nil {
		return nil, fmt.Errorf("dotenv: read %s: %w", path, err)
	}
	return data, nil
}

// limitReader limits r to one byte more than the [WithMaxFileSize]
// limit, so [parse] sees and rejects an oversized input. A limit of
// [math.MaxInt64] leaves r unlimited, since nothing can exceed it.
func limitReader(r io.Reader, o parseOptions) io.Reader {
	if o.maxFileSize > 0 && o.maxFileSize < math.MaxInt64 {
		return io.LimitReader(r, o.maxFileSize+1)
	}
	return r
}
