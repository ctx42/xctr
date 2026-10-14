// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
	"github.com/moby/moby/client"
	tc "github.com/testcontainers/testcontainers-go"
)

func Test_Sweep(t *testing.T) {
	t.Run("removes orphans", func(t *testing.T) {
		// --- Given ---
		labels := map[string]string{
			LabTestCtrName: t.Name(),
			labTCReap:      "true",
			labTCSession:   RandTag(),
		}
		id := createCtr(t, labels)
		ref := commitImg(t, labels)

		// --- When ---
		err := Sweep(t.Context(), 0)

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, ctrExists(t, id))
		assert.False(t, imgExists(t, ref))
	})

	t.Run("keeps resources within grace", func(t *testing.T) {
		// --- Given ---
		labels := map[string]string{
			LabTestCtrName: t.Name(),
			labTCReap:      "true",
			labTCSession:   RandTag(),
		}
		id := createCtr(t, labels)
		ref := commitImg(t, labels)

		// --- When ---
		err := Sweep(t.Context(), time.Hour)

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, ctrExists(t, id))
		assert.True(t, imgExists(t, ref))
	})

	t.Run("error - docker client", func(t *testing.T) {
		// --- Given ---
		orig := newDockerClient
		t.Cleanup(func() { newDockerClient = orig })
		newDockerClient = func(context.Context) (*client.Client, error) {
			return nil, errors.New("unable to parse docker host")
		}

		// --- When ---
		err := Sweep(t.Context(), 0)

		// --- Then ---
		assert.ErrorEqual(t, "sweep: unable to parse docker host", err)
	})
}

func Test_liveSessions(t *testing.T) {
	t.Run("running reaper", func(t *testing.T) {
		// --- Given ---
		sid := RandTag()
		startCtr(t, map[string]string{labTCReaper: "true", labTCSession: sid})

		// --- When ---
		have, err := liveSessions(t.Context(), dockerClient(t))

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have[sid])
	})

	t.Run("stopped reaper", func(t *testing.T) {
		// --- Given ---
		sid := RandTag()
		createCtr(t, map[string]string{labTCReaper: "true", labTCSession: sid})

		// --- When ---
		have, err := liveSessions(t.Context(), dockerClient(t))

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, have[sid])
	})

	t.Run("own session", func(t *testing.T) {
		// --- When ---
		have, err := liveSessions(t.Context(), dockerClient(t))

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have[tc.SessionID()])
	})
}

func Test_sweepContainers(t *testing.T) {
	t.Run("removes orphan", func(t *testing.T) {
		// --- Given ---
		labels := map[string]string{
			LabTestCtrName: t.Name(),
			labTCReap:      "true",
			labTCSession:   RandTag(),
		}
		id := createCtr(t, labels)
		cli := dockerClient(t)

		// --- When ---
		err := sweepContainers(t.Context(), cli, nil, time.Now())

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, ctrExists(t, id))
	})

	t.Run("keeps live session", func(t *testing.T) {
		// --- Given ---
		sid := RandTag()
		labels := map[string]string{
			LabTestCtrName: t.Name(),
			labTCReap:      "true",
			labTCSession:   sid,
		}
		id := createCtr(t, labels)
		cli := dockerClient(t)
		live := map[string]bool{sid: true}

		// --- When ---
		err := sweepContainers(t.Context(), cli, live, time.Now())

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, ctrExists(t, id))
	})

	t.Run("keeps younger than cutoff", func(t *testing.T) {
		// --- Given ---
		labels := map[string]string{
			LabTestCtrName: t.Name(),
			labTCReap:      "true",
			labTCSession:   RandTag(),
		}
		id := createCtr(t, labels)
		cli := dockerClient(t)
		cutoff := time.Now().Add(-time.Hour)

		// --- When ---
		err := sweepContainers(t.Context(), cli, nil, cutoff)

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, ctrExists(t, id))
	})

	t.Run("keeps not reaped", func(t *testing.T) {
		// --- Given ---
		labels := map[string]string{
			LabTestCtrName: t.Name(),
			labTCSession:   RandTag(),
		}
		id := createCtr(t, labels)
		cli := dockerClient(t)

		// --- When ---
		err := sweepContainers(t.Context(), cli, nil, time.Now())

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, ctrExists(t, id))
	})

	t.Run("keeps not created by xctr", func(t *testing.T) {
		// --- Given ---
		labels := map[string]string{
			labTCReap:    "true",
			labTCSession: RandTag(),
		}
		id := createCtr(t, labels)
		cli := dockerClient(t)

		// --- When ---
		err := sweepContainers(t.Context(), cli, nil, time.Now())

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, ctrExists(t, id))
	})
}

func Test_sweepImages(t *testing.T) {
	t.Run("removes orphan", func(t *testing.T) {
		// --- Given ---
		labels := map[string]string{
			labTCReap:    "true",
			labTCSession: RandTag(),
		}
		ref := commitImg(t, labels)
		cli := dockerClient(t)

		// --- When ---
		err := sweepImages(t.Context(), cli, nil, time.Now())

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, imgExists(t, ref))
	})

	t.Run("keeps live session", func(t *testing.T) {
		// --- Given ---
		sid := RandTag()
		labels := map[string]string{
			labTCReap:    "true",
			labTCSession: sid,
		}
		ref := commitImg(t, labels)
		cli := dockerClient(t)
		live := map[string]bool{sid: true}

		// --- When ---
		err := sweepImages(t.Context(), cli, live, time.Now())

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, imgExists(t, ref))
	})

	t.Run("keeps younger than cutoff", func(t *testing.T) {
		// --- Given ---
		labels := map[string]string{
			labTCReap:    "true",
			labTCSession: RandTag(),
		}
		ref := commitImg(t, labels)
		cli := dockerClient(t)
		cutoff := time.Now().Add(-time.Hour)

		// --- When ---
		err := sweepImages(t.Context(), cli, nil, cutoff)

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, imgExists(t, ref))
	})

	t.Run("keeps not reaped", func(t *testing.T) {
		// --- Given ---
		ref := commitImg(t, map[string]string{labTCSession: RandTag()})
		cli := dockerClient(t)

		// --- When ---
		err := sweepImages(t.Context(), cli, nil, time.Now())

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, imgExists(t, ref))
	})

	t.Run("keeps other tags", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		labels := map[string]string{
			labTCReap:    "true",
			labTCSession: RandTag(),
		}
		ref := commitImg(t, labels)
		cli := dockerClient(t)

		other := "xctr-sweep-test:" + RandTag()
		tagOpts := client.ImageTagOptions{Source: ref, Target: other}
		must.Value(cli.ImageTag(ctx, tagOpts))
		t.Cleanup(func() {
			rmOpts := client.ImageRemoveOptions{Force: true}
			_, _ = cli.ImageRemove(context.WithoutCancel(ctx), other, rmOpts)
		})

		// --- When ---
		err := sweepImages(ctx, cli, nil, time.Now())

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, imgExists(t, ref))
		assert.True(t, imgExists(t, other))
	})

	t.Run("keeps tag used by container", func(t *testing.T) {
		// --- Given ---
		labels := map[string]string{
			labTCReap:    "true",
			labTCSession: RandTag(),
		}
		ref := commitImg(t, labels)
		cli := dockerClient(t)
		opts := client.ContainerCreateOptions{Image: ref}
		res := must.Value(cli.ContainerCreate(t.Context(), opts))
		t.Cleanup(func() {
			ctx := context.WithoutCancel(t.Context())
			rmOpts := client.ContainerRemoveOptions{Force: true}
			_, _ = cli.ContainerRemove(ctx, res.ID, rmOpts)
		})

		// --- When ---
		err := sweepImages(t.Context(), cli, nil, time.Now())

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, imgExists(t, ref))
	})
}
