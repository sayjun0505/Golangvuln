// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package govulnchecklib

import (
	"errors"
	"os"
	"strings"
)

const noGoModErrorMessage = `govulncheck: no go.mod file

govulncheck only works Go with modules. To make your project a module, run go mod init.

See https://go.dev/doc/modules/managing-dependencies for more information.`

const noGoSumErrorMessage = `govulncheck: no go.sum file

Your module is missing a go.sum file. Try running go mod tidy.

See https://go.dev/doc/modules/managing-dependencies for more information.`

// fileExists checks if file path exists. Returns true
// if the file exists or it cannot prove that it does
// not exist. Otherwise, returns false.
func fileExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else if errors.Is(err, os.ErrNotExist) {
		return false
	}
	// Conservatively return true if os.Stat fails
	// for some other reason.
	return true
}

const goVersionMismatchErrorMessage = `govulncheck: Go version mismatch

Loading packages failed, possibly due to a mismatch between the Go version
used to build govulncheck and the Go version on PATH. Consider rebuilding
govulncheck with the current Go version.`

// isGoVersionMismatchError checks if err is due to mismatch between
// the Go version used to build govulncheck and the one currently
// on PATH.
func isGoVersionMismatchError(err error) bool {
	msg := err.Error()
	// See golang.org/x/tools/go/packages/packages.go.
	return strings.Contains(msg, "This application uses version go") &&
		strings.Contains(msg, "It may fail to process source files")
}
