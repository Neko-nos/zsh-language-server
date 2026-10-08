package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourcePaths(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"app", "helpers", "external"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"app/helper.zsh", "app/helper with spaces.zsh", "helpers/shared.zsh", "external/other.zsh"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# Dummy helper.\npaint_tile() { :; }\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct{ text, want string }{
		{`source ./helper.zsh`, "app/helper.zsh"},
		{`'source' ./helper.zsh`, "app/helper.zsh"},
		{`builtin source ./helper.zsh`, "app/helper.zsh"},
		{`builtin . ./helper.zsh`, "app/helper.zsh"},
		{`. -- ./helper.zsh`, "app/helper.zsh"},
		{`source ./helper\ with\ spaces.zsh`, "app/helper with spaces.zsh"},
		{`directory=.; source $directory/helper\ with\ spaces.zsh`, "app/helper with spaces.zsh"},
		{`source $'./helper with spaces.zsh'`, "app/helper with spaces.zsh"},
		{`load_tiles() { local library=./helper.zsh; source "$library"; }`, "app/helper.zsh"},
		{`load_tiles() { typeset library=./helper.zsh; source "${library}"; }`, "app/helper.zsh"},
		{`library=./missing.zsh; load_tiles() { local library=./helper.zsh; source "$library"; }; source "$library"`, "app/helper.zsh"},
		{`library=./helper.zsh; unused() { local library=./missing.zsh; }; source "$library"`, "app/helper.zsh"},
		{`library=./helper.zsh; load_tiles() { local library=$1; source "$library"; }`, ""},
		{`library=./helper.zsh; load_tiles() { local library; source "$library"; }`, ""},
		{`library=./helper.zsh; load_tiles() { typeset library; source "$library"; }`, ""},
		{`load_tiles() { local library=; source "${library:-./helper.zsh}"; }`, "app/helper.zsh"},
		{`load_tiles() { local library=./helper.zsh; local library; source "$library"; }`, "app/helper.zsh"},
		{`library=./helper.zsh; source "${library%.zsh}.zsh"`, "app/helper.zsh"},
		{`library=./helper; library+=.zsh; source "$library"`, "app/helper.zsh"},
		{`library=./helper.zsh; unset library; source "$library"`, ""},
		{`library=./helper.zsh; unset library; source "${library:-./helper.zsh}"`, "app/helper.zsh"},
		{`library=./helper.zsh; read -r library; source "$library"`, ""},
		{`library=./helper.zsh; if [[ $choice == yes ]]; then library=$1; fi; source "$library"`, ""},
		{`if [[ $choice == yes ]]; then library=./helper.zsh; source "$library"; fi`, "app/helper.zsh"},
		{`library=./helper.zsh; if [[ $choice == yes ]]; then library=./missing.zsh; else source "$library"; fi`, "app/helper.zsh"},
		{`case $choice in yes) library=./missing.zsh;; no) library=./helper.zsh; source "$library";; esac`, "app/helper.zsh"},
		{`for item in one two; do library=./helper.zsh; source "$library"; done`, "app/helper.zsh"},
		{`library=./helper.zsh; for library in $input; do source "$library"; done`, ""},
		{`for library in ./helper.zsh; do source "$library"; done`, "app/helper.zsh"},
		{"# shellcheck source=helper.zsh\nsource \"$1\"", "app/helper.zsh"},
		{"# shellcheck source='helper with spaces.zsh'\nsource \"$(locate_tiles)\"", "app/helper with spaces.zsh"},
		{"# shellcheck source=helper.zsh\nsource \"$1\" || true", "app/helper.zsh"},
		{"# shellcheck source-path=../helpers\nsource shared.zsh", "helpers/shared.zsh"},
		{"# shellcheck source-path=SCRIPTDIR\nsource helper.zsh", "app/helper.zsh"},
		{"# shellcheck source=/dev/null\nsource ./helper.zsh", ""},
		{`source helpers/shared.zsh`, "helpers/shared.zsh"},
		{`source '../external/other.zsh'`, "external/other.zsh"},
		{`source '` + filepath.Join(dir, "external/other.zsh") + `'`, "external/other.zsh"},
		{`source ./`, ""},
		{`source "$unknown/helper.zsh"`, ""},
	} {
		t.Run(test.text, func(t *testing.T) {
			d := Parse(TextDocument{URI: fileURI(filepath.Join(dir, "app/main.zsh")), Text: test.text + "\npaint_tile\n"})
			if d.err != nil {
				t.Fatalf("upstream parser must support this fixture: %v", d.err)
			}
			s := NewServer()
			s.roots = []string{dir}
			definitions := s.definitions(d, d.Position(strings.LastIndex(d.Text, "paint_tile")))
			if test.want == "" {
				if len(definitions) != 0 {
					t.Fatalf("unresolved path: %+v", definitions)
				}
				return
			}
			want := fileURI(filepath.Join(dir, test.want))
			if len(definitions) != 1 || definitions[0].URI != want || definitions[0].Range.Start.Line != 1 {
				t.Fatalf("definition: %+v; want %s at line 1", definitions, want)
			}
			found := false
			for _, src := range d.sources {
				if s.sourceURI(d, src) != want {
					continue
				}
				found = true
				if links := s.definitions(d, src.range_.Start); len(links) != 1 || links[0].URI != want {
					t.Fatalf("source operand definition: %+v", links)
				}
			}
			if !found {
				t.Fatal("missing source link")
			}
		})
	}
}

func TestSourceFileIdentity(t *testing.T) {
	dir := t.TempDir()
	library := filepath.Join(dir, "library.zsh")
	alias := filepath.Join(dir, "alias.zsh")
	if err := os.WriteFile(library, []byte("paint_tile() { :; }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(library, alias); err != nil {
		t.Fatal(err)
	}
	for _, open := range []bool{false, true} {
		s := NewServer()
		line := 0
		if open {
			line = 1
			s.documents[fileURI(library)] = Parse(TextDocument{URI: fileURI(library), Text: "# Unsaved comment.\npaint_tile() { :; }\n"})
		}
		d := Parse(TextDocument{URI: fileURI(filepath.Join(dir, "main.zsh")), Text: "source ./library.zsh\nsource ./alias.zsh\npaint_tile\n"})
		defs := s.definitions(d, Position{2, 0})
		if len(defs) != 1 || defs[0].URI != fileURI(library) || defs[0].Range.Start.Line != line {
			t.Fatalf("same sourced file (open=%v): %+v", open, defs)
		}
	}
	text := "source ./alias.zsh\npaint_tile() { :; }\npaint_tile\n"
	if err := os.WriteFile(library, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	d := Parse(TextDocument{URI: fileURI(library), Text: text})
	s := NewServer()
	s.documents[d.URI] = d
	if defs := s.definitions(d, Position{2, 0}); len(defs) != 1 {
		t.Fatalf("self-source through a symlink: %+v", defs)
	}
}
