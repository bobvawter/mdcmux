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

// Package mdctest contains test rig behaviors.
package mdctest

import (
	"context"
	"testing"
	"time"

	"vawter.tech/stopper"
	"vawter.tech/stopper/linger"
)

func NewStopperForTest(t *testing.T) *stopper.Context {
	const grace = 5 * time.Second
	const timeout = 30 * time.Second

	stdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)

	rec := linger.NewRecorder(2)
	ctx := stopper.WithInvoker(stdCtx, rec.Invoke)
	t.Cleanup(func() {
		ctx.Stop(grace)
		if err := ctx.Wait(); err != nil {
			t.Errorf("task returned an error: %v", err)
		}
		linger.CheckClean(t, rec)
	})

	return ctx
}
