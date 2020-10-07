// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pkgpath determines the package path used by gccgo/GoLLVM symbols.
// This package is not used for the gc compiler.
package pkgpath

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

// ToSymbolFunc returns a function that may be used to convert a
// package path into a string suitable for use as a symbol.
// cmd is the gccgo/GoLLVM compiler in use, and tmpdir is a temporary
// directory to pass to ioutil.TempFile.
// For example, this returns a function that converts "net/http"
// into a string like "net..z2fhttp". The actual string varies for
// different gccgo/GoLLVM versions, which is why this returns a function
// that does the conversion appropriate for the compiler in use.
func ToSymbolFunc(cmd, tmpdir string) (func(string) string, error) {
	// To determine the scheme used by cmd, we compile a small
	// file and examine the assembly code. Older versions of gccgo
	// use a simple mangling scheme where there can be collisions
	// between packages whose paths are different but mangle to
	// the same string. More recent versions use a new mangler
	// that avoids these collisions.
	const filepat = "*_gccgo_manglechck.go"
	f, err := ioutil.TempFile(tmpdir, filepat)
	if err != nil {
		return nil, err
	}
	gofilename := f.Name()
	f.Close()
	defer os.Remove(gofilename)

	if err := ioutil.WriteFile(gofilename, []byte(mangleCheckCode), 0644); err != nil {
		return nil, err
	}

	command := exec.Command(cmd, "-S", "-o", "-", gofilename)
	buf, err := command.Output()
	if err != nil {
		return nil, err
	}

	// New mangling: expect go.l..u00e4ufer.Run
	// Old mangling: expect go.l__ufer.Run
	if bytes.Contains(buf, []byte("go.l..u00e4ufer.Run")) {
		return toSymbolV2, nil
	} else if bytes.Contains(buf, []byte("go.l__ufer.Run")) {
		return toSymbolV1, nil
	} else {
		return nil, errors.New(cmd + ": unrecognized mangling scheme")
	}
}

// mangleCheckCode is the package we compile to determine the mangling scheme.
const mangleCheckCode = `
package läufer
func Run(x int) int {
  return 1
}
`

// toSymbolV1 converts a package path using the original mangling scheme.
func toSymbolV1(ppath string) string {
	clean := func(r rune) rune {
		switch {
		case 'A' <= r && r <= 'Z', 'a' <= r && r <= 'z',
			'0' <= r && r <= '9':
			return r
		}
		return '_'
	}
	return strings.Map(clean, ppath)
}

// toSymbolV2 converts a package path using the newer mangling scheme.
func toSymbolV2(ppath string) string {
	// This has to build at boostrap time, so it has to build
	// with Go 1.4, so we don't use strings.Builder.
	bsl := make([]byte, 0, len(ppath))
	changed := false
	for _, c := range ppath {
		if ('A' <= c && c <= 'Z') || ('a' <= c && c <= 'z') || ('0' <= c && c <= '9') || c == '_' {
			bsl = append(bsl, byte(c))
			continue
		}
		var enc string
		switch {
		case c == '.':
			enc = ".x2e"
		case c < 0x80:
			enc = fmt.Sprintf("..z%02x", c)
		case c < 0x10000:
			enc = fmt.Sprintf("..u%04x", c)
		default:
			enc = fmt.Sprintf("..U%08x", c)
		}
		bsl = append(bsl, enc...)
		changed = true
	}
	if !changed {
		return ppath
	}
	return string(bsl)
}