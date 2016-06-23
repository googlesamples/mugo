//
// Copyright 2016 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package transpiler

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/googlesamples/mugo/transpiler"
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
			t.Errorf("failed to read %s.ino: %s", s, err)
			continue
		}
		out := &bytes.Buffer{}
		if _, err = transpiler.Transpile(out, g); err != nil {
			t.Errorf("failed to transpile sketch %q: %s", s, err)
			continue
		}
		if ino := string(bs); ino != out.String() {
			t.Errorf("expected:\n%s-- got:\n%s", ino, out.String())
		}
	}
}
