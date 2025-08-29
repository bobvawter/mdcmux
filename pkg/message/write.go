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
	"fmt"
	"io"
	"log/slog"
	"regexp"
)

var (
	writePattern  = regexp.MustCompile(`^(?P<variable>\d+)\s+(?P<value>[+-]?\d+(?:\.\d*)?)\s*$`)
	writeVariable = writePattern.SubexpIndex("variable")
	writeValue    = writePattern.SubexpIndex("value")
)

type writeCommand struct {
	commandBase
	variable Number
	value    Number
}

var _ Command = (*writeCommand)(nil)

// WriteCommand represents an assignment to a macro variable.
func WriteCommand(target, value Number) Command {
	return &writeCommand{variable: target, value: value}
}

func (w *writeCommand) IsWrite() bool { return true }

func (w *writeCommand) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("write", true),
		slog.Any("variable", w.variable),
		slog.Any("value", w.value),
	)
}
func (w *writeCommand) Value() (Number, bool) { return w.value, true }

func (w *writeCommand) Variable() (Number, bool) { return w.variable, true }

func (w *writeCommand) ParseResponse(buf []byte) (Response, error) {
	return OpaqueResponse(buf, len(buf) == 1 && buf[0] == '!'), nil
}

func (w *writeCommand) String() string { return stringify(w) }

func (w *writeCommand) WriteTo(out io.Writer) (int64, error) {
	count, err := fmt.Fprintf(out, "?E%d %f\n", w.variable, w.value)
	return int64(count), err
}
