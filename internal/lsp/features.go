package lsp

import (
	"regexp"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var commentSection = regexp.MustCompile(`^([A-Z][A-Za-z ]*):$`)
var commentItem = regexp.MustCompile(`^([A-Za-z_0-9.*-]+(?: \([^)]*\))?):[ \t]+(.*)$`)
var commentFence = regexp.MustCompile("^(`{3,}|~{3,})(.*)$")

func (d *Document) Symbols() []Symbol {
	result := []Symbol{}
	for _, o := range d.occurrences {
		if !o.declaration {
			continue
		}
		kind := 13
		if o.kind == "function" {
			kind = 12
		}
		result = append(result, Symbol{o.name, kind, o.whole, d.span(o.start, o.end)})
	}
	return result
}

func (s *Server) completion(d *Document, pos Position) []Completion {
	end := d.Offset(pos)
	start := end
	for start > 0 && parameterByte(d.Text[start-1]) {
		start--
	}
	variable := start > 0 && d.Text[start-1] == '$'
	if parameterPrefix.MatchString(d.Text[d.lines[d.Position(end).Line]:end]) {
		variable = true
	}
	if o := d.at(pos); o != nil && o.kind == "variable" {
		variable = true
	}
	if !variable {
		for start > 0 && nameByte(d.Text[start-1]) {
			start--
		}
	}
	replaceEnd := end
	for replaceEnd < len(d.Text) && (parameterByte(d.Text[replaceEnd]) || !variable && nameByte(d.Text[replaceEnd])) {
		replaceEnd++
	}
	if !variable && end > 0 {
		if o := d.at(d.Position(end - 1)); o != nil && o.kind == "function" {
			start, replaceEnd = o.start, o.end
		}
	}
	prefix := d.Text[start:end]
	scope, allowed := d.completionContext(end, variable)
	if !allowed {
		return []Completion{}
	}
	items := map[string]Completion{}
	add := func(name string, kind int, detail string, docs *Markup) {
		if strings.HasPrefix(name, prefix) {
			items[name] = Completion{name, kind, detail, docs, TextEdit{d.span(start, replaceEnd), name}}
		}
	}
	if !variable {
		for name, help := range s.builtins {
			add(name, 3, "Zsh builtin or standard function", builtinDocumentation(help))
		}
		for _, name := range strings.Fields("if then elif else fi for while until do done case esac function select repeat in time coproc") {
			add(name, 14, "Zsh keyword", nil)
		}
	}
	for _, doc := range s.related(d) {
		if !variable {
			for name := range doc.autoloads {
				o := occurrence{name: name, scope: scope, start: end}
				if doc != d {
					o.scope, o.start = 0, len(doc.Text)
				}
				if doc.autoloadPath(o) != "" {
					add(name, 3, "Autoloaded Zsh function", nil)
				}
			}
		}
		for _, o := range doc.occurrences {
			if !o.declaration {
				continue
			}
			if binding := doc.binding(o); binding != 0 && (doc != d || binding != scope) {
				continue
			}
			if variable && o.kind == "variable" {
				add(o.name, 6, "Zsh parameter", nil)
			}
			if !variable && o.kind == "function" {
				add(o.name, 3, "Zsh function", nil)
			}
		}
	}
	if variable {
		for _, name := range strings.Fields("HOME PATH path fpath FPATH ZDOTDIR PWD OLDPWD IFS REPLY reply status pipestatus argv argc ZSH_VERSION ZSH_NAME RANDOM SECONDS PROMPT RPROMPT PS1 PS2 MATCH match MBEGIN MEND mbegin mend") {
			add(name, 6, "Zsh special parameter", nil)
		}
	}
	result := make([]Completion, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Label < result[j].Label })
	return result
}

func nameByte(b byte) bool {
	return parameterByte(b) || b == '-'
}

func parameterByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

type declaration struct {
	doc *Document
	occurrence
}

func (s *Server) declarations(d *Document, o occurrence) []declaration {
	if o.kind == "builtin" || o.kind == "external" {
		return nil
	}
	result := []declaration{}
	binding := d.binding(o)
	documents := s.related(d)
	for _, doc := range documents {
		if doc != d && binding != 0 {
			continue
		}
		for _, candidate := range doc.occurrences {
			if !candidate.declaration || candidate.name != o.name || candidate.kind != o.kind {
				continue
			}
			if doc.binding(candidate) != binding {
				continue
			}
			result = append(result, declaration{doc, candidate})
		}
	}
	if len(result) == 0 && o.kind == "function" {
		for _, doc := range documents {
			context := o
			if doc != d {
				context.scope, context.start, context.fpath = 0, len(doc.Text), nil
			}
			if def := s.autoloadDeclaration(doc, context); def != nil {
				return []declaration{*def}
			}
		}
		if def := s.siblingDeclaration(d, o); def != nil {
			return []declaration{*def}
		}
	}
	return result
}

func (s *Server) definitions(d *Document, pos Position) []Location {
	result := []Location{}
	offset := d.Offset(pos)
	for _, src := range d.sources {
		if offset >= d.Offset(src.range_.Start) && offset < d.Offset(src.range_.End) {
			if uri := s.sourceURI(d, src); uri != "" {
				return []Location{{URI: uri}}
			}
		}
	}
	if o := d.at(pos); o != nil {
		for _, def := range s.declarations(d, *o) {
			result = append(result, Location{def.doc.URI, def.doc.span(def.start, def.end)})
		}
	}
	return result
}

func (s *Server) hover(d *Document, pos Position) *Hover {
	o := d.at(pos)
	if o == nil {
		return nil
	}
	value := ""
	if defs := s.declarations(d, *o); len(defs) > 0 {
		def := defs[0]
		lines := strings.Split(def.doc.Text, "\n")
		line := def.whole.Start.Line
		value = "```zsh\n" + strings.TrimSpace(lines[line]) + "\n```"
		start := line
		for start > 0 && strings.HasPrefix(strings.TrimSpace(lines[start-1]), "#") && !strings.HasPrefix(strings.TrimSpace(lines[start-1]), "#!") {
			start--
		}
		comments := []string{}
		for _, comment := range lines[start:line] {
			comment = strings.TrimSpace(comment)
			if len(comment) > 1 && strings.Trim(comment, "#") == "" {
				continue
			}
			comment = strings.TrimPrefix(strings.TrimPrefix(comment, "#"), " ")
			comments = append(comments, comment)
		}
		if len(comments) > 0 {
			value += "\n\n" + commentMarkdown(comments)
		}
	} else if o.kind == "function" || o.kind == "builtin" || o.kind == "external" {
		if help, ok := s.builtins[o.name]; ok && o.kind != "external" {
			value = builtinDocumentation(help).Value
		} else if o.kind != "builtin" {
			value = commandDocumentation(o.name)
		}
	}
	if value == "" {
		return nil
	}
	return &Hover{Markup{"markdown", value}, d.span(o.start, o.end)}
}

func commentMarkdown(comments []string) string {
	var result []string
	section, indent, fence := "", "", ""
	for i, line := range comments {
		if heading := commentSection.FindStringSubmatch(line); fence == "" && heading != nil {
			section, indent = heading[1], ""
			result = append(result, "", "### "+section, "")
			for _, body := range comments[i+1:] {
				if strings.TrimSpace(body) != "" {
					indent = body[:len(body)-len(strings.TrimLeft(body, " \t"))]
					break
				}
			}
			continue
		}
		line = strings.TrimPrefix(line, indent)
		if marker := commentFence.FindStringSubmatch(strings.TrimSpace(line)); marker != nil {
			if fence == "" {
				fence = marker[1]
			} else if strings.HasPrefix(marker[1], fence) && strings.TrimSpace(marker[2]) == "" {
				fence = ""
			}
		} else if fence == "" {
			switch section {
			case "Args", "Arguments", "Parameters", "Globals", "Returns", "Raises":
				if item := commentItem.FindStringSubmatch(line); item != nil {
					line = "- `" + item[1] + "`: " + item[2]
				}
			}
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func (d *Document) references(pos Position, includeDeclaration bool) []Location {
	result := []Location{}
	o := d.at(pos)
	if o == nil {
		return result
	}
	for _, candidate := range d.matching(*o, false) {
		if includeDeclaration || !candidate.declaration {
			result = append(result, Location{d.URI, d.span(candidate.start, candidate.end)})
		}
	}
	return result
}

func (d *Document) renameRange(pos Position) *Range {
	o := d.at(pos)
	if o == nil || !o.renameName(o.name) || len(d.matching(*o, true)) == 0 || d.err != nil {
		return nil
	}
	r := d.span(o.start, o.end)
	return &r
}

func (o occurrence) renameName(name string) bool {
	if o.kind != "function" {
		return identifier.MatchString(name)
	}
	// Function names need not be parameter identifiers; let the parser validate
	// that the name remains one literal word in both declarations and calls.
	tree, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(name+"() { :; }\n"+name), "")
	if err != nil || len(tree.Stmts) != 2 {
		return false
	}
	fn, function := tree.Stmts[0].Cmd.(*syntax.FuncDecl)
	call, command := tree.Stmts[1].Cmd.(*syntax.CallExpr)
	if !function || fn.Name == nil || fn.Name.Value != name || !command || len(call.Args) != 1 || call.Args[0].Lit() != name {
		return false
	}
	literal, ok := literalWord(call.Args[0])
	return ok && literal == name
}
