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

package conn

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"vawter.tech/mdcmux/internal/dummy"
	"vawter.tech/mdcmux/pkg/message"
	"vawter.tech/stopper"
)

func TestConn(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	ctx := stopper.WithContext(context.Background())
	defer func() {
		ctx.Stop(10 * time.Millisecond)
		r.NoError(ctx.Wait())
	}()

	ctx.Go(func(ctx *stopper.Context) error {
		select {
		case <-time.After(30 * time.Second):
			r.Fail("timeout")
		case <-ctx.Stopping():
		}
		return nil
	})

	svr, err := dummy.New(ctx, "127.0.0.1:0")
	r.NoError(err)

	c := New(svr.Addr().String())
	r.Nil(c.peek()) // Don't dial until later.

	for cmd := range dummy.Canned {
		resp, err := c.RoundTrip(ctx, cmd)
		r.NoError(err)
		slog.InfoContext(ctx, "OK", slog.Any("resp", resp))
	}
	resp, err := c.RoundTrip(ctx, message.BasicCommand(message.Int64(99)))
	r.NoError(err)
	slog.InfoContext(ctx, "OK", slog.Any("resp", resp))
	r.False(resp.IsSuccess())
	if buf, ok := resp.Buffer(); a.True(ok) {
		a.Equal([]byte("?, ?Q99"), buf)
	}

	resp, err = c.RoundTrip(ctx, message.QueryCommand(message.Int64(10900)))
	r.NoError(err)
	slog.InfoContext(ctx, "OK", slog.Any("resp", resp))
	r.True(resp.IsSuccess())
	if value, ok := resp.Value(); a.True(ok) {
		a.Zero(value)
	}

	resp, err = c.RoundTrip(ctx, message.WriteCommand(message.Int64(10900), message.Int64(4)))
	r.NoError(err)
	slog.InfoContext(ctx, "OK", slog.Any("resp", resp))
	r.True(resp.IsSuccess())

	resp, err = c.RoundTrip(ctx, message.QueryCommand(message.Int64(10900)))
	r.NoError(err)
	slog.InfoContext(ctx, "OK", slog.Any("resp", resp))
	if value, ok := resp.Value(); a.True(ok) {
		a.Equal(message.Int64(4), value)
	}

	resp, err = c.RoundTrip(ctx, message.QueryCommand(message.Int64(0)))
	r.NoError(err)
	slog.InfoContext(ctx, "OK", slog.Any("resp", resp))
	if value, ok := resp.Value(); a.True(ok) {
		a.Equal(message.NaN, value)
	}
}
