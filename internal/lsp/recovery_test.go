package lsp

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNavigationAroundUnsupportedStatements(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("requires Zsh")
	}
	for _, statement := range []string{
		"repeat 2 { print sample }",
		"for item (amber blue) { print $item }",
		"{ print before } always { print after }",
		"repeat 2 {\ncat <<'TEXT'\nfictional() { :; }\nTEXT\n}",
		"{\nprint 'first\nfictional() { :; }\nlast'\n} always {\nprint done\n}",
		"container() {\nlocal value=sample\nrepeat 2 { print $value }\n}",
	} {
		t.Run(statement, func(t *testing.T) {
			text := "before() { :; }\n" + statement + "\nafter() { :; }\nbefore\nafter\n"
			d := doc(text)
			for _, name := range []string{"before", "after"} {
				position := d.Position(strings.LastIndex(text, name))
				defs := NewServer().definitions(d, position)
				want := d.Position(strings.Index(text, name+"()"))
				if len(defs) != 1 || defs[0].Range.Start != want {
					t.Fatalf("%s definition: %+v; want %+v", name, defs, want)
				}
			}
			for _, symbol := range d.Symbols() {
				if symbol.Name == "fictional" {
					t.Fatal("string or heredoc contents indexed as a function")
				}
			}
			if diagnostics := d.Diagnostics(); len(diagnostics) != 0 {
				t.Fatalf("valid syntax: %+v", diagnostics)
			}
		})
	}
}
