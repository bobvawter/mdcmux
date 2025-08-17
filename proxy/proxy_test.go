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
	"fmt"
	"log/slog"
	"net/netip"
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

	cfg := &Config{
		Bind:   netip.AddrFrom4([4]byte{127, 0, 0, 1}),
		Policy: nil,
		Targets: map[string]*Target{
			d.Addr().String(): {
				ProxyPort: 0,
			},
		},
	}

	p, err := New(ctx, notify.VarOf(cfg))
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

	resp, err := pConn.Write(ctx, message.Basic(message.CommandMachineModel))
	r.NoError(err)
	r.Equal(">>MODEL, MDCMUX", resp.(fmt.Stringer).String())
}
