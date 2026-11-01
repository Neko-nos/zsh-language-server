package lsp

import (
	"strings"
	"testing"
)

func assertSettingHover(t *testing.T, s *Server, text, word, description string) {
	t.Helper()
	d := doc(text + "\n")
	if d.err != nil {
		t.Fatal(d.err)
	}
	start := strings.LastIndex(d.Text, word)
	hover := s.hover(d, d.Position(start+len(word)/2))
	if description == "" {
		if hover != nil {
			t.Fatalf("unexpected hover: %+v", hover)
		}
		return
	}
	if hover == nil || !strings.Contains(strings.Join(strings.Fields(hover.Contents.Value), " "), description) {
		t.Fatalf("missing %q in hover: %+v", description, hover)
	}
	if hover.Range != d.span(start, start+len(word)) {
		t.Fatalf("hover range: %+v", hover.Range)
	}
}

func TestBuiltinSettingHover(t *testing.T) {
	s := NewServer()
	for _, test := range []struct{ text, word, description string }{
		{"autoload -Uz sample_function", "-Uz", "alias expansion"},
		{"compinit -d /dummy/completion-cache", "-d", "dump"},
		{"compdef -an sample_function sample_command", "-an", "autoload"},
		{"typeset -gU sample_paths", "-gU", "first occurrence of each duplicated value"},
		{"local -a sample_values", "-a", "array parameters"},
		{"export -U SAMPLE_PATH", "-U", "first occurrence of each duplicated value"},
		{"alias -g SAMPLE=dummy", "-g", "global alias"},
		{"bindkey -e", "-e", "Selects keymap"},
		{"bindkey '^X' forward-word", "forward-word", "beginning of the next word"},
		{"bindkey -M emacs '^X' forward-word", "emacs", "EMACS emulation"},
		{"bindkey -M emacs '^X' forward-word", "forward-word", "beginning of the next word"},
		{"bindkey -Mviins '^X' forward-word", "-Mviins", "insert mode"},
		{"bindkey -Mviins '^X' forward-word", "forward-word", "beginning of the next word"},
		{"bindkey -RMviins ab backward-char", "backward-char", "Move backward one character"},
		{"bindkey -M sample-map '^X' backward-char '^Y' forward-char", "forward-char", "Move forward one character"},
		{"bindkey -- '+X' forward-char", "forward-char", "Move forward one character"},
		{"bindkey -sMviins '^X' forward-word", "forward-word", "out-string"},
		{"bindkey -A emacs main", "main", "linked to the"},
		{"zle .accept-line", ".accept-line", "Finish editing the buffer"},
		{"zle -A accept-line sample-widget", "accept-line", "Finish editing the buffer"},
		{"zle -C sample-widget complete-word sample_function", "complete-word", "completion"},
		{"zle -K viins", "viins", "insert mode"},
		{"zle -M forward-word", "forward-word", ""},
		{"zle -N forward-word sample_function", "forward-word", ""},
		{"zmodload zsh/complist", "zsh/complist", "Completion listing extensions"},
		{"zmodload -F zsh/parameter p:options", "zsh/parameter", "internal hash tables"},
		{"zmodload -F zsh/parameter p:options", "-F", "features"},
		{"emulate -L zsh", "-L", "LOCAL_OPTIONS"},
		{"emulate -L zsh", "zsh", "LOCAL_OPTIONS"},
		{"ulimit -n 128", "-n", "file descriptors"},
		{"limit stacksize 128", "stacksize", "resource limits"},
		{"umask 027", "027", "octal number"},
		{"print -- -r", "-r", ""},
		{"print dummy -r", "-r", ""},
		{"autoload() { :; }; autoload -Uz sample_function", "-Uz", ""},
		{"autoload() { :; }; builtin autoload -Uz sample_function", "-Uz", "alias expansion"},
	} {
		t.Run(test.text+"/"+test.word, func(t *testing.T) {
			assertSettingHover(t, s, test.text, test.word, test.description)
		})
	}
}
