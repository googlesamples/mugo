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

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"
)

func main() {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "sketch.go", os.Stdin, 0)
	if err != nil {
		fmt.Println(err)
		return
	}

	ast.Fprint(os.Stderr, fset, f, nil)

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Type.Results != nil {
				log.Fatalf("unsupported return type: %v", x.Type.Results)
			}
			if x.Type.Params.List != nil {
				log.Fatalf("unsupported param type: %v", x.Type.Params)
			}
			fmt.Printf("void %s() {\n", x.Name)
			for _, s := range x.Body.List {
				x, ok := s.(*ast.ExprStmt)
				if !ok {
					log.Fatalf("unsupported statement: %v", x)
				}
				c, ok := x.X.(*ast.CallExpr)
				if !ok {
					log.Fatalf("unsupported expr: %v", x.X)
				}
				funcName, _ := c.Fun.(*ast.Ident)
				args := []string{}
				for _, a := range c.Args {
					switch expr := a.(type) {
					case *ast.BasicLit:
						args = append(args, expr.Value)
					case *ast.Ident:
						args = append(args, expr.Name)
					default:
						log.Fatalf("unsupported expr: %v", expr)
					}
				}
				fmt.Printf("  %s(%s);\n", funcName, strings.Join(args, ", "))
			}
			fmt.Println("}")
		}
		return true
	})
}
