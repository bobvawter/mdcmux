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

package message

import (
	"bytes"
	"io"
	"log/slog"
)

type opaqueResponse struct {
	responseBase

	buf     []byte
	success bool
}

var _ Response = (*opaqueResponse)(nil)

// An OpaqueResponse contains arbitrary MDC wire data.
func OpaqueResponse(data []byte, success bool) Response {
	ret := opaqueResponse{buf: bytes.Clone(data), success: success}
	return &ret
}

func (r *opaqueResponse) Buffer() ([]byte, bool) {
	return r.buf, true
}

func (r *opaqueResponse) IsSuccess() bool { return r.success }

func (r *opaqueResponse) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("payload", string(r.buf)),
	)
}

func (r *opaqueResponse) String() string { return stringify(r) }

func (r *opaqueResponse) WriteTo(out io.Writer) (int64, error) {
	count, err := out.Write(r.buf)
	return int64(count), err
}
