package vcd

import (
	"bufio"
	"os"
	"path"
	"strings"
	"testing"
)

// This test runs in the directory //vcd.  See BUILD.bazel file for details.
func TestVCDFiles(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("samples")
	if err != nil {
		t.Fatalf("could not read dir: %v", err)
	}
	for _, entry := range entries {
		entry := entry
		t.Run(entry.Name(), func(t *testing.T) {
			name := path.Join("samples", entry.Name())
			if name == "." || name == ".." || !strings.HasSuffix(name, ".vcd") || entry.IsDir() {
				return
			}
			f, err := os.Open(name)
			if err != nil {
				t.Errorf("could not open file: %v: %v", name, err)
			}
			parser := NewParser[File]()

			r := bufio.NewReader(f)

			if _, err := parser.Parse(name, r); err != nil {
				t.Errorf("parse error: `%v`: %+v", name, err)
			}
		})
	}
}
