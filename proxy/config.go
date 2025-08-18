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
	"bytes"
	"log/slog"
	"net/netip"
	"slices"
	"time"

	"vawter.tech/mdcmux/message"
)

const defaultMaxIdle = 5 * time.Minute

type Config struct {
	Bind    netip.Addr               `json:"bind"`
	MaxIdle time.Duration            `json:"max_idle"`
	Policy  map[netip.Prefix]*Policy `json:"policy"`
	Targets map[string]*Target       `json:"targets"`
}

func (c *Config) expandPolicy() {
	if c.MaxIdle == 0 {
		c.MaxIdle = defaultMaxIdle
	}
	for dest, tgt := range c.Targets {
		ordered := make([]*orderedPolicy, 0, len(c.Policy)+len(tgt.Policy))

		// Copy base policies into target map.
		for k, v := range c.Policy {
			if !k.IsValid() {
				continue
			}
			ordered = append(ordered, &orderedPolicy{
				Prefix: k,
				Policy: v,
			})
		}

		// Per-target policies have higher priority.
		for k, v := range tgt.Policy {
			if !k.IsValid() {
				continue
			}
			ordered = append(ordered, &orderedPolicy{
				Prefix:   k,
				Priority: 1,
				Policy:   v,
			})
		}

		if len(ordered) == 0 {
			slog.Warn("using default localhost policy", slog.Any("hostname", dest))
			policy := &Policy{}
			tgt.ordered = []*orderedPolicy{
				{
					Prefix: netip.PrefixFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 32),
					Policy: policy,
				},
				{
					Prefix: netip.PrefixFrom(netip.IPv6Loopback(), 128),
					Policy: policy,
				},
			}
		} else {
			slices.SortFunc(ordered, func(a, b *orderedPolicy) int {
				// Sort by priority.
				if c := a.Priority - b.Priority; c != 0 {
					return c
				}
				// Then by number of bits in the mask.
				if c := a.Prefix.Bits() - b.Prefix.Bits(); c != 0 {
					return c
				}
				// Then by starting IP address.
				return bytes.Compare(a.Prefix.Addr().AsSlice(), b.Prefix.Addr().AsSlice())
			})

			tgt.ordered = ordered
		}

	}
}

type Policy struct {
	// AllowUndocumentedQ allows Q commands that are not present in the Haas
	// Mill User's Guide to be proxied.
	AllowUndocumentedQ bool `json:"allow_undocumented_q"`

	// AllowWrites contains inclusive pairs of macro variable numbers that may
	// be written to.
	AllowWrites [][2]int `json:"allow_writes"`

	// Audit triggers additional logging for each message.
	Audit bool `json:"audit"`
}

// Allow returns true if the message is permitted by the policy.
func (p *Policy) Allow(msg message.Message) bool {
	if msg.IsSafe() {
		return true
	}
	if msg.IsWrite() {
		v, _ := msg.Variable()
		return p.AllowWrite(int(v.Whole()))
	}
	if _, ok := msg.Command(); ok && p.AllowUndocumentedQ {
		return true
	}
	return false
}

// AllowWrite returns true if writes to the given variable number are permitted
// by the policy.
func (p *Policy) AllowWrite(v int) bool {
	for _, pair := range p.AllowWrites {
		if pair[0] <= v && v <= pair[1] {
			return true
		}
	}
	return false
}

type Target struct {
	Policy    map[netip.Prefix]*Policy `json:"policy"`
	ProxyPort uint16                   `json:"proxy_port"`

	ordered []*orderedPolicy
}

// PolicyFor returns the access policy for the given source address, if
// configured.
func (t *Target) PolicyFor(source netip.Addr) (*Policy, bool) {
	for _, policy := range t.ordered {
		if policy.Contains(source) {
			return policy.Policy, true
		}
	}
	return nil, false
}

type orderedPolicy struct {
	*Policy
	netip.Prefix
	Priority int
}
