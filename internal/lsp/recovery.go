package lsp

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

// A syntax unsupported by shfmt must not hide the rest of a valid script.
// Use Zsh's parser to find complete statements; splitting at arbitrary newlines
// could mistake heredoc or string contents for code. No script is executed.
func (d *Document) recoverSyntax() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	text := []byte(d.Text)
	valid := func(end int) bool {
		command := exec.CommandContext(ctx, "zsh", "-dfn", "-o", "NO_EQUALS")
		command.Stdin = bytes.NewReader(text[:end])
		return command.Run() == nil
	}
	if !valid(len(text)) {
		return
	}
	err := d.err
	for err != nil && ctx.Err() == nil {
		var parseErr syntax.ParseError
		var langErr syntax.LangError
		var offset int
		switch {
		case errors.As(err, &parseErr):
			offset = int(parseErr.Pos.Offset())
		case errors.As(err, &langErr):
			offset = int(langErr.Pos.Offset())
		default:
			return
		}
		line := d.Position(min(offset, max(0, len(text)-1))).Line
		start := d.lines[line]
		for start > 0 && !valid(start) && ctx.Err() == nil {
			line--
			start = d.lines[line]
		}
		end := d.Offset(Position{d.Position(offset).Line + 1, 0})
		for end < len(text) && !valid(end) && ctx.Err() == nil {
			end = d.Offset(Position{d.Position(end).Line + 1, 0})
		}
		if ctx.Err() != nil || start >= end {
			return
		}
		for i := start; i < end; i++ {
			if text[i] != '\n' && text[i] != '\r' {
				text[i] = ' '
			}
		}
		d.skipped = append(d.skipped, d.span(start, end))
		d.tree, err = syntax.NewParser(syntax.Variant(syntax.LangZsh), syntax.KeepComments(true)).Parse(bytes.NewReader(text), "")
	}
}
