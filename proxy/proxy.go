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

		routes map[*net.TCPListener]*listenerRoute
	}
}

type listenerRoute struct {
	mu struct {
		sync.RWMutex

		mdc      *conn.Conn
		policies []*orderedPolicy
	}
}

func (r *listenerRoute) get(client netip.Addr) (*conn.Conn, *Policy, bool) {
	r.mu.RLock()
	mdc := r.mu.mdc
	policies := r.mu.policies
	r.mu.RUnlock()

	for _, policy := range policies {
		if policy.Contains(client) {
			return mdc, policy.Policy, true
		}
	}
	return nil, nil, false
}

func New(ctx *stopper.Context, cfg *notify.Var[*Config]) (*Proxy, error) {
	p := &Proxy{cfg: cfg}
	p.mu.connByHostname = make(map[string]*conn.Conn)
	p.mu.listeners = make(map[netip.AddrPort]*net.TCPListener)
	p.mu.routes = make(map[*net.TCPListener]*listenerRoute)

	ctx.Go(func(ctx *stopper.Context) error {
		_, err := notifyx.DoWhenChanged(ctx, nil, cfg, func(ctx *stopper.Context, _, cfg *Config) error {
			slog.DebugContext(ctx, "updating configuration")
			cfg.expandPolicy()

			p.mu.Lock()
			defer p.mu.Unlock()

			nextConns := make(map[string]*conn.Conn)
			nextListeners := make(map[netip.AddrPort]*net.TCPListener)
			nextRoutes := make(map[*net.TCPListener]*listenerRoute)

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
					r = &listenerRoute{}
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
	ctx.Go(func(ctx *stopper.Context) error {
		for {
			tcpConn, err := listener.AcceptTCP()
			if err != nil {
				// Being shut down, just exit.
				return nil
			}

			client := tcpConn.RemoteAddr().(*net.TCPAddr).AddrPort()
			logger := slog.With(
				slog.Any("client", client),
				slog.Any("listener", tcpConn.LocalAddr()))

			// Allow late-binding of policies to reflect configuration file
			// changes.
			router := func() (*conn.Conn, *Policy, bool) {
				return p.policyFor(listener, client.Addr())
			}

			// Immediately drop connections that we cannot route.
			if _, _, ok := router(); !ok {
				logger.DebugContext(ctx, "no route for connection")
				_ = tcpConn.Close()
				continue
			}

			// Service the individual connection.
			ctx.Go(func(ctx *stopper.Context) error {
				if err := p.proxy(ctx, logger, tcpConn, router); err != nil {
					logger.ErrorContext(ctx, "could not proxy connection", "error", err)
				}
				return nil
			})
		}
	})
}

func (p *Proxy) proxy(ctx *stopper.Context,
	logger *slog.Logger,
	tcpConn *net.TCPConn,
	router func() (mdc *conn.Conn, policy *Policy, ok bool)) error {
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

		// Record time between client requests.
		clientLatency := time.Since(idleSince)

		// Ignore empty lines.
		if len(in.Bytes()) == 0 {
			continue
		}

		// We now have a message to parse.
		msg, err := message.Parse(in.Bytes())
		if err != nil {
			logger.DebugContext(ctx, "could not parse message",
				"error", err)
			return err
		}

		// Look up the route on each incoming message. This prevents old
		// connections from retaining stale policies.
		mdc, policy, ok := router()

		// Deconfigured.
		if !ok {
			logger.DebugContext(ctx, "no route found")
			return nil
		}

		logger := logger.With(slog.String("backend", mdc.Addr()))

		var auditData []slog.Attr
		if policy.Audit {
			auditData = append(make([]slog.Attr, 0, 16),
				slog.Bool("audit", true),
				slog.Any("request", msg),
			)
		}

		// A failed access check doesn't kill the connection.
		if !policy.Allow(msg) {
			if len(auditData) > 0 {
				auditData = append(auditData, slog.Bool("deny", true))
				logger.LogAttrs(ctx, slog.LevelInfo, "deny", auditData...)
			}
			if err := writeError("MDCMUX DENY POLICY"); err != nil {
				return err
			}
			continue
		}

		// Proxy the message across.
		writeStart := time.Now()
		resp, err := mdc.Write(ctx, msg)
		if err != nil {
			_ = writeError("MDCMUX PROXY ERROR")
			return err
		}
		flushStart := time.Now()
		if _, err := resp.WriteTo(out); err != nil {
			return err
		}
		if _, err := out.WriteString("\n"); err != nil {
			return err
		}
		if err := out.Flush(); err != nil {
			return err
		}
		flushEnd := time.Now()

		if len(auditData) > 0 {
			auditData = append(auditData,
				slog.Group("latency",
					slog.Duration("backend", flushStart.Sub(writeStart)),
					slog.Duration("client", clientLatency),
					slog.Duration("flush", flushEnd.Sub(flushStart)),
				),
				slog.Any("response", resp),
			)
			logger.LogAttrs(ctx, slog.LevelInfo, "proxy", auditData...)
		}

		idleSince = time.Now()
	}
}

func (p *Proxy) policyFor(l *net.TCPListener, client netip.Addr) (
	backend *conn.Conn, policy *Policy, ok bool,
) {
	p.mu.RLock()
	route := p.mu.routes[l]
	p.mu.RUnlock()

	if route == nil {
		return nil, nil, false
	}
	return route.get(client)
}
