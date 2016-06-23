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

// mugo transpiles a subset of the Go language to C++.
//
// See package transpiler for more details.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"io/ioutil"
	"log"
	"os"

	"github.com/googlesamples/mugo/transpiler"
)

func mainImpl() error {
	verbose := flag.Bool("verbose", false, "log data")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	if !*verbose {
		log.SetOutput(ioutil.Discard)
	}
	f, err := transpiler.Transpile(os.Stdout, os.Stdin)
	os.Stdout.Sync()
	if *verbose {
		ast.Fprint(os.Stderr, nil, f, nil)
	}
	return err
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "mugo: %s\n", err)
		os.Exit(1)
	}
}
