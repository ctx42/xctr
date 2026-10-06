# xctr

A [testcontainers-go](https://github.com/testcontainers/testcontainers-go)
wrapper for running and driving Docker containers from Go tests.

## Overview

`CTR` wraps a `testcontainers-go` `GenericContainerRequest` with a small,
test-friendly lifecycle API: start it, run commands in it, copy files to and
from it, and clean it up — without repeating the same testcontainers-go
boilerplate in every test package.

On `Start`, `CTR` stamps the container (or, when building from a Dockerfile, the
build) with OCI Image Spec labels (source, version, revision, creation date) and
matching `C42_SCM_*` and `C42_BLD_DATE` environment variables — see
[`xdef`](https://pkg.go.dev/github.com/ctx42/xdef/pkg/xdef) — resolved from the
git checkout via
[`gitaid`](https://pkg.go.dev/github.com/ctx42/gitaid/pkg/gitaid), falling back
to placeholder values when that information isn't available (for example, no
`origin` remote). This gives containers built in CI and locally a consistent,
inspectable provenance trail.

## Import

```go
import "github.com/ctx42/xctr/pkg/xctr"
```

## Features

- `NewCTR` / `Start` / `Terminate` — construct a container from a
  `tc.GenericContainerRequest` and manage its full lifecycle; the
  `WithCTRLogger` and `WithCTRImgRm` options attach a log consumer and remove
  the image on `Terminate`.
- `ImageReq(ref)` — a container request for an image reference.
- `ExposePort`, `AddFile`, `Setenv`, `SetLabel` — configure the request before
  `Start`; `CreateTemp` writes a host temp file removed on `Cleanup`.
- `Exec`, `ExecContent`, `ExecFile` — run a command, or upload and run a
  script, in the running container.
- `ExecResult` — the command's `ExitCode`, `SOut`, `EOut`, and
  `CombinedOutput`, with `Err` reporting a failed or non-zero exit.
- `CopyTo` / `ReadFile` — copy a file to, or read a file from, the container.
- `SetReadOnly` / `SetExecWhitelist` — restrict a container to a fixed set of
  allowed `Exec` commands (matched against a whitelist of regular
  expressions); the standalone `Whitelist` (`NewWhitelist`, `Add`, `Check`)
  applies the same matching anywhere.
- `Pause` / `Unpause`, `MappedPort`, `ContainerIP`, `GatewayIP` — inspect and
  control a running container.
- `Once` (`NewOnce`; the zero value is ready to use) — a registry for
  containers meant to be shared across many tests instead of started once per
  test; `OnceAdd`, `OnceAddNamed`, `OnceGet`, `OnceNames`, `OnceStop`, and
  `OnceStopAll` drive a package-level instance.
- `Logger` / `NewLogger` / `LogConsumerCfg` — a testcontainers log consumer
  that collects container logs and optionally prints them.
- `CleanupTrait` (`RegisterCleanup`, `Cleanup`) — reverse-order, run-once
  cleanup callbacks to embed in your own types.
- `Ref`, `ParseRef`, `RandRef`, `RandName`, `RandTag`, `RandNet`, `ShortID` —
  build, parse, and randomize image references, names, tags, network names,
  and IDs.
- `Archive`, `MustArchive`, `Unarchive`, `DockerfileArchive`, `WithArchFile`,
  `WithArchDockerfile`, `NewArchFile` — build and parse tar archives, such as
  Dockerfile build contexts.
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

	xctrtest.CanStart(t, nil, ctr) // Cleaned up on t.Cleanup.

	res := ctr.Exec(context.Background(), "echo", "hello")
	if err := res.Err(); err != nil {
		t.Fatal(err)
	}
	t.Log(res.SOut) // "hello\n"
}
```

Outside a test — or when you need full control over the lifecycle — call
`Start` and `Cleanup` directly. `Cleanup` terminates the container and
removes the temp files `CreateTemp` created:

```go
ctx := context.Background()

ctr := xctr.NewCTR("echo", xctrtest.ImageReq())
if err := ctr.Start(ctx, nil); err != nil {
	// handle error
}
defer func() { _ = ctr.Cleanup(ctx) }()

res := ctr.Exec(ctx, "echo", "hello")
```

`ConfigHost` / `ConfigGuest` return the connection details collected once the
container reports itself started. `ConfigHost` holds the host address and the
host-mapped ports; `ConfigGuest` holds the container IP and the in-container
ports. `PORT_<i>` follows the order of the request's `ExposedPorts`:

```go
cfg := ctr.ConfigHost()  // e.g. {"HOST": "localhost", "PORT_0": "32942/tcp"}
gst := ctr.ConfigGuest() // e.g. {"HOST": "172.17.0.3", "PORT_0": "80/tcp"}
```

### Errors

Sentinel errors, matched with `errors.Is`:

| Error           | Returned when                                                        |
|-----------------|----------------------------------------------------------------------|
| `ErrEmptyCmd`   | `Exec` is called with an empty command.                              |
| `ErrNotStarted` | The container is not started.                                        |
| `ErrRunning`    | The container is already running.                                    |
| `ErrExitCode`   | An `Exec` command returns a non-zero exit code.                      |
| `ErrNoNetwork`  | The container has no networks connected.                             |
| `ErrReadOnly`   | A mutating action is called on a read-only running container.        |
| `ErrNotAllowed` | An action is not allowed, such as a command outside the whitelist.   |

`ExecResult` unwraps to its error, so `errors.Is(res, xctr.ErrExitCode)`
matches a command that exited non-zero.

## Configuration

`Start` reads the following from the `env []string` slice passed to it
(`ring.EnvGet` / `ring.EnvSet` semantics — same shape as `os.Environ()`):

| Variable              | Set by                | Effect                                                                     |
|-----------------------|-----------------------|----------------------------------------------------------------------------|
| `C42_XCTR_LOG`        | `SetEnvCTRLog`        | Print container run logs to standard output.                               |
| `C42_XCTR_BUILD_LOG`  | `SetEnvCTRBuildLog`   | Print container **build** logs to standard output.                         |
| `C42_XCTR_ENTRYPOINT` | `SetEnvCTREntrypoint` | Override the container's entrypoint; clears `WaitingFor`, keeps the ports. |

The two log switches take effect only when the value is exactly `true`, which
is what `SetEnvCTRLog` and `SetEnvCTRBuildLog` set. `C42_XCTR_LOG` is ignored
when the request already has a `LogConsumerCfg` or a consumer was passed with
`WithCTRLogger`, which is attached regardless of the switch.

The `C42_XCTR_ENTRYPOINT` value is split on single spaces, so quoted arguments
are not supported. `SetEnvCTREntrypoint(env)` with no command sets
`tini -- tail -f /dev/null`.

In addition to the `xdef` labels described above, `Start` sets the
`dev.ctx42.test.ctr.name` label (`xctr.LabTestCtrName`) to the name passed to
`NewCTR`.

## Testing helpers (xctrtest)

```go
import "github.com/ctx42/xctr/pkg/xctr/xctrtest"
```

`xctrtest` provides container requests and lifecycle helpers used by `xctr`'s
own test suite, and reusable in any package that tests against a container:

- `ImageReq()` — a `GenericContainerRequest` for the pinned test image
  `docker.io/ealen/echo-server:0.9.2` (`EchoServerRef`, built from
  `EchoServerImgName` and `EchoServerTag`), exposing port 80.
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
