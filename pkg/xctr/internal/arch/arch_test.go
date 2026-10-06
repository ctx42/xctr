// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package arch

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
)

func Test_NewFile(t *testing.T) {
	// --- When ---
	have := NewFile("name", 0o0755, []byte("abc"))

	// --- Then ---
	assert.Equal(t, "name", have.Name)
	assert.Equal(t, int64(0o0755), have.Mode)
	assert.Equal(t, []byte("abc"), have.Content)
}

func Test_WithFile(t *testing.T) {
	// --- Given ---
	fil := NewFile("name", 0o0755, []byte("abc"))
	opt := &Opts{}

	// --- When ---
	WithFile(fil)(opt)

	// --- Then ---
	assert.Len(t, 1, opt.Files)
	assert.Equal(t, fil, opt.Files[0])
}

func Test_WithDockerfile(t *testing.T) {
	// --- Given ---
	opt := &Opts{}

	// --- When ---
	WithDockerfile([]byte("abc"))(opt)

	// --- Then ---
	assert.Len(t, 1, opt.Files)
	assert.Equal(t, "Dockerfile", opt.Files[0].Name)
	assert.Equal(t, int64(0o0644), opt.Files[0].Mode)
	assert.Equal(t, []byte("abc"), opt.Files[0].Content)
}

func Test_Archive(t *testing.T) {
	t.Run("empty archive", func(t *testing.T) {
		// --- When ---
		rdr, err := Archive()

		// --- Then ---
		assert.NoError(t, err)
		arch := must.Value(Unarchive(rdr))
		assert.Len(t, 0, arch.Files)
	})

	t.Run("single file", func(t *testing.T) {
		// --- Given ---
		fil0 := WithDockerfile([]byte("fil0c"))

		// --- When ---
		rdr, err := Archive(fil0)

		// --- Then ---
		assert.NoError(t, err)
		arch := must.Value(Unarchive(rdr))
		assert.Len(t, 1, arch.Files)
		assert.Equal(t, "Dockerfile", arch.Files[0].Name)
		assert.Equal(t, int64(0o0644), arch.Files[0].Mode)
		assert.Equal(t, []byte("fil0c"), arch.Files[0].Content)
	})

	t.Run("multiple files", func(t *testing.T) {
		// --- Given ---
		fil0 := WithDockerfile([]byte("fil0c"))
		fil1 := WithFile(NewFile("/dir/fil1", 0o0755, []byte("fil1c")))

		// --- When ---
		rdr, err := Archive(fil0, fil1)

		// --- Then ---
		assert.NoError(t, err)
		arch := must.Value(Unarchive(rdr))
		assert.Len(t, 2, arch.Files)

		fil := arch.Files[0]
		assert.Equal(t, "Dockerfile", fil.Name)
		assert.Equal(t, int64(0o0644), fil.Mode)
		assert.Equal(t, []byte("fil0c"), fil.Content)

		fil = arch.Files[1]
		assert.Equal(t, "/dir/fil1", fil.Name)
		assert.Equal(t, int64(0o0755), fil.Mode)
		assert.Equal(t, []byte("fil1c"), fil.Content)
	})
}

func Test_MustArchive(t *testing.T) {
	// --- Given ---
	fil0 := WithDockerfile([]byte("fil0c"))

	// --- When ---
	rdr := MustArchive(fil0)

	// --- Then ---
	arch := must.Value(Unarchive(rdr))
	assert.Len(t, 1, arch.Files)
	assert.Equal(t, "Dockerfile", arch.Files[0].Name)
	assert.Equal(t, int64(0o0644), arch.Files[0].Mode)
	assert.Equal(t, []byte("fil0c"), arch.Files[0].Content)
}

func Test_Unarchive(t *testing.T) {
	t.Run("round-trip", func(t *testing.T) {
		// --- Given ---
		fil0 := WithFile(NewFile("/dir/fil0", 0o0644, []byte("fil0c")))
		rdr := must.Value(Archive(fil0))

		// --- When ---
		arch, err := Unarchive(rdr)

		// --- Then ---
		assert.NoError(t, err)
		assert.Len(t, 1, arch.Files)
		assert.Equal(t, "/dir/fil0", arch.Files[0].Name)
		assert.Equal(t, int64(0o0644), arch.Files[0].Mode)
		assert.Equal(t, []byte("fil0c"), arch.Files[0].Content)
	})

	t.Run("skips non-regular entries", func(t *testing.T) {
		// --- Given ---
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		must.Nil(tw.WriteHeader(&tar.Header{
			Name:     "dir/",
			Typeflag: tar.TypeDir,
			Mode:     0o0755,
		}))
		content := []byte("fil0c")
		must.Nil(tw.WriteHeader(&tar.Header{
			Name:     "dir/fil0",
			Typeflag: tar.TypeReg,
			Mode:     0o0644,
			Size:     int64(len(content)),
		}))
		must.Value(tw.Write(content))
		must.Nil(tw.Close())

		// --- When ---
		arch, err := Unarchive(&buf)

		// --- Then ---
		assert.NoError(t, err)
		assert.Len(t, 1, arch.Files)
		assert.Equal(t, "dir/fil0", arch.Files[0].Name)
		assert.Equal(t, []byte("fil0c"), arch.Files[0].Content)
	})

	t.Run("error - invalid tar header", func(t *testing.T) {
		// --- Given ---
		rdr := bytes.NewReader(bytes.Repeat([]byte{0xFF}, 512))

		// --- When ---
		arch, err := Unarchive(rdr)

		// --- Then ---
		assert.ErrorContain(t, "tar next:", err)
		assert.Equal(t, Opts{}, arch)
	})

	t.Run("error - truncated file content", func(t *testing.T) {
		// --- Given ---
		content := []byte("fil0c")
		full := must.Value(Archive(WithFile(NewFile("fil0", 0o0644, content))))
		raw := must.Value(io.ReadAll(full))
		rdr := bytes.NewReader(raw[:513])

		// --- When ---
		arch, err := Unarchive(rdr)

		// --- Then ---
		assert.ErrorContain(t, "tar read fil0:", err)
		assert.Equal(t, Opts{}, arch)
	})
}

func Test_DockerfileArchive(t *testing.T) {
	// --- When ---
	rdr, err := DockerfileArchive([]byte("content"))

	// --- Then ---
	assert.NoError(t, err)
	arch := must.Value(Unarchive(rdr))
	assert.Len(t, 1, arch.Files)
	assert.Equal(t, "Dockerfile", arch.Files[0].Name)
	assert.Equal(t, int64(0o0644), arch.Files[0].Mode)
	assert.Equal(t, []byte("content"), arch.Files[0].Content)
}

func Test_MustDockerfileArchive(t *testing.T) {
	// --- When ---
	rdr := MustDockerfileArchive([]byte("content"))

	// --- Then ---
	arch := must.Value(Unarchive(rdr))
	assert.Len(t, 1, arch.Files)
	assert.Equal(t, "Dockerfile", arch.Files[0].Name)
	assert.Equal(t, int64(0o0644), arch.Files[0].Mode)
	assert.Equal(t, []byte("content"), arch.Files[0].Content)
}
