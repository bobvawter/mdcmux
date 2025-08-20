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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecoration(t *testing.T) {
	tcs := []struct {
		M        Message
		Command  bool
		Safe     bool
		Variable bool
		Value    bool
		Write    bool
	}{
		{M: Basic(CommandMachineSN), Command: true, Safe: true},
		{M: Basic(Int64(999)), Command: true}, // Not safe because it's not documented.

		{M: Response([]byte(">!"))},

		{M: Query(Int64(999)), Command: true, Safe: true, Variable: true},
		{M: Query(NewNumber(999, 999)), Command: true, Variable: true}, // Not safe because of fractional number.

		{M: Write(Int64(99), Int64(101)), Variable: true, Value: true, Write: true},
	}

	for idx, tc := range tcs {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			a := assert.New(t)
			var ok bool

			_, ok = tc.M.Command()
			a.Equal(tc.Command, ok)

			a.Equal(tc.Safe, tc.M.IsSafe())

			_, ok = tc.M.Value()
			a.Equal(tc.Value, ok)

			_, ok = tc.M.Variable()
			a.Equal(tc.Variable, ok)

			a.Equal(tc.Write, tc.M.IsWrite())
		})
	}
}

func TestFormatNumber(t *testing.T) {
	tcs := []struct {
		S string
		F string
		E string
	}{
		{S: "1.2", F: "%d", E: "1"},
		{S: "-1.2", F: "%d", E: "-1"},

		{S: "1.0", F: "%f", E: "1.0"},

		{S: "1.0", F: "%.0f", E: "1"},
		{S: "1.0", F: "%.0s", E: "1"},
		{S: "1.0", F: "%.0v", E: "1"},
	}

	for _, tc := range tcs {
		t.Run(tc.S+" "+tc.F, func(t *testing.T) {
			r := require.New(t)

			parsed, err := ParseNumber([]byte(tc.S))
			r.NoError(err)
			r.Equal(tc.E, fmt.Sprintf(tc.F, parsed))
		})
	}
}

func TestParseMessage(t *testing.T) {
	tcs := []struct {
		S   string
		M   Message
		Err string
		C   string // Canonical representation
	}{
		{S: "", Err: "undersized message"},
		{S: "Q100", Err: "no leading '?'"},
		{S: "?U1", Err: "invalid character"},

		{S: "?Q", Err: "undersized message"},
		{S: "?Q100", M: Basic(Int64(100))},
		{S: "?Q100  ", M: Basic(Int64(100)), C: "?Q100"},
		{S: "?Q100.1", Err: "expecting"},
		{S: "?Q600 ", Err: "must specify a variable"},
		{S: "?Q600 XYZ", Err: "expecting"},
		{S: "?Q600 1234", M: Query(Int64(1234)), C: "?Q600 1234.0"},
		{S: "?Q600 1234 ", M: Query(Int64(1234)), C: "?Q600 1234.0"},
		{S: "?Q600 1234.", M: Query(Int64(1234)), C: "?Q600 1234.0"},
		{S: "?Q600 1234.567", M: Query(NewNumber(1234, 567))},

		{S: "?E", Err: "undersized message"},
		{S: "?E1", Err: "expecting a variable number"},
		{S: "?E1X", Err: "expecting a variable number"},
		{S: "?E1 Y", Err: "expecting a variable number"},

		{S: "?E12 567", M: Write(Int64(12), Int64(567)), C: "?E12 567.0"},
		{S: "?E12 -567", M: Write(Int64(12), Int64(-567)), C: "?E12 -567.0"},
		{S: "?E12 +567", M: Write(Int64(12), Int64(567)), C: "?E12 567.0"},
		{S: "?E12 567.", M: Write(Int64(12), Int64(567)), C: "?E12 567.0"},
		{S: "?E12.34 567.8", Err: "expecting a variable number"},
	}

	for _, tc := range tcs {
		t.Run(tc.S, func(t *testing.T) {
			r := require.New(t)

			parsed, err := Parse([]byte(tc.S))
			if tc.Err != "" {
				r.ErrorContains(err, tc.Err)
				return
			}
			r.NoError(err)
			r.Equal(tc.M, parsed)

			s := fmt.Sprint(parsed)
			reparsed, err := Parse([]byte(s))
			r.NoError(err, s)
			r.Equal(parsed, reparsed)

			c := tc.C
			if c == "" {
				c = tc.S
			}
			c += "\n"
			r.Equal(c, fmt.Sprint(parsed))
		})
	}
}

func TestParseNumber(t *testing.T) {
	tcs := []struct {
		S   string
		N   Number
		Err string
	}{
		{S: "", Err: "empty number"},

		{S: "0", N: Int64(0)},
		{S: " 0 ", N: Int64(0)},
		{S: "1", N: Int64(1)},
		{S: "+1", N: Int64(1)},
		{S: "-1", N: Int64(-1)},

		{S: "0.1", N: NewNumber(0, 1)},
		{S: "1.1", N: NewNumber(1, 1)},
		{S: "-1.1", N: NewNumber(-1, 1)},

		{S: "-1.-1", Err: "invalid number"},
	}

	for _, tc := range tcs {
		t.Run(tc.S, func(t *testing.T) {
			r := require.New(t)

			parsed, err := ParseNumber([]byte(tc.S))
			if tc.Err != "" {
				r.ErrorContains(err, tc.Err)
				return
			}
			r.NoError(err)
			r.Equal(tc.N, parsed)
		})
	}
}
