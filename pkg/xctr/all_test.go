// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"errors"
)

// Test sentinel errors. Messages preserved from the original kit package so
// existing assertions on the printed text stay valid.
var (
	errTest      = errors.New("kit test error")
	errTestOther = errors.New("kit test other error")
)
