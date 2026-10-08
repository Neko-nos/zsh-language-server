package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoloadFromSymlinkedScript(t *testing.T) {
	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(dir, "settings", "helpers")
	if err := os.MkdirAll(library, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"paint_sample": "# Paint a sample.\npaint_sample() { print painted; }\n",
		"body_sample":  "# Print a sample.\nprint body\n",
	} {
		if err := os.WriteFile(filepath.Join(library, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	text := `typeset -g helper_dir="${${(%):-%N}:A:h}/helpers"
if [[ -d "$helper_dir" ]]; then
    fpath=("$helper_dir" "${fpath[@]}")
fi
unset helper_dir
typeset -gU path fpath
autoload -Uz paint_sample body_sample
paint_sample
body_sample
`
	script := filepath.Join(dir, "settings", "init.zsh")
	if err := os.WriteFile(script, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".zshrc")
	if err := os.Symlink(script, link); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("zsh", "-df", link).CombinedOutput()
	if err != nil || string(output) != "painted\nbody\n" {
		t.Fatalf("Zsh autoload: %s (%v)", output, err)
	}
	s := NewServer()
	s.roots = []string{t.TempDir()}
	d := Parse(TextDocument{URI: fileURI(link), Text: text})
	for _, name := range []string{"paint_sample", "body_sample"} {
		for _, offset := range []int{strings.Index(text, name), strings.LastIndex(text, name)} {
			defs := s.definitions(d, d.Position(offset))
			if len(defs) != 1 || defs[0].URI != fileURI(filepath.Join(library, name)) || defs[0].Range.Start.Line != 1 {
				t.Fatalf("%s at %d: %+v", name, offset, defs)
			}
		}
		if hover := s.hover(d, d.Position(strings.LastIndex(text, name))); hover == nil {
			t.Fatalf("missing hover for %s", name)
		}
	}
	items := s.completion(d, d.Position(strings.LastIndex(text, "paint_sample")+4))
	if len(items) != 1 || items[0].Label != "paint_sample" {
		t.Fatalf("autoload completion: %+v", items)
	}
	uri := fileURI(filepath.Join(library, "paint_sample"))
	s.documents[uri] = Parse(TextDocument{URI: uri, Version: 2, Text: "\n\npaint_sample() { print edited; }\n"})
	defs := s.definitions(d, d.Position(strings.LastIndex(text, "paint_sample")))
	if len(defs) != 1 || defs[0].Range.Start.Line != 2 {
		t.Fatalf("unsaved autoload function: %+v", defs)
	}
}

func TestAutoloadSearchOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	for _, name := range []string{"first", "second"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "show_sample"), []byte("print sample\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		setup, want string
	}{
		{`fpath=("$HOME/first" "$HOME/second")`, "first"},
		{`fpath=(~/first ~/second)`, "first"},
		{`fpath=("$HOME/first"); fpath+=("$HOME/second")`, "first"},
		{`fpath=("$HOME/first"); fpath=("$HOME/second" $fpath)`, "second"},
		{`FPATH="$HOME/second:$HOME/first"`, "second"},
		{`fpath=("$HOME/missing" "$HOME/second")`, "second"},
		{`fpath=("$unknown" "$HOME/second")`, ""},
		{`fpath=("$HOME/first"); unused() { fpath=("$HOME/second"); }`, "first"},
		{`fpath=("$HOME/first"); (fpath=("$HOME/second"))`, "first"},
		{`fpath=("$HOME/first"); result=$(fpath=("$HOME/second"))`, "first"},
		{`fpath=("$HOME/first"); FPATH="$HOME/second" print sample`, "first"},
		{`fpath=("$HOME/first"); if [[ -d "$HOME/missing" ]]; then fpath=("$HOME/second"); fi`, "first"},
		{`fpath=("$HOME/first"); if [[ $choice == yes ]]; then fpath=("$HOME/second"); fi`, ""},
	} {
		t.Run(test.setup, func(t *testing.T) {
			text := test.setup + "\nautoload -U show_sample\nshow_sample\n"
			d := Parse(TextDocument{URI: fileURI(filepath.Join(dir, "init.zsh")), Text: text})
			defs := NewServer().definitions(d, d.Position(strings.LastIndex(text, "show_sample")))
			if test.want == "" {
				if len(defs) != 0 {
					t.Fatalf("unknown path: %+v", defs)
				}
			} else if len(defs) != 1 || defs[0].URI != fileURI(filepath.Join(dir, test.want, "show_sample")) {
				t.Fatalf("wanted %s: %+v", test.want, defs)
			}
		})
	}
}

func TestAutoloadExpandedPathsThroughSource(t *testing.T) {
	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	helpers := filepath.Join(dir, "helpers")
	if err := os.Mkdir(helpers, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(helpers, "paint_sample")
	if err := os.WriteFile(target, []byte("# Paint a sample.\npaint_sample() { print painted; }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, setup := range []string{
		`autoload -Uz "${${(%):-%N}:A:h}/helpers/paint_sample"`,
		`builtin autoload -Uz "${${(%):-%N}:A:h}/helpers/paint_sample"`,
		`helper_dir="${${(%):-%N}:A:h}/helpers"; autoload -Uz "$helper_dir/paint_sample"`,
		`autoload -Uz "$HOME/helpers/paint_sample"`,
		`autoload -Uz ~/helpers/paint_sample`,
		`function_path="$HOME/helpers/paint_sample"; autoload -Uz "$function_path"`,
	} {
		t.Run(setup, func(t *testing.T) {
			library := filepath.Join(dir, "library.zsh")
			if err := os.WriteFile(library, []byte(setup+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			text := `source "${${(%):-%N}:A:h}/library.zsh"
render_sample() { paint_sample; }
render_sample
`
			filename := filepath.Join(dir, "main.zsh")
			if err := os.WriteFile(filename, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			output, err := exec.Command("zsh", "-df", filename).CombinedOutput()
			if err != nil || string(output) != "painted\n" {
				t.Fatalf("native Zsh autoload: %s (%v)", output, err)
			}
			s := NewServer()
			d := Parse(TextDocument{URI: fileURI(filename), Text: text})
			position := d.Position(strings.Index(text, "paint_sample"))
			defs := s.definitions(d, position)
			if len(defs) != 1 || defs[0].URI != fileURI(target) || defs[0].Range.Start.Line != 1 {
				t.Fatalf("expanded autoload definition: %+v", defs)
			}
			if s.hover(d, position) == nil {
				t.Fatal("missing expanded autoload hover")
			}
			items := s.completion(d, d.Position(d.Offset(position)+5))
			if len(items) != 1 || items[0].Label != "paint_sample" {
				t.Fatalf("expanded autoload completion: %+v", items)
			}
			loader := Parse(TextDocument{URI: fileURI(library), Text: setup})
			for _, variable := range []string{"helper_dir", "function_path"} {
				if offset := strings.LastIndex(setup, "$"+variable); offset >= 0 {
					reference := loader.at(loader.Position(offset + 1))
					if reference == nil || reference.name != variable || reference.kind != "variable" {
						t.Fatalf("path parameter reference: %+v", reference)
					}
				}
			}
		})
	}
}

func TestAutoloadScopes(t *testing.T) {
	dir := t.TempDir()
	for _, folder := range []string{"first", "second"} {
		if err := os.Mkdir(filepath.Join(dir, folder), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, folder, "paint_tile"), []byte("paint_tile() { print -r -- "+folder+"; }\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	for _, test := range []struct{ text, want string }{
		{"load_tiles() { local fpath=('" + first + "'); autoload -Uz paint_tile; paint_tile; }", first},
		{"autoload -Uz paint_tile\nload_tiles() { local fpath=('" + first + "'); paint_tile; }", first},
		{"fpath=('" + second + "')\nautoload -Uz paint_tile\nload_tiles() { local fpath=('" + first + "'); paint_tile; }", first},
		{"load_tiles() { autoload -Uz paint_tile; paint_tile; }\nfpath=('" + first + "')", first},
		{"fpath=('" + first + "')\nautoload -Uz paint_tile\nload_tiles() { local fpath=($input); paint_tile; }", ""},
		{"fpath=('" + first + "')\nautoload -Uz paint_tile\nload_tiles() { local fpath; paint_tile; }", ""},
		{"fpath=('" + first + "')\nautoload -Uz paint_tile\nload_tiles() { unset fpath; paint_tile; }", ""},
		{"load_tiles() { autoload -Uz '" + first + "/paint_tile'; paint_tile; }\nother_tiles() { autoload -Uz '" + second + "/paint_tile'; }", first},
		{"unused() { autoload -Uz '" + first + "/paint_tile'; }\npaint_tile", ""},
	} {
		t.Run(test.text, func(t *testing.T) {
			d := Parse(TextDocument{URI: fileURI(filepath.Join(dir, "main.zsh")), Text: test.text})
			if d.err != nil {
				t.Fatal(d.err)
			}
			offset := strings.Index(test.text, "paint_tile;")
			if offset < 0 {
				offset = strings.LastIndex(test.text, "paint_tile")
			}
			defs := NewServer().definitions(d, d.Position(offset))
			if test.want == "" {
				if len(defs) != 0 {
					t.Fatalf("unresolved autoload: %+v", defs)
				}
			} else if len(defs) != 1 || defs[0].URI != fileURI(filepath.Join(test.want, "paint_tile")) {
				t.Fatalf("expected %s: %+v", test.want, defs)
			}
			if test.want != "" {
				output, err := exec.Command("zsh", "-dfc", test.text+"\nload_tiles\n").CombinedOutput()
				if err != nil || strings.TrimSpace(string(output)) != filepath.Base(test.want) {
					t.Fatalf("native Zsh resolution: %s (%v)", output, err)
				}
			}
		})
	}
}

func TestAutoloadDoesNotExecute(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	text := "fpath=($(touch '" + marker + "'; print '" + dir + "'))\nautoload -Uz show_sample\nshow_sample\n"
	d := Parse(TextDocument{URI: fileURI(filepath.Join(dir, "init.zsh")), Text: text})
	if defs := NewServer().definitions(d, d.Position(strings.LastIndex(text, "show_sample"))); len(defs) != 0 {
		t.Fatalf("dynamic path: %+v", defs)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("command substitution executed: %v", err)
	}
}

func TestSiblingFunctionDefinitions(t *testing.T) {
	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"ask_sample":   "# Ask about a sample.\nask_sample() { print asked; }\n",
		"log_sample":   "# Log a sample.\nlog_sample() { print logged; }\n",
		"body_sample":  "# Render a sample.\nprint rendered\n",
		"wrong_sample": "unrelated_sample() { :; }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"compose_sample", "compose.sample"} {
		t.Run(name, func(t *testing.T) {
			text := name + "() {\n  autoload -Uz ask_sample body_sample\n  ask_sample\n  log_sample\n  body_sample\n  wrong_sample\n}\n"
			filename := filepath.Join(dir, name)
			if err := os.WriteFile(filename, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			d := Parse(TextDocument{URI: fileURI(filename), Text: text})
			s := NewServer()
			for _, target := range []string{"ask_sample", "log_sample", "body_sample"} {
				for _, offset := range []int{strings.Index(text, target), strings.LastIndex(text, target)} {
					defs := s.definitions(d, d.Position(offset))
					if len(defs) != 1 || defs[0].URI != fileURI(filepath.Join(dir, target)) || defs[0].Range.Start.Line != 1 {
						t.Fatalf("%s at %d: %+v", target, offset, defs)
					}
					if s.hover(d, d.Position(offset)) == nil {
						t.Fatalf("missing hover for %s", target)
					}
				}
			}
			if defs := s.definitions(d, d.Position(strings.Index(text, "wrong_sample"))); len(defs) != 0 {
				t.Fatalf("unrelated sibling declaration: %+v", defs)
			}
			uri := fileURI(filepath.Join(dir, "log_sample"))
			s.documents[uri] = Parse(TextDocument{URI: uri, Text: "\n\nlog_sample() { print edited; }\n"})
			if defs := s.definitions(d, d.Position(strings.Index(text, "log_sample"))); len(defs) != 1 || defs[0].Range.Start.Line != 2 {
				t.Fatalf("unsaved sibling function: %+v", defs)
			}
		})
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "ask_sample"), []byte("\n\nask_sample() { print other; }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, setup := range []string{
		"fpath=('" + other + "')\nautoload -Uz ask_sample\n",
		"autoload -Uz '" + filepath.Join(other, "ask_sample") + "'\n",
		"source '" + filepath.Join(other, "ask_sample") + "'\n",
	} {
		text := setup + "compose_sample() { ask_sample; }\n"
		d := Parse(TextDocument{URI: fileURI(filepath.Join(dir, "compose_sample")), Text: text})
		defs := NewServer().definitions(d, d.Position(strings.LastIndex(text, "ask_sample")))
		if len(defs) != 1 || defs[0].URI != fileURI(filepath.Join(other, "ask_sample")) || defs[0].Range.Start.Line != 2 {
			t.Fatalf("explicit path precedence %q: %+v", setup, defs)
		}
	}
}
