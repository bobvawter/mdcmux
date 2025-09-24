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

// Package fetch contains a command that will retrieve a range of macro
// variables and write them into a CSV file.
package fetch

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"vawter.tech/mdcmux/pkg/conn"
	"vawter.tech/mdcmux/pkg/message"
)

// Command is an entrypoint to retrieve a range of macro variables as a CSV
// file.
func Command() *cobra.Command {
	f := &fetcher{}
	cmd := &cobra.Command{
		Args:  cobra.NoArgs,
		Use:   "fetch",
		Short: "Fetch a range of macro variables",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return f.Run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&f.host, "host", "", "The hostname:port to connect to")
	cmd.Flags().StringVarP(&f.path, "out", "o", "", "The path to write the results to; defaults to stdout if unset")
	cmd.Flags().IntVarP(&f.start, "start", "s", 0, "The first macro variable number to fetch")
	cmd.Flags().IntVarP(&f.end, "end", "e", 0, "The last macro variable number to fetch; defaults to start if unset")
	return cmd
}

type fetcher struct {
	host, path string
	start, end int
}

func (f *fetcher) Run(ctx context.Context) error {
	if f.host == "" {
		return errors.New("no host specified")
	}
	if f.start == 0 {
		return errors.New("no starting macro variable number specified")
	}
	if f.end == 0 {
		f.end = f.start
	}

	count := f.end - f.start + 1
	if count <= 0 {
		return errors.New("end variable number must be less than or equal to start")
	}
	buf := make([]message.Number, count)
	if err := f.fetch(ctx, buf); err != nil {
		return err
	}
	return f.write(ctx, buf)
}

func (f *fetcher) fetch(ctx context.Context, buf []message.Number) error {
	c := conn.New(f.host)
	defer c.Close()

	for i := range buf {
		resp, err := c.RoundTrip(ctx, message.QueryCommand(message.Int(f.start+i)))
		if err != nil {
			return err
		}
		var ok bool
		buf[i], ok = resp.Value()
		if !ok {
			return fmt.Errorf("unexpected response: %v", resp)
		}
	}
	return nil
}

func (f *fetcher) write(_ context.Context, buf []message.Number) error {
	var w io.Writer
	if f.path == "" || f.path == "-" {
		w = os.Stdout
	} else {
		out, err := os.OpenFile(f.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", f.path, err)
		}
		defer func() { _ = out.Close() }()
	}

	table := csv.NewWriter(w)
	for i, num := range buf {
		if err := table.Write([]string{
			strconv.Itoa(f.start + i),
			num.String(),
		}); err != nil {
			return err
		}
	}
	table.Flush()
	return table.Error()
}
