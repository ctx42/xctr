# xctr

`github.com/ctx42/xctr` is a Go module providing a
[testcontainers-go](https://github.com/testcontainers/testcontainers-go)
wrapper for starting, driving, and tearing down Docker containers from Go
tests — a single `CTR` type that starts a container, runs commands and
copies files in and out of it, and stamps it with OCI provenance labels via
`xdef` and `gitaid`, plus an `xctrtest` sub-package of ready-made test
requests and lifecycle helpers.

## Packages

| Package                                                   | What it does                                                                                    |
|-----------------------------------------------------------|-------------------------------------------------------------------------------------------------|
| [`xctr`](pkg/xctr/README.md)                              | Testcontainers-based Docker container wrapper for Go tests: start, exec, copy files, tear down. |
| [`xctrtest`](pkg/xctr/README.md#testing-helpers-xctrtest) | Container requests and test-lifecycle helpers for container-backed tests.                       |

See the [`pkg/xctr` README](pkg/xctr/README.md) for the full package
overview, usage examples, and configuration reference.

## Installation

```shell
go get github.com/ctx42/xctr
```

> [!NOTE]
> The module is not published yet, so `go get` fails until it is.

## ctx42-family dependencies

`xctr` is built on the following ctx42 modules:

| Module                                                   | Role                                                                                      |
|----------------------------------------------------------|-------------------------------------------------------------------------------------------|
| [`xdef`](https://pkg.go.dev/github.com/ctx42/xdef)       | Shared OCI label / env-var / placeholder definitions stamped on every container.          |
| [`gitaid`](https://pkg.go.dev/github.com/ctx42/gitaid)   | Wraps the git CLI to resolve a checkout's origin, description, and commit hash.           |
| [`ring`](https://pkg.go.dev/github.com/ctx42/ring)       | Environment-variable helpers (`EnvGet`/`EnvSet`) used to read `xctr`'s own settings.      |
| [`testing`](https://pkg.go.dev/github.com/ctx42/testing) | The `tester.T`, `assert`, and `must` packages `xctr`'s own tests and `xctrtest` build on. |
| [`testkit`](https://pkg.go.dev/github.com/ctx42/testkit) | Docker, HTTP, network, and OS test helpers used by `xctr`'s test suite.                   |

It also depends on `testcontainers-go`, Docker's own `moby/moby` API and
client packages, and `distribution/reference` for image reference parsing.

## License

MIT — see [LICENSE.md](LICENSE.md).
