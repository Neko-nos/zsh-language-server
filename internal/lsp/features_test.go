package lsp

import (
	"strings"
	"testing"
)

func TestFunctionArgumentReferences(t *testing.T) {
	for _, test := range []struct {
		command string
		refs    int
	}{
		{"unset -f paint_sample", 1},
		{"'paint_sample'", 1},
		{"noglob paint_sample", 1},
		{"add-zsh-hook -Uz precmd paint_sample", 1},
		{"add-zsh-hook -L paint_sample", 0},
		{"add-zsh-hook -D precmd paint_sample", 0},
		{"zle -N example-widget paint_sample", 1},
		{"zle -N paint_sample", 1},
		{"zle -D paint_sample", 0},
		{"typeset -f paint_sample", 1},
		{"command paint_sample", 0},
		{"unset -vf -- paint_sample paint_sample", 2},
		{"unset -f 'paint_sample'", 1},
		{"unfunction paint_sample", 1},
		{"compdef paint_sample demo", 1},
		{"compdef -an 'paint_sample' demo", 1},
		{"compdef -k paint_sample complete-word '^X'", 1},
		{"compdef -K paint_sample sample-widget complete-word '^X'", 1},
		{"compdef paint_sample paint_sample", 1},
		{"compdef -d paint_sample", 0},
		{"compdef another_handler paint_sample", 0},
		{"compdef paint_sample=another_service", 0},
		{"unset -v paint_sample", 0},
		{"print paint_sample", 0},
	} {
		t.Run(test.command, func(t *testing.T) {
			d := doc("paint_sample() { :; }\n" + test.command + "\n")
			if d.err != nil {
				t.Fatal(d.err)
			}
			refs := d.references(Position{0, 3}, false)
			if len(refs) != test.refs {
				t.Fatalf("references: %+v", refs)
			}
			for _, ref := range refs {
				defs := NewServer().definitions(d, ref.Range.Start)
				if len(defs) != 1 || defs[0].Range.Start != (Position{0, 0}) {
					t.Fatalf("definition: %+v", defs)
				}
			}
		})
	}
}

func TestHoverCommentLayout(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		text := strings.Join([]string{
			"#!/usr/bin/env zsh",
			"########################",
			"# Draw a dummy label.",
			"#",
			"# Arguments:",
			"#   1: Label, or `--` for the default.",
			"# Outputs:",
			"#   Writes the `label` in **bold**.",
			"# Examples:",
			"#   ```zsh",
			"#   paint_sample 'dummy label'",
			"#   ```",
			"########################",
			"paint_sample() { :; }",
			"paint_sample",
		}, newline)
		d := doc(text)
		hover := NewServer().hover(d, d.Position(strings.LastIndex(text, "paint_sample")))
		want := "```zsh\npaint_sample() { :; }\n```\n\n" +
			"Draw a dummy label.\n\n\n### Arguments\n\n" +
			"- `1`: Label, or `--` for the default.\n\n### Outputs\n\n" +
			"Writes the `label` in **bold**.\n\n### Examples\n\n```zsh\npaint_sample 'dummy label'\n```"
		if hover == nil || hover.Contents.Kind != "markdown" || hover.Contents.Value != want {
			t.Fatalf("hover: %+v", hover)
		}
	}
}

func TestFunctionRenameNames(t *testing.T) {
	o := occurrence{kind: "function"}
	for _, name := range []string{"paint_tile", "paint-tile", "tile.paint", "tile::paint"} {
		if !o.renameName(name) {
			t.Errorf("valid function name %q rejected", name)
		}
	}
	for _, name := range []string{"", "paint tile", "paint;tile", "paint\ntile", "paint*", "paint$(tile)", "paint\\ tile", "'paint'"} {
		if o.renameName(name) {
			t.Errorf("nonliteral function name %q accepted", name)
		}
	}
}
