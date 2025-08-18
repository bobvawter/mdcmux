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

package mdcmux

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"vawter.tech/mdcmux/proxy"
	"vawter.tech/notify"
	"vawter.tech/stopper"
)

// Command is the entrypoint for starting the proxy server.
func Command() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Args:  cobra.NoArgs,
		Use:   "start",
		Short: "Start the MDC proxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfgPath == "" {
				return errors.New("no config file specified")
			}
			ctx := stopper.From(cmd.Context())

			var cfg notify.Var[*proxy.Config]
			ctx.Go(func(ctx *stopper.Context) error {
				after := time.After(0)
				var lastModTime time.Time

				for {
					select {
					case <-after:
						after = time.After(time.Second)

						info, err := os.Stat(cfgPath)
						if err != nil {
							return err
						}
						if mod := info.ModTime(); !mod.After(lastModTime) {
							continue
						}
						lastModTime = info.ModTime()
						nextCfg := &proxy.Config{}

						f, err := os.Open(cfgPath)
						if err != nil {
							if lastModTime.IsZero() {
								return fmt.Errorf("could not open configuration file %s: %w", cfgPath, err)
							}
							continue
						}

						dec := json.NewDecoder(f)
						dec.DisallowUnknownFields()
						if err := dec.Decode(nextCfg); err != nil {
							slog.ErrorContext(ctx, "could not decode configuration file",
								slog.String("path", cfgPath),
								slog.Any("error", err))
							continue
						}

						slog.DebugContext(ctx, "loaded new configuration")
						cfg.Set(nextCfg)

					case <-ctx.Stopping():
						return nil
					}
				}
			})

			_, err := proxy.New(ctx, &cfg)
			if err != nil {
				return err
			}
			return ctx.Wait()
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "configuration file")
	return cmd
}
