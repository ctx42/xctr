// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"fmt"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
	tc "github.com/testcontainers/testcontainers-go"
)

func Test_NewLogger(t *testing.T) {
	// --- When ---
	have := NewLogger(true)

	// --- Then ---
	assert.Equal(t, "", have.cid)
	assert.NotNil(t, have.printer)
	assert.NotNil(t, have.lines)
	assert.Len(t, 0, have.lines)
}

func Test_LogConsumerCfg(t *testing.T) {
	// --- When ---
	have := LogConsumerCfg()

	// --- Then ---
	assert.NotNil(t, have)
	assert.Len(t, 1, have.Consumers)
	assert.NotNil(t, have.Consumers[0].(*Logger).printer) //nolint:forbidigo
}

func Test_Logger_SetCID(t *testing.T) {
	// --- Given ---
	log := NewLogger(true)

	// --- When ---
	have := log.SetCID("cid")

	// --- Then ---
	assert.Same(t, log, have)
	assert.Equal(t, "cid", log.cid)
}

//nolint:forbidigo
func Test_Logger_Accept(t *testing.T) {
	t.Run("printing", func(t *testing.T) {
		// --- Given ---
		var lines []string
		printer := func(format string, a ...any) (n int, err error) {
			msg := fmt.Sprintf(format, a...)
			lines = append(lines, msg)
			return len(msg), nil
		}

		log := NewLogger(true)
		log.printer = printer

		// --- When ---
		log.Accept(tc.Log{Content: []byte("abc")})
		log.Accept(tc.Log{Content: []byte("def")})

		// --- Then ---
		assert.Len(t, 2, log.lines)
		assert.Len(t, 2, lines)
		assert.Equal(t, "abc", log.lines[0])
		assert.Equal(t, "def", log.lines[1])
		assert.Equal(t, "\t |> abc", lines[0])
		assert.Equal(t, "\t |> def", lines[1])
	})

	t.Run("not printing", func(t *testing.T) {
		// --- Given ---
		log := NewLogger(false)

		// --- When ---
		log.Accept(tc.Log{Content: []byte("abc")})
		log.Accept(tc.Log{Content: []byte("def")})

		// --- Then ---
		assert.Len(t, 2, log.lines)
		assert.Equal(t, "abc", log.lines[0])
		assert.Equal(t, "def", log.lines[1])
	})
}

//nolint:forbidigo
func Test_Logger_Printf(t *testing.T) {
	t.Run("printing", func(t *testing.T) {
		// --- Given ---
		var lines []string
		printer := func(format string, a ...any) (n int, err error) {
			msg := fmt.Sprintf(format, a...)
			lines = append(lines, msg)
			return len(msg), nil
		}

		log := NewLogger(true)
		log.printer = printer

		// --- When ---
		log.Printf("%s", "a")
		log.Printf("%d", 1)

		// --- Then ---
		assert.Len(t, 0, log.lines)
		assert.Len(t, 2, lines)
		assert.Equal(t, "\t |> a", lines[0])
		assert.Equal(t, "\t |> 1", lines[1])
	})

	t.Run("not printing", func(t *testing.T) {
		// --- Given ---
		log := NewLogger(false)

		// --- When ---
		log.Printf("%s", "a")
		log.Printf("%d", 1)

		// --- Then ---
		assert.Len(t, 0, log.lines)
	})
}

func Test_Logger_Print(t *testing.T) {
	// --- Given ---
	log := NewLogger(false)
	log.Accept(tc.Log{Content: []byte("abc\n")})
	log.Accept(tc.Log{Content: []byte("def\n")})

	// --- When ---
	have := log.Print()

	// --- Then ---
	assert.Equal(t, "abc\ndef\n", have)
}
