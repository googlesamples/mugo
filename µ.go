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
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"strings"
)

func transpile(out io.Writer, in io.Reader, debug io.Writer) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "sketch.go", in, 0)
	if err != nil {
		return fmt.Errorf("failed to parse file: %v", err)
	}

	if debug != nil {
		ast.Fprint(debug, fset, f, nil)
	}

	for _, d := range f.Decls {
		if err := handleDecl(out, d); err != nil {
			return fmt.Errorf("error handling decl %#v: %v", d, err)
		}
	}
	return nil
}

func handleDecl(out io.Writer, d ast.Decl) error {
	switch decl := d.(type) {
	case *ast.GenDecl:
		return handleGenDecl(out, decl)
	case *ast.FuncDecl:
		return handleFuncDecl(out, decl)
	default:
		return fmt.Errorf("unsupported decl: %#v", d)
	}
}

func handleGenDecl(out io.Writer, gd *ast.GenDecl) error {
	for _, s := range gd.Specs {
		vs, ok := s.(*ast.ValueSpec)
		if !ok {
			return fmt.Errorf("unsupported spec: %#v", s)
		}
		if len(vs.Names) > 1 {
			return fmt.Errorf("unsupported # of value names: %v", vs.Names)
		}
		decl := []string{}
		if vs.Names[0].Obj.Kind == ast.Con {
			decl = append(decl, "const")
		}
		if len(vs.Values) > 1 {
			return fmt.Errorf("unsupported # of values: %v", vs.Names)
		}
		l, ok := vs.Values[0].(*ast.BasicLit)
		if !ok {
			return fmt.Errorf("unsupported value: %#v", vs.Values[0])
		}
		if l.Kind != token.INT {
			return fmt.Errorf("unsupported literal kind: %#v", l.Kind)
		}
		decl = append(decl, "int", vs.Names[0].Name, "=", l.Value)
		fmt.Fprintf(out, "%s;\n", strings.Join(decl, " "))
	}
	return nil
}

func handleFuncDecl(out io.Writer, fd *ast.FuncDecl) error {
	if fd.Type.Results != nil {
		return fmt.Errorf("unsupported return type: %#v", fd.Type.Results)
	}
	if fd.Type.Params.List != nil {
		return fmt.Errorf("unsupported param type: %#v", fd.Type.Params)
	}
	fmt.Fprintf(out, "void %s() {\n", fd.Name)
	if err := handleBlockStmt(out, fd.Body); err != nil {
		return fmt.Errorf("error handling block statement for %q: %v", fd.Name, err)
	}
	fmt.Fprintln(out, "}")
	return nil
}

func handleBlockStmt(out io.Writer, bs *ast.BlockStmt) error {
	for _, s := range bs.List {
		fmt.Fprintf(out, "  ")
		switch st := s.(type) {
		case *ast.ExprStmt:
			if err := handleExpr(out, st.X); err != nil {
				return fmt.Errorf("error handling expr stmt %v: %v", st.X, err)
			}
			fmt.Fprint(out, ";\n")
		case *ast.AssignStmt:
			if len(st.Lhs) > 1 {
				return fmt.Errorf("unsupported # of lhs exprs: %v", st.Lhs)

			}
			if err := handleExpr(out, st.Lhs[0]); err != nil {
				return fmt.Errorf("error handling left expr %v: %v", st.Lhs[0], err)
			}
			fmt.Fprintf(out, "=")
			if len(st.Rhs) > 1 {
				return fmt.Errorf("unsupported # of rhs exprs: %v", st.Rhs)

			}
			if err := handleExpr(out, st.Rhs[0]); err != nil {
				return fmt.Errorf("error handling right expr %v: %v", st.Rhs[0], err)
			}
			fmt.Fprint(out, ";\n")
		case *ast.IfStmt:
			fmt.Fprintf(out, "if (")
			if err := handleExpr(out, st.Cond); err != nil {
				return fmt.Errorf("error handling if block conditionx: %v", err)
			}
			fmt.Fprint(out, ") {\n")
			if err := handleBlockStmt(out, st.Body); err != nil {
				return fmt.Errorf("error handling if block statements: %v", err)
			}
			fmt.Fprintf(out, "}")
			if st.Else != nil {
				bs, ok := st.Else.(*ast.BlockStmt)
				if !ok {
					return fmt.Errorf("unsupported statement: %v", st.Else)
				}
				fmt.Fprintf(out, " else {\n")
				if err := handleBlockStmt(out, bs); err != nil {
					return fmt.Errorf("error handling else block statements: %v", err)
				}
				fmt.Fprintf(out, "}")
			}
			fmt.Fprintln(out)
		default:
			return fmt.Errorf("unsupported statement: %v", s)

		}
	}
	return nil
}

func handleCallExpr(out io.Writer, c *ast.CallExpr) error {
	funcName, ok := c.Fun.(*ast.Ident)
	if !ok {
		return fmt.Errorf("unsupported func expr: %#v", c.Fun)
	}
	args := []string{}
	for _, a := range c.Args {
		var buf bytes.Buffer
		if err := handleExpr(&buf, a); err != nil {
			return fmt.Errorf("error handling func arg expr %#v: %v", a, err)
		}
		args = append(args, buf.String())
	}
	fmt.Fprintf(out, "%s(%s)", funcName, strings.Join(args, ", "))
	return nil
}

func handleBinaryExpr(out io.Writer, be *ast.BinaryExpr) error {
	if err := handleExpr(out, be.X); err != nil {
		return fmt.Errorf("error handling left part %v of binary expr: %v", be.X, err)
	}
	fmt.Fprint(out, be.Op)
	if err := handleExpr(out, be.Y); err != nil {
		return fmt.Errorf("error handling right part %v of binary expr: %v", be.Y, err)
	}
	return nil
}

func handleUnaryExpr(out io.Writer, ue *ast.UnaryExpr) error {
	fmt.Fprint(out, ue.Op)
	if err := handleExpr(out, ue.X); err != nil {
		return err
	}
	return nil
}

func handleIdent(out io.Writer, ident *ast.Ident) error {
	fmt.Fprintf(out, ident.Name)
	return nil
}

func handleBasicLit(out io.Writer, lit *ast.BasicLit) error {
	fmt.Fprintf(out, lit.Value)
	return nil
}

func handleExpr(out io.Writer, e ast.Expr) error {
	switch expr := e.(type) {
	case *ast.CallExpr:
		return handleCallExpr(out, expr)
	case *ast.BinaryExpr:
		return handleBinaryExpr(out, expr)
	case *ast.UnaryExpr:
		return handleUnaryExpr(out, expr)
	case *ast.Ident:
		return handleIdent(out, expr)
	case *ast.BasicLit:
		return handleBasicLit(out, expr)
	default:
		return fmt.Errorf("unsupported expr: %#v", e)
	}
}

func main() {
	if err := transpile(os.Stdout, os.Stdin, os.Stderr); err != nil {
		log.Fatalf("failed to transpile: %v", err)
	}
}
