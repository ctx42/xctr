// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

// Package arch builds and reads simple tar archives used by container
// Dockerfile contexts. It is internal so both xctr and xctrtest can share
// it without an import cycle.
package arch

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// File represents one file in the tar archive.
type File struct {
	Name    string // File name (path).
	Mode    int64  // File mode (default should be: 0o0644).
	Content []byte // File content.
}

// NewFile returns a new [File].
func NewFile(name string, mode int64, content []byte) File {
	return File{
		Name:    name,
		Mode:    mode,
		Content: content,
	}
}

// Opts represents tar archive options.
type Opts struct {
	Files []File // Files to archive.
}

// WithFile is an option for [Archive] that appends a file.
func WithFile(fil File) func(*Opts) {
	return func(opts *Opts) { opts.Files = append(opts.Files, fil) }
}

// WithDockerfile is an option for [Archive] that adds a "Dockerfile" entry.
func WithDockerfile(content []byte) func(*Opts) {
	return func(opts *Opts) {
		fil := File{Name: "Dockerfile", Mode: 0o0644, Content: content}
		opts.Files = append(opts.Files, fil)
	}
}

// Archive returns a reader for a tar archive.
func Archive(opts ...func(*Opts)) (*bytes.Reader, error) {
	def := &Opts{}
	for _, opt := range opts {
		opt(def)
	}

	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, fil := range def.Files {
		hdr := &tar.Header{
			Name:     fil.Name,
			Mode:     fil.Mode,
			Size:     int64(len(fil.Content)),
			Typeflag: tar.TypeReg,
			Format:   tar.FormatGNU,
		}

		if err := w.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("tar header %s: %w", fil.Name, err)
		}
		if _, err := w.Write(fil.Content); err != nil {
			return nil, fmt.Errorf("tar write %s: %w", fil.Name, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("tar close: %w", err)
	}
	return bytes.NewReader(buf.Bytes()), nil
}

// MustArchive works like [Archive] but panics on error.
func MustArchive(opts ...func(*Opts)) *bytes.Reader {
	rdr, err := Archive(opts...)
	if err != nil {
		panic(err)
	}
	return rdr
}

// Unarchive reverses what [Archive] does.
func Unarchive(r io.Reader) (Opts, error) {
	tr := tar.NewReader(r)
	opts := Opts{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return opts, nil
		}
		if err != nil {
			return Opts{}, fmt.Errorf("tar next: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		bs, err := io.ReadAll(tr)
		if err != nil {
			return Opts{}, fmt.Errorf("tar read %s: %w", hdr.Name, err)
		}
		fil := NewFile(hdr.Name, hdr.Mode, bs)
		opts.Files = append(opts.Files, fil)
	}
}

// DockerfileArchive returns a reader to a tar archive with a single Dockerfile.
func DockerfileArchive(dockerfile []byte) (*bytes.Reader, error) {
	return Archive(WithDockerfile(dockerfile))
}

// MustDockerfileArchive works like [DockerfileArchive] but panics on error.
func MustDockerfileArchive(dockerfile []byte) *bytes.Reader {
	rdr, err := Archive(WithDockerfile(dockerfile))
	if err != nil {
		panic(err)
	}
	return rdr
}
