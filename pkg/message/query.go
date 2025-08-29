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
	"fmt"
	"io"
	"log/slog"
	"regexp"
)

var (
	queryPattern       = regexp.MustCompile(`^(?P<command>\d+)(?:\s+(?P<variable>\d+(?:\.\d*)?)?)?\s*$`)
	queryCommandIndex  = queryPattern.SubexpIndex("command")
	queryVariableIndex = queryPattern.SubexpIndex("variable")
)

type queryCommand struct {
	commandBase
	variable Number
}

var _ Command = (*queryCommand)(nil)

// QueryCommand retrieves a macro variable.
func QueryCommand(n Number) Command {
	return &queryCommand{variable: n}
}

func (q *queryCommand) Command() (Number, bool)  { return QMacroVariable, true }
func (q *queryCommand) IsSafe() bool             { return q.variable.whole >= 0 && q.variable.frac == 0 }
func (q *queryCommand) Variable() (Number, bool) { return q.variable, true }

func (q *queryCommand) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Any("command", QMacroVariable),
		slog.Bool("safe", q.IsSafe()),
		slog.Any("variable", q.variable),
	)
}

func (q *queryCommand) ParseResponse(buf []byte) (Response, error) {
	parts := bytes.Split(buf, []byte(", "))
	if len(parts) != 2 {
		return OpaqueResponse(buf, false), nil
	}
	num, err := ParseNumber(parts[1])
	if err != nil {
		return OpaqueResponse(buf, false), nil
	}
	return QueryResponse(num), nil
}

func (q *queryCommand) String() string { return stringify(q) }

func (q *queryCommand) WriteTo(out io.Writer) (int64, error) {
	count, err := fmt.Fprintf(out, "?Q600 %f\n", q.variable)
	return int64(count), err
}

type queryResponse struct {
	responseBase
	value Number
}

var _ Response = (*queryResponse)(nil)

// QueryResponse returns a message containing the given value.
func QueryResponse(value Number) Response { return &queryResponse{value: value} }

func (r *queryResponse) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Any("payload", r.value),
	)
}

func (r *queryResponse) IsSuccess() bool       { return true }
func (r *queryResponse) String() string        { return stringify(r) }
func (r *queryResponse) Value() (Number, bool) { return r.value, true }
func (r *queryResponse) WriteTo(out io.Writer) (int64, error) {
	count, err := fmt.Fprintf(out, "MACRO, %f", r.value)
	return int64(count), err
}
