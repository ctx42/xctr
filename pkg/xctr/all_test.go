// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/tester"
	"github.com/ctx42/testkit/pkg/dkrkit"
	tc "github.com/testcontainers/testcontainers-go"
)

// ctrGone polls until the container with the given ID is no longer present,
// failing the test if it is still present after the timeout. Container removal
// is asynchronous (AutoRemove plus the reaper), so a container may briefly
// remain listed with a "removing" status right after Terminate; polling
// tolerates that window. It also tolerates the transient inspect error a
// mid-removal container yields, which surfaces as a nil result.
func ctrGone(t tester.T, id string) {
	t.Helper()
	dkr := dkrkit.NewT(t)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if have, _ := dkr.CtrPs().FindByID(id); have == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("container %s still present after timeout", id)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// endpoint returns HTTP URL for given ctr. When params are present they will
// be added after "?" joined by "&".
func endpoint(ctr *CTR, params ...string) string {
	cfg := ctr.ConfigHost()
	u := "http://" + net.JoinHostPort(cfg["HOST"], cfg["PORT_0"])
	if len(params) > 0 {
		u += "?" + strings.Join(params, "&")
	}
	return u
}

func Test_endpoint(t *testing.T) {
	t.Run("without params", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{
			cfgHost: map[string]string{
				"HOST":   "host",
				"PORT_0": "42",
				"PORT_1": "43",
			},
		}

		// --- When ---
		have := endpoint(ctr)

		// --- Then ---
		assert.Equal(t, "http://host:42", have)
	})

	t.Run("one param", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{
			cfgHost: map[string]string{
				"HOST":   "host",
				"PORT_0": "42",
				"PORT_1": "43",
			},
		}

		// --- When ---
		have := endpoint(ctr, "a=b")

		// --- Then ---
		assert.Equal(t, "http://host:42?a=b", have)
	})

	t.Run("with param", func(t *testing.T) {
		// --- Given ---
		ctr := &CTR{
			cfgHost: map[string]string{
				"HOST":   "host",
				"PORT_0": "42",
				"PORT_1": "43",
			},
		}

		// --- When ---
		have := endpoint(ctr, "a=b", "c=d")

		// --- Then ---
		assert.Equal(t, "http://host:42?a=b&c=d", have)
	})
}

// addrCTR returns a [CTR] bound as if started with ports "80/tcp", "443/tcp",
// and "53/udp" exposed, without a running container.
func addrCTR() *CTR {
	return &CTR{
		dc: &tc.DockerContainer{},
		cfgHost: map[string]string{
			"HOST":   "localhost",
			"PORT_0": "32768/tcp",
			"PORT_1": "32769/tcp",
			"PORT_2": "32770/udp",
		},
		cfgGuest: map[string]string{
			"HOST":   "172.17.0.3",
			"PORT_0": "80/tcp",
			"PORT_1": "443/tcp",
			"PORT_2": "53/udp",
		},
	}
}

// Test sentinel errors. Messages preserved from the original kit package so
// existing assertions on the printed text stay valid.
var (
	errTest      = errors.New("kit test error")
	errTestOther = errors.New("kit test other error")
)
