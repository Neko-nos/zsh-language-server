package lsp

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type shellFlag struct {
	names    string
	word     *syntax.Word
	argument *syntax.Word
	value    string
}

func shellArguments(args []*syntax.Word, prefixes, takesValue string) ([]*syntax.Word, []shellFlag) {
	var flags []shellFlag
	for len(args) > 0 {
		word := args[0]
		value, literal := literalWord(word)
		if !literal || value == "" || !strings.ContainsRune(prefixes, rune(value[0])) {
			break
		}
		args = args[1:]
		if len(value) == 1 || value == "--" {
			break
		}
		flag := shellFlag{names: value[1:], word: word}
		if i := strings.IndexAny(flag.names, takesValue); i >= 0 {
			flag.value = flag.names[i+1:]
			flag.names = flag.names[:i+1]
			if flag.value != "" {
				flag.argument = word
			} else if len(args) > 0 {
				flag.argument = args[0]
				flag.value, _ = literalWord(args[0])
				args = args[1:]
			}
		}
		flags = append(flags, flag)
	}
	return args, flags
}

func builtinArgs(words []*syntax.Word) []*syntax.Word {
	for len(words) > 0 {
		name, _ := literalWord(words[0])
		if name != "builtin" && name != "noglob" && name != "nocorrect" {
			break
		}
		words = words[1:]
		if name == "builtin" && len(words) > 0 && words[0].Lit() == "--" {
			words = words[1:]
		}
	}
	return words
}

func (s *Server) argumentHover(d *Document, pos Position) *Hover {
	if d.tree == nil {
		return nil
	}
	offset := d.Offset(pos)
	var hover *Hover
	syntax.Walk(d.tree, func(n syntax.Node) bool {
		if hover != nil {
			return false
		}
		var words []*syntax.Word
		switch node := n.(type) {
		case *syntax.CallExpr:
			words = builtinArgs(node.Args)
		case *syntax.DeclClause:
			words = []*syntax.Word{{Parts: []syntax.WordPart{node.Variant}}}
			for _, arg := range node.Args {
				if arg.Name != nil || arg.Value == nil {
					break
				}
				words = append(words, arg.Value)
			}
		default:
			return true
		}
		if offset < int(n.Pos().Offset()) || offset >= int(n.End().Offset()) {
			return false
		}
		if len(words) > 0 {
			if command := d.at(d.Position(int(words[0].Pos().Offset()))); command != nil && len(s.declarations(d, *command)) > 0 {
				return true
			}
		}
		for i, word := range words {
			if i == 0 || offset < int(word.Pos().Offset()) || offset >= int(word.End().Offset()) {
				continue
			}
			if _, literal := literalWord(word); literal {
				if help := s.argumentDocumentation(words, i); help != "" {
					hover = &Hover{Markup{"markdown", help}, d.nodeRange(word)}
				}
			}
			break
		}
		return hover == nil
	})
	return hover
}

func (s *Server) builtinHelp(command string) string {
	page := "zshbuiltins"
	switch command {
	case "bindkey", "zle":
		page = "zshzle"
	case "compinit", "compdef":
		page = "zshcompsys"
	case "zstyle", "zparseopts", "zformat":
		page = "zshmodules"
	}
	return s.shellDocumentation(page, command)
}

func (s *Server) argumentDocumentation(words []*syntax.Word, index int) string {
	command, _ := literalWord(words[0])
	value, _ := literalWord(words[index])
	switch command {
	case "set", "setopt", "unsetopt", "emulate":
		return s.optionDocumentation(words, index)
	case "zstyle":
		if help := s.styleDocumentation(words, index); help != "" {
			return help
		}
	case "bindkey", "zle":
		return s.widgetDocumentation(words, index)
	case "zmodload":
		if strings.HasPrefix(value, "zsh/") {
			return s.shellDocumentation("zshmodules", value)
		}
	case "limit", "ulimit", "umask":
		return s.builtinHelp(command)
	}
	_, flags := shellArguments(words[1:], "-+", "")
	for _, flag := range flags {
		if flag.word == words[index] {
			switch command {
			case "declare", "local", "export", "readonly", "integer", "float":
				command = "typeset"
			}
			return s.builtinHelp(command)
		}
	}
	return ""
}

func (s *Server) optionDocumentation(words []*syntax.Word, index int) string {
	command, _ := literalWord(words[0])
	word := words[index]
	args, flags := shellArguments(words[1:], "-+", "oA")
	if command == "emulate" && len(args) > 0 {
		if word == args[0] {
			return s.builtinHelp(command)
		}
		var extra []shellFlag
		args, extra = shellArguments(args[1:], "-+", "oc")
		flags = append(flags, extra...)
	}
	for _, flag := range flags {
		// -oNAME is both an option and its argument in a single word.
		if flag.argument == word {
			if strings.HasSuffix(flag.names, "o") {
				return s.shellDocumentation("zshoptions", flag.value)
			}
			return ""
		}
		if flag.word == word {
			return s.builtinHelp(command)
		}
	}
	if command != "set" && command != "emulate" && slices.Contains(args, word) {
		name, _ := literalWord(word)
		return s.shellDocumentation("zshoptions", name)
	}
	return ""
}

func (s *Server) widgetDocumentation(words []*syntax.Word, index int) string {
	command, _ := literalWord(words[0])
	value, _ := literalWord(words[index])
	takesValue := ""
	if command == "bindkey" {
		takesValue = "M"
	}
	args, flags := shellArguments(words[1:], "-", takesValue)
	options := ""
	for _, flag := range flags {
		if flag.argument == words[index] {
			return s.shellDocumentation("zshzle", "keymaps")
		}
		if flag.word == words[index] {
			return s.builtinHelp(command)
		}
		options += flag.names
	}
	operand := slices.Index(args, words[index])
	if operand < 0 {
		return ""
	}
	widget := false
	if command == "bindkey" {
		if strings.ContainsAny(options, "ANDl") {
			return s.shellDocumentation("zshzle", "keymaps")
		}
		widget = !strings.ContainsAny(options, "sr") && operand%2 == 1
	} else {
		switch options {
		case "K":
			return s.shellDocumentation("zshzle", "keymaps")
		case "", "A":
			widget = operand == 0
		case "C":
			widget = operand == 1
		case "D":
			widget = true
		default:
			return ""
		}
	}
	if widget {
		if help := s.shellDocumentation("zshzle", strings.TrimPrefix(value, ".")); help != "" {
			return help
		}
	}
	if command == "bindkey" {
		return s.builtinHelp(command)
	}
	return ""
}
