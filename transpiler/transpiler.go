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

// transpiler convert a subset of Go to Arduino C++ dialect.
//
// Use case is to help port Go code on MCUs (small microcontroller) where there
// is no operating system. A strict C++ subset (without RTTI) is used.
//
// ** Do not take this project too seriously **
//
// Challenges and transformations:
//  - function with multiple return values is converted to returning as a
//    temporary struct
//  - string constant is converted to const char *
//  - interface{} is converted to void *
//  - interface inheritance is figured out at parsing time
//  - out of bound check for slice and strings
//  - string indexing is done via byte offset, not runes
//  - struct are manually zero initialized
//  - recursive type resolution of imported packages
//
// Out of scope:
//  - channel type
//  - defer statement
//  - go statement
//  - len function
//  - map type
//  - range expression
//  - select statement
//  - switch statement
//  - anonymous function
//  - function pointer
//  - pointer to member of struct
//  - unnamed struct embedding
//  - memory management, all memory allocation is leaked
//  - dynamic type casting involving RTTI
//  - using the STL in the generated code
package transpiler

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"strings"

	"github.com/kr/pretty"
)

// Transpile converts a Go (.go) source file to C++ (.cc).
func Transpile(out io.Writer, in io.Reader) (*ast.File, error) {
	fset := token.NewFileSet()
	// Keep a copy of the input file to do a byte offset to line conversion.
	content, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, err
	}
	f, err := parser.ParseFile(fset, "src.go", bytes.NewReader(content), parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: %s", err)
	}
	lines := make([]int, 0, 128)
	for i, c := range content {
		if c == '\n' {
			lines = append(lines, i)
		}
	}
	o := &output{out, content, lines, nil, f.Comments, nil}
	/*
		// Explicitly push everything up to package name so it doesn't get printed.
		// It's kind of annoying as there's no PackageStmt so it has to be explicitly
		// emulated.
		if f.Package != 0 && len(lines) != 0 {
			o.out.Write(content[:f.Package-1])
			o.lastNode = &fakeNode{token.Pos(lines[o.findLine(int(f.Package))-1] + 1)}
		}
	*/
	for _, i := range f.Imports {
		o.Writef(i, "")
	}
	for _, d := range f.Decls {
		if err := handleDecl(o, d); err != nil {
			return f, err
		}
		if o.err != nil {
			break
		}
	}
	return f, o.err
}

type fakeNode struct {
	e token.Pos
}

func (f *fakeNode) Pos() token.Pos {
	return 0
}

func (f *fakeNode) End() token.Pos {
	return f.e
}

type output struct {
	out      io.Writer
	content  []byte
	lines    []int
	lastNode ast.Node
	c        []*ast.CommentGroup
	err      error
}

// Writef makes sure that all comments up to the point where the Node is
// declared are flushed.
func (o *output) Writef(n ast.Node, format string, a ...interface{}) {
	// TODO(maruel): Print characters between symbols and statement.
	// TODO(maruel): This should be done with ast.CommentMap.
	if o.err == nil {
		for len(o.c) != 0 && n.Pos() > o.c[0].Pos() {
			for _, c := range o.c[0].List {
				// TODO(maruel): Include additional spacing.
				if _, err := fmt.Fprintf(o.out, "%s\n", c.Text); err != nil {
					o.err = err
					return
				}
			}
			o.c = o.c[1:]
		}
		if _, err := fmt.Fprintf(o.out, format, a...); err != nil {
			o.err = err
		}
	}
}

func (o *output) findLine(p int) int {
	l := 0
	for ; len(o.lines) > l && p >= o.lines[l]; l++ {
	}
	return l + 1
}

// Errorf returns an error with the node position.
func (o *output) Errorf(n ast.Node, format string, a ...interface{}) error {
	l := o.findLine(int(n.Pos()))
	return fmt.Errorf("line %d: %s\n%# v", l, fmt.Sprintf(format, a...), pretty.Formatter(n))
}

// handleDecl handles a declaration.
//
// It can be a function, a variable, a constant, an import, etc.
func handleDecl(out *output, d ast.Decl) error {
	switch decl := d.(type) {
	case *ast.GenDecl:
		return handleGenDecl(out, decl)
	case *ast.FuncDecl:
		return handleFuncDecl(out, decl)
	default:
		return out.Errorf(d, "unsupported decl")
	}
}

// handleGenDecl handles a file level declaration; a constant, a variable or an
// import statement.
func handleGenDecl(out *output, gd *ast.GenDecl) error {
	for _, s := range gd.Specs {
		switch spec := s.(type) {
		case *ast.ValueSpec:
			//pretty.Print(spec)
			if err := handleValueSpec(out, spec); err != nil {
				return err
			}
		case *ast.ImportSpec:
			// Ignore imports except for comments.
			out.Writef(s, "")
		default:
			return out.Errorf(s, "unsupported spec")
		}
		// TODO(maruel): Print spacing between declarations.
	}
	return nil
}

func guessType(vs *ast.ValueSpec) (token.Token, string, error) {
	if len(vs.Values) > 1 {
		return token.ILLEGAL, "", fmt.Errorf("unsupported # of values: %v", vs.Names)
	}
	if len(vs.Values) == 0 {
		// It is an default value, e.g. "var a int". It can't be const.
		// token.Lookup() is not very useful as it expects "STRING" instead of
		// "string".
		switch n := vs.Type.(*ast.Ident).Name; n {
		case "int":
			return token.INT, "0", nil
		case "string":
			return token.STRING, "\"\"", nil
		default:
			return token.ILLEGAL, "", fmt.Errorf("unsupported type: %s", n)
		}
	}
	// Normal declaration of type "var a = 1" or "const a = 1".
	l, ok := vs.Values[0].(*ast.BasicLit)
	if !ok {
		return token.ILLEGAL, "", fmt.Errorf("unsupported value: %#v", vs.Values[0])
	}
	return l.Kind, l.Value, nil
}

func isValueConst(vs *ast.ValueSpec) bool {
	return vs.Names[0].Obj.Kind == ast.Con
}

// handleValueSpec handles a file level a constant or variable.
func handleValueSpec(out *output, vs *ast.ValueSpec) error {
	if len(vs.Names) == 0 {
		return out.Errorf(vs, "unsupported # of value names: %v", vs.Names)
	}
	var decl []string
	kind, lit, err := guessType(vs)
	if err != nil {
		return out.Errorf(vs, "%s", err)
	}
	isConst := isValueConst(vs)
	typ := tokenStr(kind, isConst)
	if len(typ) == 0 {
		return out.Errorf(vs, "unsupported literal kind: %s", kind)
	}
	// Strictly speaking the C++ version could also define all the variables on
	// one line but the following is easier to implement.
	for _, name := range vs.Names {
		out.Writef(vs, "%s;\n", strings.Join(append(decl, typ, name.Name, "=", lit), " "))
	}
	return nil
}

// tokenStr returns the closest 'C' type for a token.Token.
func tokenStr(kind token.Token, isConst bool) string {
	switch kind {
	case token.INT:
		if isConst {
			return "const int"
		}
		return "int"
	case token.STRING:
		if isConst {
			return "const char * const"
		}
		return "const char *"
	default:
		return ""
	}
}

// exprTypeToType returns a "C" representation of the Node.
//
// For some value of "C".
//
// Can be used to return the name of an identifier.
//
// Returns true on the second parameter if the type includes ellipsis '...'.
func exprTypeToType(out *output, n ast.Expr) (string, bool, error) {
	// TODO(maruel): This is a very adhoc implementation.
	switch arg := n.(type) {
	case *ast.ArrayType:
		name, extra, err := exprTypeToType(out, arg.Elt)
		if err != nil {
			return "", false, err
		}
		return "*" + name, extra, nil
	case *ast.Ellipsis:
		// TODO(maruel): '...' -> pointer?
		name, _, err := exprTypeToType(out, arg.Elt)
		return name, true, err
	case *ast.FuncType:
		return "", false, out.Errorf(n, "function pointers are not supported")
	case *ast.Ident:
		return arg.Name, false, nil
	case *ast.InterfaceType:
		return "void *", false, nil
	case *ast.SelectorExpr:
		x, _, err := exprTypeToType(out, arg.X)
		if err != nil {
			return "", false, err
		}
		s, _, err := exprTypeToType(out, arg.Sel)
		if err != nil {
			return "", false, err
		}
		// TODO(maruel): '->' when arg.X is known to be a pointer.
		return x + "." + s, false, nil
	case *ast.StarExpr:
		x, extra, err := exprTypeToType(out, arg.X)
		if err != nil {
			return "", extra, err
		}
		return "*" + x, extra, nil
	default:
		return "", false, out.Errorf(n, "unexpected param type")
	}
}

// extractArgumentsType returns the name of the type of each input argument.
func extractArgumentsType(out *output, f *ast.FuncDecl) ([]string, error) {
	var fields []*ast.Field
	if f.Recv != nil {
		if len(f.Recv.List) != 1 {
			return nil, out.Errorf(f.Recv, "expect only one receiver; please fix code")
		}
		// If it is an object receiver (vs a pointer receiver), its address is not
		// printed in the stack trace so it needs to be ignored.
		if _, ok := f.Recv.List[0].Type.(*ast.StarExpr); ok {
			fields = append(fields, f.Recv.List[0])
		}
	}
	var types []string
	for _, arg := range append(fields, f.Type.Params.List...) {
		// Assert that extra is only set on the last item of fields?
		t, extra, err := exprTypeToType(out, arg.Type)
		if err != nil {
			return nil, err
		}
		if extra {
			return nil, out.Errorf(arg, "unsupported param type")
		}
		mult := len(arg.Names)
		if mult == 0 {
			mult = 1
		}
		for i := 0; i < mult; i++ {
			types = append(types, t)
		}
	}
	return types, nil
}

func handleFuncDecl(out *output, fd *ast.FuncDecl) error {
	ret := "void"
	if fd.Type.Results != nil {
		if len(fd.Type.Results.List) != 1 {
			return out.Errorf(fd, "unsupported return type: %# v", pretty.Formatter(fd.Type.Results))
		}
		var err error
		ret, _, err = exprTypeToType(out, fd.Type.Results.List[0].Type)
		if err != nil {
			return err
		}
	}
	params, err := extractArgumentsType(out, fd)
	if err != nil {
		return err
	}
	out.Writef(fd, "%s %s(%s) {\n", ret, fd.Name, strings.Join(params, " "))
	if err := handleBlockStmt(out, fd.Body); err != nil {
		return err
	}
	// TODO(maruel): fd.Body.Rbrace
	out.Writef(fd.Body, "}\n")
	return nil
}

// handleStmt handles a single statement inside a block.
func handleStmt(out *output, s ast.Stmt) error {
	// TODO(maruel): Implement indentation by printing characters between AST
	// items via output.Writef().
	out.Writef(s, "  ")
	switch st := s.(type) {
	case *ast.ExprStmt:
		if err := handleExpr(out, st.X); err != nil {
			return err
		}
		out.Writef(s, ";\n")
	case *ast.AssignStmt:
		// TODO(maruel): Correctly support for multiple return values, it is
		// currently adhoc.
		if st.Tok != token.DEFINE && st.Tok != token.ASSIGN {
			return out.Errorf(st, "unexpected assignment: %s", st.Tok)
		}
		for i, lhs := range st.Lhs {
			if i != 0 {
				out.Writef(lhs, ", ")
			} else if st.Tok == token.DEFINE {
				// Need to add type before.
				out.Writef(st, typeFromExpr(st.Rhs[i])+" ")
			}
			if err := handleExpr(out, lhs); err != nil {
				return err
			}
		}
		out.Writef(st, " = ")
		for i, rhs := range st.Rhs {
			if i != 0 {
				out.Writef(rhs, ", ")
			}
			if err := handleExpr(out, rhs); err != nil {
				return err
			}
		}
		out.Writef(st, ";\n")
	case *ast.IfStmt:
		out.Writef(st, "if (")
		if err := handleExpr(out, st.Cond); err != nil {
			return err
		}
		out.Writef(st, ") {\n")
		if err := handleBlockStmt(out, st.Body); err != nil {
			return err
		}
		out.Writef(st, "}")
		if st.Else != nil {
			bs, ok := st.Else.(*ast.BlockStmt)
			if !ok {
				return out.Errorf(st.Else, "unsupported else statement")
			}
			out.Writef(st, " else {\n")
			if err := handleBlockStmt(out, bs); err != nil {
				return err
			}
			out.Writef(st, "}")
		}
		out.Writef(st, "\n")
	case *ast.ReturnStmt:
		out.Writef(st, "return ")
		for i, r := range st.Results {
			if i != 0 {
				// TODO(maruel): Effectively support multiple return values.
				out.Writef(r, ", ")
			}
			if err := handleExpr(out, r); err != nil {
				return err
			}
		}
		out.Writef(st, ";\n")
	default:
		return out.Errorf(s, "unsupported statement")
	}
	return nil
}

// handleBlockStmt handles a series of statements in a block delimited with "{"
// and "}".
func handleBlockStmt(out *output, bs *ast.BlockStmt) error {
	for _, s := range bs.List {
		if err := handleStmt(out, s); err != nil {
			return err
		}
	}
	return nil
}

// handleCallExpr handles a function call.
func handleCallExpr(out *output, c *ast.CallExpr) error {
	args := []string{}
	buf := &bytes.Buffer{}
	tmp := &output{buf, out.content, out.lines, c, nil, nil}
	for _, a := range c.Args {
		buf.Reset()
		if err := handleExpr(tmp, a); err != nil {
			return err
		}
		args = append(args, buf.String())
		out.lastNode = a
	}
	ident, _, err := exprTypeToType(out, c.Fun)
	if err != nil {
		return err
	}
	out.Writef(c, "%s(%s)", ident, strings.Join(args, ", "))
	return nil
}

// handleBinaryExpr handles an expression for the form "X <op> Y".
func handleBinaryExpr(out *output, be *ast.BinaryExpr) error {
	if err := handleExpr(out, be.X); err != nil {
		return err
	}
	out.Writef(be, "%s", be.Op)
	if err := handleExpr(out, be.Y); err != nil {
		return err
	}
	return nil
}

// handleExpr handles a generic expression, like "a()", "a + b", "a != nil",
// "a++", etc.
func handleExpr(out *output, e ast.Expr) error {
	switch expr := e.(type) {
	case *ast.BasicLit:
		// a constant
		out.Writef(expr, "%s", expr.Value)
	case *ast.BinaryExpr:
		return handleBinaryExpr(out, expr)
	case *ast.CallExpr:
		return handleCallExpr(out, expr)
	case *ast.Ident:
		// identifier
		out.Writef(expr, "%s", expr.Name)
	case *ast.SelectorExpr:
		// can be either a symbol from a package or a member or method dereference.
		if err := handleExpr(out, expr.X); err != nil {
			return err
		}
		// TODO(maruel): have to be converted to "->" for pointer dereference.
		out.Writef(expr, ".")
		return handleExpr(out, expr.Sel)
	case *ast.StarExpr:
		out.Writef(expr, "*")
		return handleExpr(out, expr.X)
	case *ast.UnaryExpr:
		// handles an expression with only one operator, e.g. "!", "++", etc
		out.Writef(expr, "%s", expr.Op)
		return handleExpr(out, expr.X)
	default:
		return out.Errorf(e, "unsupported expr")
	}
	return nil
}

// typeFromExpr extracts the type from a constant.
//
// For example a node containing the integer constant '2' would return 'int'.
func typeFromExpr(e ast.Expr) string {
	switch expr := e.(type) {
	case *ast.BasicLit:
		// a constant
		return tokenStr(expr.Kind, false)
	//case *ast.BinaryExpr:
	//case *ast.CallExpr:
	case *ast.Ident:
		// identifier
		return expr.Name
	//case *ast.SelectorExpr:
	//case *ast.StarExpr:
	//case *ast.UnaryExpr:
	default:
		return ""
	}
}
