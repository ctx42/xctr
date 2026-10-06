// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
	"github.com/ctx42/testkit/pkg/dkrkit"
	"github.com/ctx42/testkit/pkg/httpkit"
	"github.com/ctx42/testkit/pkg/netkit"
	"github.com/ctx42/testkit/pkg/oskit"
	"github.com/ctx42/testkit/pkg/randkit"
	"github.com/ctx42/xdef/pkg/xdef"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	tc "github.com/testcontainers/testcontainers-go"
	tcnet "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ctx42/xctr/pkg/xctr/xctrtest"
)

func Test_SetEnvCTRLog(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		// --- Given ---
		var env []string

		// --- When ---
		have := SetEnvCTRLog(env)

		// --- Then ---
		assert.Equal(t, []string{"C42_XCTR_LOG=true"}, have)
	})

	t.Run("overwrites", func(t *testing.T) {
		// --- Given ---
		env := []string{"C42_XCTR_LOG=false"}

		// --- When ---
		have := SetEnvCTRLog(env)

		// --- Then ---
		assert.Equal(t, []string{"C42_XCTR_LOG=true"}, have)
	})
}

func Test_SetEnvCTRBuildLog(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		// --- Given ---
		var env []string

		// --- When ---
		have := SetEnvCTRBuildLog(env)

		// --- Then ---
		assert.Equal(t, []string{"C42_XCTR_BUILD_LOG=true"}, have)
	})

	t.Run("overwrites", func(t *testing.T) {
		// --- Given ---
		env := []string{"C42_XCTR_BUILD_LOG=false"}

		// --- When ---
		have := SetEnvCTRBuildLog(env)

		// --- Then ---
		assert.Equal(t, []string{"C42_XCTR_BUILD_LOG=true"}, have)
	})
}

func Test_SetEnvCTREntrypoint(t *testing.T) {
	t.Run("set default", func(t *testing.T) {
		// --- Given ---
		var env []string

		// --- When ---
		have := SetEnvCTREntrypoint(env)

		// --- Then ---
		want := []string{"C42_XCTR_ENTRYPOINT=tini -- tail -f /dev/null"}
		assert.Equal(t, want, have)
	})

	t.Run("overwrites", func(t *testing.T) {
		// --- Given ---
		env := []string{"C42_XCTR_ENTRYPOINT=other"}

		// --- When ---
		have := SetEnvCTREntrypoint(env)

		// --- Then ---
		want := []string{"C42_XCTR_ENTRYPOINT=tini -- tail -f /dev/null"}
		assert.Equal(t, want, have)
	})

	t.Run("custom", func(t *testing.T) {
		// --- Given ---
		var env []string

		// --- When ---
		have := SetEnvCTREntrypoint(env, "custom", "arg1", "arg2")

		// --- Then ---
		assert.Equal(t, []string{"C42_XCTR_ENTRYPOINT=custom arg1 arg2"}, have)
	})
}

func Test_WithCTRImgRm(t *testing.T) {
	// --- Given ---
	ctr := &CTR{}

	// --- When ---
	WithCTRImgRm(ctr)

	// --- Then ---
	assert.True(t, ctr.removeImage)
}

func Test_NewCTR(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		req := tc.GenericContainerRequest{
			ContainerRequest: tc.ContainerRequest{
				Image: xctrtest.EchoServerRef,
			},
		}

		// --- When ---
		ctr := NewCTR("echo", req)

		// --- Then ---
		assert.Equal(t, "", ctr.id)
		assert.Equal(t, "echo", ctr.name)
		assert.Equal(t, "", ctr.reference)
		assert.Equal(t, req, ctr.req)
		assert.Nil(t, ctr.dc)
		assert.Equal(t, xctrtest.EchoServerRef, ctr.req.Image)
		assert.False(t, ctr.removeImage)
		assert.Nil(t, ctr.cfgHost)
		assert.Nil(t, ctr.cfgGuest)
	})

	t.Run("request is not shared", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.ImageReq()
		req.Labels = map[string]string{"lab": "val"}
		req.Env = map[string]string{"ENV": "val"}

		// --- When ---
		ctr := NewCTR("echo", req)

		// --- Then ---
		assert.NoError(t, ctr.SetLabel("other", "val"))
		assert.NoError(t, ctr.Setenv("OTHER", "val"))
		assert.NoError(t, ctr.ExposePort("81/tcp"))
		assert.Equal(t, map[string]string{"lab": "val"}, req.Labels)
		assert.Equal(t, map[string]string{"ENV": "val"}, req.Env)
		assert.Equal(t, []string{"80/tcp"}, req.ExposedPorts)
	})
}

func Test_cloneReq(t *testing.T) {
	t.Run("copies maps and slices", func(t *testing.T) {
		// --- Given ---
		req := tc.GenericContainerRequest{
			ContainerRequest: tc.ContainerRequest{
				Labels:       map[string]string{"lab": "val"},
				Env:          map[string]string{"ENV": "val"},
				ExposedPorts: []string{"80/tcp"},
				Files:        []tc.ContainerFile{{HostFilePath: "a"}},
				FromDockerfile: tc.FromDockerfile{
					BuildArgs: map[string]*string{"ARG": new("val")},
				},
			},
		}

		// --- When ---
		have := cloneReq(req)

		// --- Then ---
		assert.Equal(t, req, have)

		have.Labels["lab"] = "changed"
		have.Env["ENV"] = "changed"
		have.BuildArgs["ARG"] = new("changed")
		have.ExposedPorts[0] = "81/tcp"
		have.Files[0].HostFilePath = "b"
		assert.Equal(t, "val", req.Labels["lab"])
		assert.Equal(t, "val", req.Env["ENV"])
		assert.Equal(t, "val", *req.BuildArgs["ARG"])
		assert.Equal(t, "80/tcp", req.ExposedPorts[0])
		assert.Equal(t, "a", req.Files[0].HostFilePath)
	})

	t.Run("nil maps and slices", func(t *testing.T) {
		// --- When ---
		have := cloneReq(tc.GenericContainerRequest{})

		// --- Then ---
		assert.Equal(t, tc.GenericContainerRequest{}, have)
	})
}

func Test_CTR_Cleanup(t *testing.T) {
	t.Run("zero value", func(t *testing.T) {
		// --- Given ---
		var ctr CTR
		var called bool
		ctr.RegisterCleanup(func(context.Context) error {
			called = true
			return nil
		})

		// --- When ---
		err := ctr.Cleanup(t.Context())

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, called)
	})
}

func Test_CTR_ID(t *testing.T) {
	// --- Given ---
	ctr := &CTR{id: "id"}

	// --- When ---
	have := ctr.ID()

	// --- Then ---
	assert.Equal(t, "id", have)
}

func Test_CTR_Name(t *testing.T) {
	// --- Given ---
	ctr := &CTR{name: "name"}

	// --- When ---
	have := ctr.Name()

	// --- Then ---
	assert.Equal(t, "name", have)
}

func Test_CTR_Reference(t *testing.T) {
	// --- Given ---
	ctr := &CTR{reference: "ref"}

	// --- When ---
	have := ctr.Reference()

	// --- Then ---
	assert.Equal(t, "ref", have)
}

func Test_CTR_IsReadOnly(t *testing.T) {
	t.Run("is read-only", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{readOnly: true}

		// --- When ---
		have := ctr.IsReadOnly()

		// --- Then ---
		assert.True(t, have)
	})

	t.Run("is not read-only", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{readOnly: false}

		// --- When ---
		have := ctr.IsReadOnly()

		// --- Then ---
		assert.False(t, have)
	})
}

func Test_CTR_IsRunning(t *testing.T) {
	t.Run("not started container", func(t *testing.T) {
		// --- Given ---
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have := ctr.IsRunning()

		// --- Then ---
		assert.False(t, have)
	})

	t.Run("running container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.IsRunning()

		// --- Then ---
		assert.True(t, have)
	})

	t.Run("terminated container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		assert.NoError(t, ctr.Cleanup(ctx))

		// --- When ---
		have := ctr.IsRunning()

		// --- Then ---
		assert.False(t, have)
	})

	t.Run("terminated container by tcc", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		assert.NoError(t, ctr.dc.Stop(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.IsRunning()

		// --- Then ---
		assert.False(t, have)
	})
}

func Test_CTR_ConfigHost(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{
			cfgHost: map[string]string{
				"HOST":   "localhost",
				"PORT_0": "42",
				"PORT_1": "43",
			},
		}

		// --- When ---
		have := ctr.ConfigHost()

		// --- Then ---
		want := map[string]string{
			"HOST":   "localhost",
			"PORT_0": "42",
			"PORT_1": "43",
		}
		assert.Equal(t, want, have)
	})
}

func Test_CTR_ConfigGuest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{
			cfgGuest: map[string]string{
				"HOST":   "localhost",
				"PORT_0": "42",
				"PORT_1": "43",
			},
		}

		// --- When ---
		have := ctr.ConfigGuest()

		// --- Then ---
		want := map[string]string{
			"HOST":   "localhost",
			"PORT_0": "42",
			"PORT_1": "43",
		}
		assert.Equal(t, want, have)
	})
}

func Test_CTR_SetReadOnly(t *testing.T) {
	t.Run("success without expressions", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := &CTR{readOnly: false}

		// --- When ---
		err := ctr.SetReadOnly(ctx)

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, ctr.readOnly)
		assert.ErrorIs(t, ErrNotAllowed, ctr.wl.Check("echo", "test"))
	})

	t.Run("success with expressions", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := &CTR{readOnly: false}

		// --- When ---
		err := ctr.SetReadOnly(ctx, "echo .*")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, ctr.readOnly)
		assert.NoError(t, ctr.wl.Check("echo", "test"))
	})

	t.Run("error - invalid regular expression", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := &CTR{readOnly: false}

		// --- When ---
		err := ctr.SetReadOnly(ctx, "((unbalanced)")

		// --- Then ---
		assert.ErrorContain(t, "error parsing regexp", err)
		assert.False(t, ctr.readOnly)
		assert.Nil(t, ctr.wl)
	})

	t.Run("error - setting twice", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := &CTR{readOnly: false}
		must.Nil(ctr.SetReadOnly(ctx))

		// --- When ---
		err := ctr.SetReadOnly(ctx)

		// --- Then ---
		assert.ErrorEqual(t, "whitelist already set", err)
	})
}

func Test_CTR_SetExecWhitelist(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{}

		// --- When ---
		err := ctr.SetExecWhitelist("^echo", "^ls$")

		// --- Then ---
		assert.NoError(t, err)
		assert.NoError(t, ctr.wl.Check("echo"))
		assert.NoError(t, ctr.wl.Check("ls"))
		assert.Error(t, ctr.wl.Check("ls file"))
	})

	t.Run("error - already set", func(t *testing.T) {
		// --- Given ---
		orig := NewWhitelist()
		must.Nil(orig.Add("^echo"))

		ctr := &CTR{wl: orig}

		// --- When ---
		err := ctr.SetExecWhitelist("^ls$")

		// --- Then ---
		assert.ErrorEqual(t, "whitelist already set", err)
	})
}

func Test_CTR_CreateTemp(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		content := []byte("test")
		ctr := NewCTR("echo", xctrtest.ImageReq())
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		pth, err := ctr.CreateTemp("ctr_*.txt", content)

		// --- Then ---
		assert.NoError(t, err)
		assert.FileExist(t, pth)
		assert.Regexp(t, "ctr_[0-9]+.txt", filepath.Base(pth))
		assert.Equal(t, content, oskit.ReadFile(t, pth))
	})

	t.Run("is removed on cleanup", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		pth, err := ctr.CreateTemp("ctr_*.txt", []byte("test"))
		assert.NoError(t, err)
		assert.FileExist(t, pth)

		// --- When ---
		err = ctr.Cleanup(ctx)

		// --- Then ---
		assert.NoError(t, err)
		assert.NoFileExist(t, pth)
	})
}

func Test_CTR_Exec(t *testing.T) {
	t.Run("success command prints to std out", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.Exec(ctx, "echo", "-n", t.Name())

		// --- Then ---
		assert.NoError(t, have.Unwrap())
		assert.Equal(t, 0, have.ExitCode)
		assert.Equal(t, t.Name(), have.SOut)
		assert.Equal(t, "", have.EOut)
	})

	t.Run("success command exits code 1 prints to std err", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.Exec(ctx, "ls", "/not-existing")

		// --- Then ---
		assert.ErrorIs(t, ErrExitCode, have.Unwrap())
		assert.Equal(t, 1, have.ExitCode)
		assert.Equal(t, "", have.SOut)
		assert.NotEmpty(t, have.EOut)
	})

	t.Run("error - stopped container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		// Something external kills the container.
		dkrkit.NewT(t).CtrKill(ctr.ID())

		// --- When ---
		have := ctr.Exec(ctx, "echo")

		// --- Then ---
		assert.ErrorContain(t, "is not running", have.Unwrap())
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("error - context timeout", func(t *testing.T) {
		// --- Given ---
		deadline := time.Now().Add(1500 * time.Millisecond)
		ctx, cxl := context.WithDeadline(t.Context(), deadline)
		defer cxl()

		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.Exec(ctx, "sleep", "10")

		// --- Then ---
		assert.ErrorIs(t, context.DeadlineExceeded, have.Unwrap())
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("error - empty command", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have := ctr.Exec(ctx)

		// --- Then ---
		assert.ErrorIs(t, ErrEmptyCmd, have.Unwrap())
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("error - not running container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have := ctr.Exec(ctx, "echo")

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, have.Unwrap())
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("error - instantiating Docker client", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		orig := newDockerClient
		t.Cleanup(func() { newDockerClient = orig })
		newDockerClient = func() (*client.Client, error) {
			return nil, errors.New("unable to parse docker host")
		}

		// --- When ---
		have := ctr.Exec(ctx, "echo")

		// --- Then ---
		assert.ErrorContain(t, "unable to parse docker host", have.Unwrap())
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("whitelist match", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		must.Nil(ctr.SetExecWhitelist("echo .*"))
		must.Nil(ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.Exec(ctx, "echo", "-n", t.Name())

		// --- Then ---
		assert.NoError(t, have.Unwrap())
		assert.Equal(t, 0, have.ExitCode)
		assert.Equal(t, t.Name(), have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("error - whitelist does not match", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		must.Nil(ctr.SetExecWhitelist("^ls"))
		must.Nil(ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.Exec(ctx, "echo", "-n", t.Name())

		// --- Then ---
		assert.ErrorIs(t, ErrNotAllowed, have)
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})
}

func Test_CTR_ExecContent(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		content := []byte("#!/bin/sh\necho Hello World\n")

		// --- When ---
		have := ctr.ExecContent(ctx, content)

		// --- Then ---
		assert.NoError(t, have.Unwrap())
		assert.Equal(t, 0, have.ExitCode)
		assert.Equal(t, "Hello World\n", have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("exit code 1", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		content := oskit.ReadFile(t, "testdata/exit_one.sh")

		// --- When ---
		have := ctr.ExecContent(ctx, content)

		// --- Then ---
		assert.ErrorIs(t, ErrExitCode, have.Unwrap())
		assert.Equal(t, 1, have.ExitCode)
		assert.Contain(t, "Not good.\n", have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("error - stopped container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		dkrkit.NewT(t).CtrKill(ctr.ID())
		content := []byte("#!/bin/sh\necho hi\n")

		// --- When ---
		have := ctr.ExecContent(ctx, content)

		// --- Then ---
		assert.ErrorRegexp(t, "^copy to /tmp/file_[0-9]+\\.sh: ", have.Err())
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("error - not running container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		content := []byte("#!/bin/sh\necho hi\n")

		// --- When ---
		have := ctr.ExecContent(ctx, content)

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, have.Err())
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("error - read-only container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		must.Nil(ctr.SetReadOnly(ctx))
		content := []byte("#!/bin/sh\necho hi\n")

		// --- When ---
		have := ctr.ExecContent(ctx, content)

		// --- Then ---
		assert.ErrorIs(t, ErrReadOnly, have.Err())
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})
}

func Test_CTR_ExecFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.ExecFile(ctx, "testdata/hello.sh")

		// --- Then ---
		assert.NoError(t, have.Unwrap())
		assert.Equal(t, 0, have.ExitCode)
		assert.Equal(t, "Hello World\n", have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("exit code 1", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.ExecFile(ctx, "testdata/exit_one.sh")

		// --- Then ---
		assert.ErrorIs(t, ErrExitCode, have.Unwrap())
		assert.Equal(t, 1, have.ExitCode)
		assert.Contain(t, "Not good.\n", have.SOut)
		assert.Empty(t, have.EOut)
	})

	t.Run("error - not existing file", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have := ctr.ExecFile(ctx, "testdata/not-existing.sh")

		// --- Then ---
		assert.ErrorContain(t, "no such file or directory", have.Err())
		assert.Equal(t, 0, have.ExitCode)
		assert.Empty(t, have.SOut)
		assert.Empty(t, have.EOut)
	})
}

func Test_CTR_AddFile(t *testing.T) {
	t.Run("files added", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		files := []tc.ContainerFile{
			{
				HostFilePath:      "testdata/file0.txt",
				ContainerFilePath: "/file0.txt",
			},
			{
				HostFilePath:      "testdata/file1.txt",
				ContainerFilePath: "/file1.txt",
			},
			{
				Reader:            strings.NewReader("reader content"),
				ContainerFilePath: "/file2.txt",
			},
		}

		// --- When ---
		err := ctr.AddFile(files...)

		// --- Then ---
		assert.NoError(t, err)
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		rsp := httpkit.NewRequest(t).Get(endpoint(ctr, "echo_file=/file0.txt"))
		assert.Equal(t, "file0.txt content", rsp)
		rsp = httpkit.NewRequest(t).Get(endpoint(ctr, "echo_file=/file1.txt"))
		assert.Equal(t, "file1.txt content", rsp)
		rsp = httpkit.NewRequest(t).Get(endpoint(ctr, "echo_file=/file2.txt"))
		assert.Equal(t, "reader content", rsp)
	})

	t.Run("directory may not exist on the container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		file := tc.ContainerFile{
			HostFilePath:      "testdata/file1.txt",
			ContainerFilePath: "/not-existing/file1.txt",
		}

		// --- When ---
		err := ctr.AddFile(file)

		// --- Then ---
		assert.NoError(t, err)
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		rsp := httpkit.NewRequest(t).
			Get(endpoint(ctr, "echo_file=/not-existing/file1.txt"))
		assert.Equal(t, "file1.txt content", rsp)
	})

	t.Run("error - container running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.AddFile(tc.ContainerFile{})

		// --- Then ---
		assert.ErrorIs(t, ErrRunning, err)
	})
}

func Test_CTR_CopyTo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		src := strings.NewReader("abc")

		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.CopyTo(ctx, src, "/file0.txt", 644)

		// --- Then ---
		assert.NoError(t, err)
		rsp := httpkit.NewRequest(t).Get(endpoint(ctr, "echo_file=/file0.txt"))
		assert.Equal(t, "abc", rsp)
	})

	t.Run("files are overwritten", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()

		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		src0 := strings.NewReader("abc")
		src1 := strings.NewReader("xyz")

		// --- When ---
		assert.NoError(t, ctr.CopyTo(ctx, src0, "/file0.txt", 644))
		err := ctr.CopyTo(ctx, src1, "/file0.txt", 644)

		// --- Then ---
		assert.NoError(t, err)
		rsp := httpkit.NewRequest(t).Get(endpoint(ctr, "echo_file=/file0.txt"))
		assert.Equal(t, "xyz", rsp)
	})

	t.Run("error - container not running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		src := oskit.Open(t, "testdata/file0.txt")

		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		err := ctr.CopyTo(ctx, src, "/file0.txt", 644)

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, err)
	})

	t.Run("error - container is read only", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		src := oskit.Open(t, "testdata/file0.txt")

		ctr := NewCTR("echo", xctrtest.ImageReq())
		must.Nil(ctr.SetReadOnly(ctx))
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.CopyTo(ctx, src, "/file0.txt", 644)

		// --- Then ---
		assert.ErrorIs(t, ErrReadOnly, err)
	})
}

func Test_CTR_ReadFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()

		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have, err := ctr.ReadFile(ctx, "/etc/shells")

		// --- Then ---
		assert.NoError(t, err)
		want := "# valid login shells\n/bin/sh\n/bin/ash\n"
		assert.Equal(t, want, string(have))
	})

	t.Run("error - nonexistent file", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()

		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have, err := ctr.ReadFile(ctx, "/nonexistent")

		// --- Then ---
		wMsg := "Could not find the file /nonexistent in container %s"
		wMsg = fmt.Sprintf(wMsg, ctr.ID())
		assert.ErrorContain(t, wMsg, err)
		assert.Nil(t, have)
	})

	t.Run("error - container not running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have, err := ctr.ReadFile(ctx, "/file0.txt")

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, err)
		assert.Nil(t, have)
	})
}

func Test_CTR_Setenv(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		val := randkit.Str()

		// --- When ---
		err := ctr.Setenv("ABC", val)

		// --- Then ---
		assert.NoError(t, err)
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		rsp := httpkit.NewRequest(t).Get(endpoint(ctr, "echo_env_body=ABC"))
		assert.Equal(t, val, must.Value(strconv.Unquote(rsp)))
	})

	t.Run("nil environment map", func(t *testing.T) {
		// --- Given ---
		ctr := NewCTR("echo", tc.GenericContainerRequest{})

		// --- When ---
		err := ctr.Setenv("ABC", "val")

		// --- Then ---
		assert.NoError(t, err)
		assert.HasKeyValue(t, "ABC", "val", ctr.Request().Env)
	})

	t.Run("error - already running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Setenv("A", "1")

		// --- Then ---
		assert.ErrorIs(t, ErrRunning, err)
	})
}

func Test_CTR_SetLabel(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		val := randkit.Str()

		// --- When ---
		err := ctr.SetLabel(val, val)

		// --- Then ---
		assert.NoError(t, err)
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		hLabels := must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID())).Labels
		assert.HasKeyValue(t, val, val, hLabels)
	})

	t.Run("nil labels map", func(t *testing.T) {
		// --- Given ---
		ctr := NewCTR("echo", tc.GenericContainerRequest{})

		// --- When ---
		err := ctr.SetLabel("lab", "val")

		// --- Then ---
		assert.NoError(t, err)
		assert.HasKeyValue(t, "lab", "val", ctr.Request().Labels)
	})

	t.Run("error - container is running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.SetLabel("A", "1")

		// --- Then ---
		assert.ErrorIs(t, ErrRunning, err)
	})
}

func Test_CTR_Request(t *testing.T) {
	// --- Given ---
	req := xctrtest.ImageReq()
	ctr := NewCTR("echo", req)

	// --- When ---
	have := ctr.Request()

	// --- Then ---
	assert.Equal(t, req, have)
}

func Test_CTR_Container(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.Container()

		// --- Then ---
		assert.NotNil(t, have)
	})

	t.Run("error - not running", func(t *testing.T) {
		// --- Given ---
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have := ctr.Container()

		// --- Then ---
		assert.Nil(t, have)
	})
}

func Test_CTR_ExposePort(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		err := ctr.ExposePort("42/tcp")

		// --- Then ---
		assert.NoError(t, err)
		want := []string{"80/tcp", "42/tcp"}
		assert.Equal(t, want, ctr.Request().ExposedPorts)
	})

	t.Run("error - container is running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.ExposePort("42/tcp")

		// --- Then ---
		assert.ErrorIs(t, ErrRunning, err)
	})
}

func Test_CTR_MappedPort(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have, err := ctr.MappedPort(ctx, network.MustParsePort("80"))

		// --- Then ---
		assert.NoError(t, err)
		assert.NotEmpty(t, have)
		assert.Contain(t, "tcp", string(have.Proto()))
	})

	t.Run("error - container not running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have, err := ctr.MappedPort(ctx, network.MustParsePort("80"))

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, err)
		assert.Empty(t, have)
	})
}

func Test_CTR_ContainerIP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have, err := ctr.ContainerIP(ctx)

		// --- Then ---
		assert.NoError(t, err)
		assert.NotEmpty(t, have)
	})

	t.Run("error - container not running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have, err := ctr.ContainerIP(ctx)

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, err)
		assert.Empty(t, have)
	})
}

func Test_CTR_GatewayIP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have, err := ctr.GatewayIP(ctx)

		// --- Then ---
		assert.NoError(t, err)
		assert.NotEmpty(t, have)
	})

	t.Run("multiple networks", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		net0 := must.Value(tcnet.New(ctx))
		net1 := must.Value(tcnet.New(ctx))
		t.Cleanup(func() {
			assert.NoError(t, net0.Remove(context.WithoutCancel(ctx)))
			assert.NoError(t, net1.Remove(context.WithoutCancel(ctx)))
		})

		req := xctrtest.ImageReq()
		req.Networks = []string{net0.Name, net1.Name}
		ctr := NewCTR("echo", req)
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		ins := must.Value(ctr.dc.Inspect(ctx))
		first := min(net0.Name, net1.Name)
		want := ins.NetworkSettings.Networks[first].Gateway.String()

		// --- When ---
		var have []string
		for range 20 {
			have = append(have, must.Value(ctr.GatewayIP(ctx)))
		}

		// --- Then ---
		assert.Equal(t, slices.Repeat([]string{want}, 20), have)
	})

	t.Run("error - container not running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have, err := ctr.GatewayIP(ctx)

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, err)
		assert.Empty(t, have)
	})
}

func Test_CTR_Pause(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Pause(ctx)

		// --- Then ---
		assert.NoError(t, err)
		hCtr := must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID()))
		assert.True(t, hCtr.State.Paused)
	})

	t.Run("error - already paused", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		assert.NoError(t, ctr.Pause(ctx))

		// --- When ---
		err := ctr.Pause(ctx)

		// --- Then ---
		assert.ErrorContain(t, "is already paused", err)
	})

	t.Run("error - not running container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		// Something external kills the container.
		dkrkit.NewT(t).CtrKill(ctr.ID())

		// --- When ---
		err := ctr.Pause(ctx)

		// --- Then ---
		assert.ErrorContain(t, "is not running", err)
	})

	t.Run("error - not started", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		err := ctr.Pause(ctx)

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, err)
	})
}

func Test_CTR_Unpause(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		assert.NoError(t, ctr.Pause(ctx))

		// --- When ---
		err := ctr.Unpause(ctx)

		// --- Then ---
		assert.NoError(t, err)
		hCtr := must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID()))
		assert.True(t, hCtr.State.Running)
	})

	t.Run("error - already running", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Unpause(ctx)

		// --- Then ---
		assert.ErrorContain(t, "is not paused", err)
	})

	t.Run("error - not running container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		// Something external kills the container.
		dkrkit.NewT(t).CtrKill(ctr.ID())

		// --- When ---
		err := ctr.Unpause(ctx)

		// --- Then ---
		assert.ErrorContain(t, "is not paused", err)
	})

	t.Run("error - not started", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		err := ctr.Unpause(ctx)

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, err)
	})
}

func Test_CTR_Terminate(t *testing.T) {
	t.Run("terminate started", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		assert.NotNil(t, must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID())))

		// --- When ---
		err := ctr.Terminate(ctx)

		// --- Then ---
		assert.NoError(t, err)
		ctrGone(t, ctr.ID())
		assert.Nil(t, ctr.dc)
	})

	t.Run("error - not started", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		err := ctr.Terminate(ctx)

		// --- Then ---
		assert.ErrorIs(t, ErrNotStarted, err)
	})

	t.Run("remove image", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		imgRef, imgID := dkrkit.NewT(t).Build(
			dkrkit.WithBuildPth("xctrtest/data/simple/Dockerfile"),
		)

		req := tc.GenericContainerRequest{
			Started: true,
			ContainerRequest: tc.ContainerRequest{
				Image: imgRef,
			},
		}
		ctr := NewCTR("remove", req, WithCTRImgRm)
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		assert.NotNil(t, dkrkit.NewT(t).ImgLs().FindByID(imgID))
		assert.NotNil(t, must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID())))

		// --- When ---
		err := ctr.Terminate(ctx)

		// --- Then ---
		assert.NoError(t, err)
		assert.Nil(t, dkrkit.NewT(t).ImgLs().FindByID(imgID))
		ctrGone(t, ctr.ID())
		assert.Nil(t, ctr.dc)
	})
}

func Test_handleErr(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		// --- When ---
		have := handleErr(nil)

		// --- Then ---
		assert.False(t, have)
	})
}

func Test_handleErr_tabular(t *testing.T) {
	inProgress := "removal of container abc is already in progress"

	tt := []struct {
		testN string

		err  error
		want bool
	}{
		{"in progress", cerrdefs.ErrConflict.WithMessage(inProgress), false},
		{"not found", cerrdefs.ErrNotFound.WithMessage("container abc"), false},
		{"wrapped", fmt.Errorf("stop: %w", cerrdefs.ErrNotFound), false},
		{"other conflict", cerrdefs.ErrConflict.WithMessage("paused"), true},
		{"untyped", errors.New("No such container: abc"), true},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := handleErr(tc.err)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_CTR_Describe(t *testing.T) {
	t.Run("not started", func(t *testing.T) {
		// --- Given ---
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have := ctr.Describe()

		// --- Then ---
		want := "\n" +
			"\t>| (not started) [echo] > ---\n" +
			"\t>| (not started) [echo] > not started\n" +
			"\t>| (not started) [echo] > ---\n"
		assert.Equal(t, want, have)
	})

	t.Run("started with files", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR("echo", xctrtest.ImageReq())
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
		assert.NoError(t, ctr.AddFile(files...))
		must.Nil(ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		have := ctr.Describe()

		// --- Then ---
		assert.Contain(t, "CTR Description", have)
		assert.Contain(t, "HOST: ", have)
		assert.Contain(t, "PORT_0: ", have)
		assert.Contain(t, "Files added:", have)
		assert.Contain(t, "[echo] > \ttestdata/file0.txt -> /file0.txt", have)
		assert.Contain(t, "[echo] > \t<reader> -> /file2.txt", have)
	})
}

func Test_CTR_FormatInfoLine(t *testing.T) {
	t.Run("not started container", func(t *testing.T) {
		// --- Given ---
		ctr := NewCTR("echo", xctrtest.ImageReq())

		// --- When ---
		have := ctr.FormatInfoLine("ab%s", "c")

		// --- Then ---
		assert.Equal(t, "\t>| (not started) [echo] > abc\n", have)
	})

	t.Run("started container", func(t *testing.T) {
		// --- Given ---
		ctr := NewCTR("echo", xctrtest.ImageReq())
		ctr.id = "1234567890"

		// --- When ---
		have := ctr.FormatInfoLine("ab%s", "c")

		// --- Then ---
		assert.Equal(t, "\t>| (12345678) [echo] > abc\n", have)
	})
}

func Test_scmFromGit(t *testing.T) {
	// --- When ---
	have := scmFromGit(t.Context())

	// --- Then ---
	assert.NotEmpty(t, have.repo)
	assert.NotEmpty(t, have.rev)
	assert.NotEmpty(t, have.hash)
}

func Test_buildMeta(t *testing.T) {
	// --- Given ---
	env := []string{
		xdef.EnvBldDate + "=2000-01-02T03:04:05Z",
	}
	scm := scmInfo{repo: "repo", rev: "rev", hash: "hash"}

	// --- When ---
	have := buildMeta(env, "ctr-name", scm)

	// --- Then ---
	assert.Equal(t, map[string]string{
		xdef.LabImgSrc:     "repo",
		xdef.LabImgVer:     "rev",
		xdef.LabImgRev:     "hash",
		xdef.LabImgCreated: "2000-01-02T03:04:05Z",
		LabTestCtrName:     "ctr-name",
	}, have.labels)
	assert.Equal(t, map[string]string{
		xdef.EnvBldDate: "2000-01-02T03:04:05Z",
		xdef.EnvScmRepo: "repo",
		xdef.EnvScmRev:  "rev",
		xdef.EnvScmHash: "hash",
	}, have.env)
}

func Test_prepareRequest(t *testing.T) {
	t.Run("from image sets labels and env", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.ImageReq()
		meta := buildMeta(
			[]string{
				xdef.EnvBldDate + "=2000-01-02T03:04:05Z",
			},
			"echo",
			scmInfo{repo: "r", rev: "v", hash: "h"},
		)

		// --- When ---
		have := prepareRequest(req, meta, nil, nil)

		// --- Then ---
		assert.Equal(t, "r", have.Labels[xdef.LabImgSrc])
		assert.Equal(t, "2000-01-02T03:04:05Z", have.Labels[xdef.LabImgCreated])
		assert.Equal(t, "echo", have.Labels[LabTestCtrName])
		assert.Equal(t, "2000-01-02T03:04:05Z", have.Env[xdef.EnvBldDate])
	})

	t.Run("from Dockerfile sets build args and modifier", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.DockerfileReq()
		req.Repo = ""
		req.Tag = ""
		meta := buildMeta(
			[]string{
				xdef.EnvBldDate + "=2000-01-02T03:04:05Z",
			},
			"echo",
			scmInfo{repo: "r", rev: "v", hash: "h"},
		)

		// --- When ---
		have := prepareRequest(req, meta, nil, nil)

		// --- Then ---
		assert.NotEmpty(t, have.Repo)
		assert.NotEmpty(t, have.Tag)
		want := "2000-01-02T03:04:05Z"
		assert.Equal(t, want, *have.BuildArgs[xdef.EnvBldDate])
		assert.NotNil(t, have.BuildOptionsModifier)

		opts := &client.ImageBuildOptions{}
		have.BuildOptionsModifier(opts)
		assert.Equal(t, "r", opts.Labels[xdef.LabImgSrc])
		assert.Equal(t, "echo", opts.Labels[LabTestCtrName])
		assert.Equal(t, "2000-01-02T03:04:05Z", have.Env[xdef.EnvBldDate])
	})

	t.Run("build log enabled", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.DockerfileReq()
		meta := buildMeta(nil, "echo", scmInfo{})
		env := []string{EnvCTRBuildLog + "=true"}

		// --- When ---
		have := prepareRequest(req, meta, env, nil)

		// --- Then ---
		assert.Same(t, os.Stdout, have.FromDockerfile.BuildLogWriter)
	})

	t.Run("build log not enabled by other values", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.DockerfileReq()
		meta := buildMeta(nil, "echo", scmInfo{})
		env := []string{EnvCTRBuildLog + "=false"}

		// --- When ---
		have := prepareRequest(req, meta, env, nil)

		// --- Then ---
		assert.Nil(t, have.FromDockerfile.BuildLogWriter)
	})

	t.Run("entrypoint override clears WaitingFor", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.DockerfileReq()
		assert.NotNil(t, req.WaitingFor)
		env := SetEnvCTREntrypoint(nil)
		meta := buildMeta(nil, "echo", scmInfo{})

		// --- When ---
		have := prepareRequest(req, meta, env, nil)

		// --- Then ---
		assert.Nil(t, have.WaitingFor)
		want := []string{"tini", "--", "tail", "-f", "/dev/null"}
		assert.Equal(t, want, have.Entrypoint)
	})

	t.Run("log consumer when EnvCTRLog true", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.ImageReq()
		lc := NewLogger(false)
		env := []string{EnvCTRLog + "=true"}
		meta := buildMeta(nil, "echo", scmInfo{})

		// --- When ---
		have := prepareRequest(req, meta, env, lc)

		// --- Then ---
		assert.NotNil(t, have.LogConsumerCfg)
		assert.Len(t, 1, have.LogConsumerCfg.Consumers)
		assert.Same(t, lc, have.LogConsumerCfg.Consumers[0])
	})

	t.Run("preserves existing BuildOptionsModifier", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.DockerfileReq()
		req.BuildOptionsModifier = func(opts *client.ImageBuildOptions) {
			opts.Labels = map[string]string{"test": "abc"}
		}
		meta := buildMeta(nil, "echo", scmInfo{repo: "r"})

		// --- When ---
		have := prepareRequest(req, meta, nil, nil)

		// --- Then ---
		opts := &client.ImageBuildOptions{}
		have.BuildOptionsModifier(opts)
		assert.Equal(t, "abc", opts.Labels["test"])
		assert.Equal(t, "r", opts.Labels[xdef.LabImgSrc])
	})

	t.Run("does not override existing request env", func(t *testing.T) {
		// --- Given ---
		req := tc.GenericContainerRequest{
			ContainerRequest: tc.ContainerRequest{
				Image: xctrtest.EchoServerRef,
				Env:   map[string]string{xdef.EnvBldDate: "keep"},
			},
		}
		meta := buildMeta(
			[]string{xdef.EnvBldDate + "=from-env"},
			"echo",
			scmInfo{},
		)

		// --- When ---
		have := prepareRequest(req, meta, nil, nil)

		// --- Then ---
		assert.Equal(t, "keep", have.Env[xdef.EnvBldDate])
	})
}

func Test_CTR_Start(t *testing.T) {
	// Same SCM resolution Start uses, so expectations track the checkout.
	scm := scmFromGit(t.Context())

	t.Run("from image", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		env := []string{
			xdef.EnvBldDate + "=2000-01-02T03:04:05Z",
		}
		ctr := NewCTR(t.Name(), xctrtest.ImageReq())
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Start(ctx, env)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, ctr.dc.GetContainerID(), ctr.ID())
		assert.Equal(t, xctrtest.EchoServerRef, ctr.Reference())

		hCtr := must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID()))
		wEnv := map[string]string{
			xdef.EnvBldDate: "2000-01-02T03:04:05Z",
			xdef.EnvScmRepo: scm.repo,
			xdef.EnvScmRev:  scm.rev,
			xdef.EnvScmHash: scm.hash,
		}
		assert.MapSubset(t, wEnv, hCtr.Env)
		wLabels := map[string]string{
			xdef.LabImgCreated: "2000-01-02T03:04:05Z",
			xdef.LabImgSrc:     scm.repo,
			xdef.LabImgVer:     scm.rev,
			xdef.LabImgRev:     scm.hash,
			LabTestCtrName:     t.Name(),
		}
		assert.MapSubset(t, wLabels, hCtr.Labels)
		httpkit.NewRequest(t).Get(endpoint(ctr))
	})

	t.Run("from Dockerfile", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		env := []string{
			xdef.EnvBldDate + "=2000-01-02T03:04:05Z",
		}
		ctr := NewCTR("echo dkr", xctrtest.DockerfileReq())
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Start(ctx, env)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, ctr.dc.GetContainerID(), ctr.ID())
		inspect := must.Value(ctr.dc.Inspect(ctx))
		assert.Equal(t, inspect.Config.Image, ctr.Reference())

		wArgs := map[string]*string{
			xdef.EnvBldDate:    new("2000-01-02T03:04:05Z"),
			xdef.EnvBldImgBase: new(xctrtest.EchoServerRef),
		}
		assert.Equal(t, wArgs, ctr.runReq.BuildArgs)

		hCtr := must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID()))
		wEnv := map[string]string{
			xdef.EnvBldDate: "2000-01-02T03:04:05Z",
			xdef.EnvScmRepo: scm.repo,
			xdef.EnvScmRev:  scm.rev,
			xdef.EnvScmHash: scm.hash,
		}
		assert.MapSubset(t, wEnv, hCtr.Env)
		wLabels := map[string]string{
			xdef.LabImgCreated: "2000-01-02T03:04:05Z",
			xdef.LabImgSrc:     scm.repo,
			xdef.LabImgVer:     scm.rev,
			xdef.LabImgRev:     scm.hash,
			LabTestCtrName:     "echo dkr",
		}
		assert.MapSubset(t, wLabels, hCtr.Labels)
		httpkit.NewRequest(t).Get(endpoint(ctr))
	})

	t.Run("from Dockerfile not started", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		req := xctrtest.DockerfileReq()
		req.Started = false
		ctr := NewCTR("echo dkr", req)
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Start(ctx, nil)

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("default envs set even if slice is nil", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		env := []string{
			xdef.EnvBldDate + "=2000-01-02T03:04:05Z",
		}
		req := xctrtest.ImageReq()
		req.Env = nil
		ctr := NewCTR(t.Name(), req)
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Start(ctx, env)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, ctr.dc.GetContainerID(), ctr.ID())
		assert.Equal(t, xctrtest.EchoServerRef, ctr.Reference())

		hCtr := must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID()))
		wEnv := map[string]string{
			xdef.EnvBldDate: "2000-01-02T03:04:05Z",
			xdef.EnvScmRepo: scm.repo,
			xdef.EnvScmRev:  scm.rev,
			xdef.EnvScmHash: scm.hash,
		}
		assert.MapSubset(t, wEnv, hCtr.Env)

		httpkit.NewRequest(t).Get(endpoint(ctr))
	})

	t.Run("guest and host config", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()

		port := must.Value(netkit.GetFreePort())
		expose := fmt.Sprintf("%d/tcp", port)
		req := xctrtest.ImageReq()
		req.ExposedPorts = append(req.ExposedPorts, expose)

		ctr := NewCTR(t.Name(), req)
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Start(ctx, nil)

		// --- Then ---
		assert.NoError(t, err)

		wMap := map[string]string{
			"PORT_0": "80/tcp",
			"PORT_1": strconv.Itoa(port) + "/tcp",
			"HOST":   must.Value(ctr.dc.ContainerIP(ctx)),
		}
		assert.Equal(t, wMap, ctr.cfgGuest)

		wPort80 := must.Value(ctr.dc.MappedPort(ctx, "80"))
		wPortPort := must.Value(ctr.dc.MappedPort(ctx, expose))
		wMap = map[string]string{
			"PORT_0": wPort80.String(),
			"PORT_1": wPortPort.String(),
			"HOST":   "localhost",
		}
		assert.Equal(t, wMap, ctr.cfgHost)
	})

	t.Run("BuildOptionsModifier not overwritten", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		req := xctrtest.DockerfileReq()
		req.BuildOptionsModifier = func(opts *client.ImageBuildOptions) {
			opts.Labels = map[string]string{"test": "abc"}
		}
		ctr := NewCTR(t.Name(), req)
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Start(ctx, nil)

		// --- Then ---
		assert.NoError(t, err)
		gotCtr := must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID()))
		assert.HasKeyValue(t, "test", "abc", gotCtr.Labels)
	})

	t.Run("collect logs", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		env := []string{
			xdef.EnvBldDate + "=2000-01-02T03:04:05Z",
			EnvCTRLog + "=true",
		}
		lc := NewLogger(false)
		ctr := NewCTR(t.Name(), xctrtest.ImageReq(), WithCTRLogger(lc))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		// --- When ---
		err := ctr.Start(ctx, env)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "Listening on port 80.\n", lc.Print())
	})

	t.Run("override entrypoint", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		env := SetEnvCTREntrypoint(nil)
		req := xctrtest.DockerfileReq()

		ctr := NewCTR(t.Name(), req)
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})

		assert.NotNil(t, ctr.Request().WaitingFor)
		assert.Nil(t, ctr.Request().Entrypoint)
		assert.Equal(t, []string{"80/tcp"}, ctr.Request().ExposedPorts)

		// --- When ---
		err := ctr.Start(ctx, env)

		// --- Then ---
		assert.NoError(t, err)
		assert.Nil(t, ctr.runReq.WaitingFor)
		wEP := []string{"tini", "--", "tail", "-f", "/dev/null"}
		assert.Equal(t, wEP, ctr.runReq.Entrypoint)
		assert.Equal(t, []string{"80/tcp"}, ctr.runReq.ExposedPorts)
		assert.Equal(t, ctr.dc.GetContainerID(), ctr.ID())
		assert.NotNil(t, ctr.Request().WaitingFor)
		assert.Nil(t, ctr.Request().Entrypoint)
	})

	t.Run("restart from Dockerfile", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR(t.Name(), xctrtest.DockerfileReq())
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		assert.NoError(t, ctr.Start(ctx, SetEnvCTREntrypoint(nil)))
		assert.NoError(t, ctr.Terminate(ctx))

		// --- When ---
		err := ctr.Start(ctx, nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, ctr.IsRunning())
		assert.NotNil(t, ctr.runReq.WaitingFor)
		assert.Nil(t, ctr.runReq.Entrypoint)
	})

	t.Run("error - wait strategy terminates container", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		req := xctrtest.ImageReq()
		req.WaitingFor = wait.ForLog("never logged").
			WithStartupTimeout(time.Second)
		ctr := NewCTR(t.Name(), req)

		// --- When ---
		err := ctr.Start(ctx, nil)

		// --- Then ---
		assert.ErrorContain(t, "start container", err)

		cli := xctrtest.NewClient(t)
		label := LabTestCtrName + "=" + t.Name()
		opts := client.ContainerListOptions{
			All:     true,
			Filters: make(client.Filters).Add("label", label),
		}
		lst := must.Value(cli.ContainerList(ctx, opts))
		for _, itm := range lst.Items {
			ctrGone(t, itm.ID)
		}
	})

	t.Run("error - already started", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr := NewCTR(t.Name(), xctrtest.ImageReq())
		assert.NoError(t, ctr.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(ctx)))
		})
		id := ctr.ID()

		// --- When ---
		err := ctr.Start(ctx, nil)

		// --- Then ---
		assert.ErrorIs(t, ErrRunning, err)
		assert.Equal(t, id, ctr.ID())
	})
}
