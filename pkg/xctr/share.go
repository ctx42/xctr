// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/ctx42/ring/pkg/ring"
	"github.com/ctx42/xdef/pkg/xdef"
	"github.com/moby/moby/client"
	tc "github.com/testcontainers/testcontainers-go"
)

// sharedImg is an image built once for identical Dockerfile requests.
type sharedImg struct {
	done chan struct{} // Closed when the build ends.
	ref  string        // Reference of the built image.
	err  error         // Build error.
}

// sharedImgs holds the images built for sharing in this process by share key.
var (
	sharedImgs   = make(map[string]*sharedImg)
	sharedImgsMx sync.Mutex
)

// sharedImage returns the reference of the image shared under key, building
// it with bld on first use. Concurrent callers wait for the one build. A
// failed build is not kept, so the next caller builds again.
func sharedImage(
	ctx context.Context,
	key string,
	bld func(context.Context) (string, error),
) (string, error) {

	sharedImgsMx.Lock()
	img, ok := sharedImgs[key]
	if !ok {
		img = &sharedImg{done: make(chan struct{})}
		sharedImgs[key] = img
	}
	sharedImgsMx.Unlock()

	if ok {
		select {
		case <-img.done:
			return img.ref, img.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	img.ref, img.err = bld(ctx)
	if img.err != nil {
		sharedImgsMx.Lock()
		delete(sharedImgs, key)
		sharedImgsMx.Unlock()
	}
	close(img.done)
	return img.ref, img.err
}

// dropShared removes the tags of the images built for sharing in this process
// and forgets them, so the next [CTR.Start] builds again. Images still being
// built are kept. A tag a container still uses is left for the reaper.
func dropShared(ctx context.Context, cli *client.Client) error {
	var refs []string
	sharedImgsMx.Lock()
	for key, img := range sharedImgs {
		select {
		case <-img.done:
			delete(sharedImgs, key)
			refs = append(refs, img.ref)
		default:
		}
	}
	sharedImgsMx.Unlock()

	var ers error
	for _, ref := range refs {
		ers = errors.Join(ers, untag(ctx, cli, ref))
	}
	return ers
}

// untag removes the image reference without force, which deletes the image
// with its last tag. A missing reference, and one a container uses, are not
// errors.
func untag(ctx context.Context, cli *client.Client, ref string) error {
	opts := client.ImageRemoveOptions{PruneChildren: true}
	_, err := cli.ImageRemove(ctx, ref, opts)
	if err == nil || cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
		return nil
	}
	return fmt.Errorf("remove image %s: %w", ref, err)
}

// buildImage builds the image of the Dockerfile request the way
// testcontainers does and returns its reference.
func buildImage(
	ctx context.Context,
	req tc.GenericContainerRequest,
) (string, error) {

	var opts []tc.DockerProviderOption
	if req.Logger != nil {
		opts = append(opts, tc.WithLogger(req.Logger))
	}
	prv, err := tc.NewDockerProvider(opts...)
	if err != nil {
		return "", fmt.Errorf("build image: %w", err)
	}
	defer func() { _ = prv.Close() }()
	return prv.BuildImage(ctx, &req.ContainerRequest)
}

// buildLogged returns true when the Dockerfile request prints its build log.
func buildLogged(req tc.GenericContainerRequest) bool {
	lw := req.FromDockerfile.BuildLogWriter
	logged := req.PrintBuildLog //nolint:staticcheck
	return logged || (lw != nil && lw != io.Discard)
}

// shareKey returns the key the run request's image is shared under, and
// false when the image must be built for the container alone: for an image
// request, a kept or caller-named image, a request whose build xctr cannot
// fully see (build options modifier, build hooks, image substitutors, the
// Docker Hub prefix), a printed build log, [WithCTROwnImage], and
// [WithCTRImgRm]. The key covers the build context, Dockerfile, build
// arguments, labels, and builder. The build date counts only when env pins
// it, so builds started at different times share.
func (ctr *CTR) shareKey(env []string, meta startMeta) (string, bool) {
	req, run := ctr.req, ctr.runReq
	if !run.ShouldBuildImage() || ctr.ownImage || ctr.removeImage {
		return "", false
	}
	if req.KeepImage || req.Repo != "" || req.Tag != "" {
		return "", false
	}
	if req.BuildOptionsModifier != nil || len(req.ImageSubstitutors) > 0 {
		return "", false
	}
	if buildLogged(run) || tc.ReadConfig().Config.HubImageNamePrefix != "" {
		return "", false
	}
	for _, hks := range req.LifecycleHooks {
		if len(hks.PreBuilds) > 0 || len(hks.PostBuilds) > 0 {
			return "", false
		}
	}

	hsh := sha256.New()
	_, _ = fmt.Fprintf(hsh, "dockerfile %q\n", run.GetDockerfile())
	_, _ = fmt.Fprintf(hsh, "builder %q\n", meta.builder)
	_, _ = fmt.Fprintf(hsh, "date %q\n", ring.EnvGet(env, xdef.EnvBldDate))
	for _, key := range slices.Sorted(maps.Keys(req.BuildArgs)) {
		if val := req.BuildArgs[key]; val != nil {
			_, _ = fmt.Fprintf(hsh, "arg %q=%q\n", key, *val)
			continue
		}
		_, _ = fmt.Fprintf(hsh, "arg %q\n", key)
	}
	for _, key := range slices.Sorted(maps.Keys(req.Labels)) {
		_, _ = fmt.Fprintf(hsh, "label %q=%q\n", key, req.Labels[key])
	}
	for _, key := range slices.Sorted(maps.Keys(meta.labels)) {
		if key == xdef.LabImgCreated || key == LabTestCtrName {
			continue
		}
		_, _ = fmt.Fprintf(hsh, "meta %q=%q\n", key, meta.labels[key])
	}

	// GetContext returns the context archive itself, which is rewound for
	// the build, or a tar stream of the context directory.
	bld := run.ContainerRequest
	src, err := bld.GetContext()
	if err != nil {
		return "", false
	}
	_, err = io.Copy(hsh, src)
	if arc := run.ContextArchive; arc != nil {
		_, sErr := arc.Seek(0, io.SeekStart)
		err = errors.Join(err, sErr)
	} else if cls, ok := src.(io.Closer); ok {
		err = errors.Join(err, cls.Close())
	}
	if err != nil {
		return "", false
	}
	return hex.EncodeToString(hsh.Sum(nil)), true
}

// useShared points the run request at the image shared under key, building
// the image when no earlier [CTR.Start] in this process did.
func (ctr *CTR) useShared(ctx context.Context, key string) error {
	// The image is not one container's: it gets the caller's labels only,
	// the container gets its own name label.
	bld := ctr.runReq
	bld.Labels = ctr.req.Labels
	build := func(ctx context.Context) (string, error) {
		return buildImage(ctx, bld)
	}
	ref, err := sharedImage(ctx, key, build)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	ctr.runReq.FromDockerfile = tc.FromDockerfile{}
	ctr.runReq.Image = ref
	return nil
}
