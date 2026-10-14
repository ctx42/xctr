// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	tc "github.com/testcontainers/testcontainers-go"
)

// Labels testcontainers sets on the resources it creates.
const (
	labTCReap    = "org.testcontainers.reap"
	labTCReaper  = "org.testcontainers.reaper"
	labTCSession = "org.testcontainers.sessionId"
)

// imgPrefix starts the name of every image [RandName] names.
const imgPrefix = "c42-tst-img-"

// Sweep removes the containers and built images xctr created in earlier
// testcontainers sessions and that the session's reaper (Ryuk) left behind:
// Ryuk gives up when listing images takes longer than its request timeout,
// and it exits for good after one pass. Sweep leaves a resource alone when its
// session's reaper still runs, when it was created or tagged within grace,
// or when it is not marked for reaping. Images are removed by tag.
//
// Sweep lists containers and images, which is slow on a busy daemon, so xctr
// never calls it; call it at the end of a test run:
//
//	func TestMain(m *testing.M) {
//		code := m.Run()
//		_ = xctr.Sweep(context.Background(), 10*time.Minute)
//		os.Exit(code)
//	}
//
// The grace period protects sessions that are still starting: testcontainers
// builds an image before it starts the reaper.
func Sweep(ctx context.Context, grace time.Duration) error {
	cli, err := newDockerClient(ctx)
	if err != nil {
		return fmt.Errorf("sweep: %w", err)
	}
	defer func() { _ = cli.Close() }()

	live, err := liveSessions(ctx, cli)
	if err != nil {
		return fmt.Errorf("sweep: %w", err)
	}
	cutoff := time.Now().Add(-grace)

	// Images go after containers, which keep them in use.
	ers := sweepContainers(ctx, cli, live, cutoff)
	ers = errors.Join(ers, sweepImages(ctx, cli, live, cutoff))
	if ers != nil {
		return fmt.Errorf("sweep: %w", ers)
	}
	return nil
}

// liveSessions returns the IDs of sessions whose reaper is running, and the
// session of the calling process.
func liveSessions(
	ctx context.Context,
	cli *client.Client,
) (map[string]bool, error) {

	opts := client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", labTCReaper+"=true"),
	}
	lst, err := cli.ContainerList(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list reapers: %w", err)
	}
	live := map[string]bool{tc.SessionID(): true}
	for _, itm := range lst.Items {
		live[itm.Labels[labTCSession]] = true
	}
	return live, nil
}

// sweepContainers removes xctr containers of sessions not in live that were
// created before cutoff.
func sweepContainers(
	ctx context.Context,
	cli *client.Client,
	live map[string]bool,
	cutoff time.Time,
) error {

	opts := client.ContainerListOptions{
		All: true,
		Filters: make(client.Filters).
			Add("label", LabTestCtrName).
			Add("label", labTCReap+"=true"),
	}
	lst, err := cli.ContainerList(ctx, opts)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	rmOpts := client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}
	var ers error
	for _, itm := range lst.Items {
		if live[itm.Labels[labTCSession]] {
			continue
		}
		if time.Unix(itm.Created, 0).After(cutoff) {
			continue
		}
		if _, err = cli.ContainerRemove(ctx, itm.ID, rmOpts); handleErr(err) {
			err = fmt.Errorf("remove container %s: %w", ShortID(itm.ID), err)
			ers = errors.Join(ers, err)
		}
	}
	return ers
}

// sweepImages removes the xctr tags of images marked for reaping, of sessions
// not in live, that were created and last tagged before cutoff. A tag a
// container still uses is left.
func sweepImages(
	ctx context.Context,
	cli *client.Client,
	live map[string]bool,
	cutoff time.Time,
) error {

	// Listing by name is fast; listing by label reads every image's config.
	opts := client.ImageListOptions{
		Filters: make(client.Filters).Add("reference", imgPrefix+"*"),
	}
	lst, err := cli.ImageList(ctx, opts)
	if err != nil {
		return fmt.Errorf("list images: %w", err)
	}
	var ers error
	for _, sum := range lst.Items {
		if sum.Labels[labTCReap] != "true" || live[sum.Labels[labTCSession]] {
			continue
		}
		// A cached BuildKit build reuses an image created long ago, so the
		// last tag time decides.
		if time.Unix(sum.Created, 0).After(cutoff) {
			continue
		}
		ins, err := cli.ImageInspect(ctx, sum.ID)
		if err != nil {
			if !cerrdefs.IsNotFound(err) {
				err = fmt.Errorf("inspect image %s: %w", ShortID(sum.ID), err)
				ers = errors.Join(ers, err)
			}
			continue
		}
		if ins.Metadata.LastTagTime.After(cutoff) {
			continue
		}
		for _, ref := range sum.RepoTags {
			if !strings.HasPrefix(ref, imgPrefix) {
				continue
			}
			rmOpts := client.ImageRemoveOptions{PruneChildren: true}
			_, err = cli.ImageRemove(ctx, ref, rmOpts)
			if err == nil || cerrdefs.IsNotFound(err) {
				continue
			}
			if cerrdefs.IsConflict(err) {
				continue // A container still uses the tag.
			}
			ers = errors.Join(ers, fmt.Errorf("remove image %s: %w", ref, err))
		}
	}
	return ers
}
