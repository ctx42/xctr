// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctrtest

import (
	"context"
	"errors"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
	"github.com/ctx42/testing/pkg/tester"
	"github.com/ctx42/testkit/pkg/dkrkit"
	"github.com/ctx42/xctr/pkg/xctr"
	"github.com/ctx42/xdef/pkg/xdef"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	tc "github.com/testcontainers/testcontainers-go"
)

// errTest is a sentinel error used in tests.
var errTest = errors.New("test error")

func Test_ImageReq(t *testing.T) {
	t.Run("setup", func(t *testing.T) {
		// --- When ---
		have := ImageReq()

		// --- Then ---
		assert.True(t, have.Started)
		assert.Equal(t, EchoServerRef, have.Image)
		cc := &container.Config{}
		have.ConfigModifier(cc)
		assert.Equal(t, "echo", cc.Hostname)
		assert.Equal(t, []string{"80/tcp"}, have.ExposedPorts)
		hc := &container.HostConfig{}
		have.HostConfigModifier(hc)
		assert.True(t, hc.AutoRemove)
		assert.Nil(t, have.Env)
		assert.Nil(t, have.Labels)
	})

	t.Run("start", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		req := ImageReq()

		// --- When ---
		have, err := tc.GenericContainer(ctx, req)

		// --- Then ---
		assert.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, tc.TerminateContainer(have)) })
		cid := have.GetContainerID()
		hCtr := must.Value(dkrkit.NewT(t).CtrPs().FindByID(cid))
		assert.NotNil(t, hCtr)
	})
}

func Test_DockerfileReq(t *testing.T) {
	t.Run("setup", func(t *testing.T) {
		// --- When ---
		have := DockerfileReq()

		// --- Then ---
		assert.True(t, have.Started)
		assert.False(t, have.KeepImage)
		assert.NotNil(t, have.ContextArchive)
		assert.Equal(t, "Dockerfile", have.Dockerfile)
		assert.Equal(t, EchoServerRef, *have.BuildArgs[xdef.EnvBldImgBase])
		assert.Equal(t, []string{"80/tcp"}, have.ExposedPorts)
		cc := &container.Config{}
		have.ConfigModifier(cc)
		assert.Equal(t, "echo", cc.Hostname)
		hc := &container.HostConfig{}
		have.HostConfigModifier(hc)
		assert.True(t, hc.AutoRemove)
		assert.Nil(t, have.Env)
		assert.Nil(t, have.Labels)
		assert.Nil(t, have.BuildOptionsModifier)
	})

	t.Run("build", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		req := DockerfileReq()

		// --- When ---
		have, err := tc.GenericContainer(ctx, req)

		// --- Then ---
		assert.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, tc.TerminateContainer(have)) })
		cid := have.GetContainerID()
		hCtr := must.Value(dkrkit.NewT(t).CtrPs().FindByID(cid))
		assert.NotNil(t, hCtr)
	})
}

func Test_NewClient(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		tspy := tester.New(t)
		tspy.ExpectCleanups(1)
		tspy.Close()

		// --- When ---
		have := NewClient(tspy)

		// --- Then ---
		_, err := have.Info(t.Context(), client.InfoOptions{})
		assert.NoError(t, err)
	})

	t.Run("error - invalid docker host", func(t *testing.T) {
		// --- Given ---
		t.Setenv("DOCKER_HOST", "invalid")
		tspy := tester.New(t)
		tspy.ExpectFatal()
		tspy.IgnoreLogs()
		tspy.Close()

		// --- When ---
		msg := assert.PanicMsg(t, func() { NewClient(tspy) })

		// --- Then ---
		assert.Equal(t, tester.FailNowMsg, *msg)
	})
}

// failCtr is a [Container] whose Start always fails.
type failCtr struct{}

func (failCtr) Start(context.Context, []string) error { return errTest }
func (failCtr) Cleanup(context.Context) error         { return nil }

func Test_CanStart(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		tspy := tester.New(t)
		tspy.ExpectCleanups(1)
		tspy.ExpectLogContain(") [echo] > CTR Description:\n\t>|")
		tspy.Close()

		ctr := xctr.NewCTR("echo", DockerfileReq())

		// --- When ---
		CanStart(tspy, nil, ctr)

		// --- Then ---
		t.Cleanup(func() {
			assert.NoError(t, ctr.Cleanup(context.WithoutCancel(t.Context())))
		})
		assert.True(t, ctr.IsRunning())
		hCtr := must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr.ID()))
		assert.NotNil(t, hCtr)
	})

	t.Run("error - start fails", func(t *testing.T) {
		// --- Given ---
		tspy := tester.New(t)
		tspy.ExpectError()
		tspy.ExpectCleanups(1)
		tspy.IgnoreLogs()
		tspy.Close()

		// --- When ---
		CanStart(tspy, nil, failCtr{})

		// --- Then ---
		assert.True(t, tspy.Failed())
	})
}
