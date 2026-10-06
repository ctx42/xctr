# xctr

A [testcontainers-go](https://github.com/testcontainers/testcontainers-go)
wrapper for running and driving Docker containers from Go tests.

## Overview

`CTR` wraps a `testcontainers-go` `GenericContainerRequest` with a small,
test-friendly lifecycle API: start it, run commands in it, copy files to and
from it, and clean it up — without repeating the same testcontainers-go
boilerplate in every test package.

On `Start`, `CTR` stamps the container (or, when building from a Dockerfile,
the build) with OCI Image Spec labels (source, version, revision, creation
date) and matching `C42_SCM_*` and `C42_BLD_DATE` environment variables — see
[`xdef`](https://pkg.go.dev/github.com/ctx42/xdef/pkg/xdef) — resolved from the
git checkout via [`gitaid`](https://pkg.go.dev/github.com/ctx42/gitaid/pkg/gitaid),
falling back to placeholder values when that information isn't available (for
example, no `origin` remote). This gives containers built in CI and locally a
consistent, inspectable provenance trail.

## Import

```go
import "github.com/ctx42/xctr/pkg/xctr"
```

## Features

- `NewCTR` / `Start` / `Terminate` — construct a container from a
  `tc.GenericContainerRequest` and manage its full lifecycle.
- `Exec`, `ExecContent`, `ExecFile` — run a command, or upload and run a
  script, in the running container.
- `CopyTo` / `ReadFile` — copy a file to, or read a file from, the container.
- `SetReadOnly` / `SetExecWhitelist` — restrict a container to a fixed set of
  allowed `Exec` commands (matched against a whitelist of regular
  expressions).
- `Pause` / `Unpause`, `MappedPort`, `ContainerIP`, `GatewayIP` — inspect and
  control a running container.
- `OnceAdd` / `OnceGet` / `OnceStopAll` — a package-level registry for
  containers meant to be shared across many tests instead of started once
  per test.
- `xctrtest` sub-package — ready-made container requests and test-lifecycle
  helpers for writing container-backed tests (see below).

## Prerequisites

- Go 1.26 or newer.
- A reachable Docker daemon (`DOCKER_HOST`, or the default local socket) —
  `xctr` is a thin layer over `testcontainers-go`, which drives Docker
  directly.

## Usage

The smallest working example: start a container from an image, run a
command in it, and clean up. `xctrtest.CanStart` starts the container and
registers its `Cleanup` with `t.Cleanup`; `xctrtest.ImageReq` returns a ready
`GenericContainerRequest` for a small test image.

```go
package example_test

import (
	"context"
	"testing"

	"github.com/ctx42/xctr/pkg/xctr"
	"github.com/ctx42/xctr/pkg/xctr/xctrtest"
)

func Test_Example(t *testing.T) {
	ctr := xctr.NewCTR("echo", xctrtest.ImageReq())

	xctrtest.CanStart(t, nil, ctr) // Starts the container; cleans it up on t.Cleanup.

	res := ctr.Exec(context.Background(), "echo", "hello")
	if err := res.Err(); err != nil {
		t.Fatal(err)
	}
	t.Log(res.SOut) // "hello\n"
}
```

Outside a test — or when you need full control over the lifecycle — call
`Start` and `Terminate` directly:

```go
ctx := context.Background()

ctr := xctr.NewCTR("echo", xctrtest.ImageReq())
if err := ctr.Start(ctx, nil); err != nil {
	// handle error
}
defer func() { _ = ctr.Terminate(ctx) }()

res := ctr.Exec(ctx, "echo", "hello")
```

`ConfigHost` / `ConfigGuest` return the host- and guest-side connection
details (mapped ports, host address) collected once the container reports
itself started:

```go
cfg := ctr.ConfigHost() // e.g. {"HOST": "localhost", "PORT_0": "32942/tcp"}
```

## Configuration

`Start` reads the following from the `env []string` slice passed to it
(`ring.EnvGet` / `ring.EnvSet` semantics — same shape as `os.Environ()`):

| Variable              | Set by                | Effect                                                                       |
|-----------------------|-----------------------|------------------------------------------------------------------------------|
| `C42_XCTR_LOG`        | `SetEnvCTRLog`        | Print container run logs to standard output.                                 |
| `C42_XCTR_BUILD_LOG`  | `SetEnvCTRBuildLog`   | Print container **build** logs to standard output.                           |
| `C42_XCTR_ENTRYPOINT` | `SetEnvCTREntrypoint` | Override the container's entrypoint; clears `WaitingFor` and `ExposedPorts`. |

In addition to the `xdef` labels described above, `Start` sets the
`dev.ctx42.test.ctr.name` label (`xctr.LabTestCtrName`) to the name passed to
`NewCTR`.

## Testing helpers (xctrtest)

```go
import "github.com/ctx42/xctr/pkg/xctr/xctrtest"
```

`xctrtest` provides container requests and lifecycle helpers used by `xctr`'s
own test suite, and reusable in any package that tests against a container:

- `ImageReq()` — a `GenericContainerRequest` for the
  `ealen/echo-server` test image, exposing port 80.
- `DockerfileReq()` — the same test image, built from an embedded
  Dockerfile that additionally installs `tini`.
- `CanStart(t, env, ctr)` — starts a container, fails the test via `t.Error`
  and returns early on failure, and registers `t.Cleanup` for it. Logs the
  container's `Describe()` output, if it has one.
- `NewClient(t)` — a Docker API client (`*client.Client`) closed
  automatically via `t.Cleanup`.
- `AssertBuildArg(t, key, want, set)` — asserts a Docker build-args map
  (`map[string]*string`) has `key` set to `want`.

## Resources

- [testcontainers-go documentation](https://golang.testcontainers.org/)
- [`xdef`](https://pkg.go.dev/github.com/ctx42/xdef/pkg/xdef) — shared label
  and environment-variable definitions.
- Root [README](../../README.md) — module overview and the full ctx42-family
  dependency list.
