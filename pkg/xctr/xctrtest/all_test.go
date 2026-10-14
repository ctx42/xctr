// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctrtest

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ctx42/xctr/pkg/xctr"
)

// TestMain runs the tests, then removes what the reaper left behind in
// earlier test runs.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := xctr.Sweep(context.Background(), time.Minute); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}
