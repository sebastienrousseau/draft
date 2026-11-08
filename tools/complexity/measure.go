// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fzipp/gocyclo"
	"github.com/uudashr/gocognit"
)

// Metric names, as they appear in the baseline.
const (
	cyclomatic = "cyclomatic"
	cognitive  = "cognitive"
	halstead   = "halstead"
	lines      = "lines"
	fileLines  = "filelines"
)

// ceilings are the per-function limits (and the per-file line limit) every
// change is held to. A unit over one is an offender, allowed only up to what
// the baseline records for it.
var ceilings = map[string]float64{
	cyclomatic: 10,
	cognitive:  15,
	halstead:   30,
	lines:      60,
	fileLines:  500,
}

// fileUnit names the whole-file unit in a key, where a function name would go.
const fileUnit = "(file)"

// Metrics maps a unit key ("path name") to its measured values.
type Metrics map[string]map[string]float64

// measure walks root and measures every non-test, non-generated Go file.
// Tests are left out on purpose: table-driven tests are long by design, and
// the ceilings are about the logic a reader has to follow.
func measure(root string) (Metrics, error) {
	m := Metrics{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return skipDir(path, root, d.Name())
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return measureFile(m, path, filepath.ToSlash(rel))
	})
	return m, err
}

// skipDir leaves out hidden directories, vendored code and test fixtures.
func skipDir(path, root, name string) error {
	if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata") {
		return filepath.SkipDir
	}
	return nil
}

func measureFile(m Metrics, path, rel string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return err
	}
	if ast.IsGenerated(f) {
		return nil
	}
	m[rel+" "+fileUnit] = map[string]float64{fileLines: float64(bytes.Count(src, []byte("\n")))}
	seen := map[string]int{}
	for _, fn := range units(f) {
		// A name can repeat in one file only for init (any number are
		// allowed); numbering by order keeps both measured, and keeps the
		// key stable when unrelated code above them moves.
		name := funcName(fn)
		if seen[name]++; seen[name] > 1 {
			name += "#" + strconv.Itoa(seen[name])
		}
		m[rel+" "+name] = measureFunc(fset, src, fn)
	}
	return nil
}

// units returns every function the file declares, in source order: each
// FuncDecl, and each function literal assigned to a package-level variable,
// as a FuncDecl named after the variable. Leaving the latter out let logic
// escape the ceilings by being written as "var f = func(...) {...}".
func units(f *ast.File) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			out = append(out, d)
		case *ast.GenDecl:
			out = append(out, varFuncs(d)...)
		}
	}
	return out
}

// varFuncs wraps each function literal a var declaration assigns directly.
// The wrapper's body and type are the literal's own, so every metric reads
// exactly what the literal holds.
func varFuncs(d *ast.GenDecl) []*ast.FuncDecl {
	if d.Tok != token.VAR {
		return nil
	}
	var out []*ast.FuncDecl
	for _, spec := range d.Specs {
		vs := spec.(*ast.ValueSpec)
		for i, v := range vs.Values {
			if lit, ok := v.(*ast.FuncLit); ok && i < len(vs.Names) {
				out = append(out, &ast.FuncDecl{Name: vs.Names[i], Type: lit.Type, Body: lit.Body})
			}
		}
	}
	return out
}

func measureFunc(fset *token.FileSet, src []byte, fn *ast.FuncDecl) map[string]float64 {
	start, end := fset.Position(fn.Pos()), fset.Position(fn.End())
	return map[string]float64{
		cyclomatic: float64(gocyclo.Complexity(fn)),
		cognitive:  float64(gocognit.Complexity(fn)),
		halstead:   math.Round(halsteadDifficulty(src[start.Offset:end.Offset])*10) / 10,
		lines:      float64(end.Line - start.Line + 1),
	}
}

// funcName qualifies a method with its receiver type, as "T.Name", so two
// methods of one name in a file are told apart.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return recvType(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func recvType(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvType(t.X)
	case *ast.IndexExpr:
		return recvType(t.X)
	case *ast.IndexListExpr:
		return recvType(t.X)
	case *ast.Ident:
		return t.Name
	}
	return "?"
}
