// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"testing"

	"github.com/ctx42/testing/pkg/assert"
)

func Test_ExecResult_Err(t *testing.T) {
	t.Run("non-zero exit code", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			ExitCode: 1,
			EOut:     "test error",
		}

		// --- When ---
		have := er.Err()

		// --- Then ---
		assert.ErrorIs(t, ErrExitCode, have)
		assert.ErrorEqual(t, "exit code: test error", have)
	})

	t.Run("ErrBuf whitespace trimmed", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			ExitCode: 1,
			EOut:     "  \ntest error  \n",
		}

		// --- When ---
		have := er.Err()

		// --- Then ---
		assert.ErrorIs(t, ErrExitCode, have)
		assert.ErrorEqual(t, "exit code: test error", have)
	})

	t.Run("execution error takes precedence", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			ExecError: errTest,
			ExitCode:  1,
			EOut:      "test error",
		}

		// --- When ---
		have := er.Err()

		// --- Then ---
		assert.ErrorIs(t, errTest, have)
	})

	t.Run("non-zero exit code with empty ErrBuf", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			ExitCode: 1,
		}

		// --- When ---
		have := er.Err()

		// --- Then ---
		assert.ErrorIs(t, ErrExitCode, have)
		assert.ErrorEqual(t, "exit code: 1", have)
	})

	t.Run("zero exit code", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			ExitCode: 0,
			EOut:     "test error",
		}

		// --- When ---
		have := er.Err()

		// --- Then ---
		assert.NoError(t, have)
	})
}

func Test_ExecResult_Error(t *testing.T) {
	t.Run("non-zero exit code", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			ExitCode: 1,
			EOut:     "test error",
		}

		// --- When ---
		have := er.Error()

		// --- Then ---
		assert.Equal(t, "exit code: test error", have)
	})

	t.Run("zero exit code", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			ExitCode: 0,
			EOut:     "test error",
		}

		// --- When ---
		have := er.Error()

		// --- Then ---
		assert.Empty(t, have)
	})

	t.Run("execution error takes precedence", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			ExecError: errTest,
			ExitCode:  1,
			EOut:      "test error",
		}

		// --- When ---
		have := er.Error()

		// --- Then ---
		assert.Equal(t, errTest.Error(), have)
	})
}

func Test_ExecResult_CombinedOutput(t *testing.T) {
	t.Run("both outputs", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			SOut: "\n out\n",
			EOut: " err  \n ",
		}

		// --- When ---
		have := er.CombinedOutput()

		// --- Then ---
		assert.Equal(t, "out\n err", have)
	})

	t.Run("standard output only", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			SOut: "\n out\n",
		}

		// --- When ---
		have := er.CombinedOutput()

		// --- Then ---
		assert.Equal(t, "out", have)
	})

	t.Run("standard error output", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{
			EOut: " err  \n ",
		}

		// --- When ---
		have := er.CombinedOutput()

		// --- Then ---
		assert.Equal(t, "err", have)
	})

	t.Run("no outputs", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{}

		// --- When ---
		have := er.CombinedOutput()

		// --- Then ---
		assert.Equal(t, "", have)
	})
}

func Test_ExecResult_Unwrap(t *testing.T) {
	t.Run("nil execution error", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{}

		// --- When ---
		err := er.Unwrap()

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("non-nil execution error", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{ExecError: errTest}

		// --- When ---
		err := er.Unwrap()

		// --- Then ---
		assert.ErrorIs(t, errTest, err)
	})

	t.Run("non-zero exit code", func(t *testing.T) {
		// --- Given ---
		er := ExecResult{ExitCode: 1}

		// --- When ---
		err := er.Unwrap()

		// --- Then ---
		assert.ErrorIs(t, ErrExitCode, err)
	})
}
