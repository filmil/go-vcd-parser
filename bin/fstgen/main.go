// SPDX-License-Identifier: Apache-2.0

// Command fstgen writes the small FST dump the tests read.
//
// The dump is checked in rather than built during the test run, so the
// tests read a fixed file the way they read the VCD samples, and so a
// change to the writer cannot quietly change what the reader is tested
// against. Regenerate it with:
//
//	bazel run //bin/fstgen -- $PWD/cvt/testdata/small.fst
//
// A regenerated dump does not compare equal to the old one byte for byte.
// libfst stamps the creation date into the header, so the bytes differ on
// every run while the signals and value changes stay the same. Expect the
// diff, and check the content rather than the bytes.
package main

import (
	"fmt"
	"os"

	"github.com/filmil/go-vcd-parser/fst"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: fstgen <out.fst>\n")
		os.Exit(1)
	}
	if err := fst.WriteSampleDump(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "fstgen: %v\n", err)
		os.Exit(1)
	}
}
