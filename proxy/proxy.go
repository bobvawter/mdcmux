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

package proxy

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"

	"vawter.tech/mdcmux/conn"
	"vawter.tech/mdcmux/message"
	"vawter.tech/notify"
	"vawter.tech/notify/notifyx"
	"vawter.tech/stopper"
)

type Proxy struct {
	cfg          *notify.Var[*Config]
	reconfigured notify.Var[struct{}] // For testing.

	mu struct {
		sync.RWMutex

		// Network connections to the MDC servers are conserved across
		// reconfiguration.
		connByHostname map[string]*conn.Conn

		// Network listeners are conserved.
		listeners map[netip.AddrPort]*net.TCPListener

		routes map[*net.TCPListener]*Route
	}
}

type Route struct {
	mu struct {
		sync.RWMutex

		mdc      *conn.Conn
		policies []*orderedPolicy
	}
}

func New(ctx *stopper.Context, cfg *notify.Var[*Config]) (*Proxy, error) {
	p := &Proxy{cfg: cfg}
	p.mu.connByHostname = make(map[string]*conn.Conn)
	p.mu.listeners = make(map[netip.AddrPort]*net.TCPListener)
	p.mu.routes = make(map[*net.TCPListener]*Route)

	ctx.Go(func(ctx *stopper.Context) error {
		_, err := notifyx.DoWhenChanged(ctx, nil, cfg, func(ctx *stopper.Context, _, cfg *Config) error {
			slog.DebugContext(ctx, "updating configuration")
			cfg.expandPolicy()

			p.mu.Lock()
			defer p.mu.Unlock()

			nextConns := make(map[string]*conn.Conn)
			nextListeners := make(map[netip.AddrPort]*net.TCPListener)
			nextRoutes := make(map[*net.TCPListener]*Route)

			for hostname, target := range cfg.Targets {
				// Find connection from previous generation.
				c := p.mu.connByHostname[hostname]
				if c == nil {
					c = conn.NewConn(hostname)
				}
				nextConns[hostname] = c

				// Find existing listener, or create one.
				addrPort := netip.AddrPortFrom(cfg.Bind, target.ProxyPort)
				l := p.mu.listeners[addrPort]
				if l == nil {
					var err error

					l, err = net.ListenTCP("tcp", net.TCPAddrFromAddrPort(addrPort))
					if err != nil {
						slog.ErrorContext(ctx, "could not create listener, not reconfiguring",
							slog.String("hostname", hostname),
							slog.String("addrPort", addrPort.String()),
							slog.Any("error", err))
						return nil
					}
					slog.DebugContext(ctx, "proxy listening",
						slog.String("target", hostname),
						slog.Any("proxy", l.Addr()))
					p.accept(ctx, l)
				}
				nextListeners[addrPort] = l

				r := p.mu.routes[l]
				if r == nil {
					r = &Route{}
				}
				nextRoutes[l] = r

				r.mu.Lock()
				r.mu.mdc = c
				r.mu.policies = target.ordered
				r.mu.Unlock()

			}

			// Close unreferenced listeners.
			for listenAddr, oldListener := range p.mu.listeners {
				if nextListeners[listenAddr] == nil {
					_ = oldListener.Close()
					slog.DebugContext(ctx, "closing listener due to reconfiguration", "address", listenAddr)
				}
			}

			p.mu.connByHostname = nextConns
			p.mu.listeners = nextListeners
			p.mu.routes = nextRoutes

			p.reconfigured.Notify()
			return nil
		})

		// Context is stopping, close all listeners.
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, listener := range p.mu.listeners {
			_ = listener.Close()
		}

		return err
	})

	return p, nil
}

func (p *Proxy) accept(ctx *stopper.Context, listener *net.TCPListener) {
	logger := slog.With(slog.String("listener", listener.Addr().String()))

	ctx.Go(func(ctx *stopper.Context) error {
		for {
			tcpConn, err := listener.AcceptTCP()
			if err != nil {
				// Being shut down, just exit.
				logger.DebugContext(ctx, "no longer accepting connection")
				return nil
			}

			// Service the individual connection.
			ctx.Go(func(ctx *stopper.Context) error {
				if err := p.proxy(ctx, listener, tcpConn); err != nil {
					slog.ErrorContext(ctx, "could not proxy connection", "error", err)
				}
				return nil
			})
		}
	})
}

func (p *Proxy) proxy(ctx *stopper.Context, listener *net.TCPListener, tcpConn *net.TCPConn) error {
	defer func() { _ = tcpConn.Close() }()

	in := bufio.NewScanner(tcpConn)
	out := bufio.NewWriter(tcpConn)
	defer func() { _ = out.Flush() }()

	writeError := func(msg string) error {
		if _, err := fmt.Fprintf(out, ">>?, %s\n", msg); err != nil {
			return err
		}
		return out.Flush()
	}

	remote := tcpConn.RemoteAddr().(*net.TCPAddr).AddrPort()
	logger := slog.With(slog.Any("client", remote))

	// Updated at the bottom of the loop.
	idleSince := time.Now()
	for {
		if ctx.IsStopping() {
			return nil
		}

		// Impose maximum connection idle time behavior.
		cfg, _ := p.cfg.Get()
		if time.Since(idleSince) >= cfg.MaxIdle {
			logger.DebugContext(ctx, "dropping idle connection")
			return nil
		}

		// Set read deadlines to allow clean shutdown.
		_ = tcpConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		didScan := in.Scan()

		if err := in.Err(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if netErr := (net.Error)(nil); errors.As(err, &netErr) && netErr.Timeout() {
				in = bufio.NewScanner(tcpConn)
				continue
			}
			return err
		}

		if !didScan {
			continue
		}

		buf := in.Bytes()

		// Ignore empty lines.
		if len(buf) == 0 {
			continue
		}

		// We now have a message to parse.
		msg, err := message.Parse(buf)
		if err != nil {
			logger.DebugContext(ctx, "could not parse message",
				"error", err)
			return err
		}

		// Look up the route on each incoming message. This prevents old
		// connections from retaining stale policies.
		p.mu.RLock()
		route := p.mu.routes[listener]
		p.mu.RUnlock()

		// Deconfigured.
		if route == nil {
			logger.DebugContext(ctx, "no route found")
			return nil
		}

		// Extract routing info to local stack.
		route.mu.RLock()
		logger := logger.With(slog.String("server", route.mu.mdc.Addr()))
		mdc := route.mu.mdc
		policies := route.mu.policies
		route.mu.RUnlock()

		// First match on policy wins.
		var policy *orderedPolicy
		for _, p := range policies {
			if p.Contains(remote.Addr()) {
				policy = p
				break
			}
		}

		// If there's no matching policy for the remote IP, we want to hang up.
		if policy == nil {
			if err := writeError("MDCMUX NO POLICY MATCH"); err != nil {
				return err
			}
			return nil
		}

		// A failed access check doesn't kill the connection.
		if !policy.Allow(msg) {
			if err := writeError("MDCMUX DENY POLICY"); err != nil {
				return err
			}
			continue
		}

		// Proxy the message across.
		resp, err := mdc.Write(ctx, msg)
		if err != nil {
			_ = writeError("MDCMUX PROXY ERROR")
			return err
		}
		if _, err := resp.WriteTo(out); err != nil {
			return err
		}
		if _, err := out.WriteString("\n"); err != nil {
			return err
		}
		if err := out.Flush(); err != nil {
			return err
		}
		logger.DebugContext(ctx, "proxy response",
			slog.Any("request", msg),
			slog.Any("response", resp))

		idleSince = time.Now()
	}
}
