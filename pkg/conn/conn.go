// Copyright (c) 2025 Bob Vawter (bob@vawter.org)
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
//
// SPDX-License-Identifier: MIT

// Package conn provides message transport for a Machine Data Collection host.
package conn

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"runtime"
	"sync"
	"time"

	"vawter.tech/mdcmux/pkg/message"
)

const writeTimeout = 30 * time.Second

// Conn represents a connection to a single MDC host.
type Conn struct {
	hostname string
	idleTime time.Duration
	logger   *slog.Logger

	mu struct {
		sync.Mutex
		conn        net.Conn
		keepAlive   chan<- struct{}
		respScanner *bufio.Scanner
	}
}

// New constructs a connection to an MDC host.
func New(hostname string) *Conn {
	ret := &Conn{
		hostname: hostname,
		idleTime: writeTimeout,
		logger:   slog.With("hostname", hostname),
	}
	runtime.SetFinalizer(ret, (*Conn).Close)
	return ret
}

// Addr returns the target MDC hostname.
func (c *Conn) Addr() string {
	return c.hostname
}

// Close all resources associated with the connection.
func (c *Conn) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}

// RoundTrip writes a message to the MDC host and receives a response. The
// response message will be interpreted based on the type of message sent.
func (c *Conn) RoundTrip(ctx context.Context, cmd message.Command) (message.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.mu.conn == nil {
		if err := c.dialLocked(ctx); err != nil {
			return nil, err
		}

		resp, err := c.writeLocked(ctx, message.CommandMachineSN)
		if err != nil {
			return nil, err
		}
		c.logger.LogAttrs(ctx, slog.LevelInfo, "connected", slog.Any("sn", resp))
	}

	return c.writeLocked(ctx, cmd)
}

func (c *Conn) closeLocked() {
	if c.mu.conn != nil {
		_ = c.mu.conn.Close()
		c.mu.conn = nil
	}
	if c.mu.keepAlive != nil {
		close(c.mu.keepAlive)
		c.mu.keepAlive = nil
	}
	c.mu.respScanner = nil
}

func (c *Conn) dialLocked(ctx context.Context) error {
	deadline, _ := ctx.Deadline()
	conn, err := net.DialTimeout("tcp", c.hostname, time.Until(deadline))
	if err != nil {
		return err
	}

	// This keepalive channel also acts as an epoch.
	keep := make(chan struct{}, 1)

	c.mu.conn = conn
	c.mu.keepAlive = keep
	c.mu.respScanner = bufio.NewScanner(c.mu.conn)
	c.mu.respScanner.Split(message.ScanPrompt)
	go func() {
		for {
			select {
			case <-time.After(c.idleTime): // Go 1.23 makes this form preferred.
				c.mu.Lock()
				if c.mu.keepAlive == keep {
					c.closeLocked()
					c.logger.LogAttrs(ctx, slog.LevelDebug, "disconnected")
				}
				c.mu.Unlock()
				return

			case _, ok := <-keep: // Exit if connection is closed.
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

func (c *Conn) peek() net.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mu.conn
}

func (c *Conn) writeLocked(ctx context.Context, cmd message.Command) (_ message.Response, err error) {
	c.mu.keepAlive <- struct{}{}

	defer func() {
		if err != nil {
			c.closeLocked()
		}
	}()

	// Guaranteed by Write.
	deadline, _ := ctx.Deadline()
	if err := c.mu.conn.SetDeadline(deadline); err != nil {
		return nil, err
	}

	c.logger.LogAttrs(ctx, slog.LevelDebug, "sending command", slog.Any("command", cmd))

	if _, err := cmd.WriteTo(c.mu.conn); err != nil {
		return nil, err
	}

	if c.mu.respScanner.Scan() {
		data := c.mu.respScanner.Bytes()
		if len(data) > 0 {
			resp, err := cmd.ParseResponse(data)
			if err != nil {
				return nil, err
			}
			c.logger.LogAttrs(ctx, slog.LevelDebug, "received response", slog.Any("response", resp))
			return resp, nil
		}
	}

	if err := c.mu.respScanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}
