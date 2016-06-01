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

const sketchDir = "../sketches"

func TestSketches(t *testing.T) {
	for _, s := range sketches {
		g, err := os.Open(filepath.Join(sketchDir, s, s+".go"))
		if err != nil {
			t.Errorf("failed to open %s.go: %v", s, err)
			continue
		}
		defer g.Close()
		bs, err := ioutil.ReadFile(filepath.Join(sketchDir, s, s+".ino"))
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

func nospace(s string) string {
	s = strings.Replace(s, " ", "", -1)
	s = strings.Replace(s, "\r", "", -1)
	s = strings.Replace(s, "\n", "", -1)
	return s
}
