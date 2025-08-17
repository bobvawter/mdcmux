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
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"vawter.tech/mdcmux/message"
	"vawter.tech/stopper"
)

// Canned messages to send for basic commands.
var Canned = map[message.Number]string{
	message.CommandMachineSN:         ">>SERIAL NUMBER, 1024",
	message.CommandControlVersion:    ">>SOFTWARE VERSION, 100.24.000.1024",
	message.CommandMachineModel:      ">>MODEL, MDCMUX",
	message.CommandMode:              ">>MODE, STARTUP_MODE",
	message.CommandToolChanges:       ">>TOOL CHANGES, 1024",
	message.CommandToolNumber:        ">>USING TOOL, 16",
	message.CommandPowerOnTime:       ">>P.O. TIME, 00012:34:56",
	message.CommandMotionTime:        ">>C.S. TIME, 00012:34:56",
	message.CommandLastCycleTime:     ">>LAST CYCLE, 00012:34:56",
	message.CommandPreviousCycleTime: ">>PREV CYCLE, 00012:34:56",
	message.CommandPartsCounter1:     ">>M30 #1, 22",
	message.CommandPartsCounter2:     ">>M30 #2, 33",
	message.CommandThreeInOne:        ">>PROGRAM, MDI, ALARM ON, PARTS, 3205",
}

// Server implements a dummy MDC host for testing purposes.
type Server struct {
	listener net.Listener

	mu struct {
		sync.Mutex
		data map[message.Number]message.Number
	}
}

// New runs a dummy MDC server within the context.
func New(ctx *stopper.Context, bind string) (*Server, error) {
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "dummy server listening", slog.Any("address", listener.Addr()))
	ctx.Go(func(ctx *stopper.Context) error {
		<-ctx.Stopping()
		_ = listener.Close()
		slog.InfoContext(ctx, "dummy server listener closed")
		return nil
	})

	s := &Server{
		listener: listener,
	}
	s.mu.data = make(map[message.Number]message.Number)

	ctx.Go(func(ctx *stopper.Context) error {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				return nil
			}
			ctx.Go(func(ctx *stopper.Context) error {
				err := s.run(ctx, conn)
				if err != nil {
					slog.ErrorContext(ctx, "handler exiting", slog.Any("error", err))
				}
				return nil
			})
		}
	})
	return s, nil
}

// Addr returns the address to which the server is bound.
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

func (s *Server) handle(_ *stopper.Context, msg message.Message, out *bufio.Writer) error {
	cmd, _ := msg.Command()
	var data string

	if msg.IsWrite() {
		num, _ := msg.Variable()
		val, _ := msg.Value()

		s.mu.Lock()
		s.mu.data[num] = val
		s.mu.Unlock()

		data = ">>!"
	} else if found, ok := Canned[cmd]; ok {
		data = found
	} else if cmd == message.CommandMacroVariable {
		num, _ := msg.Variable()

		if num.Whole() < 0 || num.Frac() != 0 {
			data = ">>?, BAD VARIABLE NUMBER"
		} else {
			s.mu.Lock()
			val := s.mu.data[num]
			s.mu.Unlock()

			data = fmt.Sprintf(">>MACRO, %f", val)
		}

	} else {
		data = fmt.Sprintf(">>?, ?Q%d", cmd)
	}

	if _, err := out.WriteString(data); err != nil {
		return err
	}
	_, err := out.WriteString("\r\n")
	return err
}

func (s *Server) run(ctx *stopper.Context, conn net.Conn) error {
	scanner := bufio.NewScanner(conn)
	out := bufio.NewWriter(conn)
	for scanner.Scan() {
		buf := scanner.Bytes()
		msg, err := message.Parse(buf)
		if err != nil {
			slog.DebugContext(ctx, "inbound parse error",
				slog.String("message", string(buf)),
				slog.Any("error", err))
			if _, err := out.WriteString("? BAD MESSAGE\r\n"); err != nil {
				return err
			}
			continue
		}
		if err := s.handle(ctx, msg, out); err != nil {
			return err
		}
		if err := out.Flush(); err != nil {
			return err
		}
	}
	err := scanner.Err()
	if err == io.EOF {
		return nil
	}
	return err
}
