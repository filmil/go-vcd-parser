package vcd

import (
	"bufio"
	"os"
	"testing"
)

func TestVCDFiles(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("could not read dir: %v", err)
	}
	for _, entry := range entries {
		entry := entry
		t.Run(entry.Name(), func(t *testing.T) {
			name := entry.Name()
			if name == "." || name == ".." || entry.IsDir() {
				return
			}
			f, err := os.Open(name)
			if err != nil {
				t.Errorf("could not open file: %v: %v", name, err)
			}
			parser := NewParser()

			r := bufio.NewReader(f)

			if _, err := parser.Parse(name, r); err != nil {
				t.Errorf("parse error: `%v`: %+v", name, err)
			}
		})
	}
}
