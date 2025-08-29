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
	"log/slog"
	"math"
	"regexp"
	"strconv"
)

var (
	numberFormat = regexp.MustCompile(`^\s*(?P<whole>[+-]?\d+)(?:\.(?P<frac>\d*))?\s*$`)
	numberWhole  = numberFormat.SubexpIndex("whole")
	numberFrac   = numberFormat.SubexpIndex("frac")
)

// A Number represents a fixed-point decimal value.
type Number struct {
	whole, frac int64
	nan         bool
}

// NaN is not a [Number].
var NaN = Number{nan: true}

// NewNumber constructs a number with a whole and fractional part.
func NewNumber(whole, frac int64) Number {
	if frac < 0 {
		panic("frac must be non-negative")
	}
	return Number{whole: whole, frac: frac}
}

// Int returns the integer as a parsed Number.
func Int(i int) Number {
	return Number{whole: int64(i)}
}

// Int64 returns the integer as a parsed Number.
func Int64(i int64) Number {
	return Number{whole: i}
}

// ParseNumber validates the input is numeric.
func ParseNumber(buf []byte) (Number, error) {
	buf = bytes.TrimSpace(buf)

	if len(buf) == 0 {
		return NaN, errors.New("empty number")
	}

	if bytes.Equal(buf, []byte("NaN")) {
		return NaN, nil
	}

	match := numberFormat.FindSubmatch(buf)
	if len(match) == 0 {
		return NaN, errors.New("invalid number format")
	}
	whole, err := strconv.ParseInt(string(match[numberWhole]), 10, 64)
	if err != nil {
		return NaN, err
	}
	var frac int64
	if len(match[numberFrac]) > 0 {
		frac, err = strconv.ParseInt(string(match[numberFrac]), 10, 64)
		if err != nil {
			return NaN, err
		}
	}

	return NewNumber(whole, frac), nil
}

// Frac returns the fractional portion of the Number.
func (n Number) Frac() int64 {
	return n.frac
}

// IsNaN returns true if the value is not a number.
func (n Number) IsNaN() bool { return n.nan }

// LogValue implements [slog.LogValuer].
func (n Number) LogValue() slog.Value {
	if n.IsNaN() {
		return slog.StringValue("NaN")
	}
	if n.frac == 0 {
		return slog.Int64Value(n.whole)
	}
	w := float64(n.whole)
	f := float64(n.frac)
	v := math.FMA(f, math.Pow(10, -math.Ceil(math.Log10(f))), w)
	return slog.Float64Value(v)
}

// Whole returns the whole portion of the Number.
func (n Number) Whole() int64 {
	return n.whole
}

// Format implements [io.Formatter].
func (n Number) Format(state fmt.State, verb rune) {
	var err error
	if n.IsNaN() {
		_, err = state.Write([]byte("NaN"))
	} else {
		switch verb {
		case 'd': // Decimal
			_, err = fmt.Fprintf(state, "%d", n.whole)
		case 'f', 's', 'v': // Float
			if prec, ok := state.Precision(); ok && prec == 0 {
				_, err = fmt.Fprintf(state, "%d", n.whole)
			} else {
				_, err = fmt.Fprintf(state, "%d.%d", n.whole, n.frac)
			}
		default:
			panic("unsupported verb")
		}
	}
	if err != nil {
		panic(err)
	}
}

func (n Number) String() string {
	return fmt.Sprintf("%s", n)
}
