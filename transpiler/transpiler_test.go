package transpiler

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sketches = []string{
	"blink",
	"button",
	"fade",
}

var tests = []string{
	"language-basics",
}

const sketchDir = "../sketches"
const testDir = "../tests"

func runTests(t *testing.T, testList []string, testDir string) {
	for _, s := range testList {
		g, err := os.Open(filepath.Join(testDir, s, s+".go"))
		if err != nil {
			t.Errorf("failed to open %s.go: %v", s, err)
			continue
		}
		defer g.Close()
		bs, err := ioutil.ReadFile(filepath.Join(testDir, s, s+".ino"))
		if err != nil {
			t.Errorf("failed to read %s.ino: %v", s, err)
			continue
		}
		ino := string(bs)
		var out bytes.Buffer
		if err := Transpile(&out, g, nil); err != nil {
			t.Errorf("failed to transpile sketch %q: %v", s, err)
			continue
		}
		if nospace(ino) != nospace(out.String()) {
			t.Errorf("expected:\n%s-- got:\n%s", ino, out.String())
		}
	}
}

func TestTests(t *testing.T) {
	runTests(t, tests, testDir)
}

func TestSketches(t *testing.T) {
	runTests(t, sketches, sketchDir)
}

func nospace(s string) string {
	s = strings.Replace(s, " ", "", -1)
	s = strings.Replace(s, "\r", "", -1)
	s = strings.Replace(s, "\n", "", -1)
	return s
}
