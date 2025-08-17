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

package dummy

import (
	"github.com/spf13/cobra"
	"vawter.tech/mdcmux/dummy"
	"vawter.tech/stopper"
)

// Command is the entrypoint for the dummy MDC server.
func Command() *cobra.Command {
	var bind string
	cmd := &cobra.Command{
		Use:   "dummy",
		Args:  cobra.NoArgs,
		Short: "start a dummy MDC server for demo purposes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := stopper.From(cmd.Context())
			_, err := dummy.New(ctx, bind)
			if err != nil {
				return err
			}
			return ctx.Wait()
		},
	}
	cmd.Flags().StringVarP(&bind, "bind", "b", "127.0.0.1:13013", "bind address")

	return cmd
}
