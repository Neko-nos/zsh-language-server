package lsp

import (
	"strings"
	"testing"
)

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
