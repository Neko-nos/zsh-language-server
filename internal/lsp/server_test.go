package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLiteralSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.zsh")
	if err := os.WriteFile(path, []byte("helper() { print -r -- example; }\nsource ./main.zsh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.zsh")
	text := "source './library.zsh'\nhelper\n"
	if err := os.WriteFile(main, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.roots = []string{dir}
	d := Parse(TextDocument{URI: fileURI(main), Text: text})
	defs := s.definitions(d, Position{1, 1})
	if len(defs) != 1 || defs[0].URI != fileURI(path) {
		t.Fatalf("source definition: %+v", defs)
	}
	s.documents[fileURI(path)] = Parse(TextDocument{URI: fileURI(path), Version: 2, Text: "\nhelper() { print -r -- unsaved; }\n"})
	defs = s.definitions(d, Position{1, 1})
	if len(defs) != 1 || defs[0].Range.Start.Line != 1 {
		t.Fatalf("unsaved source: %+v", defs)
	}
}

func TestSourceParameterScopes(t *testing.T) {
	dir := t.TempDir()
	library := filepath.Join(dir, "shared.zsh")
	if err := os.WriteFile(library, []byte("label=shared\n# Sample\nhelper() { local label=private; print -r -- $label; }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	d := Parse(TextDocument{URI: fileURI(filepath.Join(dir, "main.zsh")), Text: "source './shared.zsh'\ncaller() { print -r -- $label; }\n"})
	defs := s.definitions(d, Position{1, 24})
	if len(defs) != 1 || defs[0].URI != fileURI(library) || defs[0].Range.Start != (Position{0, 0}) {
		t.Fatalf("global source parameter: %+v", defs)
	}
}
