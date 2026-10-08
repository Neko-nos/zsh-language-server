package lsp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

var manFormatting = regexp.MustCompile(`.\x08|\x1b\[[0-?]*[ -/]*[@-~]`)

func prefixedWords(call *syntax.CallExpr) ([]*syntax.Word, string) {
	args := call.Args
	for len(args) > 0 {
		name, _ := literalWord(args[0])
		switch name {
		case "noglob", "nocorrect", "-":
			args = args[1:]
		case "exec":
			args = args[1:]
			for len(args) > 0 {
				option, ok := literalWord(args[0])
				if !ok {
					return nil, ""
				}
				if !strings.HasPrefix(option, "-") {
					break
				}
				args = args[1:]
				if option == "--" {
					break
				}
				if strings.Trim(option[1:], "cla") != "" {
					return nil, ""
				}
				if strings.Contains(option, "a") {
					if len(args) == 0 {
						return nil, ""
					}
					args = args[1:]
				}
			}
		case "builtin", "command":
			allowed, kind := "", "builtin"
			if name == "command" {
				allowed, kind = "pvV", "external"
			}
			words, options, ok := commandArgs(&syntax.CallExpr{Args: args}, allowed)
			if !ok {
				return nil, ""
			}
			if strings.ContainsAny(options, "vV") {
				return words, "function"
			}
			return words[:min(1, len(words))], kind
		default:
			if len(args) < len(call.Args) {
				return args[:1], "function"
			}
			return nil, ""
		}
	}
	return nil, ""
}

func commandDocumentation(name string) string {
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Read the installed manual without running the command or an interactive pager.
	cmd := exec.CommandContext(ctx, "man", "-P", "cat", "--", filepath.Base(name))
	cmd.Env = append(os.Environ(), "LC_ALL=C", "MANWIDTH=80", "MAN_KEEP_FORMATTING=")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(manFormatting.ReplaceAllString(string(output), ""))
	if text == "" {
		return ""
	}
	return "```text\n" + text + "\n```"
}
