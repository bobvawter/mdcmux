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
	"unique"
)

var (
	canonicalBasic = &cmap[Number, basicCommand]{
		new: func(n unique.Handle[Number]) *basicCommand {
			return &basicCommand{command: n}
		},
	}
	hMacroVariable = unique.Make(QMacroVariable)
)

// A basic query with no parameters.
type basicCommand struct {
	commandBase
	command    unique.Handle[Number]
	documented bool
}

var _ Command = (*basicCommand)(nil)

// A BasicCommand is a Q command with no parameters. This function will return
// canonicalized instances, allowing for pointer comparisons.
func BasicCommand(n Number) Command {
	return canonicalBasic.get(n)
}

func basicDocumented(n Number) Command {
	cmd := canonicalBasic.get(n)
	cmd.documented = true
	return cmd
}

func (c *basicCommand) Command() (Number, bool) { return c.command.Value(), true }

// IsSafe returns true if the command is documented in official sources.
func (c *basicCommand) IsSafe() bool {
	return c.documented || c.command == hMacroVariable
}

// LogValue implements [slog.LogValuer].
func (c *basicCommand) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Any("command", c.command.Value()),
		slog.Bool("safe", c.IsSafe()),
	)
}

func (c *basicCommand) String() string { return stringify(c) }

func (c *basicCommand) WriteTo(out io.Writer) (int64, error) {
	count, err := fmt.Fprintf(out, "?Q%d\n", c.command.Value())
	return int64(count), err
}
