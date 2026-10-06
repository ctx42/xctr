// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"testing"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
)

func Test_Whitelist_Add(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		wl := NewWhitelist()

		// --- When ---
		err := wl.Add("^echo")

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("error - invalid regexp", func(t *testing.T) {
		// --- Given ---
		wl := NewWhitelist()

		// --- When ---
		err := wl.Add("((unbalanced)")

		// --- Then ---
		assert.ErrorContain(t, "error parsing regexp", err)
	})
}

func Test_Whitelist_Check(t *testing.T) {
	t.Run("error - nil instance", func(t *testing.T) {
		// --- Given ---
		var wl *Whitelist

		// --- When ---
		err := wl.Check("ls")

		// --- Then ---
		assert.ErrorIs(t, ErrNotAllowed, err)
	})

	t.Run("single match", func(t *testing.T) {
		// --- Given ---
		wl := NewWhitelist()
		must.Nil(wl.Add("echo .*"))

		// --- When ---
		err := wl.Check("echo", "hello")

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("multi expr match", func(t *testing.T) {
		// --- Given ---
		wl := NewWhitelist()
		must.Nil(wl.Add("echo .*", "cat file.*"))

		// --- When ---
		err := wl.Check("cat", "file-prefix")

		// --- Then ---
		assert.NoError(t, err)
	})
}

func Test_Whitelist_Check_tabular(t *testing.T) {
	tt := []struct {
		testN string

		expr []string
		cmd  []string
		err  error
		msg  string
	}{
		{
			"error - not matched",
			[]string{"^echo", "^cat file$"},
			[]string{"cat", "file-prefix"},
			ErrNotAllowed,
			"not allowed: cat file-prefix",
		},
		{
			"error - trailing arguments",
			[]string{"^cat file"},
			[]string{"cat", "file", "/etc/shadow"},
			ErrNotAllowed,
			"not allowed: cat file /etc/shadow",
		},
		{
			"error - empty command",
			[]string{"^echo", "^cat file$"},
			[]string{},
			ErrNotAllowed,
			"not allowed: ",
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			wl := NewWhitelist()
			must.Nil(wl.Add(tc.expr...))

			// --- When ---
			err := wl.Check(tc.cmd...)

			// --- Then ---
			assert.ErrorIs(t, tc.err, err)
			assert.ErrorEqual(t, tc.msg, err)
		})
	}
}
