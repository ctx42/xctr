// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
	"github.com/ctx42/xdef/pkg/xdef"
	"github.com/moby/moby/client"
	tc "github.com/testcontainers/testcontainers-go"

	"github.com/ctx42/xctr/pkg/xctr/xctrtest"
)

func Test_sharedImage(t *testing.T) {
	t.Run("builds on first use", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		var calls atomic.Int32
		bld := func(context.Context) (string, error) {
			calls.Add(1)
			return "repo:tag", nil
		}

		// --- When ---
		have, err := sharedImage(t.Context(), "key", bld)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "repo:tag", have)
		assert.Equal(t, int32(1), calls.Load())
	})

	t.Run("reuses the build", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		var calls atomic.Int32
		bld := func(context.Context) (string, error) {
			calls.Add(1)
			return "repo:tag", nil
		}
		must.Value(sharedImage(t.Context(), "key", bld))

		// --- When ---
		have, err := sharedImage(t.Context(), "key", bld)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "repo:tag", have)
		assert.Equal(t, int32(1), calls.Load())
	})

	t.Run("concurrent caller waits for the build", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		resetShared(t)
		var calls atomic.Int32
		started, release := make(chan struct{}), make(chan struct{})
		bld := func(context.Context) (string, error) {
			calls.Add(1)
			close(started)
			<-release
			return "repo:tag", nil
		}
		first := make(chan string, 1)
		go func() {
			ref, _ := sharedImage(ctx, "key", bld)
			first <- ref
		}()
		<-started
		time.AfterFunc(50*time.Millisecond, func() { close(release) })

		// --- When ---
		have, err := sharedImage(ctx, "key", bld)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "repo:tag", have)
		assert.Equal(t, "repo:tag", <-first)
		assert.Equal(t, int32(1), calls.Load())
	})

	t.Run("builds each key", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		var calls atomic.Int32
		bld := func(context.Context) (string, error) {
			calls.Add(1)
			return "repo:tag", nil
		}
		must.Value(sharedImage(t.Context(), "key", bld))

		// --- When ---
		have, err := sharedImage(t.Context(), "other", bld)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "repo:tag", have)
		assert.Equal(t, int32(2), calls.Load())
	})

	t.Run("builds again after error", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		fail := func(context.Context) (string, error) { return "", errTest }
		_, _ = sharedImage(t.Context(), "key", fail)
		bld := func(context.Context) (string, error) { return "repo:tag", nil }

		// --- When ---
		have, err := sharedImage(t.Context(), "key", bld)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "repo:tag", have)
	})

	t.Run("error - build", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		fail := func(context.Context) (string, error) { return "", errTest }

		// --- When ---
		have, err := sharedImage(t.Context(), "key", fail)

		// --- Then ---
		assert.ErrorIs(t, errTest, err)
		assert.Empty(t, have)
		assert.HasNoKey(t, "key", sharedImgs)
	})

	t.Run("error - context cancelled while waiting", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		sharedImgs["key"] = &sharedImg{done: make(chan struct{})}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		bld := func(context.Context) (string, error) { return "repo:tag", nil }

		// --- When ---
		have, err := sharedImage(ctx, "key", bld)

		// --- Then ---
		assert.ErrorIs(t, context.Canceled, err)
		assert.Empty(t, have)
	})
}

func Test_dropShared(t *testing.T) {
	t.Run("removes shared images", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		ref := commitImg(t, nil)
		sharedImgs["key"] = builtImg(ref)

		// --- When ---
		err := dropShared(t.Context(), dockerClient(t))

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, imgExists(t, ref))
		assert.Len(t, 0, sharedImgs)
	})

	t.Run("keeps building", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		sharedImgs["key"] = &sharedImg{done: make(chan struct{})}

		// --- When ---
		err := dropShared(t.Context(), dockerClient(t))

		// --- Then ---
		assert.NoError(t, err)
		assert.HasKey(t, "key", sharedImgs)
	})

	t.Run("keeps tag used by container", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		ref := commitImg(t, nil)
		sharedImgs["key"] = builtImg(ref)
		cli := dockerClient(t)
		opts := client.ContainerCreateOptions{Image: ref}
		res := must.Value(cli.ContainerCreate(t.Context(), opts))
		t.Cleanup(func() {
			ctx := context.WithoutCancel(t.Context())
			rmOpts := client.ContainerRemoveOptions{Force: true}
			_, _ = cli.ContainerRemove(ctx, res.ID, rmOpts)
		})

		// --- When ---
		err := dropShared(t.Context(), cli)

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, imgExists(t, ref))
		assert.Len(t, 0, sharedImgs)
	})

	t.Run("error - remove", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		sharedImgs["key"] = builtImg("Invalid:Ref")

		// --- When ---
		err := dropShared(t.Context(), dockerClient(t))

		// --- Then ---
		assert.ErrorContain(t, "remove image Invalid:Ref", err)
	})
}

func Test_untag(t *testing.T) {
	t.Run("removes image", func(t *testing.T) {
		// --- Given ---
		ref := commitImg(t, nil)

		// --- When ---
		err := untag(t.Context(), dockerClient(t), ref)

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, imgExists(t, ref))
	})

	t.Run("missing", func(t *testing.T) {
		// --- When ---
		err := untag(t.Context(), dockerClient(t), RandRef())

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("error - invalid reference", func(t *testing.T) {
		// --- When ---
		err := untag(t.Context(), dockerClient(t), "Invalid:Ref")

		// --- Then ---
		assert.ErrorContain(t, "remove image Invalid:Ref", err)
	})
}

func Test_buildImage(t *testing.T) {
	t.Run("builds", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.DockerfileReq()
		req.Repo, req.Tag = RandName(), RandTag()
		t.Cleanup(func() {
			ctx := context.WithoutCancel(t.Context())
			_ = untag(ctx, dockerClient(t), req.Repo+":"+req.Tag)
		})

		// --- When ---
		have, err := buildImage(t.Context(), req)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, req.Repo+":"+req.Tag, have)
		assert.True(t, imgExists(t, have))
	})

	t.Run("error - build fails", func(t *testing.T) {
		// --- Given ---
		req := xctrtest.DockerfileReq()
		dockerfile := "FROM " + xctrtest.EchoServerRef + "\nRUN exit 3\n"
		req.ContextArchive = MustDockerfileArchive([]byte(dockerfile))
		req.Repo, req.Tag = RandName(), RandTag()

		// --- When ---
		have, err := buildImage(t.Context(), req)

		// --- Then ---
		assert.ErrorContain(t, "non-zero code: 3", err)
		assert.Empty(t, have)
	})
}

func Test_buildLogged_tabular(t *testing.T) {
	tt := []struct {
		testN string

		prt  bool
		lw   io.Writer
		want bool
	}{
		{"not printed", false, nil, false},
		{"discarded", false, io.Discard, false},
		{"print build log", true, nil, true},
		{"log writer", false, os.Stdout, true},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			req := xctrtest.DockerfileReq()
			req.PrintBuildLog = tc.prt //nolint:staticcheck
			req.FromDockerfile.BuildLogWriter = tc.lw

			// --- When ---
			have := buildLogged(req)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_CTR_shareKey(t *testing.T) {
	t.Run("same for identical requests", func(t *testing.T) {
		// --- Given ---
		oth := NewCTR("other", xctrtest.DockerfileReq())
		want, _ := oth.shareKey(nil, prepare(oth, nil))
		ctr := NewCTR("ctr", xctrtest.DockerfileReq())
		meta := prepare(ctr, nil)

		// --- When ---
		have, ok := ctr.shareKey(nil, meta)

		// --- Then ---
		assert.True(t, ok)
		assert.Equal(t, want, have)
	})

	t.Run("same for directory context", func(t *testing.T) {
		// --- Given ---
		req := tc.GenericContainerRequest{
			ContainerRequest: tc.ContainerRequest{
				FromDockerfile: tc.FromDockerfile{
					Context: "xctrtest/data/simple",
				},
			},
		}
		oth := NewCTR("other", req)
		want, _ := oth.shareKey(nil, prepare(oth, nil))
		ctr := NewCTR("ctr", req)
		meta := prepare(ctr, nil)

		// --- When ---
		have, ok := ctr.shareKey(nil, meta)

		// --- Then ---
		assert.True(t, ok)
		assert.Equal(t, want, have)
	})

	t.Run("rewinds context archive", func(t *testing.T) {
		// --- Given ---
		ctr := NewCTR("ctr", xctrtest.DockerfileReq())
		meta := prepare(ctr, nil)

		// --- When ---
		_, ok := ctr.shareKey(nil, meta)

		// --- Then ---
		assert.True(t, ok)
		arc := must.Value(io.ReadAll(ctr.runReq.ContextArchive))
		assert.NotEmpty(t, arc)
	})
}

func Test_CTR_shareKey_differs_tabular(t *testing.T) {
	label := func(req *tc.GenericContainerRequest) {
		req.Labels = map[string]string{"key": "val"}
	}
	arg := func(req *tc.GenericContainerRequest) {
		req.BuildArgs["key"] = new("val")
	}
	argNil := func(req *tc.GenericContainerRequest) {
		req.BuildArgs["key"] = nil
	}
	dockerfile := func(req *tc.GenericContainerRequest) {
		df := "FROM " + xctrtest.EchoServerRef + "\n"
		req.ContextArchive = MustDockerfileArchive([]byte(df))
	}
	none := func(*tc.GenericContainerRequest) {}
	date := xdef.EnvBldDate + "=2000-01-02T03:04:05Z"

	tt := []struct {
		testN string

		mod func(*tc.GenericContainerRequest)
		env []string
	}{
		{"label", label, nil},
		{"build arg", arg, nil},
		{"nil build arg", argNil, nil},
		{"context", dockerfile, nil},
		{"pinned build date", none, []string{date}},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			oth := NewCTR("other", xctrtest.DockerfileReq())
			want, _ := oth.shareKey(nil, prepare(oth, nil))
			req := xctrtest.DockerfileReq()
			tc.mod(&req)
			ctr := NewCTR("ctr", req)
			meta := prepare(ctr, tc.env)

			// --- When ---
			have, ok := ctr.shareKey(tc.env, meta)

			// --- Then ---
			assert.True(t, ok)
			assert.NotEqual(t, want, have)
		})
	}
}

func Test_CTR_shareKey_not_shared_tabular(t *testing.T) {
	type modifier = func(*tc.GenericContainerRequest)
	req := func(mod modifier) tc.GenericContainerRequest {
		req := xctrtest.DockerfileReq()
		mod(&req)
		return req
	}
	dfr := xctrtest.DockerfileReq()
	sub := tc.NewCustomHubSubstitutor("example.com")
	hook := func(context.Context, tc.ContainerRequest) error { return nil }

	tt := []struct {
		testN string

		ctr *CTR
		env []string
	}{
		{"image request", NewCTR("ctr", xctrtest.ImageReq()), nil},
		{"own image", NewCTR("ctr", dfr, WithCTROwnImage), nil},
		{"image removal", NewCTR("ctr", dfr, WithCTRImgRm), nil},
		{"kept image", NewCTR("ctr", dfr, WithCTRKeepImage), nil},
		{
			"repo",
			NewCTR("ctr", req(func(r *tc.GenericContainerRequest) {
				r.Repo = "repo"
			})),
			nil,
		},
		{
			"tag",
			NewCTR("ctr", req(func(r *tc.GenericContainerRequest) {
				r.Tag = "tag"
			})),
			nil,
		},
		{
			"build options modifier",
			NewCTR("ctr", req(func(r *tc.GenericContainerRequest) {
				r.BuildOptionsModifier = func(*client.ImageBuildOptions) {}
			})),
			nil,
		},
		{
			"image substitutor",
			NewCTR("ctr", req(func(r *tc.GenericContainerRequest) {
				r.ImageSubstitutors = []tc.ImageSubstitutor{sub}
			})),
			nil,
		},
		{
			"pre-build hook",
			NewCTR("ctr", req(func(r *tc.GenericContainerRequest) {
				r.LifecycleHooks = []tc.ContainerLifecycleHooks{
					{PreBuilds: []tc.ContainerRequestHook{hook}},
				}
			})),
			nil,
		},
		{
			"post-build hook",
			NewCTR("ctr", req(func(r *tc.GenericContainerRequest) {
				r.LifecycleHooks = []tc.ContainerLifecycleHooks{
					{PostBuilds: []tc.ContainerRequestHook{hook}},
				}
			})),
			nil,
		},
		{"build log", NewCTR("ctr", dfr), SetEnvCTRBuildLog(nil)},
		{
			"error - context",
			NewCTR("ctr", req(func(r *tc.GenericContainerRequest) {
				r.ContextArchive = errReadSeeker{}
			})),
			nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			meta := prepare(tc.ctr, tc.env)

			// --- When ---
			have, ok := tc.ctr.shareKey(tc.env, meta)

			// --- Then ---
			assert.False(t, ok)
			assert.Empty(t, have)
		})
	}
}

func Test_CTR_useShared(t *testing.T) {
	t.Run("builds the image", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		resetShared(t)
		ctr := NewCTR(t.Name(), xctrtest.DockerfileReq())
		prepare(ctr, nil)

		// --- When ---
		err := ctr.useShared(ctx, "key")

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, ctr.runReq.ShouldBuildImage())
		assert.Equal(t, t.Name(), ctr.runReq.Labels[LabTestCtrName])

		cli := xctrtest.NewClient(t)
		img := must.Value(cli.ImageInspect(ctx, ctr.runReq.Image))
		assert.Equal(t, "true", img.Config.Labels[labTCReap])
		assert.Equal(t, "r", img.Config.Labels[xdef.LabImgSrc])
		assert.HasNoKey(t, LabTestCtrName, img.Config.Labels)
	})

	t.Run("reuses the image", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		sharedImgs["key"] = builtImg("repo:tag")
		ctr := NewCTR(t.Name(), xctrtest.DockerfileReq())
		prepare(ctr, nil)

		// --- When ---
		err := ctr.useShared(t.Context(), "key")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "repo:tag", ctr.runReq.Image)
		assert.False(t, ctr.runReq.ShouldBuildImage())
	})

	t.Run("error - build fails", func(t *testing.T) {
		// --- Given ---
		resetShared(t)
		req := xctrtest.DockerfileReq()
		dockerfile := "FROM " + xctrtest.EchoServerRef + "\nRUN exit 3\n"
		req.ContextArchive = MustDockerfileArchive([]byte(dockerfile))
		ctr := NewCTR(t.Name(), req)
		prepare(ctr, nil)

		// --- When ---
		err := ctr.useShared(t.Context(), "key")

		// --- Then ---
		assert.ErrorContain(t, "exit code: 3", err)
		assert.True(t, ctr.runReq.ShouldBuildImage())
	})
}
