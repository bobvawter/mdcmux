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

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"vawter.tech/mdcmux/cmd/dummy"
	"vawter.tech/mdcmux/cmd/mdcmux"
	"vawter.tech/stopper"
)

func main() {
	var drainTime time.Duration
	var verbose bool
	root := &cobra.Command{
		Use: "mdcmux",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if verbose {
				slog.SetLogLoggerLevel(slog.LevelDebug)
			}
			return nil
		},
	}
	root.PersistentFlags().DurationVar(&drainTime, "drain", time.Minute, "connection drain time")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	root.AddCommand(mdcmux.Command())
	root.AddCommand(dummy.Command())

	ctx := stopper.WithContext(context.Background())
	ctx.Go(func(ctx *stopper.Context) error {
		ch := make(chan os.Signal, 1)
		defer close(ch)

		signal.Notify(ch, os.Interrupt)
		defer signal.Stop(ch)

		select {
		case <-ch:
			ctx.Stop(drainTime)
		case <-ctx.Stopping():
		}
		return nil
	})

	if err := root.ExecuteContext(ctx); err != nil {
		slog.Error("fatal error", slog.Any("error", err))
		os.Exit(1)
	}
	os.Exit(0)
}
