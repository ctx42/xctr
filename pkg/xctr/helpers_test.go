// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"strings"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/moby/moby/api/types/container"
	tc "github.com/testcontainers/testcontainers-go"
)

func Test_ImageReq(t *testing.T) {
	t.Run("setup", func(t *testing.T) {
		// --- When ---
		have := ImageReq("busybox:1.38-uclibc")

		// --- Then ---
		assert.True(t, have.Started)
		assert.Equal(t, "busybox:1.38-uclibc", have.Image)
		assert.Equal(t, []string{"tail", "-f", "/dev/null"}, have.Entrypoint)
		hc := &container.HostConfig{}
		have.HostConfigModifier(hc)
		assert.True(t, hc.AutoRemove)
	})
}

func Test_ToBuildArgs(t *testing.T) {
	// --- Given ---
	m := map[string]string{
		"key0": "val0",
		"key1": "val1",
	}

	// --- When ---
	have := ToBuildArgs(m)

	// --- Then ---
	assert.Len(t, 2, have)
	assert.Equal(t, "val0", *have["key0"])
	assert.Equal(t, "val1", *have["key1"])
}

func Test_Ref_tabular(t *testing.T) {
	tt := []struct {
		testN string

		repo string
		name string
		tag  string
		want string
	}{
		{
			"repo, name, tag",
			"example.com/repo",
			"name",
			"tag",
			"example.com/repo/name:tag",
		},
		{
			"repo ending with slash",
			"example.com/repo/",
			"name",
			"tag",
			"example.com/repo/name:tag",
		},
		{"name, tag", "", "name", "tag", "name:tag"},
		{"name", "", "name", "", "name"},
		{"no name, tag", "", "", "tag", ""},
		{"all empty", "", "", "", ""},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			assert.Equal(t, tc.want, Ref(tc.repo, tc.name, tc.tag))
		})
	}
}

func Test_ParseRef_tabular(t *testing.T) {
	tt := []struct {
		testN string

		ref  string
		repo string
		name string
		tag  string
	}{
		{
			"registry with port",
			"registry.example.com:5000/team/app:v1.2.3",
			"registry.example.com:5000",
			"team/app",
			"v1.2.3",
		},
		{
			"image no tag",
			"ubuntu",
			"docker.io",
			"ubuntu",
			"latest",
		},
		{
			"image with tag",
			"ubuntu:20.04",
			"docker.io",
			"ubuntu",
			"20.04",
		},
		{
			"repo and image",
			"docker.io/ubuntu",
			"docker.io",
			"ubuntu",
			"latest",
		},
		{
			"repo and image with tag",
			"library/ubuntu:20.04",
			"docker.io",
			"ubuntu",
			"20.04",
		},
		{
			"digest",
			"" +
				"ubuntu@sha256:1234567890abcdef1234567890abcdef" +
				"1234567890abcdef1234567890abcdef",
			"docker.io",
			"ubuntu",
			"" +
				"sha256:1234567890abcdef1234567890abcdef" +
				"1234567890abcdef1234567890abcdef",
		},
		{
			"invalid",
			"",
			"",
			"",
			"",
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			haveRepo, haveName, haveTag := ParseRef(tc.ref)

			// --- Then ---
			assert.Equal(t, tc.repo, haveRepo)
			assert.Equal(t, tc.name, haveName)
			assert.Equal(t, tc.tag, haveTag)
		})
	}
}

func Test_RandName(t *testing.T) {
	// --- When ---
	have := RandName()

	// --- Then ---
	assert.Regexp(t, "^c42-tst-img-[0-9a-f]{12}$", have)
}

func Test_RandTag(t *testing.T) {
	// --- When ---
	have := RandTag()

	// --- Then ---
	assert.Regexp(t, "^c42-tst-tag-[0-9a-f]{12}$", have)
}

func Test_RandRef(t *testing.T) {
	// --- When ---
	have := RandRef()

	// --- Then ---
	want := "^c42-tst-img-[0-9a-f]{12}:c42-tst-tag-[0-9a-f]{12}$"
	assert.Regexp(t, want, have)
}

func Test_RandNet(t *testing.T) {
	// --- When ---
	have := RandNet()

	// --- Then ---
	assert.Regexp(t, "^c42-tst-net-[0-9a-f]{12}$", have)
}

func Test_SetMissing(t *testing.T) {
	t.Run("empty destination", func(t *testing.T) {
		// --- Given ---
		dst := make(map[string]string)
		src := map[string]string{
			"KEY0": "VAL0",
			"KEY1": "VAL1",
		}

		// --- When ---
		have := SetMissing(dst, src)

		// --- Then ---
		want := map[string]string{
			"KEY0": "VAL0",
			"KEY1": "VAL1",
		}
		assert.Equal(t, want, have)
	})

	t.Run("does not override", func(t *testing.T) {
		// --- Given ---
		dst := map[string]string{
			"KEY0": "XXX",
		}
		src := map[string]string{
			"KEY0": "VAL0",
			"KEY1": "VAL1",
		}

		// --- When ---
		have := SetMissing(dst, src)

		// --- Then ---
		want := map[string]string{
			"KEY0": "XXX",
			"KEY1": "VAL1",
		}
		assert.Equal(t, want, have)
	})

	t.Run("nil destination", func(t *testing.T) {
		// --- Given ---
		var dst map[string]string
		src := map[string]string{
			"KEY0": "VAL0",
			"KEY1": "VAL1",
		}

		// --- When ---
		have := SetMissing(dst, src)

		// --- Then ---
		want := map[string]string{
			"KEY0": "VAL0",
			"KEY1": "VAL1",
		}
		assert.Equal(t, want, have)
	})

	t.Run("nil source", func(t *testing.T) {
		// --- Given ---
		dst := map[string]string{
			"KEY0": "VAL0",
		}
		var src map[string]string

		// --- When ---
		have := SetMissing(dst, src)

		// --- Then ---
		want := map[string]string{
			"KEY0": "VAL0",
		}
		assert.Equal(t, want, have)
	})

	t.Run("nil destination and source", func(t *testing.T) {
		// --- Given ---
		var dst, src map[string]string

		// --- When ---
		have := SetMissing(dst, src)

		// --- Then ---
		assert.NotNil(t, have)
		assert.Empty(t, have)
	})
}

func Test_ShortID_tabular(t *testing.T) {
	tt := []struct {
		testN string

		id   string
		want string
	}{
		{"0 chars", "", ""},
		{"5 chars", "01234", "01234"},
		{"8 chars", "01234567", "01234567"},
		{"10 chars", "0123456789", "01234567"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := ShortID(tc.id)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_DescribeFiles(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		// --- When ---
		have := DescribeFiles(nil)

		// --- Then ---
		assert.Equal(t, []string{}, have)
	})

	t.Run("with files", func(t *testing.T) {
		// --- Given ---
		files := []tc.ContainerFile{
			{
				HostFilePath:      "testdata/file0.txt",
				ContainerFilePath: "/file0.txt",
			},
			{
				Reader:            strings.NewReader("reader content"),
				ContainerFilePath: "/file2.txt",
			},
		}

		// --- When ---
		have := DescribeFiles(files)

		// --- Then ---
		want := []string{
			"Files added:",
			"\ttestdata/file0.txt -> /file0.txt",
			"\t<reader> -> /file2.txt",
		}
		assert.Equal(t, want, have)
	})
}
