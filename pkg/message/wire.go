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
	"bufio"
	"fmt"
	"io"
)

const (
	// EOL is a CRLF.
	EOL = "\r\n"
	// A Prompt is emitted on each line of output.
	Prompt = '>'
)

// WriteFlusher is implemented by [bufio.Writer], for example.
type WriteFlusher interface {
	io.Writer
	Flush() error
}

// ScanPrompt is a [bufio.ScanFunc] that reads each line of text and strips any
// [Prompt] prefix characters.
func ScanPrompt(data []byte, atEOF bool) (int, []byte, error) {
	advance, token, err := bufio.ScanLines(data, atEOF)
	if err != nil {
		return 0, nil, err
	}
	for idx := range token {
		if token[idx] != Prompt {
			token = token[idx:]
			break
		}
	}
	return advance, token, nil
}

// WritePrompt writes a prompt to the output and then flushes it.
func WritePrompt(w WriteFlusher) error {
	if _, err := w.Write([]byte{Prompt}); err != nil {
		return err
	}
	return w.Flush()
}

// WriteResponse writes a complete response message line to the output, followed
// by a new prompt, and flushes.
func WriteResponse(w WriteFlusher, format string, args ...any) error {
	if _, err := w.Write([]byte{Prompt}); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		return err
	}
	if _, err := w.Write([]byte(EOL)); err != nil {
		return err
	}
	return WritePrompt(w)
}
