// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"bytes"
	"io"

	"github.com/ctx42/xctr/pkg/xctr/internal/arch"
)

// ArchFile represents one file in the tar archive.
type ArchFile = arch.File

// NewArchFile returns a new instance of [ArchFile].
func NewArchFile(name string, mode int64, content []byte) ArchFile {
	return arch.NewFile(name, mode, content)
}

// ArchOpts represents tar archive options.
type ArchOpts = arch.Opts

// WithArchFile is an option for [Archive] adding a file.
func WithArchFile(fil ArchFile) func(*ArchOpts) {
	return arch.WithFile(fil)
}

// WithArchDockerfile is an option for [Archive] adding a file named
// "Dockerfile" with mode 0644 and the given content.
func WithArchDockerfile(content []byte) func(*ArchOpts) {
	return arch.WithDockerfile(content)
}

// Archive returns a reader for a tar archive.
func Archive(opts ...func(*ArchOpts)) (*bytes.Reader, error) {
	return arch.Archive(opts...)
}

// MustArchive works like [Archive] but panics on error.
func MustArchive(opts ...func(*ArchOpts)) *bytes.Reader {
	return arch.MustArchive(opts...)
}

// Unarchive reverses what [Archive] does.
func Unarchive(r io.Reader) (ArchOpts, error) {
	return arch.Unarchive(r)
}

// DockerfileArchive returns a reader to tar archive with single Dockerfile.
func DockerfileArchive(dockerfile []byte) (*bytes.Reader, error) {
	return arch.DockerfileArchive(dockerfile)
}

// MustDockerfileArchive works like [DockerfileArchive] but panics on error.
func MustDockerfileArchive(dockerfile []byte) *bytes.Reader {
	return arch.MustDockerfileArchive(dockerfile)
}
