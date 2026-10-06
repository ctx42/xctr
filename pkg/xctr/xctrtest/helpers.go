// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctrtest

import (
	"context"
	_ "embed"
	"time"

	"github.com/ctx42/testing/pkg/tester"
	"github.com/ctx42/xdef/pkg/xdef"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ctx42/xctr/pkg/xctr/internal/arch"
)

// Docker image name and tag used for testing.
const (
	// EchoServerImgName is the Docker image name used for testing.
	EchoServerImgName = "ealen/echo-server"

	// EchoServerTag is the Docker image tag used for testing.
	EchoServerTag = "0.9.2"

	// EchoServerRef represents the test Docker image reference.
	EchoServerRef = "docker.io/" + EchoServerImgName + ":" + EchoServerTag
)

// dockerfile is an embedded example dockerfile.
//
//go:embed data/echo/Dockerfile
var dockerfile []byte

// ImageReq returns a container request based on [EchoServerRef].
func ImageReq() tc.GenericContainerRequest {
	return tc.GenericContainerRequest{
		Started: true,
		ContainerRequest: tc.ContainerRequest{
			Image:        EchoServerRef,
			ExposedPorts: []string{"80/tcp"},
			WaitingFor: wait.ForListeningPort("80/tcp").
				WithStartupTimeout(10 * time.Second),
			ConfigModifier: func(cfg *container.Config) {
				cfg.Hostname = "echo"
			},
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.AutoRemove = true
			},
		},
	}
}

// DockerfileReq returns a container request based on the [EchoServerRef] image.
//
// Additionally, the Dockerfile installs tini during build.
func DockerfileReq() tc.GenericContainerRequest {
	return tc.GenericContainerRequest{
		Started: true,
		ContainerRequest: tc.ContainerRequest{
			FromDockerfile: tc.FromDockerfile{
				KeepImage:      false,
				ContextArchive: arch.MustDockerfileArchive(dockerfile),
				Dockerfile:     "Dockerfile",
				BuildArgs: map[string]*string{
					xdef.EnvBldImgBase: new(EchoServerRef),
				},
			},
			ExposedPorts: []string{"80/tcp"},
			WaitingFor:   wait.ForListeningPort("80/tcp"),
			ConfigModifier: func(cfg *container.Config) {
				cfg.Hostname = "echo"
			},
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.AutoRemove = true
			},
		},
	}
}

// NewClient returns a new Docker API client which is automatically closed when
// the test and all its subtests complete. It fails the test immediately when
// the client cannot be created.
func NewClient(t tester.T) *client.Client {
	t.Helper()
	cli, err := client.New(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

// Container is the minimal container behavior CanStart needs.
type Container interface {
	Start(ctx context.Context, env []string) error
	Cleanup(ctx context.Context) error
}

// CanStart starts a container and registers automatic cleanup after the test
// ends, also when starting fails, since a container may be left running by a
// failed start. On error, it marks the test failed and returns. When the
// container also describes itself, its description is logged.
func CanStart(t tester.T, env []string, ctr Container) {
	t.Helper()

	t.Cleanup(func() { _ = ctr.Cleanup(context.Background()) })
	if err := ctr.Start(t.Context(), env); err != nil {
		t.Error(err)
		return
	}
	if desc, ok := ctr.(interface{ Describe() string }); ok {
		t.Log(desc.Describe())
	}
}
