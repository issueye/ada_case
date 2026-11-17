// Package symbols lists the declarations of a file for the outline: Go
// files through go/parser, Markdown files through their headings, and
// other languages through the declarations their lines start with.
package symbols

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Symbol is one entry of the outline.
type Symbol struct {
	// Name is the declaration's name, as "main" or "Add".
	Name string
	// Kind is what it declares: "函数", "方法", "类型", "变量", "常量"
	// for Go, "标题" for Markdown, "声明" otherwise.
	Kind string
	// Line is 1-based, Col the 1-based rune of the name.
	Line int
	Col  int
	// Depth is 0 for a top-level symbol, 1 under a heading or a type.
	Depth int
}

// File lists the symbols of text, which is path's content.
func File(path, text string) []Symbol {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		if syms := goSymbols(path, text); len(syms) > 0 {
			return syms
		}
		return generic(text)
	case ".md", ".markdown":
		return markdown(text)
	default:
		return generic(text)
	}
}

// goSymbols parses a Go file's declarations.
func goSymbols(path, text string) []Symbol {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, text, parser.SkipObjectResolution)
	if err != nil && f == nil {
		return nil
	}
	var out []Symbol
	add := func(name, kind string, pos token.Pos) {
		p := fset.Position(pos)
		out = append(out, Symbol{Name: name, Kind: kind, Line: p.Line, Col: p.Column})
	}
	if f != nil {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				kind := "函数"
				name := d.Name.Name
				if d.Recv != nil && len(d.Recv.List) > 0 {
					kind = "方法"
					if t := typeName(d.Recv.List[0].Type); t != "" {
						name = t + "." + d.Name.Name
					}
				}
				add(name, kind, d.Name.Pos())
			case *ast.GenDecl:
				switch d.Tok {
				case token.TYPE:
					for _, sp := range d.Specs {
						if ts, ok := sp.(*ast.TypeSpec); ok {
							add(ts.Name.Name, "类型", ts.Name.Pos())
						}
					}
				case token.VAR, token.CONST:
					kind := "变量"
					if d.Tok == token.CONST {
						kind = "常量"
					}
					for _, sp := range d.Specs {
						if vs, ok := sp.(*ast.ValueSpec); ok {
							for _, n := range vs.Names {
								add(n.Name, kind, n.Pos())
							}
						}
					}
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}

func typeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return typeName(t.X)
	case *ast.IndexListExpr:
		return typeName(t.X)
	}
	return ""
}

// markdown lists headings.
func markdown(text string) []Symbol {
	var out []Symbol
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level > 6 || level >= len(trimmed) || trimmed[level] != ' ' {
			continue
		}
		name := strings.TrimSpace(trimmed[level:])
		if name == "" {
			continue
		}
		out = append(out, Symbol{Name: name, Kind: "标题", Line: i + 1, Col: 1, Depth: level - 1})
	}
	return out
}

var declLine = regexp.MustCompile(`^(?:export\s+)?(?:pub\s+)?(func|class|function|def|interface|type|const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)`)

// generic lists declarations that start a line, for languages without a
// parser here.
func generic(text string) []Symbol {
	var out []Symbol
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if m := declLine.FindStringSubmatch(trimmed); m != nil {
			out = append(out, Symbol{Name: m[2], Kind: "声明", Line: i + 1, Col: 1})
		}
	}
	return out
}
