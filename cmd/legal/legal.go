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

package legal

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:generate go tool github.com/google/go-licenses save ../.. --save_path ./data/licenses --force

//go:embed data
var data embed.FS

// Command is an entrypoint to print licenses for redistributed code.
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "legal",
		Args:  cobra.NoArgs,
		Short: "print licenses for redistributed modules",
		RunE: func(cmd *cobra.Command, args []string) error {
			const base = "data/licenses"
			return fs.WalkDir(data, base, func(path string, d fs.DirEntry, err error) error {
				switch {
				case os.IsNotExist(err):
					return errors.New("development binaries should not be distributed")
				case err != nil:
					return err
				case d.IsDir():
					return nil
				}

				// Trim leading "data/" and trailing slash.
				pkg, _ := filepath.Split(path)
				fmt.Printf("Module %s:\n\n", pkg[len(base)+1:len(pkg)-1])

				f, err := data.Open(path)
				if err != nil {
					return fmt.Errorf("%s %w", path, err)
				}
				defer func() { _ = f.Close() }()
				if _, err := io.Copy(os.Stdout, f); err != nil {
					return fmt.Errorf("%s %w", path, err)
				}
				fmt.Print("=====\n\n")
				return nil
			})
		},
	}
}
