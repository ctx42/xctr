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
| Connect to it from the host               | `ctr.HostAddr("5432/tcp")` → `localhost:32768`                  |
| Connect to it from another container      | `ctr.GuestAddr("5432/tcp")` → `172.17.0.3:5432`                 |
| Copy a file in or out                     | `ctr.CopyTo(…)`, `ctr.ReadFile(…)`                              |
| Test against the real production service  | Ship a `pgctr` package; a test calls `pgctr.NewDB(t)`           |
| Start Postgres once, not once per test    | `xctr.OnceStart(ctx, env, name, New)`                           |
| Stop tests from mutating a shared fixture | `ctr.SetReadOnly(ctx, "allowed regex", …)`                      |
| See container logs when a test fails      | Set `C42_XCTR_LOG=true` — no code change                        |
| Know which checkout left a container      | Automatic OCI labels: source repo, revision, version, date      |
| Leave nothing behind                      | `Cleanup` terminates it and runs every cleanup once, in reverse |

And because `CTR` is a plain struct, you can embed it. Wrap it once as
`PostgresCTR` or `RedisCTR`, add the two or three domain methods your tests
care about, and every test in every package gets a typed, ready-to-use service
container. `xctr` was extracted from an internal library that does exactly
that for PostgreSQL, RabbitMQ, FTP, and Traefik test containers; [Containers
as building blocks](#containers-as-building-blocks) shows the pattern.

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

// NewRedis returns a Redis container that is not started yet.
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

// Addr returns the address tests connect to, e.g. "localhost:34978".
func (r *Redis) Addr() (string, error) { return r.HostAddr("6379/tcp") }

// Ping runs redis-cli inside the container.
func (r *Redis) Ping(ctx context.Context) error {
	return r.Exec(ctx, "redis-cli", "PING").Err()
}
```

`HostAddr` and `GuestAddr` look a port up by its container-side number, so a
typo or a port the request never exposed is an `xctr.ErrNotExposed` error, not
an empty string.

### Share one container across tests

Starting a server per test is slow. `xctr.OnceStart` starts the container on
first use and hands every later caller the same one, even from parallel
tests. A failed start is cleaned up and not remembered, so the next caller
tries again. Stop everything when the package is done:

```go
func TestMain(m *testing.M) {
	defer func() { _, _ = xctr.OnceStopAll(context.Background()) }()
	m.Run()
}

// SharedRedis returns the Redis all tests share, starting it on first use.
func SharedRedis(t *testing.T) *Redis {
	t.Helper()
	r, err := xctr.OnceStart(t.Context(), os.Environ(), "redis", NewRedis)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
```

### Containers as building blocks

The service that runs in production can own its test container. Ship a small
package next to the service that builds the production image, and the
container becomes a building block: any test in any module starts the real
service with one call, and bigger setups combine several blocks.

The service module embeds the Dockerfile it ships:

```go
// Package pgdb is the PostgreSQL service; its Dockerfile is the production one.
package pgdb

//go:embed Dockerfile
var dockerfile []byte

// BuildContext returns the build context of the image this module ships.
func BuildContext() io.ReadSeeker {
	return xctr.MustDockerfileArchive(dockerfile)
}
```

A container package next to it builds from that context and shares one server
between all tests. Tests need isolation, not a server each, so `NewDB` gives
every test an empty database of its own on that server:

```go
// Package pgctr starts the PostgreSQL container built from this module.
package pgctr

// Name is the name of the PostgreSQL container all tests share.
const Name = "pgctr"

// CTR is the PostgreSQL server built from this module's Dockerfile.
type CTR struct{ *xctr.CTR }

// New returns a PostgreSQL container that is not started yet.
func New(name string) *CTR {
	ready := "database system is ready to accept connections"
	req := tc.GenericContainerRequest{
		Started: true,
		ContainerRequest: tc.ContainerRequest{
			FromDockerfile: tc.FromDockerfile{
				ContextArchive: pgdb.BuildContext(),
				Dockerfile:     "Dockerfile",
			},
			ExposedPorts: []string{"5432/tcp"},
			Env:          map[string]string{"POSTGRES_PASSWORD": "test"},
			WaitingFor:   wait.ForLog(ready).WithOccurrence(2),
		},
	}
	return &CTR{xctr.NewCTR(name, req)}
}

// Start returns the shared PostgreSQL container, starting it on first use.
func Start(ctx context.Context, env []string) (*CTR, error) {
	return xctr.OnceStart(ctx, env, Name, New)
}

// DSN returns the connection string to database db for tests on the host.
func (ctr *CTR) DSN(db string) (string, error) {
	addr, err := ctr.HostAddr("5432/tcp")
	if err != nil {
		return "", err
	}
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("postgres", "test"),
		Host:     addr,
		Path:     db,
		RawQuery: "sslmode=disable",
	}
	return dsn.String(), nil
}

// NewDB creates an empty database in the shared container and returns its
// connection string. It fails the test on error.
func NewDB(t testing.TB) string {
	t.Helper()
	ctr, err := Start(t.Context(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	db := "db_" + strings.ToLower(rand.Text())
	res := ctr.Exec(t.Context(), "createdb", "-U", "postgres", db)
	if err = res.Err(); err != nil {
		t.Fatal(err)
	}
	dsn, err := ctr.DSN(db)
	if err != nil {
		t.Fatal(err)
	}
	return dsn
}
```

A test in any other module now runs against the production database, built
from the same Dockerfile that ships, in one line — and in parallel with the
rest:

```go
func TestMain(m *testing.M) {
	defer func() { _, _ = xctr.OnceStopAll(context.Background()) }()
	m.Run()
}

func Test_CreateOrder(t *testing.T) {
	t.Parallel()
	store, err := orders.Open(pgctr.NewDB(t)) // A database of its own.
	if err != nil {
		t.Fatal(err)
	}
	// ...
}
```

Blocks combine. `GuestAddr` is the address other containers reach a container
on, so a service made of several containers wires them together before it
starts:

```go
// Start returns the shared API container, wired to the shared database and
// starting both on first use.
func Start(ctx context.Context, env []string) (*CTR, error) {
	db, err := pgctr.Start(ctx, env)
	if err != nil {
		return nil, err
	}
	addr, err := db.GuestAddr("5432/tcp") // Where other containers reach it.
	if err != nil {
		return nil, err
	}
	return xctr.OnceStart(ctx, env, Name, func(name string) *CTR {
		ctr := New(name)
		_ = ctr.Setenv("DB_ADDR", addr)
		return ctr
	})
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
