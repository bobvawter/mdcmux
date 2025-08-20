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

package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"vawter.tech/mdcmux/conn"
	"vawter.tech/mdcmux/dummy"
	"vawter.tech/mdcmux/message"
	"vawter.tech/notify"
	"vawter.tech/stopper"
)

func TestProxy(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	r := require.New(t)

	ctx := stopper.WithContext(context.Background())
	defer func() {
		ctx.Stop(100 * time.Millisecond)
		r.NoError(ctx.Wait())
	}()

	// Start a dummy server.
	d, err := dummy.New(ctx, "127.0.0.1:0")
	r.NoError(err)

	cfg := notify.VarOf(&Config{
		Bind: netip.AddrFrom4([4]byte{127, 0, 0, 1}),
		Policy: map[netip.Prefix]*Policy{
			netip.MustParsePrefix("127.0.0.1/32"): {
				AllowWrites: [][2]int{
					{1, 33},
				},
				Audit: true,
			},
		},
		Targets: map[string]*Target{
			d.Addr().String(): {
				ProxyPort: 0,
			},
		},
	})

	p, err := New(ctx, cfg)
	r.NoError(err)

	var pConn *conn.Conn
	for {
		p.mu.RLock()
		_, reconfigured := p.reconfigured.Get()
		bindings := p.mu.listeners
		p.mu.RUnlock()

		if len(bindings) == 0 {
			select {
			case <-ctx.Stopping():
				r.Fail("never saw configuration update")
			case <-reconfigured:
				continue
			}
		}

		r.Len(bindings, 1)
		for _, b := range bindings {
			pConn = conn.NewConn(b.Addr().String())
		}
		break
	}

	check := func(r *require.Assertions, expected string, msg message.Message) {
		resp, err := pConn.Write(ctx, msg)
		r.NoError(err)
		r.Equal(expected, resp.(fmt.Stringer).String())
	}

	t.Run("basic", func(t *testing.T) {
		r := require.New(t)
		check(r, "MODEL, MDCMUX", message.Basic(message.CommandMachineModel))
		check(r, "?, MDCMUX DENY POLICY", message.Basic(message.Int64(999)))
	})

	t.Run("writes", func(t *testing.T) {
		r := require.New(t)
		check(r, "!", message.Write(message.Int64(2), message.NewNumber(3, 141592)))
		check(r, "?, MDCMUX DENY POLICY", message.Write(message.Int64(200), message.NewNumber(3, 141592)))
		check(r, "MACRO, 3.141592", message.Query(message.Int64(2)))
	})

	t.Run("no_policy_match", func(t *testing.T) {
		r := require.New(t)

		_, reconfigured := p.reconfigured.Get()
		cfg.Set(&Config{
			Bind: netip.AddrFrom4([4]byte{127, 0, 0, 1}),
			Policy: map[netip.Prefix]*Policy{
				netip.MustParsePrefix("1.1.1.1/32"): {},
			},
			Targets: map[string]*Target{
				d.Addr().String(): {
					ProxyPort: 0,
				},
			},
		})
		<-reconfigured

		// Ensure that a connection which is de-configured is dropped.
		msg, err := pConn.Write(ctx, message.Basic(message.CommandMachineModel))
		r.NoError(err)
		r.Equal(string(message.Prompt), msg.String())
		_, err = pConn.Write(ctx, message.Basic(message.CommandMachineModel))
		if errors.Is(err, io.EOF) {
		} else if errors.Is(err, syscall.Errno(0)) {
			var errno syscall.Errno
			r.ErrorAs(err, &errno)
			r.Equal(syscall.ECONNRESET, errno)
		} else {
			r.Fail("connection not dropped")
		}
	})
}
