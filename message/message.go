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
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var (
	numberFormat = regexp.MustCompile(`^\s*(?P<whole>[+-]?\d+)(?:\.(?P<frac>\d*))?\s*$`)
	numberWhole  = numberFormat.SubexpIndex("whole")
	numberFrac   = numberFormat.SubexpIndex("frac")
)

// These basic commands are defined in Haas Mill Operator's Manual.
var (
	CommandMachineSN         = Int64(100)
	CommandControlVersion    = Int64(101)
	CommandMachineModel      = Int64(102)
	CommandMode              = Int64(104)
	CommandToolChanges       = Int64(200)
	CommandToolNumber        = Int64(201)
	CommandPowerOnTime       = Int64(300)
	CommandMotionTime        = Int64(301)
	CommandLastCycleTime     = Int64(303)
	CommandPreviousCycleTime = Int64(304)
	CommandPartsCounter1     = Int64(402)
	CommandPartsCounter2     = Int64(403)
	CommandThreeInOne        = Int64(500)
	CommandMacroVariable     = Int64(600)
)

var documentedCommands = map[Number]struct{}{
	CommandMachineSN:         {},
	CommandControlVersion:    {},
	CommandMachineModel:      {},
	CommandMode:              {},
	CommandToolChanges:       {},
	CommandToolNumber:        {},
	CommandPowerOnTime:       {},
	CommandMotionTime:        {},
	CommandLastCycleTime:     {},
	CommandPreviousCycleTime: {},
	CommandPartsCounter1:     {},
	CommandPartsCounter2:     {},
	CommandThreeInOne:        {},
	CommandMacroVariable:     {},
}

// A Number represents a fixed-point decimal value.
type Number struct {
	whole, frac int64
}

// NewNumber constructs a number with a whole and fractional part.
func NewNumber(whole, frac int64) Number {
	return Number{whole, frac}
}

// Int64 returns the integer as a parsed Number.
func Int64(i int64) Number {
	return Number{whole: i}
}

// ParseNumber validates the input is numeric.
func ParseNumber(buf []byte) (Number, error) {
	if len(buf) == 0 {
		return Number{}, errors.New("empty number")
	}

	match := numberFormat.FindSubmatch(buf)
	if len(match) == 0 {
		return Number{}, errors.New("invalid number format")
	}
	whole, err := strconv.ParseInt(string(match[numberWhole]), 10, 64)
	if err != nil {
		return Number{}, err
	}
	var frac int64
	if len(match[numberFrac]) > 0 {
		frac, err = strconv.ParseInt(string(match[numberFrac]), 10, 64)
		if err != nil {
			return Number{}, err
		}
	}

	return Number{whole, frac}, nil
}

// Frac returns the fractional portion of the Number.
func (n Number) Frac() int64 {
	return n.frac
}

// Whole returns the whole portion of the Number.
func (n Number) Whole() int64 {
	return n.whole
}

// Format implements [io.Formatter].
func (n Number) Format(state fmt.State, verb rune) {
	switch verb {
	case 'd': // Decimal
		_, _ = fmt.Fprintf(state, "%d", n.whole)
	case 'f', 's', 'v': // Float
		if prec, ok := state.Precision(); ok && prec == 0 {
			_, _ = fmt.Fprintf(state, "%d", n.whole)
		} else {
			_, _ = fmt.Fprintf(state, "%d.%d", n.whole, n.frac)
		}
	default:
		panic("unsupported verb")
	}
}

// String is for debugging use only.
func (n Number) String() string {
	return fmt.Sprintf("%s", n)
}

// A Message is a Machine Data Collection message.
type Message interface {
	Command() (Number, bool)
	Variable() (Number, bool)
	Value() (Number, bool)

	// IsSafe returns true if the message is unlikely to cause damage to the MDC
	// receiver.
	IsSafe() bool

	// IsWrite returns true if the message writes to a remote variable.
	IsWrite() bool

	// WriteTo implements [io.WriterTo].
	WriteTo(out io.Writer) (int64, error)

	isMessage()
}

type messageBase struct{}

func (m *messageBase) Command() (Number, bool)  { return Number{}, false }
func (m *messageBase) IsWrite() bool            { return false }
func (m *messageBase) IsSafe() bool             { return false }
func (m *messageBase) Variable() (Number, bool) { return Number{}, false }
func (m *messageBase) Value() (Number, bool)    { return Number{}, false }
func (m *messageBase) isMessage()               {}

var (
	queryPattern  = regexp.MustCompile(`^(?P<command>\d+)(?:\s+(?P<variable>\d+(?:\.\d*)?)?)?\s*$`)
	queryCommand  = queryPattern.SubexpIndex("command")
	queryVariable = queryPattern.SubexpIndex("variable")

	writePattern  = regexp.MustCompile(`^(?P<variable>\d+)\s+(?P<value>[+-]?\d+(?:\.\d*)?)\s*$`)
	writeVariable = writePattern.SubexpIndex("variable")
	writeValue    = writePattern.SubexpIndex("value")
)

// Parse the input as a [Message].
func Parse(bytes []byte) (Message, error) {
	if len(bytes) == 0 {
		return nil, errors.New("empty message")
	}
	if bytes[0] != '?' {
		return nil, errors.New("invalid message: no leading '?'")
	}
	switch bytes[1] {
	case 'E':
		match := writePattern.FindSubmatch(bytes[2:])
		if len(match) == 0 {
			return nil, errors.New("invalid query: expecting a variable number and a numeric argument")
		}
		if len(match[writeVariable]) == 0 {
			return nil, errors.New("invalid query: no variable")
		}
		if len(match[writeValue]) == 0 {
			return nil, errors.New("invalid query: no value")
		}
		var err error
		ret := &write{}
		ret.variable, err = ParseNumber(match[writeVariable])
		if err != nil {
			return nil, fmt.Errorf("invalid query: bad variable number: %w", err)
		}
		ret.value, err = ParseNumber(match[writeValue])
		if err != nil {
			return nil, fmt.Errorf("invalid query: bad value number: %w", err)
		}

		return ret, nil

	case 'Q':
		match := queryPattern.FindSubmatch(bytes[2:])
		if len(match) == 0 {
			return nil, errors.New("invalid query: expecting a whole number and optional numeric value")
		}
		cmd, err := ParseNumber(match[queryCommand])
		if err != nil {
			return nil, fmt.Errorf("invalid query: %w", err)
		}
		if cmd.frac != 0 {
			return nil, errors.New("invalid query: not expecting fractional Q command")
		}
		if cmd.whole == 600 {
			if len(match[queryVariable]) == 0 {
				return nil, errors.New("a Q600 command must specify a variable")
			}
			n, err := ParseNumber(match[queryVariable])
			if err != nil {
				return nil, fmt.Errorf("could not parse Q600 variable number: %w", err)
			}
			return Query(n), nil
		}
		return &basic{command: cmd}, nil

	default:
		return nil, fmt.Errorf("invalid message: invalid character '%c'", bytes[1])
	}
}

// A basic query with no parameters.
type basic struct {
	messageBase
	command Number
}

var _ Message = (*basic)(nil)

// A Basic query message with no parameters.
func Basic(n Number) Message {
	return &basic{command: n}
}

func (c *basic) Command() (Number, bool) { return c.command, true }

// IsSafe returns true if the command is documented in official sources.
func (c *basic) IsSafe() bool {
	_, ok := documentedCommands[c.command]
	return ok
}

func (c *basic) String() string {
	var sb strings.Builder
	_, _ = c.WriteTo(&sb)
	return sb.String()
}

func (c *basic) WriteTo(out io.Writer) (int64, error) {
	count, err := fmt.Fprintf(out, "?Q%d\n", c.command)
	return int64(count), err
}

type query struct {
	messageBase
	variable Number
}

var _ Message = (*query)(nil)

// Query a macro variable number.
func Query(n Number) Message {
	return &query{variable: n}
}

func (q *query) Command() (Number, bool)  { return CommandMacroVariable, true }
func (q *query) IsSafe() bool             { return q.variable.whole >= 1 && q.variable.frac == 0 }
func (q *query) Variable() (Number, bool) { return q.variable, true }

func (q *query) String() string {
	var sb strings.Builder
	_, _ = q.WriteTo(&sb)
	return sb.String()
}

func (q *query) WriteTo(out io.Writer) (int64, error) {
	count, err := fmt.Fprintf(out, "?Q600 %f\n", q.variable)
	return int64(count), err
}

type response struct {
	messageBase

	buf []byte
}

var _ Message = (*response)(nil)

// A Response to an MDC command.
func Response(data []byte) Message {
	ret := response{buf: bytes.Clone(data)}
	return &ret
}

func (r *response) String() string {
	return string(r.buf)
}

func (r *response) WriteTo(out io.Writer) (int64, error) {
	count, err := out.Write(r.buf)
	return int64(count), err
}

type write struct {
	messageBase
	variable Number
	value    Number
}

var _ Message = (*write)(nil)

// Write to a macro variable.
func Write(target, value Number) Message {
	return &write{variable: target, value: value}
}

func (w *write) IsWrite() bool            { return true }
func (w *write) Variable() (Number, bool) { return w.variable, true }
func (w *write) Value() (Number, bool)    { return w.value, true }

func (w *write) String() string {
	var sb strings.Builder
	_, _ = w.WriteTo(&sb)
	return sb.String()
}

func (w *write) WriteTo(out io.Writer) (int64, error) {
	count, err := fmt.Fprintf(out, "?E%d %f\n", w.variable, w.value)
	return int64(count), err
}

func (*write) isMessage() {}
