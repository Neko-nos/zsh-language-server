package lsp

import (
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

type pathValues struct {
	filename string
	values   map[string][]string
	locals   map[string]bool
	fpathSet bool
}

func (p *pathValues) set(name string, values []string) {
	p.values[name] = values
	if name == "fpath" {
		p.fpathSet = true
	}
}

func newPathValues(uri string) *pathValues {
	filename, _ := filePath(uri)
	home, _ := os.UserHomeDir()
	return &pathValues{filename: filename, values: map[string][]string{
		"HOME": {home}, "0": {filename},
		"IFS": {" \t\n"},
		// An unknown entry could take precedence over a match in a later directory.
		"fpath": {""},
	}}
}

func (p *pathValues) assign(node *syntax.Assign, stack []syntax.Node) {
	if node.Name == nil {
		return
	}
	name := node.Name.Value
	if name == "FPATH" {
		name = "fpath"
	}
	inFunction := false
	for _, parent := range stack {
		if _, ok := parent.(*syntax.FuncDecl); ok {
			inFunction = true
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i].(type) {
		case *syntax.FuncDecl, *syntax.Subshell, *syntax.CmdSubst:
			stack = stack[i+1:]
			i = 0
		}
	}
	if decl, ok := stack[len(stack)-1].(*syntax.DeclClause); ok && (decl.Variant.Value == "local" || decl.Variant.Value == "typeset" || decl.Variant.Value == "declare") && !strings.ContainsAny(declarationOptions(decl), "gpf") {
		if node.Naked {
			if _, known := p.values[name]; !known || inFunction && !p.locals[name] {
				p.set(name, []string{""})
			}
		}
		if inFunction {
			p.locals[name] = true
		}
	}
	if node.Naked {
		return
	}
	for _, parent := range stack {
		switch parent := parent.(type) {
		case *syntax.CallExpr:
			if len(parent.Args) > 0 {
				return
			}
		case *syntax.IfClause:
			// Directory guards are common around fpath; other conditions need runtime state.
			if directory := p.guardedDirectory(parent); directory != "" {
				info, err := os.Stat(directory)
				if err != nil || !info.IsDir() {
					return
				}
			}
		}
	}
	var values []string
	if node.Index == nil {
		if node.Array != nil {
			for _, elem := range node.Array.Elems {
				part := p.word(elem.Value)
				if elem.Index != nil || len(part) == 0 {
					part = []string{""}
				}
				values = append(values, part...)
			}
		} else {
			values = p.word(node.Value)
		}
	}
	if node.Name.Value == "FPATH" && len(values) == 1 {
		values = strings.Split(values[0], ":")
	}
	if node.Append {
		if node.Array != nil {
			values = append(p.values[name], values...)
		} else if previous := p.values[name]; len(previous) == 1 && len(values) == 1 {
			values[0] = previous[0] + values[0]
		} else {
			values = nil
		}
	}
	p.set(name, values)
}

func (p *pathValues) guardedDirectory(clause *syntax.IfClause) string {
	if clause.Else != nil || len(clause.Cond) != 1 || clause.Cond[0].Negated {
		return ""
	}
	test, ok := clause.Cond[0].Cmd.(*syntax.TestClause)
	if !ok {
		return ""
	}
	unary, ok := test.X.(*syntax.UnaryTest)
	if !ok || unary.Op != syntax.TsDirect {
		return ""
	}
	word, ok := unary.X.(*syntax.Word)
	if !ok {
		return ""
	}
	values := p.word(word)
	if len(values) != 1 || !filepath.IsAbs(values[0]) {
		return ""
	}
	return values[0]
}

func (p *pathValues) word(word *syntax.Word) []string {
	if word == nil {
		return []string{""}
	}
	if value := word.Lit(); strings.HasPrefix(value, "~/") {
		home := p.values["HOME"]
		if len(home) != 1 || !filepath.IsAbs(home[0]) {
			return nil
		}
		return p.part(&syntax.Lit{Value: filepath.Join(home[0], value[2:])})
	}
	zsh, dynamic := false, dynamicPattern(word)
	syntax.Walk(word, func(n syntax.Node) bool {
		switch node := n.(type) {
		case *syntax.ParamExp:
			zsh = zsh || node.Flags != nil || node.NestedParam != nil || len(node.Modifiers) > 0 || node.Index != nil || node.Split != syntax.OptUnset || node.GlobSubst != syntax.OptUnset || node.RcExpand != syntax.OptUnset
		case *syntax.CmdSubst, *syntax.ProcSubst:
			dynamic = true
		}
		return !dynamic
	})
	if dynamic {
		return nil
	}
	if zsh {
		return p.parts(word.Parts)
	}
	parts := append([]syntax.WordPart(nil), word.Parts...)
	for i, part := range parts {
		if _, ok := part.(*syntax.Lit); ok {
			value, ok := literalWord(&syntax.Word{Parts: []syntax.WordPart{part}})
			if !ok {
				return nil
			}
			parts[i] = &syntax.SglQuoted{Value: value}
		}
	}
	env := &pathEnvironment{values: p.values}
	value, err := expand.Literal(&expand.Config{Env: env}, &syntax.Word{Parts: parts})
	if err != nil || env.unknown {
		return nil
	}
	return []string{value}
}

type pathEnvironment struct {
	values  map[string][]string
	unknown bool
}

func (e *pathEnvironment) Get(name string) expand.Variable {
	values, known := e.values[name]
	if !known || values == nil || len(values) > 1 {
		e.unknown = true
		return expand.Variable{}
	}
	if len(values) == 0 {
		return expand.Variable{}
	}
	return expand.Variable{Set: true, Kind: expand.String, Str: values[0]}
}

func (e *pathEnvironment) Each(func(string, expand.Variable) bool) {}

func (p *pathValues) parts(parts []syntax.WordPart) []string {
	if len(parts) == 1 {
		return p.part(parts[0])
	}
	var result strings.Builder
	for _, part := range parts {
		values := p.part(part)
		if len(values) != 1 || values[0] == "" {
			return nil
		}
		result.WriteString(values[0])
	}
	return []string{result.String()}
}

func (p *pathValues) part(part syntax.WordPart) []string {
	switch node := part.(type) {
	case *syntax.Lit:
		value := node.Value
		if strings.ContainsAny(value, `\*?[]{}~`) {
			return nil
		}
		return []string{value}
	case *syntax.SglQuoted:
		if !node.Dollar {
			return []string{node.Value}
		}
	case *syntax.DblQuoted:
		return p.parts(node.Parts)
	case *syntax.ParamExp:
		if node.Flags != nil && node.Flags.Value == "%" && node.Param == nil && node.Exp != nil && node.Exp.Word != nil && node.Exp.Op == syntax.DefaultUnsetOrNull && (node.Exp.Word.Lit() == "%N" || node.Exp.Word.Lit() == "%x") {
			return []string{p.filename}
		}
		if node.Flags != nil || node.Excl || node.Length || node.Width || node.IsSet || node.Split != syntax.OptUnset || node.GlobSubst != syntax.OptUnset || node.RcExpand != syntax.OptUnset || node.Slice != nil || node.Repl != nil || node.Names != 0 || node.Exp != nil {
			return nil
		}
		if node.Index != nil {
			index, ok := node.Index.(*syntax.Word)
			if !ok || (index.Lit() != "@" && index.Lit() != "*") {
				return nil
			}
		}
		var values []string
		if node.Param != nil {
			values = p.values[node.Param.Value]
		} else if node.NestedParam != nil {
			values = p.part(node.NestedParam)
		}
		values = append([]string(nil), values...)
		for _, modifier := range node.Modifiers {
			for i, value := range values {
				if value == "" {
					continue
				}
				switch modifier.Value {
				case "A":
					if !filepath.IsAbs(value) {
						return nil
					}
					values[i], _ = filepath.EvalSymlinks(value)
				case "a":
					if !filepath.IsAbs(value) {
						return nil
					}
					values[i] = filepath.Clean(value)
				case "h":
					values[i] = filepath.Dir(value)
				case "t":
					values[i] = filepath.Base(value)
				default:
					return nil
				}
			}
		}
		return values
	}
	return nil
}

func autoloadName(name string) string {
	if name == "" || strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") || strings.Contains(name, "/") && !filepath.IsAbs(name) {
		return ""
	}
	return filepath.Base(name)
}

func (d *Document) autoloadPath(o occurrence) string {
	for _, scope := range []int{o.scope, 0} {
		entries := d.autoloads[o.name]
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if entry.scope == scope && (entry.start <= o.start || scope == 0 && o.scope != 0) {
				return entry.path
			}
		}
	}
	return ""
}

func (s *Server) autoloadDeclaration(d *Document, o occurrence) *declaration {
	path := d.autoloadPath(o)
	if path == "" {
		return nil
	}
	paths := []string{path}
	if !filepath.IsAbs(path) {
		paths = nil
		fpath := o.fpath
		if fpath == nil {
			fpath = d.fpath
		}
		for _, directory := range fpath {
			// Relative fpath entries depend on the shell's working directory.
			if !filepath.IsAbs(directory) {
				break
			}
			paths = append(paths, filepath.Join(directory, path))
		}
	}
	for _, path := range paths {
		doc := s.readDocument(fileURI(path))
		if doc == nil {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				return nil
			}
			continue
		}
		return functionFileDeclaration(doc, o.name, true)
	}
	return nil
}

func (s *Server) siblingDeclaration(d *Document, o occurrence) *declaration {
	fpath := o.fpath
	if fpath == nil {
		fpath = d.fpath
	}
	if len(fpath) != 1 || fpath[0] != "" {
		return nil
	}
	target := d.autoloadPath(o)
	if target != "" && target != o.name {
		return nil
	}
	filename, err := filePath(d.URI)
	if err != nil || filepath.Base(o.name) != o.name {
		return nil
	}
	filename, err = filepath.EvalSymlinks(filename)
	if err != nil || functionFileDeclaration(d, filepath.Base(filename), false) == nil {
		return nil
	}
	// A file named after its function can rely on helper autoloads set up by
	// its caller. Only inspect the matching sibling, after explicit paths fail.
	doc := s.readDocument(fileURI(filepath.Join(filepath.Dir(filename), o.name)))
	if doc == nil {
		return nil
	}
	return functionFileDeclaration(doc, o.name, target == o.name)
}

func functionFileDeclaration(doc *Document, name string, autoload bool) *declaration {
	for _, o := range doc.occurrences {
		if o.declaration && o.kind == "function" && o.name == name {
			return &declaration{doc, o}
		}
	}
	if !autoload {
		return nil
	}
	// Zsh also accepts an autoload file containing only the function body.
	o := occurrence{name: name, kind: "function", declaration: true}
	if doc.tree != nil && len(doc.tree.Stmts) > 0 {
		stmt := doc.tree.Stmts[0]
		o.start, o.end, o.whole = int(stmt.Pos().Offset()), int(stmt.End().Offset()), doc.nodeRange(stmt)
	}
	return &declaration{doc, o}
}
