package lsp

import (
	"os/exec"
	"strings"
	"testing"
)

func TestShellOptionHover(t *testing.T) {
	s := NewServer()
	for _, test := range []struct{ text, word, description string }{
		{"setopt hist_ignore_all_dups", "hist_ignore_all_dups", "older command is removed"},
		{"unsetopt HIST_IGNORE_ALL_DUPS", "HIST_IGNORE_ALL_DUPS", "older command is removed"},
		{"setopt 'HiStIgnoreAllDups'", "'HiStIgnoreAllDups'", "older command is removed"},
		{"setopt no_beep", "no_beep", "negated form of `BEEP`"},
		{"setopt nonomatch", "nonomatch", "negated form of `NOMATCH`"},
		{"setopt notify", "notify", "NOTIFY"},
		{"setopt hist_ignore_dups hist_reduce_blanks", "hist_reduce_blanks", "Remove superfluous blanks"},
		{"builtin setopt -- hist_ignore_all_dups", "hist_ignore_all_dups", "older command is removed"},
		{"noglob builtin setopt hist_ignore_all_dups", "hist_ignore_all_dups", "older command is removed"},
		{"set -o hist_ignore_all_dups", "hist_ignore_all_dups", "older command is removed"},
		{"set +o hist_ignore_all_dups", "hist_ignore_all_dups", "older command is removed"},
		{"setopt -o hist_ignore_all_dups", "hist_ignore_all_dups", "older command is removed"},
		{"set -xoextended_glob", "-xoextended_glob", "Treat the `#', `~' and `^' characters"},
		{"unsetopt -oEXTENDED_GLOB", "-oEXTENDED_GLOB", "Treat the `#', `~' and `^' characters"},
		{"set +oNo_Beep", "+oNo_Beep", "negated form of `BEEP`"},
		{"emulate -L zsh -o extended_glob", "extended_glob", "Treat the `#', `~' and `^' characters"},
		{"emulate zsh -xoextended_glob", "-xoextended_glob", "Treat the `#', `~' and `^' characters"},
		{"emulate zsh -c 'print dummy'", "'print dummy'", ""},
		{"setopt() { :; }; setopt extended_glob", "extended_glob", ""},
		{"setopt() { :; }; builtin setopt extended_glob", "extended_glob", "Treat the `#', `~' and `^' characters"},
		{"setopt \\\n  hist_ignore_all_dups", "hist_ignore_all_dups", "older command is removed"},
		{"set -- hist_ignore_all_dups", "hist_ignore_all_dups", ""},
		{"set -A samples hist_ignore_all_dups", "hist_ignore_all_dups", ""},
		{"setopt sample_missing_option", "sample_missing_option", ""},
	} {
		t.Run(test.text, func(t *testing.T) {
			assertSettingHover(t, s, test.text, test.word, test.description)
		})
	}
}

func TestShellParameterHover(t *testing.T) {
	s := NewServer()
	for _, test := range []struct{ text, word, description string }{
		{"HISTSIZE=20000", "HISTSIZE", "maximum number of events"},
		{"print ${PATH}", "PATH", "directories to search"},
		{"print $path", "path", "directories to search"},
		{"print $PROMPT", "PROMPT", "Same as PS1"},
		{"print $pipestatus", "pipestatus", "exit statuses"},
		{"print $?", "?", "exit status returned"},
		{"print $BUFFER", "BUFFER", "entire contents of the edit buffer"},
		{"region_highlight=('0 2 bold')", "region_highlight", "highlighting for an arbitrary region"},
		{"print $sample_parameter", "sample_parameter", ""},
	} {
		t.Run(test.text, func(t *testing.T) {
			assertSettingHover(t, s, test.text, test.word, test.description)
		})
	}
}

func TestInstalledShellOptionAndWidgetHovers(t *testing.T) {
	s := NewServer()
	for _, group := range []struct{ kind, script, command string }{
		{"option", "print -rl -- ${(k)options}", "setopt "},
		{"widget", "zmodload zsh/zle; zle -la", "bindkey '^X' "},
	} {
		output, err := exec.Command("zsh", "-dfc", group.script).Output()
		if err != nil {
			t.Fatal(err)
		}
		names := strings.Fields(string(output))
		if len(names) == 0 {
			t.Fatalf("no installed %ss", group.kind)
		}
		for _, name := range names {
			t.Run(group.kind+"/"+name, func(t *testing.T) {
				text := group.command + name
				d := doc(text + "\n")
				hover := s.hover(d, d.Position(strings.LastIndex(text, name)))
				if hover == nil {
					t.Fatal("missing documentation")
				}
				entry := strings.Split(hover.Contents.Value, "```text\n")[1]
				headings, _, _ := strings.Cut(entry, "\n       ")
				if group.kind == "option" {
					if !strings.Contains(optionKey(headings), name) {
						t.Fatalf("incorrect option heading: %s", headings)
					}
				} else if !strings.Contains(headings, strings.TrimPrefix(name, ".")) {
					t.Fatalf("incorrect widget heading: %s", headings)
				}
			})
		}
	}
}
