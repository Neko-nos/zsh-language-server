package lsp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/pattern"
	"mvdan.cc/sh/v3/syntax"
)

var identifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z_0-9]*$`)
var parameterPrefix = regexp.MustCompile(`\$\{(?:\([^)]*\))?[#^~=+]*[a-zA-Z_0-9]*$`)
var zshErrorLine = regexp.MustCompile(`(?m)^.*?:([0-9]+): `)

type occurrence struct {
	name           string
	kind           string
	start, end     int
	scope          int
	declaration    bool
	local          bool
	associative    bool
	implicitWidget bool
	whole          Range
	fpath          []string
}

type autoload struct {
	path         string
	scope, start int
}

type source struct {
	path        string
	range_      Range
	directories []string
}

type Document struct {
	TextDocument
	tree        *syntax.File
	err         error
	lines       []int
	occurrences []occurrence
	sources     []source
	folds       []FoldingRange
	autoloads   map[string][]autoload
	fpath       []string
	skipped     []Range
}

func Parse(doc TextDocument) *Document {
	d := &Document{TextDocument: doc, lines: []int{0}}
	for i, c := range doc.Text {
		if c == '\n' {
			d.lines = append(d.lines, i+1)
		}
	}
	d.tree, d.err = syntax.NewParser(syntax.Variant(syntax.LangZsh), syntax.KeepComments(true)).Parse(strings.NewReader(doc.Text), "")
	if d.err != nil {
		d.recoverSyntax()
	}
	if d.tree != nil {
		d.index()
	}
	return d
}

// Position converts the parser's byte offsets to the UTF-16 units used by editors.
func (d *Document) Position(offset int) Position {
	offset = min(max(offset, 0), len(d.Text))
	line := sort.Search(len(d.lines), func(i int) bool { return d.lines[i] > offset }) - 1
	column := 0
	for _, r := range d.Text[d.lines[line]:offset] {
		column++
		if r > 0xffff {
			column++
		}
	}
	return Position{line, column}
}

func (d *Document) Offset(pos Position) int {
	if pos.Line >= len(d.lines) {
		return len(d.Text)
	}
	if pos.Line < 0 {
		return 0
	}
	start := d.lines[pos.Line]
	units := 0
	for i, r := range d.Text[start:] {
		if units >= pos.Character || r == '\n' || r == '\r' {
			return start + i
		}
		units++
		if r > 0xffff {
			units++
		}
	}
	return len(d.Text)
}

func (d *Document) span(start, end int) Range { return Range{d.Position(start), d.Position(end)} }

func (d *Document) nodeRange(n syntax.Node) Range {
	return d.span(int(n.Pos().Offset()), int(n.End().Offset()))
}

func (d *Document) index() {
	paths := newPathValues(d.URI)
	d.autoloads = map[string][]autoload{}
	skipped := 0
	var stack []syntax.Node
	var pathStack []*pathValues
	syntax.Walk(d.tree, func(n syntax.Node) bool {
		if n == nil {
			previous := pathStack[len(pathStack)-1]
			switch stack[len(stack)-1].(type) {
			case *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause, *syntax.CaseClause, *syntax.CaseItem, *syntax.BinaryCmd:
				if paths != previous {
					// A conditional write is known inside its branch, but may not happen.
					for name, value := range paths.values {
						if !slices.Equal(previous.values[name], value) || (previous.values[name] == nil) != (value == nil) {
							previous.set(name, nil)
						}
					}
					previous.fpathSet = previous.fpathSet || paths.fpathSet
				}
			}
			paths = previous
			pathStack = pathStack[:len(pathStack)-1]
			stack = stack[:len(stack)-1]
			return true
		}
		for skipped < len(d.skipped) && int(n.Pos().Offset()) >= d.Offset(d.skipped[skipped].End) {
			// Unsupported statements may change the values used to resolve paths.
			paths.values = newPathValues(d.URI).values
			paths.fpathSet = true
			skipped++
		}
		pathStack = append(pathStack, paths)
		fork := false
		switch n.(type) {
		case *syntax.FuncDecl, *syntax.Subshell, *syntax.CmdSubst,
			*syntax.ForClause, *syntax.WhileClause, *syntax.CaseClause, *syntax.CaseItem, *syntax.BinaryCmd:
			fork = true
		case *syntax.IfClause:
			fork = paths.guardedDirectory(n.(*syntax.IfClause)) == ""
		}
		if fork {
			base := paths
			if len(stack) > 0 {
				switch parent := stack[len(stack)-1].(type) {
				case *syntax.IfClause:
					if parent.Else == n {
						base = pathStack[len(stack)-1]
					}
				case *syntax.CaseClause:
					base = pathStack[len(stack)-1]
				}
			}
			paths = &pathValues{filename: base.filename, values: maps.Clone(base.values), locals: maps.Clone(base.locals), fpathSet: base.fpathSet}
			if _, function := n.(*syntax.FuncDecl); function {
				paths.locals = map[string]bool{}
				// The global search path may be set after the function is declared.
				paths.fpathSet = false
			}
		}
		scope := 0
		for _, parent := range stack {
			if fn, ok := parent.(*syntax.FuncDecl); ok {
				scope = int(fn.Pos().Offset()) + 1
			}
		}
		add := func(lit *syntax.Lit, kind string, declaration, local bool, whole syntax.Node) {
			if lit == nil || lit.Value == "" {
				return
			}
			d.occurrences = append(d.occurrences, occurrence{
				name: lit.Value, kind: kind, start: int(lit.Pos().Offset()), end: int(lit.End().Offset()),
				scope: scope, declaration: declaration, local: local, whole: d.nodeRange(whole),
			})
		}
		switch node := n.(type) {
		case *syntax.FuncDecl:
			add(node.Name, "function", true, false, node)
			for _, name := range node.Names {
				add(name, "function", true, false, node)
			}
		case *syntax.Assign:
			paths.assign(node, stack)
			local, declaration, associative := false, true, false
			kind := "variable"
			if len(stack) > 0 {
				if decl, ok := stack[len(stack)-1].(*syntax.DeclClause); ok {
					options := declarationOptions(decl)
					declaration = !node.Naked || !strings.ContainsAny(options, "pf")
					if strings.Contains(options, "f") {
						kind = "function"
					}
					associative = strings.Contains(options, "A")
					local = scope != 0 && (decl.Variant.Value == "local" || decl.Variant.Value == "typeset" || decl.Variant.Value == "declare")
					local = local && declaration && !strings.Contains(options, "g")
				}
			}
			add(node.Name, kind, declaration, local, node)
			if node.Name != nil && associative {
				d.occurrences[len(d.occurrences)-1].associative = true
			}
		case *syntax.WordIter:
			add(node.Name, "variable", true, false, node)
			paths.set(node.Name.Value, nil)
			if len(node.Items) == 1 {
				paths.set(node.Name.Value, paths.word(node.Items[0]))
			}
		case *syntax.ParamExp:
			add(node.Param, "variable", false, false, node)
		case *syntax.CallExpr:
			referenceCall := functionCall(node)
			command := ""
			if len(referenceCall.Args) > 0 {
				command, _ = literalWord(referenceCall.Args[0])
			}
			addWord := func(word *syntax.Word, name, kind string) {
				if name == "" {
					return
				}
				start, end := int(word.Pos().Offset()), int(word.End().Offset())
				// Keep surrounding quotes and absolute autoload paths intact during rename.
				if index := strings.LastIndex(d.Text[start:end], name); index >= 0 {
					start += index
					end = start + len(name)
				}
				d.occurrences = append(d.occurrences, occurrence{
					name: name, kind: kind, start: start, end: end, scope: scope, whole: d.nodeRange(word),
				})
				if scope == 0 || paths.fpathSet {
					d.occurrences[len(d.occurrences)-1].fpath = append([]string{}, paths.values["fpath"]...)
				}
			}
			for _, word := range functionWords(referenceCall) {
				path, literal := literalWord(word)
				name := path
				if command == "autoload" {
					if values := paths.word(word); len(values) == 1 {
						path = values[0]
					}
					name = autoloadName(path)
					if name != "" {
						d.autoloads[name] = append(d.autoloads[name], autoload{path, scope, int(word.Pos().Offset())})
						if !literal {
							parts := word.Parts
							if len(parts) == 1 {
								if quoted, ok := parts[0].(*syntax.DblQuoted); ok {
									parts = quoted.Parts
								}
							}
							// Parameters in a computed path remain variable references.
							tail, ok := parts[len(parts)-1].(*syntax.Lit)
							if !ok || !strings.HasSuffix(tail.Value, "/"+name) {
								continue
							}
							word = &syntax.Word{Parts: []syntax.WordPart{tail}}
						}
					}
				}
				addWord(word, name, "function")
				if command == "zle" && name != "" {
					args, _, _ := commandArgs(referenceCall, "N")
					d.occurrences[len(d.occurrences)-1].implicitWidget = len(args) == 1
				}
			}
			if len(node.Args) > 0 {
				word := node.Args[0]
				name, _ := literalWord(word)
				addWord(word, name, "function")
				words, kind := prefixedWords(node)
				for _, word := range words {
					name, _ := literalWord(word)
					addWord(word, name, kind)
				}
				if name == "unset" {
					if args, options, ok := commandArgs(node, "fv"); ok && !strings.Contains(options, "f") {
						for _, arg := range args {
							name, _ := literalWord(arg)
							if identifier.MatchString(name) {
								addWord(arg, name, "variable")
								paths.set(name, []string{})
							}
						}
					}
				}
				if name == "read" {
					for _, arg := range node.Args[1:] {
						if name := arg.Lit(); identifier.MatchString(name) {
							paths.set(name, nil)
						}
					}
				}
				d.indexSource(node, stack, paths)
			}
		case *syntax.DeclClause:
			add(node.Variant, "function", false, false, node.Variant)
		case *syntax.Word:
			// Bare names in arithmetic expressions are parameter references.
			if len(stack) > 0 {
				arithmetic := false
				array := ""
				switch parent := stack[len(stack)-1].(type) {
				case *syntax.ArithmCmd, *syntax.ArithmExp, *syntax.BinaryArithm, *syntax.UnaryArithm, *syntax.ParenArithm, *syntax.CStyleLoop, *syntax.LetClause:
					arithmetic = true
				case *syntax.ParamExp:
					if parent.Param != nil && parent.Index == node {
						array = parent.Param.Value
					}
				case *syntax.Assign:
					if parent.Name != nil && parent.Index == node {
						array = parent.Name.Value
					}
				}
				if array != "" {
					declarations := d.matching(occurrence{name: array, kind: "variable", scope: scope}, true)
					arithmetic = len(declarations) > 0
					for _, declaration := range declarations {
						if declaration.associative {
							arithmetic = false
						}
					}
				}
				if arithmetic {
					if len(node.Parts) == 1 {
						if lit, ok := node.Parts[0].(*syntax.Lit); ok && identifier.MatchString(lit.Value) {
							add(lit, "variable", false, false, node)
						}
					}
				}
			}
		}
		switch n.(type) {
		case *syntax.FuncDecl, *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause, *syntax.CaseClause, *syntax.Subshell, *syntax.ArrayExpr:
			r := d.nodeRange(n)
			if r.End.Line > r.Start.Line {
				d.folds = append(d.folds, FoldingRange{r.Start.Line, r.End.Line - 1})
			}
		}
		stack = append(stack, n)
		return true
	})
	d.fpath = paths.values["fpath"]
}

func functionCall(call *syntax.CallExpr) *syntax.CallExpr {
	if len(call.Args) > 1 {
		command, _ := literalWord(call.Args[0])
		if command == "builtin" {
			builtin, _ := literalWord(call.Args[1])
			switch builtin {
			case "autoload", "unset", "unfunction", "zle":
				unwrapped := *call
				unwrapped.Args = call.Args[1:]
				return &unwrapped
			}
		}
	}
	return call
}

func functionWords(call *syntax.CallExpr) []*syntax.Word {
	if len(call.Args) == 0 {
		return nil
	}
	command, _ := literalWord(call.Args[0])
	allowed := ""
	switch command {
	case "autoload":
		allowed = "Uzkt"
	case "unset":
		allowed = "fv"
	case "unfunction":
	case "compdef":
		allowed = "anekK"
	case "add-zsh-hook":
		allowed = "dUzk"
	case "zle":
		allowed = "N"
	default:
		return nil
	}
	args, options, ok := commandArgs(call, allowed)
	if !ok {
		return nil
	}
	if command == "unset" && !strings.Contains(options, "f") {
		return nil
	}
	if command == "compdef" {
		return args[:min(1, len(args))]
	}
	if command == "add-zsh-hook" {
		if len(args) == 2 {
			return args[1:]
		}
		return nil
	}
	if command == "zle" {
		if options == "N" && len(args) >= 1 && len(args) <= 2 {
			return args[len(args)-1:]
		}
		return nil
	}
	return args
}

func commandArgs(call *syntax.CallExpr, allowed string) ([]*syntax.Word, string, bool) {
	args := call.Args[1:]
	options := ""
	for len(args) > 0 {
		option, _ := literalWord(args[0])
		if !strings.HasPrefix(option, "-") {
			break
		}
		args = args[1:]
		if option == "--" {
			break
		}
		option = strings.TrimPrefix(option, "-")
		if strings.Trim(option, allowed) != "" {
			return nil, "", false
		}
		options += option
	}
	return args, options, true
}

func declarationOptions(decl *syntax.DeclClause) string {
	options := ""
	for _, arg := range decl.Args {
		if arg.Name != nil || arg.Value == nil || arg.Value.Lit() == "--" {
			break
		}
		if option, ok := strings.CutPrefix(arg.Value.Lit(), "-"); ok {
			options += option
		}
	}
	return options
}

func literalWord(word *syntax.Word) (string, bool) {
	static := true
	syntax.Walk(word, func(n syntax.Node) bool {
		switch n.(type) {
		case nil, *syntax.Word, *syntax.Lit, *syntax.SglQuoted, *syntax.DblQuoted:
		default:
			static = false
		}
		return static
	})
	if !static || dynamicPattern(word) {
		return "", false
	}
	values, err := expand.Fields(nil, word)
	if err != nil || len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func dynamicPattern(word *syntax.Word) bool {
	dynamic := false
	syntax.Walk(word, func(n syntax.Node) bool {
		switch node := n.(type) {
		case *syntax.SglQuoted, *syntax.DblQuoted, *syntax.ParamExp:
			return false
		case *syntax.Lit:
			dynamic = dynamic || pattern.HasMeta(node.Value, 0) || strings.ContainsAny(node.Value, "{}~")
		}
		return !dynamic
	})
	return dynamic
}

func (d *Document) Diagnostics() []Diagnostic {
	diagnostics := []Diagnostic{}
	if d.err == nil {
		return diagnostics
	}
	// shfmt is experimental; let Zsh settle rejected syntax when it is installed.
	// NO_EQUALS prevents path expansion in dynamic declarations during the no-exec pass.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "zsh", "-dfn", "-o", "NO_EQUALS")
	command.Stdin = strings.NewReader(d.Text)
	output, err := command.CombinedOutput()
	if err == nil {
		return diagnostics
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && ctx.Err() == nil && len(output) > 0 {
		line := 0
		if match := zshErrorLine.FindSubmatch(output); match != nil {
			number, _ := strconv.Atoi(string(match[1]))
			line = min(max(number-1, 0), len(d.lines)-1)
		}
		return []Diagnostic{{Range{Position{line, 0}, d.Position(d.Offset(Position{line + 1, 0}))}, 1, "zsh", strings.TrimSpace(string(output))}}
	}
	var parseErr syntax.ParseError
	var langErr syntax.LangError
	pos := syntax.Pos{}
	message := d.err.Error()
	if errors.As(d.err, &parseErr) {
		pos = parseErr.Pos
		message = parseErr.Text
	}
	if errors.As(d.err, &langErr) {
		pos = langErr.Pos
	}
	start := min(int(pos.Offset()), len(d.Text))
	_, width := utf8.DecodeRuneInString(d.Text[start:])
	diagnostics = append(diagnostics, Diagnostic{d.span(start, min(start+width, len(d.Text))), 2, "shfmt", fmt.Sprintf("%s (Experimental Zsh parser; native syntax check unavailable.)", message)})
	return diagnostics
}

func (d *Document) at(pos Position) *occurrence {
	offset := d.Offset(pos)
	for i := range d.occurrences {
		o := &d.occurrences[i]
		if offset >= o.start && offset < o.end {
			return o
		}
	}
	return nil
}

func (d *Document) binding(o occurrence) int {
	if o.kind == "function" {
		return 0
	}
	for _, candidate := range d.occurrences {
		if candidate.name == o.name && candidate.local && candidate.scope == o.scope {
			return o.scope
		}
	}
	return 0
}

func (d *Document) matching(o occurrence, declarationsOnly bool) []occurrence {
	result := []occurrence{}
	binding := d.binding(o)
	for _, candidate := range d.occurrences {
		if candidate.name == o.name && candidate.kind == o.kind && d.binding(candidate) == binding && (!declarationsOnly || candidate.declaration) {
			result = append(result, candidate)
		}
	}
	return result
}

func (d *Document) completionContext(offset int, parameter bool) (int, bool) {
	scope, allowed := 0, true
	for _, skipped := range d.skipped {
		if offset >= d.Offset(skipped.Start) && offset < d.Offset(skipped.End) {
			return scope, false
		}
	}
	if d.tree == nil {
		return scope, allowed
	}
	syntax.Walk(d.tree, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		if offset < int(n.Pos().Offset()) || offset > int(n.End().Offset()) {
			return true
		}
		switch node := n.(type) {
		case *syntax.FuncDecl:
			scope = int(node.Pos().Offset()) + 1
		case *syntax.CallExpr:
			if !parameter && len(node.Args) > 0 {
				allowed = false
				words := append([]*syntax.Word{node.Args[0]}, functionWords(functionCall(node))...)
				if prefixed, kind := prefixedWords(node); kind == "function" {
					words = append(words, prefixed...)
				}
				for _, word := range words {
					if offset >= int(word.Pos().Offset()) && offset <= int(word.End().Offset()) {
						allowed = true
					}
				}
			}
		case *syntax.Redirect:
			if node.Hdoc != nil && offset >= int(node.Hdoc.Pos().Offset()) {
				allowed = parameter && node.Word.Lit() != ""
			} else if !parameter {
				allowed = false
			}
		case *syntax.Comment:
			allowed = false
		case *syntax.SglQuoted:
			if offset < int(node.End().Offset()) {
				allowed = false
			}
		case *syntax.DblQuoted:
			if !parameter && offset < int(node.End().Offset()) {
				allowed = false
			}
		}
		return true
	})
	return scope, allowed
}
