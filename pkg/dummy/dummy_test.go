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

package dummy

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"vawter.tech/mdcmux/internal/mdctest"
	"vawter.tech/mdcmux/pkg/conn"
	"vawter.tech/mdcmux/pkg/message"
)

func TestDummyServer(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	r := require.New(t)

	ctx := mdctest.NewStopperForTest(t)

	// Start a dummy server.
	d, err := New(ctx, "127.0.0.1:0")
	r.NoError(err)

	dConn := conn.New(d.Addr().String())

	check := func(r *require.Assertions, expected string, msg message.Command) {
		resp, err := dConn.RoundTrip(ctx, msg)
		r.NoError(err)
		r.Equal(expected, resp.(fmt.Stringer).String())
	}

	t.Run("basic", func(t *testing.T) {
		r := require.New(t)
		check(r, "MODEL, MDCMUX", message.CommandMachineModel)
		check(r, "?, ?Q999", message.BasicCommand(message.Int64(999)))
	})

	t.Run("vars", func(t *testing.T) {
		r := require.New(t)

		key := message.Int(2)

		d.Poke(key, message.Int(42))
		check(r, "MACRO, 42.0", message.QueryCommand(key))

		pi := message.NewNumber(3, 141592)
		check(r, "!", message.WriteCommand(key, pi))
		found, ok := d.Peek(key)
		r.True(ok)
		r.Equal(pi, found)

		check(r, "MACRO, 3.141592", message.QueryCommand(key))
	})
}
