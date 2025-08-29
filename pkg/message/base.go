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

type commandBase struct{}

func (*commandBase) Command() (Number, bool) { return Number{}, false }
func (*commandBase) IsSafe() bool            { return false }
func (*commandBase) IsWrite() bool           { return false }
func (*commandBase) ParseResponse(buf []byte) (Response, error) {
	return OpaqueResponse(buf, false), nil
}
func (*commandBase) Value() (Number, bool)    { return Number{}, false }
func (*commandBase) Variable() (Number, bool) { return Number{}, false }
func (*commandBase) isMessage()               {}

type responseBase struct{}

func (*responseBase) Buffer() ([]byte, bool) { return nil, false }
func (*responseBase) Value() (Number, bool)  { return Number{}, false }
func (*responseBase) isMessage()             {}
