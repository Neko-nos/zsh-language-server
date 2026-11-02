package lsp

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func (s *Server) styleDocumentation(words []*syntax.Word, index int) string {
	args, options, ok := commandArgs(words, "eabsTtmgdL")
	if !ok {
		return ""
	}
	if options == "g" && len(args) > 0 {
		args = args[1:]
	}
	if len(args) < 2 {
		return ""
	}
	operand := slices.Index(args, words[index])
	if operand < 0 {
		return ""
	}
	context, _ := literalWord(args[0])
	style, _ := literalWord(args[1])
	page, prefix, format, chapter, label := "", "", "", "", ""
	switch {
	case strings.HasPrefix(context, ":completion:"):
		page, chapter, label = "zshcompsys", "Completion-System", "Completion"
		format = ":completion:function:completer:command:argument:tag"
	case strings.HasPrefix(context, ":vcs_info:"):
		page, chapter, label = "zshcontrib", "User-Contributions", "Version control"
		prefix = "vcs_info:"
		format = ":vcs_info:vcs-string:user-context:repo-root-name"
	case strings.HasPrefix(context, ":chpwd:"):
		page, chapter, label = "zshcontrib", "User-Contributions", "Recent directory"
		prefix, format = "chpwd:", ":chpwd:"
	default:
		return ""
	}
	if operand == 0 {
		return label + " context pattern. Zsh matches this pattern against `" + format +
			"` to decide where the style applies. `*` can match across multiple colon-separated fields. " +
			"More specific matching patterns take precedence.\n\n" +
			"[Zsh manual](https://zsh.sourceforge.io/Doc/Release/" + chapter + ".html)"
	}
	if operand > 1 {
		value, _ := literalWord(words[index])
		switch options {
		case "e", "a", "b", "s", "g", "L":
			// Evaluated code and output parameters are not literal style values.
			return ""
		case "d":
			style = value
		}
		if page == "zshcompsys" && style == "completer" {
			function, _, _ := strings.Cut(value, ":")
			if strings.HasPrefix(function, "_") {
				if help := s.shellDocumentation(page, function); help != "" {
					return help
				}
			}
		}
	}
	// Style values have meaning in the context of the style that owns them.
	if help := s.shellDocumentation(page, prefix+style); help != "" {
		return help
	}
	if page == "zshcompsys" {
		return s.shellDocumentation("zshcontrib", "chpwd:"+style)
	}
	return ""
}
