// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
	"github.com/ctx42/testing/pkg/tester"
	"github.com/ctx42/testkit/pkg/dkrkit"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	tc "github.com/testcontainers/testcontainers-go"

	"github.com/ctx42/xctr/pkg/xctr/xctrtest"
)

// TestMain runs the tests, then removes what the reaper left behind in
// earlier test runs.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := Sweep(context.Background(), time.Minute); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// ctrGone polls until the container with the given ID is no longer present,
// failing the test if it is still present after the timeout. Container removal
// is asynchronous (AutoRemove plus the reaper), so a container may briefly
// remain listed with a "removing" status right after Terminate; polling
// tolerates that window. It also tolerates the transient inspect error a
// mid-removal container yields, which surfaces as a nil result.
func ctrGone(t tester.T, id string) {
	t.Helper()
	dkr := dkrkit.NewT(t)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if have, _ := dkr.CtrPs().FindByID(id); have == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("container %s still present after timeout", id)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// firstLine polls lc until it has received a line, and returns that line
// with its newline, or what lc holds after two seconds. The log consumer
// receives lines asynchronously, so a container can be ready before its
// first line arrives.
func firstLine(lc *Logger) string {
	deadline := time.Now().Add(2 * time.Second)
	for {
		have := lc.Print()
		if line, _, ok := strings.Cut(have, "\n"); ok {
			return line + "\n"
		}
		if time.Now().After(deadline) {
			return have
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// resetDaemonBuilder clears the process-wide daemon builder cache for the
// test and restores it when the test ends.
func resetDaemonBuilder(t tester.T) {
	t.Helper()
	daemonBldMx.Lock()
	bld, ok := daemonBld, daemonBldOK
	daemonBld, daemonBldOK = "", false
	daemonBldMx.Unlock()
	t.Cleanup(func() {
		daemonBldMx.Lock()
		defer daemonBldMx.Unlock()
		daemonBld, daemonBldOK = bld, ok
	})
}

// resetShared clears the registry of shared images for the test and restores
// it when the test ends. Images shared during the test are untagged then.
func resetShared(t tester.T) {
	t.Helper()
	sharedImgsMx.Lock()
	orig := sharedImgs
	sharedImgs = make(map[string]*sharedImg)
	sharedImgsMx.Unlock()
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		cli := must.Value(newDockerClient(ctx))
		defer func() { _ = cli.Close() }()
		_ = dropShared(ctx, cli)
		sharedImgsMx.Lock()
		defer sharedImgsMx.Unlock()
		sharedImgs = orig
	})
}

// builtImg returns a finished shared image build of the reference.
func builtImg(ref string) *sharedImg {
	img := &sharedImg{done: make(chan struct{}), ref: ref}
	close(img.done)
	return img
}

// prepare sets the run request of ctr as [CTR.Start] does with env, without
// asking git or the daemon, and returns the metadata it used.
func prepare(ctr *CTR, env []string) startMeta {
	meta := buildMeta(env, ctr.name, scmInfo{repo: "r", rev: "v", hash: "h"})
	meta.builder = build.BuilderBuildKit
	ctr.runReq = prepareRequest(cloneReq(ctr.req), meta, env, ctr.log)
	return meta
}

// errReadSeeker is an [io.ReadSeeker] failing every read.
type errReadSeeker struct{}

func (errReadSeeker) Read([]byte) (int, error)       { return 0, errTest }
func (errReadSeeker) Seek(int64, int) (int64, error) { return 0, nil }

// endpoint returns HTTP URL for given ctr. When params are present they will
// be added after "?" joined by "&".
func endpoint(ctr *CTR, params ...string) string {
	cfg := ctr.ConfigHost()
	u := "http://" + net.JoinHostPort(cfg["HOST"], cfg["PORT_0"])
	if len(params) > 0 {
		u += "?" + strings.Join(params, "&")
	}
	return u
}

func Test_endpoint(t *testing.T) {
	t.Run("without params", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{
			cfgHost: map[string]string{
				"HOST":   "host",
				"PORT_0": "42",
				"PORT_1": "43",
			},
		}

		// --- When ---
		have := endpoint(ctr)

		// --- Then ---
		assert.Equal(t, "http://host:42", have)
	})

	t.Run("one param", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{
			cfgHost: map[string]string{
				"HOST":   "host",
				"PORT_0": "42",
				"PORT_1": "43",
			},
		}

		// --- When ---
		have := endpoint(ctr, "a=b")

		// --- Then ---
		assert.Equal(t, "http://host:42?a=b", have)
	})

	t.Run("with param", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{
			cfgHost: map[string]string{
				"HOST":   "host",
				"PORT_0": "42",
				"PORT_1": "43",
			},
		}

		// --- When ---
		have := endpoint(ctr, "a=b", "c=d")

		// --- Then ---
		assert.Equal(t, "http://host:42?a=b&c=d", have)
	})
}

// addrCTR returns a [CTR] bound as if started with ports "80/tcp", "443/tcp",
// and "53/udp" exposed, without a running container.
func addrCTR() *CTR {
	return &CTR{
		dc: &tc.DockerContainer{},
		cfgHost: map[string]string{
			"HOST":   "localhost",
			"PORT_0": "32768/tcp",
			"PORT_1": "32769/tcp",
			"PORT_2": "32770/udp",
		},
		cfgGuest: map[string]string{
			"HOST":   "172.17.0.3",
			"PORT_0": "80/tcp",
			"PORT_1": "443/tcp",
			"PORT_2": "53/udp",
		},
	}
}

// Test sentinel errors. Messages preserved from the original kit package so
// existing assertions on the printed text stay valid.
var (
	errTest      = errors.New("kit test error")
	errTestOther = errors.New("kit test other error")
)

// dockerClient returns a Docker client closed when the test ends.
func dockerClient(t tester.T) *client.Client {
	t.Helper()
	cli := must.Value(newDockerClient(t.Context()))
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

// createCtr creates, without starting it, a container from
// [xctrtest.EchoServerRef] with the labels, and removes it when the test
// ends. Returns the container ID.
func createCtr(t tester.T, labels map[string]string) string {
	t.Helper()
	cli := dockerClient(t)
	opts := client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      xctrtest.EchoServerRef,
			Entrypoint: []string{"tail", "-f", "/dev/null"},
			Labels:     labels,
		},
	}
	res := must.Value(cli.ContainerCreate(t.Context(), opts))
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		rmOpts := client.ContainerRemoveOptions{Force: true}
		_, _ = cli.ContainerRemove(ctx, res.ID, rmOpts)
	})
	return res.ID
}

// startCtr creates a container as createCtr does and starts it.
func startCtr(t tester.T, labels map[string]string) string {
	t.Helper()
	id := createCtr(t, labels)
	opts := client.ContainerStartOptions{}
	must.Value(dockerClient(t).ContainerStart(t.Context(), id, opts))
	return id
}

// commitImg commits a container with the labels to a random xctr image
// reference, removes the image when the test ends, and returns the reference.
func commitImg(t tester.T, labels map[string]string) string {
	t.Helper()
	cli := dockerClient(t)
	ref := RandRef()
	id := createCtr(t, labels)
	opts := client.ContainerCommitOptions{Reference: ref}
	must.Value(cli.ContainerCommit(t.Context(), id, opts))
	rmOpts := client.ContainerRemoveOptions{Force: true}
	must.Value(cli.ContainerRemove(t.Context(), id, rmOpts))
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		imgOpts := client.ImageRemoveOptions{Force: true}
		_, _ = cli.ImageRemove(ctx, ref, imgOpts)
	})
	return ref
}

// ctrExists returns true when the container with the ID exists.
func ctrExists(t tester.T, id string) bool {
	t.Helper()
	opts := client.ContainerInspectOptions{}
	_, err := dockerClient(t).ContainerInspect(t.Context(), id, opts)
	return err == nil
}

// imgExists returns true when an image with the reference exists.
func imgExists(t tester.T, ref string) bool {
	t.Helper()
	ims := dkrkit.NewT(t).ImgLs(dkrkit.WithImgLsFilter("reference=" + ref))
	return ims.FindByRef(ref) != nil
}
