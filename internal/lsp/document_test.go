package lsp

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"
)

func doc(text string) *Document {
	return Parse(TextDocument{URI: "file:///dummy/example.zsh", Version: 1, Text: text})
}

func TestParameterBindings(t *testing.T) {
	d := doc("value=global\nfirst() {\n local value=local\n print -r -- ${value}\n}\nsecond() {\n local value=other\n print -r -- $value\n}\nprint -r -- $value\n")
	s := NewServer()
	for _, test := range []struct{ line, definition, refs int }{{3, 2, 2}, {7, 6, 2}, {9, 0, 2}} {
		position := Position{test.line, 15}
		if test.line == 3 {
			position.Character = 16
		}
		definitions := s.definitions(d, position)
		if len(definitions) != 1 || definitions[0].Range.Start.Line != test.definition {
			t.Fatalf("line %d: definitions %+v", test.line, definitions)
		}
		refs := d.references(position, true)
		if len(refs) != test.refs {
			t.Fatalf("line %d: references %+v", test.line, refs)
		}
	}
}

func TestUTF16PositionsAndCRLF(t *testing.T) {
	d := doc("print -r -- '😀日本語'; sample=value\r\nprint -r -- $sample\r\n")
	offset := strings.Index(d.Text, "sample=value")
	position := d.Position(offset)
	if position != (Position{0, 21}) {
		t.Fatalf("UTF-16 position: %+v", position)
	}
	if actual := d.Offset(position); actual != offset {
		t.Fatalf("offset %d != %d", actual, offset)
	}
	refs := d.references(Position{1, 14}, true)
	if len(refs) != 2 || refs[0].Range.Start != position {
		t.Fatalf("references: %+v", refs)
	}
}

func TestIncompleteInput(t *testing.T) {
	for _, text := range []string{"hello() {\n", "print \"${", "if true; then\n", "for item in a; do", "typeset -A sample=(", "print $(", "() {", "case $value in", "print '😀'; if"} {
		t.Run(text, func(t *testing.T) {
			d := doc(text)
			if len(d.Diagnostics()) == 0 {
				t.Fatal("expected a syntax diagnostic")
			}
		})
	}
}

func TestFunctionsAndArithmetic(t *testing.T) {
	d := doc("# A sample helper.\nfunction first second {\n local count=1\n (( count += 1 ))\n print -r -- $count\n}\nfirst\nsecond\n")
	if d.err != nil {
		t.Fatal(d.err)
	}
	s := NewServer()
	for _, line := range []int{6, 7} {
		defs := s.definitions(d, Position{line, 1})
		if len(defs) != 1 || defs[0].Range.Start.Line != 1 {
			t.Fatalf("definition: %+v", defs)
		}
		if s.hover(d, Position{line, 1}) == nil {
			t.Fatal("missing hover")
		}
	}
	refs := d.references(Position{4, 15}, true)
	if len(refs) != 3 {
		t.Fatalf("arithmetic references: %+v", refs)
	}
}

func TestCompletionContexts(t *testing.T) {
	d := doc("shared=value\nfirst() {\n local private_value=example\n print -r -- ${private_value}\n}\nprint -r -- ${shared}\n# pri\nprint 'pri'\n")
	s := NewServer()
	for _, test := range []struct {
		position Position
		label    string
	}{
		{Position{3, 20}, "private_value"},
		{Position{5, 18}, "shared"},
	} {
		found := false
		for _, item := range s.completion(d, test.position) {
			if item.Label == test.label {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s completion at %+v", test.label, test.position)
		}
	}
	for _, position := range []Position{{6, 5}, {7, 9}} {
		if items := s.completion(d, position); len(items) != 0 {
			t.Fatalf("literal completion at %+v: %+v", position, items)
		}
	}
	for _, item := range s.completion(d, Position{5, 14}) {
		if item.Label == "private_value" {
			t.Fatal("private parameter suggested outside its function")
		}
	}
	incomplete := doc("colors=(blue green)\nprint -r -- ${(U)col")
	found := false
	for _, item := range s.completion(incomplete, incomplete.Position(len(incomplete.Text))) {
		if item.Label == "colors" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing completion inside incomplete Zsh expansion")
	}
}

func TestCompletionReplacesWholeName(t *testing.T) {
	for _, sample := range []struct {
		text     string
		position Position
		label    string
	}{
		{"show_colors() { print example; }\nshow_colors\n", Position{1, 4}, "show_colors"},
		{"colors=(blue green)\nprint -r -- ${colors}\n", Position{1, 17}, "colors"},
	} {
		d := doc(sample.text)
		found := false
		for _, item := range NewServer().completion(d, sample.position) {
			if item.Label != sample.label {
				continue
			}
			found = true
			edit := item.TextEdit
			actual := d.Text[:d.Offset(edit.Range.Start)] + edit.NewText + d.Text[d.Offset(edit.Range.End):]
			if actual != sample.text {
				t.Fatalf("completion changed the name: %q", actual)
			}
		}
		if !found {
			t.Fatalf("missing completion for %s", sample.label)
		}
	}
}

func FuzzPositions(f *testing.F) {
	for _, text := range []string{"", "print ${(U)sample}", "function example() { local name=value; }", "typeset -A colors=(blue ocean)", "for item in a; do", "cat <<EOF\n$sample\nEOF\n", "() {", "print '😀日本語'"} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if !utf8.ValidString(text) {
			t.Skip()
		}
		d := doc(text)
		for _, start := range d.lines {
			line := strings.SplitN(d.Text[start:], "\n", 2)[0]
			line = strings.SplitN(line, "\r", 2)[0]
			for offset := range line {
				if actual := d.Offset(d.Position(start + offset)); actual != start+offset {
					t.Fatalf("UTF-16 round trip: offset %d became %d", start+offset, actual)
				}
			}
		}
	})
}

func TestNativeSyntaxDiagnostics(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("requires Zsh")
	}
	t.Chdir(t.TempDir())
	for _, sample := range []struct {
		text   string
		errors int
	}{
		{"repeat 3 { print example }\n", 0},
		{"for item (one two) { print -r -- $item }\n", 0},
		{"function\n", 0},
		{"{ print before } always { print after }\n", 0},
		{"repeat 3 { print example }\nprint \"\n", 1},
		{"function example {\n", 1},
		{"{ print before } always { print after }\nprint marker > executed\nexport ${generated_name}=\"$(print marker > substituted)\"\n", 0},
	} {
		t.Run(sample.text, func(t *testing.T) {
			diagnostics := doc(sample.text).Diagnostics()
			if len(diagnostics) != sample.errors {
				t.Fatalf("diagnostics: %+v", diagnostics)
			}
			if len(diagnostics) > 0 && diagnostics[0].Source != "zsh" {
				t.Fatalf("expected native diagnostic: %+v", diagnostics)
			}
		})
	}
	for _, name := range []string{"executed", "substituted"} {
		if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("syntax checking executed the script: %s (%v)", name, err)
		}
	}
}
