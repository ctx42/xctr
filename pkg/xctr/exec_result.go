// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"fmt"
	"strings"
)

var _ error = ExecResult{}

// ExecResult represents a result of running a command on a container.
type ExecResult struct {
	// Command execution error.
	//
	// This is an error when a command could not be executed by the Docker
	// system like when container does not exist.
	ExecError error

	// Command exit code, 0 means success.
	ExitCode int

	// Stdout stream.
	SOut string

	// Stderr stream.
	EOut string
}

// Err returns the command execution error when one is set. Otherwise, when the
// exit code is not zero, it returns an error wrapping [ErrExitCode] with the
// trimmed standard error output, "exit code: <stderr>", or "exit code: N" when
// that output is blank. It returns nil on success.
func (er ExecResult) Err() error {
	if er.ExecError != nil {
		return er.ExecError
	}
	if er.ExitCode != 0 {
		if msg := strings.TrimSpace(er.EOut); msg != "" {
			return fmt.Errorf("%w: %s", ErrExitCode, msg)
		}
		return fmt.Errorf("%w: %d", ErrExitCode, er.ExitCode)
	}
	return nil
}

// Error returns the error message, or an empty string when the command
// succeeded. ExecResult is a value type that always satisfies error, so a
// successful result yields "" rather than a nil error.
func (er ExecResult) Error() string {
	if er.ExecError != nil {
		return er.ExecError.Error()
	}
	if err := er.Err(); err != nil {
		return err.Error()
	}
	return ""
}

// CombinedOutput returns combined trimmed output from standard output and
// standard error.
func (er ExecResult) CombinedOutput() string {
	return strings.TrimSpace(er.SOut + er.EOut)
}

// Unwrap returns the error [ExecResult.Err] reports, so [errors.Is] matches
// [ErrExitCode] for a command that exited with a non-zero code.
func (er ExecResult) Unwrap() error { return er.Err() }
