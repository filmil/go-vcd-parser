package files

import (
	"bufio"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/filmil/go-vcd-parser/vcd"
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
			want := parseWithGrammar(t, name)
			got := parseStreaming(t, name)
			// Whole realistic files must decode identically both
			// ways. This is what keeps the two parsers from
			// drifting apart.
			if !reflect.DeepEqual(vcd.NormalizeForCompare(want), vcd.NormalizeForCompare(got)) {
				t.Errorf("%v: streaming and grammar parses differ", name)
			}
		})
	}
}

func parseWithGrammar(t *testing.T, name string) *vcd.File {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatalf("could not open file: %v: %v", name, err)
	}
	defer f.Close()
	file, err := vcd.NewParser[vcd.File]().Parse(name, bufio.NewReader(f))
	if err != nil {
		t.Fatalf("parse error: `%v`: %+v", name, err)
	}
	return file
}

func parseStreaming(t *testing.T, name string) *vcd.File {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatalf("could not open file: %v: %v", name, err)
	}
	defer f.Close()
	file, err := vcd.ParseFile(name, f)
	if err != nil {
		t.Fatalf("streaming parse error: `%v`: %+v", name, err)
	}
	return file
}
