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

// Package dummy contains a trivial implementation of an MDC host. It supports
// various canned messages and access to macro variables.
package dummy

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"vawter.tech/mdcmux/pkg/message"
	"vawter.tech/stopper"
)

// Canned messages to send for basic commands.
var Canned = map[message.Command]string{
	message.CommandMachineSN:         "SERIAL NUMBER, 1024",
	message.CommandControlVersion:    "SOFTWARE VERSION, 100.24.000.1024",
	message.CommandMachineModel:      "MODEL, MDCMUX",
	message.CommandMode:              "MODE, STARTUP_MODE",
	message.CommandToolChanges:       "TOOL CHANGES, 1024",
	message.CommandToolNumber:        "USING TOOL, 16",
	message.CommandPowerOnTime:       "P.O. TIME, 00012:34:56",
	message.CommandMotionTime:        "C.S. TIME, 00012:34:56",
	message.CommandLastCycleTime:     "LAST CYCLE, 00012:34:56",
	message.CommandPreviousCycleTime: "PREV CYCLE, 00012:34:56",
	message.CommandPartsCounter1:     "M30 #1, 22",
	message.CommandPartsCounter2:     "M30 #2, 33",
	message.CommandThreeInOne:        "PROGRAM, MDI, ALARM ON, PARTS, 3205",
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

	openConns := make(map[net.Conn]struct{})
	var openConnsMu sync.Mutex

	// Unblock reads when the server gets shut down.
	ctx.Go(func(ctx *stopper.Context) error {
		<-ctx.Stopping()
		now := time.UnixMilli(1)
		openConnsMu.Lock()
		for conn := range openConns {
			_ = conn.SetReadDeadline(now)
		}
		openConnsMu.Unlock()
		return nil
	})

	// This is the main accept loop for the server.
	ctx.Go(func(ctx *stopper.Context) error {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				return nil
			}

			openConnsMu.Lock()
			openConns[conn] = struct{}{}
			openConnsMu.Unlock()

			if !ctx.Go(func(ctx *stopper.Context) error {
				defer func() {
					openConnsMu.Lock()
					delete(openConns, conn)
					openConnsMu.Unlock()
					_ = conn.Close()
				}()
				if err := s.run(ctx, conn); err != nil && !ctx.IsStopping() {
					slog.ErrorContext(ctx, "handler exiting", slog.Any("error", err))
				}
				return nil
			}) {
				slog.DebugContext(ctx, "dropping unaccepted connection")
				_ = conn.Close()
			}
		}
	})
	return s, nil
}

// Addr returns the address to which the server is bound.
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

// Peek gets the current value of a macro variable, if set.
func (s *Server) Peek(k message.Number) (message.Number, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.mu.data[k]
	return n, ok
}

// Poke sets a macro variable.
func (s *Server) Poke(k, v message.Number) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mu.data[k] = v
}

func (s *Server) handle(_ *stopper.Context, msg message.Command, out *bufio.Writer) error {
	if msg.IsWrite() {
		num, _ := msg.Variable()
		val, _ := msg.Value()

		s.mu.Lock()
		s.mu.data[num] = val
		s.mu.Unlock()

		return message.WriteResponse(out, "!")
	}

	if found, ok := Canned[msg]; ok {
		return message.WriteResponse(out, "%s", found)
	}

	cmd, _ := msg.Command()
	if cmd == message.QMacroVariable {
		num, _ := msg.Variable()

		if num.Whole() < 0 || num.Frac() != 0 {
			return message.WriteResponse(out, "?, BAD VARIABLE NUMBER")
		}

		// Macro variable 0 is always NaN
		if num.Whole() == 0 {
			return message.WriteResponse(out, "MACRO, NaN")
		}

		s.mu.Lock()
		val := s.mu.data[num]
		s.mu.Unlock()
		return message.WriteResponse(out, "MACRO, %f", val)
	}

	return message.WriteResponse(out, "?, ?Q%d", cmd)
}

func (s *Server) run(ctx *stopper.Context, c net.Conn) error {
	scanner := bufio.NewScanner(c)
	out := bufio.NewWriter(c)

	if err := message.WritePrompt(out); err != nil {
		return err
	}
	for scanner.Scan() {
		buf := bytes.TrimSpace(scanner.Bytes())

		// Empty lines are ignored.
		if len(buf) == 0 {
			continue
		}

		cmd, err := message.ParseCommand(buf)
		if err != nil {
			slog.DebugContext(ctx, "inbound parse error",
				slog.String("message", string(buf)),
				slog.Any("error", err))
			if err := message.WriteResponse(out, "?, BAD MESSAGE"); err != nil {
				return err
			}
		} else if err := s.handle(ctx, cmd, out); err != nil {
			return err
		}
	}
	err := scanner.Err()
	if err == io.EOF {
		return nil
	}
	return err
}
