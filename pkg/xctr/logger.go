// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"fmt"
	"strings"
	"sync"

	tc "github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
)

var (
	_ tc.LogConsumer = (*Logger)(nil)
	_ tclog.Logger   = (*Logger)(nil)
)

// Logger is a structure accepting container logs.
type Logger struct {
	cid     string                                           // Container ID.
	lines   []string                                         // Log lines.
	printer func(format string, a ...any) (n int, err error) // Log printer.
	drop    bool                                             // Print only.
	mx      sync.Mutex                                       // Guards fields.
}

// NewLogger returns a new [Logger]. If prt is true, it prints every received
// log line to the standard output.
//
//nolint:forbidigo
func NewLogger(prt bool) *Logger {
	log := &Logger{
		lines: make([]string, 0, 50),
	}
	if prt {
		log.printer = fmt.Printf
	}
	return log
}

// LogConsumerCfg is a convenience function returning [tc.LogConsumerConfig]
// with one consumer that prints to standard output.
func LogConsumerCfg() *tc.LogConsumerConfig {
	return &tc.LogConsumerConfig{Consumers: []tc.LogConsumer{NewLogger(true)}}
}

// SetCID sets container ID. When the logger instance is created the container
// ID is not yet known.
func (log *Logger) SetCID(cid string) *Logger {
	log.mx.Lock()
	defer log.mx.Unlock()
	log.cid = cid
	return log
}

func (log *Logger) Accept(msg tc.Log) {
	log.mx.Lock()
	defer log.mx.Unlock()
	if !log.drop {
		log.lines = append(log.lines, string(msg.Content))
	}
	if log.printer != nil {
		_, _ = log.printer("\t%s |> %s", ShortID(log.cid), string(msg.Content))
	}
}

// Printf writes a formatted log line to the printer when one is configured.
// Unlike Accept, the line is not retained and does not appear in Print.
func (log *Logger) Printf(format string, v ...any) {
	log.mx.Lock()
	defer log.mx.Unlock()
	msg := fmt.Sprintf(format, v...)
	if log.printer != nil {
		_, _ = log.printer("\t%s |> %s", ShortID(log.cid), msg)
	}
}

// Print returns a string representation of the log lines.
func (log *Logger) Print() string {
	log.mx.Lock()
	defer log.mx.Unlock()
	return strings.Join(log.lines, "")
}
