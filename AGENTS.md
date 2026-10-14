This file provides guidance to AI agents when working with code in this repository.

## Commands

```shell
go test -count 1 -race ./...                       # full suite (matches dev/idea run config)
go test -count 1 -run 'Test_CTR_Exec$' ./pkg/xctr  # one test
go test -count 1 -run 'Test_CTR_Exec/name' ./pkg/xctr  # one subtest
go vet ./...
```

Most tests start real containers: they need a reachable Docker daemon
(`DOCKER_HOST` or the default socket) and pull `docker.io/ealen/echo-server`.
There is no Makefile and no linter config in the repo.

## Architecture

Single module `github.com/ctx42/xctr`; all code is in `pkg/xctr`.

- `xctr.go` — the `Container` interface, split into `Manager` (lifecycle),
  `Info` (read-only state), and `Actions` (exec, file I/O, env/labels,
  read-only/whitelist). `CTR` (`ctr.go`) is the only implementation.
- `CTR.Start` pipeline (`ctr.go`): `cachedSCM` (`scmFromGit` once per working
  directory: git via `gitaid`, falling back to `xdef` placeholders) →
  `buildMeta` (pure) → `daemonBuilder` for Dockerfile requests (the daemon's
  preferred builder from its ping, once per process; BuildKit on Docker 23+) →
  `prepareRequest` (pure: stamps OCI labels/env with `SetMissing` so caller
  values win; Dockerfile builds get labels and the builder version via
  `BuildOptionsModifier`, keeping the classic builder when a build log is
  printed, plus build args and a random repo/tag; the name label goes on the
  container; applies `C42_XCTR_*` env switches) → `shareKey`/`useShared`
  (`share.go`: identical Dockerfile requests build once per process through the
  provider's `BuildImage`, then run the image by reference) → `startAndBind`
  (on failure without a container, `removeBuiltImage` drops an own built
  image) → `bindStarted` (fills `cfgHost`/`cfgGuest` with `HOST`
  and `PORT_<i>` keys, indexed by `ExposedPorts` order). `prepareRequest` works
  on a `cloneReq` copy stored in `runReq`; `req` keeps the configured request,
  which `Request` returns, so `Start` can run again after `Terminate`. Keep
  `buildMeta`/`prepareRequest` free of git and Docker calls so they stay
  unit-testable.
- `env []string` passed to `Start` is an `os.Environ()`-shaped slice read with
  `ring.EnvGet`; the package never reads the process environment directly.
- `Terminate` kills the container (`tc.StopTimeout(0)`) unless
  `WithCTRStopTimeout` sets a grace period: test containers are ephemeral and
  many PID 1s ignore SIGTERM, which cost the full 10 s default per container.
  Own built images (`WithCTROwnImage`, or a request that cannot be shared) are
  removed with the container; shared ones live until `Sweep`;
  `WithCTRKeepImage` keeps them. Tests that assert sharing call `resetShared`
  (`all_test.go`) to start from an empty share registry; tests that assert
  image removal use `WithCTROwnImage`.
- `Sweep` (`sweep.go`) removes what Ryuk missed: containers labelled
  `LabTestCtrName` and `c42-tst-img-*` images (listed by name, never by label,
  which is slow on large stores) that are marked for reaping, belong to a
  session with no running reaper, and are older than the grace period (images
  by last tag time). It is end-of-run only: nothing on the `Start`/`Terminate`
  path may list or sweep, because test speed comes before leftover cleanup.
  It also removes the images the calling process shared (`dropShared`).
- `CleanupTrait` (`cleanup_trait.go`) is embedded in `CTR` via the
  `hidCleanTrait` alias to keep the field unexported; cleanups run in reverse,
  once. `Start` registers `Terminate` as a cleanup.
- `Exec` lazily creates a Docker client through the package var
  `newDockerClient` (tests swap it to force failures) and enforces the optional
  regex `Whitelist` (`whitelist.go`); read-only mode = whitelist + rejecting
  mutating calls with `ErrReadOnly`.
- `once.go` — `Once` registry for containers shared across tests, with a
  package-level singleton behind the `Once*` functions.
- `internal/arch` — tar archive build/parse used for `CopyTo` and Dockerfile
  build contexts.
- `xctrtest` — public test helpers (`ImageReq`, `DockerfileReq` from embedded
  `data/*/Dockerfile`, `CanStart`, `NewClient`, `AssertBuildArg`). `xctr`'s
  in-package tests import it, so its non-test code must never import `xctr`
  (import cycle); it takes `Container`-shaped values via its own interfaces.

## Conventions

- Every `.go` file starts with the SPDX header
  (`SPDX-FileCopyrightText: (c) 2026 Rafal Zajac`, `SPDX-License-Identifier:
  MIT`).
- Tests use `github.com/ctx42/testing` (`assert`, `must`, `tester.T`) and
  `testkit` helpers (`dkrkit`, `httpkit`, …), not testify. Test functions are
  named `Test_<Func>` / `Test_<Type>_<Method>`, subtests use
  `// --- Given ---` / `// --- When ---` / `// --- Then ---` sections.
- Container removal is async (AutoRemove + reaper); assert disappearance with
  the polling helper `ctrGone` in `all_test.go`, not a single `ps` check.
- Shared test helpers and sentinel errors (`errTest`, `errTestOther`) live in
  `all_test.go`.
- Markdown lines wrap at 80 columns (`.editorconfig`).
