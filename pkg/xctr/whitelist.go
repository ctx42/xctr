// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"fmt"
	"regexp"
	"strings"
)

// Whitelist represents command whitelist.
type Whitelist struct{ list []*regexp.Regexp }

// NewWhitelist returns a new instance of [Whitelist].
func NewWhitelist() *Whitelist {
	return &Whitelist{list: make([]*regexp.Regexp, 0, 10)}
}

// Add adds an expression to the whitelist. The expression must be a valid
// regular expression and must match the whole command, see [Whitelist.Check].
func (wl *Whitelist) Add(expr ...string) error {
	for _, e := range expr {
		rx, err := regexp.Compile("^(?:" + e + ")$")
		if err != nil {
			return fmt.Errorf("whitelist expr %q: %w", e, err)
		}
		wl.list = append(wl.list, rx)
	}
	return nil
}

// Check reports whether the command is on the whitelist, returning
// [ErrNotAllowed] if it is not. The command arguments are joined with single
// spaces, and an expression allows the command only when it matches the whole
// joined string. It is safe to call on a nil [Whitelist], which allows
// nothing.
func (wl *Whitelist) Check(cmd ...string) error {
	c := strings.Join(cmd, " ")
	if wl == nil {
		return fmt.Errorf("%w: %s", ErrNotAllowed, c)
	}
	for _, e := range wl.list {
		if e.MatchString(c) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotAllowed, c)
}
