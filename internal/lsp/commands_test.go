package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFunctionDeclarationOrder(t *testing.T) {
	directory := t.TempDir()
	library := filepath.Join(directory, "sample.zsh")
	if err := os.WriteFile(library, []byte("print -r -- builtin-sample\n"), 0600); err != nil {
		t.Fatal(err)
	}
	text := `if [[ -r "$1" ]]; then
    source "$1"
fi
load_sample() { source "$1"; }
# Load a sample through the wrapper.
function source() { print -r -- wrapped-sample; }
source "$1"
load_sample "$1"
`
	output, err := exec.Command("zsh", "-dfc", text, "zsh", library).CombinedOutput()
	if err != nil || string(output) != "builtin-sample\nwrapped-sample\nwrapped-sample\n" {
		t.Fatalf("Zsh declaration order: %s (%v)", output, err)
	}
	d := doc(text)
	s := NewServer()
	for _, test := range []struct {
		position Position
		line     int
		help     string
	}{
		{Position{1, 6}, -1, "Read and execute a file in the current shell."},
		{Position{3, 18}, 5, "Load a sample through the wrapper."},
		{Position{5, 11}, 5, "Load a sample through the wrapper."},
		{Position{6, 2}, 5, "Load a sample through the wrapper."},
	} {
		defs := s.definitions(d, test.position)
		if test.line < 0 {
			if len(defs) != 0 {
				t.Fatalf("builtin definition: %+v", defs)
			}
		} else if len(defs) != 1 || defs[0].Range.Start.Line != test.line {
			t.Fatalf("function definition at %+v: %+v", test.position, defs)
		}
		hover := s.hover(d, test.position)
		if hover == nil || !strings.Contains(hover.Contents.Value, test.help) {
			t.Fatalf("hover at %+v: %+v", test.position, hover)
		}
	}
	for _, command := range []string{"print sample", "noglob print sample", "builtin print sample"} {
		d := doc(command + "\n# Print through a sample wrapper.\nprint() { :; }\n")
		position := d.Position(strings.Index(d.Text, "print"))
		hover := s.hover(d, position)
		if hover == nil || !strings.Contains(hover.Contents.Value, "Write arguments") {
			t.Fatalf("builtin before declaration in %q: %+v", command, hover)
		}
	}
	d = doc("add-zsh-hook precmd paint_sample\npaint_sample() { :; }\n")
	if defs := s.definitions(d, d.Position(strings.Index(d.Text, "paint_sample"))); len(defs) != 1 || defs[0].Range.Start.Line != 1 {
		t.Fatalf("deferred callback definition: %+v", defs)
	}
}

func TestCommandHover(t *testing.T) {
	for _, test := range []struct{ name, description string }{
		{"mkdir", "make directories"},
		{"curl", "transfer"},
		{"bash", "command language"},
		{"/bin/mkdir", "make directories"},
		{"mount", "synopsis"},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := doc(test.name + "\n")
			hover := NewServer().hover(d, Position{})
			if hover == nil || hover.Contents.Kind != "markdown" || !strings.Contains(strings.ToLower(hover.Contents.Value), test.description) {
				t.Fatalf("missing manual description for %s", test.name)
			}
			if strings.ContainsAny(hover.Contents.Value, "\b\x1b") {
				t.Fatal("terminal formatting in command hover")
			}
		})
	}
	d := doc("# Render a dummy directory.\nmkdir() { :; }\nmkdir\n")
	hover := NewServer().hover(d, Position{2, 0})
	if hover == nil || !strings.Contains(hover.Contents.Value, "Render a dummy directory.") {
		t.Fatalf("function documentation takes precedence: %+v", hover)
	}
}

func TestCommandPrefixHover(t *testing.T) {
	for _, test := range []struct {
		text, word, description string
		definitions             int
	}{
		{"command mkdir sample", "mkdir", "make directories", 0},
		{"command -p mkdir sample", "mkdir", "make directories", 0},
		{"command -- mkdir sample", "mkdir", "make directories", 0},
		{"noglob command mkdir sample", "mkdir", "make directories", 0},
		{"command printf sample", "printf", "NAME", 0},
		{"builtin printf sample", "printf", "Write formatted output", 0},
		{"noglob builtin printf sample", "printf", "Write formatted output", 0},
		{"exec mkdir sample", "mkdir", "make directories", 0},
		{"exec -cl mkdir sample", "mkdir", "make directories", 0},
		{"exec -a dummy mkdir sample", "mkdir", "make directories", 0},
		{"exec printf sample", "printf", "Dummy printer.", 1},
		{"exec -a dummy printf sample", "printf", "Dummy printer.", 1},
		{"command -v printf", "printf", "Dummy printer.", 1},
		{"command -V mkdir printf", "printf", "Dummy printer.", 1},
		{"exec -a printf /bin/mkdir sample", "printf", "", 0},
		{"echo printf", "printf", "", 0},
	} {
		t.Run(test.text, func(t *testing.T) {
			d := doc("# Dummy printer.\nprintf() { :; }\n" + test.text + "\n")
			position := d.Position(strings.LastIndex(d.Text, test.word))
			defs := NewServer().definitions(d, position)
			if len(defs) != test.definitions {
				t.Fatalf("definitions: %+v", defs)
			}
			hover := NewServer().hover(d, position)
			if test.description == "" {
				if hover != nil {
					t.Fatalf("hover on a data argument: %+v", hover)
				}
				return
			}
			if hover == nil || !strings.Contains(hover.Contents.Value, test.description) {
				t.Fatalf("missing %q in hover: %+v", test.description, hover)
			}
		})
	}
}
