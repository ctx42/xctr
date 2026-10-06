// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctrtest

import (
	"testing"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/tester"
)

func Test_AssertBuildArg(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		// --- Given ---
		tspy := tester.New(t)
		tspy.Close()

		ba := map[string]*string{
			"KEY0": new("VAL0"),
			"KEY1": new("VAL1"),
		}

		// --- When ---
		have := AssertBuildArg(tspy, "KEY1", "VAL1", ba)

		// --- Then ---
		assert.True(t, have)
	})

	t.Run("does not exist", func(t *testing.T) {
		// --- Given ---
		tspy := tester.New(t)
		tspy.ExpectError()
		wMsg := "" +
			"expected map to have a key:\n" +
			"  key: \"KEY2\"\n" +
			"  map:\n" +
			"       map[string]*string{\n" +
			"         \"KEY0\": \"VAL0\",\n" +
			"         \"KEY1\": \"VAL1\",\n" +
			"       }"
		tspy.ExpectLogEqual(wMsg)
		tspy.Close()

		ba := map[string]*string{
			"KEY0": new("VAL0"),
			"KEY1": new("VAL1"),
		}

		// --- When ---
		have := AssertBuildArg(tspy, "KEY2", "VAL2", ba)

		// --- Then ---
		assert.False(t, have)
	})

	t.Run("not expected value", func(t *testing.T) {
		// --- Given ---
		tspy := tester.New(t)
		tspy.ExpectError()
		wMsg := "" +
			"expected values to be equal:\n" +
			"  want: \"VAL2\"\n" +
			"  have: \"VAL1\""
		tspy.ExpectLogEqual(wMsg)
		tspy.Close()

		ba := map[string]*string{
			"KEY0": new("VAL0"),
			"KEY1": new("VAL1"),
		}

		// --- When ---
		have := AssertBuildArg(tspy, "KEY1", "VAL2", ba)

		// --- Then ---
		assert.False(t, have)
	})

	t.Run("nil value", func(t *testing.T) {
		// --- Given ---
		tspy := tester.New(t)
		tspy.ExpectError()
		tspy.ExpectLogEqual("expected non-nil value")
		tspy.Close()

		ba := map[string]*string{"KEY0": nil}

		// --- When ---
		have := AssertBuildArg(tspy, "KEY0", "VAL0", ba)

		// --- Then ---
		assert.False(t, have)
	})
}
