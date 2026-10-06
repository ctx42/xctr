# xctr

Real Docker containers in your Go tests, without the testcontainers-go
ceremony.

`xctr` is a thin, opinionated wrapper around
[testcontainers-go](https://github.com/testcontainers/testcontainers-go). You
still describe a container with the testcontainers request you already know;
`xctr` hands you back one small type, `CTR`, that starts it, runs commands in
it, moves files in and out, and cleans up after itself. The whole contract is
a single 17-method `Container` interface, small enough to read in one sitting.

```go
ctr := xctr.NewCTR("box", xctr.ImageReq("busybox:1.38-uclibc"))
_ = ctr.Start(ctx, os.Environ())
defer ctr.Cleanup(ctx)

res := ctr.Exec(ctx, "uname", "-s") // res.SOut == "Linux\n"
```

## Why xctr?

testcontainers-go can do almost anything with a container, and its API is as
big as that sounds. A test suite, though, needs the same handful of things over
and over: start a service, get its address, poke at it, share it between
tests, and make sure it is gone afterwards. `xctr` makes exactly those things
one-liners and stays out of your way for the rest.

| You want to…                              | With `xctr`                                                     |
|-------------------------------------------|-----------------------------------------------------------------|
| Run a command and check it worked         | `ctr.Exec(ctx, "cmd", "arg").Err()` — non-zero exit is an error |
| Read its output                           | `res.SOut`, `res.EOut`, `res.ExitCode` — no reader plumbing     |
| Connect to it from the host               | `ctr.ConfigHost()` → `HOST`, `PORT_0`, …                        |
| Connect to it from another container      | `ctr.ConfigGuest()` → container IP and in-container ports       |
| Copy a file in or out                     | `ctr.CopyTo(…)`, `ctr.ReadFile(…)`                              |
| Start Postgres once, not once per test    | `xctr.OnceAdd(ctr)`, `xctr.OnceGet(name)`, `xctr.OnceStopAll`   |
| Stop tests from mutating a shared fixture | `ctr.SetReadOnly(ctx, "allowed regex", …)`                      |
| See container logs when a test fails      | Set `C42_XCTR_LOG=true` — no code change                        |
| Know which checkout left a container      | Automatic OCI labels: source repo, revision, version, date      |
| Leave nothing behind                      | `Cleanup` terminates it and runs every cleanup once, in reverse |

And because `CTR` is a plain struct, you can embed it. Wrap it once as
`PostgresCTR` or `RedisCTR`, add the two or three domain methods your tests
care about, and every test in every package gets a typed, ready-to-use service
container. `xctr` was extracted from an internal library that does exactly
that for PostgreSQL, RabbitMQ, FTP, and Traefik test containers.

## Usage

### Run a command in a container

`xctr.ImageReq` gives you an idle container (its entrypoint is
`tail -f /dev/null`) to exec into:

```go
func Test_Uname(t *testing.T) {
	ctx := context.Background()

	ctr := xctr.NewCTR("box", xctr.ImageReq("busybox:1.38-uclibc"))
	if err := ctr.Start(ctx, os.Environ()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctr.Cleanup(ctx) })

	res := ctr.Exec(ctx, "uname", "-s")
	if err := res.Err(); err != nil {
		t.Fatal(err)
	}
	t.Log(res.SOut) // Linux
}
```

### Build your own container type

Describe the service with a regular testcontainers request, embed `*xctr.CTR`,
and add only what your tests need:

```go
// Redis is a Redis server container for tests.
type Redis struct{ *xctr.CTR }

func NewRedis(name string) *Redis {
	req := tc.GenericContainerRequest{
		Started: true,
		ContainerRequest: tc.ContainerRequest{
			Image:        "redis:8-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForListeningPort("6379/tcp"),
		},
	}
	return &Redis{xctr.NewCTR(name, req)}
}

// Addr returns the address the tests connect to, e.g. "localhost:34978".
func (r *Redis) Addr() string {
	cfg := r.ConfigHost()
	port, _, _ := strings.Cut(cfg["PORT_0"], "/")
	return cfg["HOST"] + ":" + port
}

// Ping runs redis-cli inside the container.
func (r *Redis) Ping(ctx context.Context) error {
	return r.Exec(ctx, "redis-cli", "PING").Err()
}
```

### Share one container across tests

Starting a database per test is slow. Register it with the `Once` registry,
fetch it by name, and stop everything when the package is done:

```go
func TestMain(m *testing.M) {
	defer func() { _, _ = xctr.OnceStopAll(context.Background()) }()
	m.Run()
}

// redisMx makes the get-or-start in SharedRedis safe for parallel tests.
var redisMx sync.Mutex

// SharedRedis starts Redis on first use; later calls get the same one.
func SharedRedis(t *testing.T) *Redis {
	t.Helper()
	redisMx.Lock()
	defer redisMx.Unlock()
	if ctr := xctr.OnceGet("redis"); ctr != nil {
		return ctr.(*Redis)
	}
	ctx := context.Background()
	r := NewRedis("redis")
	if err := r.Start(ctx, os.Environ()); err != nil {
		_ = r.Cleanup(ctx) // A failed start may leave a container behind.
		t.Fatal(err)
	}
	xctr.OnceAdd(r)
	return r
}
```

### Lock down a shared container

A shared fixture is only useful if no test quietly changes it. `SetReadOnly`
restricts `Exec` to a whitelist of regular expressions matching the whole
command; anything else fails with `xctr.ErrNotAllowed`:

```go
_ = ctr.SetReadOnly(ctx, `cat /etc/.*`)

ctr.Exec(ctx, "cat", "/etc/hostname").Err() // <nil>
ctr.Exec(ctx, "rm", "-rf", "/data").Err()   // not allowed: rm -rf /data
```

### Debug from the environment

`Start` takes an `os.Environ()`-shaped slice, so you can flip these on for a
single run without touching code:

- `C42_XCTR_LOG=true` — print container logs.
- `C42_XCTR_BUILD_LOG=true` — print Dockerfile build logs.
- `C42_XCTR_ENTRYPOINT="tail -f /dev/null"` — override the entrypoint, e.g.
  to keep a crashing service alive while you poke at it.

The [package README](pkg/xctr/README.md) has the full API tour, sentinel
errors, and configuration reference.

## Installation

```shell
go get github.com/ctx42/xctr
```

> [!NOTE]
> The module is not published yet, so `go get` fails until it is.

Requires Go 1.26 or newer and a reachable Docker daemon (`DOCKER_HOST`, or the
default local socket).

## Packages

| Package                                                   | What it does                                                         |
|-----------------------------------------------------------|----------------------------------------------------------------------|
| [`xctr`](pkg/xctr/README.md)                              | The `CTR` container, `Once` registry, exec whitelist, image helpers. |
| [`xctrtest`](pkg/xctr/README.md#testing-helpers-xctrtest) | Ready-made test requests and `t.Cleanup`-aware lifecycle helpers.    |

## Built on

- [testcontainers-go](https://github.com/testcontainers/testcontainers-go) —
  does the heavy lifting; `xctr` never hides it, and `ctr.Container()` returns
  the underlying `*tc.DockerContainer` when you need it.
- [`xdef`](https://pkg.go.dev/github.com/ctx42/xdef) and
  [`gitaid`](https://pkg.go.dev/github.com/ctx42/gitaid) — the OCI label
  definitions and the git lookup behind container provenance.
- [`ring`](https://pkg.go.dev/github.com/ctx42/ring) — reads `xctr`'s
  settings from the environment slice you pass in.

## License

MIT — see [LICENSE.md](LICENSE.md).
