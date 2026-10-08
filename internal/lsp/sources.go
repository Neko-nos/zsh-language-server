package lsp

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func (d *Document) indexSource(call *syntax.CallExpr, stack []syntax.Node, paths *pathValues) {
	args := call.Args
	name, _ := literalWord(args[0])
	if name == "builtin" && len(args) > 1 {
		args = args[1:]
		name, _ = literalWord(args[0])
	}
	if (name != "source" && name != ".") || len(args) < 2 {
		return
	}
	arg := args[1]
	if arg.Lit() == "--" && len(args) > 2 {
		arg = args[2]
	}
	src := source{range_: d.nodeRange(arg)}
	if values := paths.word(arg); len(values) == 1 {
		src.path = values[0]
	}
	for _, parent := range stack {
		stmt, ok := parent.(*syntax.Stmt)
		// A comment before a pipeline/list applies to its first command.
		if !ok || stmt.Pos() != call.Pos() {
			continue
		}
		for _, comment := range stmt.Comments {
			if comment.Pos().Offset() >= call.Pos().Offset() {
				continue
			}
			text := strings.TrimSpace(comment.Text)
			rest, ok := strings.CutPrefix(text, "shellcheck ")
			if !ok {
				continue
			}
			for word, err := range syntax.NewParser().WordsSeq(strings.NewReader(rest)) {
				if err != nil {
					break
				}
				value, ok := literalWord(word)
				if !ok {
					continue
				}
				key, value, _ := strings.Cut(value, "=")
				switch key {
				case "source":
					src.path = value
				case "source-path":
					if value == "SCRIPTDIR" {
						value = filepath.Dir(paths.filename)
					}
					src.directories = append(src.directories, value)
				}
			}
		}
	}
	if src.path != "" && src.path != "/dev/null" {
		d.sources = append(d.sources, src)
	}
}
