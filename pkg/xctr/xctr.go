// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

// Package xctr is a Docker container test harness built on testcontainers-go.
// It manages the container lifecycle and provides command execution, file
// transfer, tar archive helpers, an exec command whitelist, and single-start
// container tracking.
package xctr

import (
	"context"
	"errors"
	"io"

	tc "github.com/testcontainers/testcontainers-go"
)

// LabTestCtrName is the container label holding the name of the test that
// created the container. It has no counterpart in [xdef].
//
// Example: "TestCTR_Start".
const LabTestCtrName = "dev.ctx42.test.ctr.name"

// Environment variables.
const (
	// EnvCTRLog is an environment variable that, when set, causes container
	// run logs to be printed to the standard output.
	EnvCTRLog = "C42_XCTR_LOG"

	// EnvCTRBuildLog is an environment variable that, when set, causes
	// container build logs to be printed to the standard output.
	EnvCTRBuildLog = "C42_XCTR_BUILD_LOG"

	// EnvCTREntrypoint is an environment variable with command to overwrite
	// container's entrypoint.
	//
	// The value is split on whitespace. When set,
	// [tc.GenericContainerRequest.WaitingFor] and the request's Cmd are set to
	// nil; the exposed ports are kept.
	EnvCTREntrypoint = "C42_XCTR_ENTRYPOINT"
)

// Sentinel errors.
var (
	// ErrEmptyCmd is returned when [CTR.Exec] command is empty.
	ErrEmptyCmd = errors.New("empty exec command")

	// ErrNotStarted is returned when container is not started.
	ErrNotStarted = errors.New("container not started")

	// ErrRunning is returned when container is already running.
	ErrRunning = errors.New("container already running")

	// ErrExitCode is returned when command ran with [CTR.Exec] returns
	// non-zero exit code.
	ErrExitCode = errors.New("exit code")

	// ErrNoNetwork is returned when container has no networks connected.
	ErrNoNetwork = errors.New("no network")

	// ErrReadOnly is returned for not allowed actions on the running container
	// which is read-only.
	ErrReadOnly = errors.New("read only")

	// ErrNotAllowed represents a not allowed action error.
	ErrNotAllowed = errors.New("not allowed")
)

// Container represents a container.
type Container interface {
	Manager
	Info
	Actions
}

// Manager describes and provides methods to manage a container.
type Manager interface {
	// Name returns the container name.
	Name() string

	// Request returns the generic container request as configured. It does
	// not include what Start adds, also after the container is started.
	Request() tc.GenericContainerRequest

	// Start builds or pulls the image, injects SCM and build metadata as
	// labels and environment variables, starts the container, and registers
	// its termination as a cleanup.
	Start(ctx context.Context, env []string) error

	// Cleanup frees up resources allocated to the container.
	Cleanup(ctx context.Context) error
}

// Info provides information about started container.
type Info interface {
	// ID returns unique container ID.
	ID() string

	// Name returns the container name.
	Name() string

	// Reference returns image reference the container was created from.
	Reference() string

	// IsRunning returns true if the container is running.
	IsRunning() bool

	// ConfigHost returns configuration where keys dealing with hosts and ports
	// are set so the host can connect to the container.
	ConfigHost() map[string]string

	// ConfigGuest returns configuration where keys dealing with hosts and
	// ports are set so other containers can connect to this container.
	ConfigGuest() map[string]string

	// IsReadOnly returns true when container is read-only.
	//
	// NOTE: The semantics of the read-only status are implementation-defined.
	IsReadOnly() bool
}

// Actions provide methods operating on running container.
type Actions interface {
	// Exec executes command in a running container.
	//
	// When a whitelist is used it returns [ErrNotAllowed] when the command is
	// not on the whitelist (see [Actions.SetExecWhitelist]).
	Exec(ctx context.Context, cmd ...string) ExecResult

	// CopyTo copies contents of the reader to the running container as a file
	// at destination dst with file mode. Returns [ErrNotStarted] error if the
	// container is not running.
	CopyTo(ctx context.Context, rdr io.Reader, dst string, mode int64) error

	// ReadFile reads file from the running container. Returns [ErrNotStarted]
	// error if the container is not running.
	ReadFile(ctx context.Context, pth string) ([]byte, error)

	// Setenv sets the value of the environment variable named by the key. Must
	// be set before the container is started.
	Setenv(key, value string) error

	// SetLabel sets the value of the image label. Must be set before the
	// container is started.
	SetLabel(name, value string) error

	// SetReadOnly sets the container as read-only. The expr is a list of
	// regular expressions that form a whitelist for [Actions.Exec].
	//
	// When a container is read-only all implementor methods that may change
	// it should return an error. It may be called only once.
	//
	// NOTE: The semantics of the read-only status are implementation-defined.
	SetReadOnly(ctx context.Context, expr ...string) error

	// SetExecWhitelist sets a whitelist of commands that can be run by
	// [Actions.Exec]. The expr must be a valid regular expression matching
	// the whole command. It may be called only once.
	SetExecWhitelist(expr ...string) error
}

// Descriptor wraps the Describe method returning a container description.
type Descriptor interface{ Describe() string }
