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
  printed, plus build args and a random repo/tag; applies `C42_XCTR_*` env
  switches) → `startAndBind` (on failure without a container, `removeBuiltImage`
  drops the built image) → `bindStarted` (fills `cfgHost`/`cfgGuest` with `HOST`
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
  Built images are removed by default; `WithCTRKeepImage` keeps them.
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
