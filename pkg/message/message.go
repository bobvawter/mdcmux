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

// Package message provides an implementation of the Machine Data Collection
// wire protocol as described in the Haas Mill Operator's Manual.
package message

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// These basic Q command numbers are defined in the Haas Mill Operator's Manual.
var (
	QMachineSN         = Int(100)
	QControlVersion    = Int(101)
	QMachineModel      = Int(102)
	QMode              = Int(104)
	QToolChanges       = Int(200)
	QToolNumber        = Int(201)
	QPowerOnTime       = Int(300)
	QMotionTime        = Int(301)
	QLastCycleTime     = Int(303)
	QPreviousCycleTime = Int(304)
	QPartsCounter1     = Int(402)
	QPartsCounter2     = Int(403)
	QThreeInOne        = Int(500)
	QMacroVariable     = Int(600)
)

// These are predefined commands with no arguments.
var (
	CommandMachineSN         = basicDocumented(QMachineSN)
	CommandControlVersion    = basicDocumented(QControlVersion)
	CommandMachineModel      = basicDocumented(QMachineModel)
	CommandMode              = basicDocumented(QMode)
	CommandToolChanges       = basicDocumented(QToolChanges)
	CommandToolNumber        = basicDocumented(QToolNumber)
	CommandPowerOnTime       = basicDocumented(QPowerOnTime)
	CommandMotionTime        = basicDocumented(QMotionTime)
	CommandLastCycleTime     = basicDocumented(QLastCycleTime)
	CommandPreviousCycleTime = basicDocumented(QPreviousCycleTime)
	CommandPartsCounter1     = basicDocumented(QPartsCounter1)
	CommandPartsCounter2     = basicDocumented(QPartsCounter2)
	CommandThreeInOne        = basicDocumented(QThreeInOne)
)

// A Message is a Machine Data Collection message.
type Message interface {
	fmt.Stringer
	slog.LogValuer

	// WriteTo implements [io.WriterTo].
	WriteTo(out io.Writer) (int64, error)

	isMessage()
}

func stringify(msg Message) string {
	var sb strings.Builder
	_, _ = msg.WriteTo(&sb)
	return sb.String()
}

// A Command Message represents both Q and E wire messages.
type Command interface {
	Message

	// Command returns the Q command number associated with the message.
	Command() (Number, bool)

	// IsSafe returns true if the message is unlikely to cause damage to the MDC
	// receiver.
	IsSafe() bool

	// IsWrite returns true if the message writes to a remote variable.
	IsWrite() bool

	// ParseResponse interprets a result payload.
	ParseResponse(buf []byte) (Response, error)

	// Value returns a macro variable value associated with the message.
	Value() (Number, bool)

	// Variable returns a macro variable number associated with the message.
	Variable() (Number, bool)
}

// A Response to an MDC [Command] may contain either numeric or an arbitrary
// data buffer.
type Response interface {
	Message

	// Buffer returns otherwise-unparsed response data. The returned slice
	// should not be modified by callers.
	Buffer() ([]byte, bool)

	// IsSuccess returns true if the request was successful.
	IsSuccess() bool

	// Value returns a macro variable value associated with the message.
	Value() (Number, bool)
}

// ParseCommand interprets the input as a [Command].
func ParseCommand(bytes []byte) (Command, error) {
	if len(bytes) < 3 {
		return nil, errors.New("undersized message")
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
		ret := &writeCommand{}
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
		cmd, err := ParseNumber(match[queryCommandIndex])
		if err != nil {
			return nil, fmt.Errorf("invalid query: %w", err)
		}
		if cmd.frac != 0 {
			return nil, errors.New("invalid query: not expecting fractional Q command")
		}
		if cmd.whole == 600 {
			if len(match[queryVariableIndex]) == 0 {
				return nil, errors.New("a Q600 command must specify a variable")
			}
			n, err := ParseNumber(match[queryVariableIndex])
			if err != nil {
				return nil, fmt.Errorf("could not parse Q600 variable number: %w", err)
			}
			return QueryCommand(n), nil
		}
		return BasicCommand(cmd), nil

	default:
		return nil, fmt.Errorf("invalid message: invalid character '%c'", bytes[1])
	}
}
